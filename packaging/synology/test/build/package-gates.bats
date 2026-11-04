#!/usr/bin/env bats
# Gates of packaging/synology/ffmpeg/build/scripts/package.sh (tasks SPK-15, SPK-16): every
# readelf/objdump result is captured before it is judged, so a failing tool stops the package
# step instead of passing as "no PT_INTERP" or "no ABI note". The cross tools are replaced by a
# fake readelf/objdump on PATH (FAKE_ELF_FAIL names the readelf option that fails, FAKE_ELF_*
# change the reported values), so the cases run in the script test image without a toolchain.
#
# package.sh copies the licence texts from the recipe's fixed /build/licenses; the test images
# run as root in a throw-away container, so setup creates it.
# shellcheck disable=SC2030,SC2031 # every @test body is its own subshell; the exports are meant for it

SCRIPTS="$(cd "$BATS_TEST_DIRNAME/../../ffmpeg/build/scripts" && pwd)"
LIST="$(cd "$BATS_TEST_DIRNAME/../../ffmpeg/build" && pwd)/sources.list"

setup() {
	bin="$BATS_TEST_TMPDIR/bin"
	build="$BATS_TEST_TMPDIR/build"
	archive="$BATS_TEST_TMPDIR/archive"
	dest="$BATS_TEST_TMPDIR/dest"
	mkdir -p "$bin" "$build" "$archive" /build/licenses
	echo "licence" >/build/licenses/test-COPYING
	write_fake_tools
	export PATH="$bin:$PATH"
	unset HOST_PACKAGES FAKE_ELF_FAIL FAKE_ELF_INTERP FAKE_ELF_NEEDED FAKE_ELF_ABI FAKE_ELF_TYPE FAKE_ELF_MACHINE FAKE_OBJDUMP_FAIL
	for tool in ffmpeg ffprobe; do echo "fake $tool" >"$build/$tool"; done
	echo "--enable-gpl --enable-version3" >"$build/configuration.txt"
	echo "HAVE_X86ASM=1" >"$build/config-facts.txt"
	echo "$(printf '0%.0s' {1..64})  ffmpeg-1-source.tar" >"$archive/ffmpeg-1-source.tar.sha256"
}

# write_fake_tools puts <triplet>-readelf and -objdump of x86_64 and armv7 on PATH.
write_fake_tools() {
	cat >"$bin/fake-readelf" <<'EOF'
#!/bin/sh
option=$1
[ "${FAKE_ELF_FAIL:-}" != "$option" ] || { echo "readelf: Error: not an ELF file" >&2; exit 1; }
case "$option" in
-hW) printf '  Class:                             %s\n  Type:                              %s (Executable file)\n  Machine:                           %s\n' \
	"${FAKE_ELF_CLASS:-ELF64}" "${FAKE_ELF_TYPE:-EXEC}" "${FAKE_ELF_MACHINE:-Advanced Micro Devices X86-64}" ;;
-lW) echo "  LOAD 0x000000"; [ -z "${FAKE_ELF_INTERP:-}" ] || echo "  INTERP 0x0002a8" ;;
-dW) if [ -n "${FAKE_ELF_NEEDED:-}" ]; then echo " 0x01 (NEEDED) Shared library: [libc.so.6]"; else echo "There is no dynamic section in this file."; fi ;;
-nW) [ -z "${FAKE_ELF_ABI:-}" ] || echo "    OS: Linux, ABI: $FAKE_ELF_ABI" ;;
-A) echo "  Tag_CPU_arch: v7"; echo "  Tag_FP_arch: VFPv3" ;;
esac
EOF
	cat >"$bin/fake-objdump" <<'EOF'
#!/bin/sh
[ -z "${FAKE_OBJDUMP_FAIL:-}" ] || exit 1
echo "   0:	vld1.8	{d0}, [r0]"
EOF
	chmod 0755 "$bin/fake-readelf" "$bin/fake-objdump"
	for triplet in x86_64-linux-musl arm-linux-musleabihf; do
		ln -s fake-readelf "$bin/$triplet-readelf"
		ln -s fake-objdump "$bin/$triplet-objdump"
	done
}

run_package() { run sh "$SCRIPTS/package.sh" "${1:-x86_64}" "$build" "$LIST" "$archive" "$dest"; }

# shellcheck disable=SC2154 # status and output are set by bats' run
expect_failure_with() {
	[ "$status" -ne 0 ] && [[ "$output" == *"$1"* ]] && [ ! -e "$dest/SOURCE.txt" ] || {
		printf 'expected a failure with: %s\nstatus %s, output:\n%s\n' "$1" "$status" "$output" >&2
		return 1
	}
}

@test "a static executable of the family passes and gets a SOURCE.txt" {
	run_package
	[ "$status" -eq 0 ] || { echo "$output" >&2; return 1; }
	grep -q "ELF64 EXEC Advanced Micro Devices X86-64, static" "$dest/SOURCE.txt"
	grep -q "HAVE_X86ASM=1" "$dest/SOURCE.txt"
}

@test "a failing readelf stops the package step for every gate" {
	local option
	for option in -hW -lW -dW -nW; do
		export FAKE_ELF_FAIL="$option"
		run_package
		expect_failure_with "readelf $option failed on ffmpeg"
	done
}

@test "a failing readelf -A or objdump stops the armv7 checks" {
	export FAKE_ELF_CLASS=ELF32 FAKE_ELF_MACHINE=ARM
	run_package armv7
	[ "$status" -eq 0 ] || { echo "$output" >&2; return 1; }
	rm -rf "$dest"
	export FAKE_ELF_FAIL=-A
	run_package armv7
	expect_failure_with "readelf -A failed on ffmpeg"
	unset FAKE_ELF_FAIL
	export FAKE_OBJDUMP_FAIL=1
	run_package armv7
	expect_failure_with "objdump failed on ffmpeg"
}

@test "a dynamic, foreign or too new executable is refused" {
	FAKE_ELF_INTERP=1 run_package
	expect_failure_with "ffmpeg has a PT_INTERP"
	FAKE_ELF_NEEDED=1 run_package
	expect_failure_with "ffmpeg needs shared libraries"
	FAKE_ELF_ABI=4.18.20 run_package
	expect_failure_with "ffmpeg has ABI note 4.18.20 > 3.10.108"
	FAKE_ELF_TYPE=DYN run_package
	expect_failure_with "ffmpeg is of ELF type DYN, not EXEC"
	FAKE_ELF_MACHINE=AArch64 run_package
	expect_failure_with "the family needs 'Advanced Micro Devices X86-64'"
}

# zeros <n> prints n zeros.
zeros() { printf '0%.0s' $(seq "$1"); }

@test "SOURCE.txt carries exactly the computed values" {
	run_package
	[ "$status" -eq 0 ] || { echo "$output" >&2; return 1; }
	grep -qx "ffmpeg SHA-256:       $(sha256sum "$dest/ffmpeg" | cut -d ' ' -f 1)" "$dest/SOURCE.txt"
	grep -qx "Corresponding source: ffmpeg-1-source.tar" "$dest/SOURCE.txt"
	grep -qx "Source SHA-256:       $(zeros 64)" "$dest/SOURCE.txt"
	grep -qx "  --enable-gpl --enable-version3" "$dest/SOURCE.txt"
	# Without a package list dpkg-query is not asked, which would list every package.
	grep -qx "Host tools: unknown" "$dest/SOURCE.txt"
}

@test "SOURCE.txt is refused unless there is exactly one well-formed source checksum" {
	cp "$archive/ffmpeg-1-source.tar.sha256" "$archive/ffmpeg-2-source.tar.sha256"
	run_package
	expect_failure_with "more than one *.tar.sha256"
	rm "$archive"/*.sha256
	run_package
	expect_failure_with "no *.tar.sha256"
	local line
	for line in "abc  ffmpeg-1-source.tar" "$(printf 'A%.0s' $(seq 64))  ffmpeg-1-source.tar" \
		"$(zeros 63)  ffmpeg-1-source.tar"; do
		echo "$line" >"$archive/ffmpeg-1-source.tar.sha256"
		run_package
		expect_failure_with "is not a SHA-256"
	done
	printf '%s  a.tar\n%s  b.tar\n' "$(zeros 64)" "$(zeros 64)" >"$archive/ffmpeg-1-source.tar.sha256"
	run_package
	expect_failure_with "must hold exactly one line"
	echo "$(zeros 64)  " >"$archive/ffmpeg-1-source.tar.sha256"
	run_package
	expect_failure_with "names no archive file"
}

@test "SOURCE.txt is refused for a missing, empty or multi-line configuration" {
	printf '%s\n%s\n' "--enable-gpl" "--enable-nonfree" >"$build/configuration.txt"
	run_package
	expect_failure_with "configuration.txt must hold exactly one line"
	echo "" >"$build/configuration.txt"
	run_package
	expect_failure_with "configuration.txt is empty"
	rm "$build/configuration.txt"
	run_package
	expect_failure_with "missing $build/configuration.txt"
}

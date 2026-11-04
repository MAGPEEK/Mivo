#!/usr/bin/env bats
# Tests of inspect-spk.sh (task SPK-10) on the synthetic SPK of fixture.bash: a complete SPK
# passes, the interim and release rules hold, and every defect the acceptance criteria name
# (a removed binary, a wrong arch, adminurl, a leftover token, a dynamically linked ffmpeg,
# build leftovers, modes, line endings, JSON, icons, ...) fails; unsafe, duplicate and
# unreadable members, base-256 sizes and note padding are covered as well (SPK-10 review
# leftovers). How to run them: ../../toolkit/README.md "Output verification".

INSPECT="$BATS_TEST_DIRNAME/inspect-spk.sh"

load fixture

setup() {
	tree="$BATS_TEST_TMPDIR/tree"
	out="$BATS_TEST_TMPDIR/demo.spk"
	make_tree "$tree" x86_64 selected static 3.2.0
}

# inspect [options] <family>: packs the tree and runs the inspection with the fixture port.
inspect() {
	pack_spk "$tree" "$out"
	inspect_packed "$@"
}

# inspect_packed [options] <family>: runs the inspection on $out as it is.
inspect_packed() {
	local family="${*: -1}"
	set -- "${@:1:$#-1}"
	run sh "$INSPECT" --port "$FIXTURE_PORT" "$@" "$out" "$family"
}

# repack_payload <command...>: runs the command in a directory holding the payload tar of
# the tree as payload.tar, then compresses that tar into the tree's package.tgz.
repack_payload() {
	local scratch="$BATS_TEST_TMPDIR/payload"
	rm -rf "$scratch" && mkdir -p "$scratch"
	# shellcheck disable=SC2012 # ls order on purpose, as pkg_make_package packs
	(cd "$tree/pkg" && ls | tar cf "$scratch/payload.tar" -T /dev/stdin)
	(cd "$scratch" && "$@")
	xz -c "$scratch/payload.tar" >"$tree/spk/package.tgz"
}

# shellcheck disable=SC2154 # status and output are set by bats' run
expect_status() {
	[ "$status" -eq "$1" ] || {
		printf 'expected exit status %s, got %s; output:\n%s\n' "$1" "$status" "$output" >&2
		return 1
	}
}

expect_line() {
	grep -qF -- "$1" <<<"$output" || {
		printf 'expected a line containing: %s\noutput:\n%s\n' "$1" "$output" >&2
		return 1
	}
}

refute_line() {
	! grep -qF -- "$1" <<<"$output" || {
		printf 'expected no line containing: %s\noutput:\n%s\n' "$1" "$output" >&2
		return 1
	}
}

expect_clean() {
	expect_status 0
	refute_line "FAIL "
	refute_line "WARN "
	expect_line "0 failed, 0 warnings"
}

expect_fail() {
	expect_status 1
	expect_line "FAIL $1"
}

# --- complete packages -------------------------------------------------------------------

@test "a complete SPK of a selected static build passes without warnings" {
	inspect x86_64
	expect_clean
	expect_line 'ok   INFO arch="x86_64"'
	expect_line "ok   bin/$FIXTURE_ID: ELF class 2, machine 62 (family x86_64)"
	expect_line "ok   bin/ffmpeg: ELF ABI note 3.2.0 is at most the oldest DSM 7.2 kernel 3.10.108"
	expect_line "ok   SOURCE.txt status is selected"
	expect_line "ok   spk: members, modes and types match the expected list"
	expect_line "ok   package: members, modes and types match the expected list"
	expect_line "ok   SPK is an uncompressed tar (format gnu)"
	expect_line "ok   package.tgz is xz-compressed"
	expect_line "info SPK tar extensions beyond plain ustar: none"
}

@test "the armv8 and armv7 fixtures pass with their arch values and ELF identities" {
	make_tree "$tree" armv8 selected static 4.4.0
	inspect armv8
	expect_clean
	expect_line 'ok   INFO arch="armv8"'
	expect_line "ok   bin/ffmpeg: ELF class 2, machine 183 (family armv8)"
	make_tree "$tree" armv7 selected static 3.2.0
	inspect armv7
	expect_clean
	expect_line 'ok   INFO arch="armv7 armada38x monaco"'
	expect_line "ok   bin/ffmpeg: ELF class 1, machine 40 (family armv7)"
	expect_line "info bin/ffmpeg: ELF e_flags 0x05000400"
}

@test "release mode passes a selected static build" {
	inspect --release x86_64
	expect_clean
}

@test "a server binary without an ABI note is recorded, not judged" {
	inspect x86_64
	expect_line "info bin/$FIXTURE_ID: no GNU ABI note"
}

@test "a final version carries no beta key" {
	fixture_info x86_64 1.2.3-9000 no >"$tree/spk/INFO"
	inspect x86_64
	expect_clean
	expect_line "ok   INFO has no beta"
}

# --- interim and release rules -----------------------------------------------------------

@test "an interim dynamically linked FFmpeg warns in test mode and fails in release mode" {
	make_tree "$tree" x86_64 interim dynamic 4.18.20
	inspect x86_64
	local interim="(interim FFmpeg build, test build only)"
	expect_status 0
	expect_line "WARN SOURCE.txt status is interim $interim"
	expect_line "WARN bin/ffmpeg: dynamically linked (has a program interpreter) $interim"
	expect_line "WARN bin/ffprobe: ELF ABI note 4.18.20 is above the oldest DSM 7.2 kernel 3.10.108 $interim"
	expect_line "0 failed, 5 warnings"
	inspect --release x86_64
	expect_fail "SOURCE.txt status is interim"
	expect_line "FAIL bin/ffmpeg: dynamically linked (has a program interpreter)"
	expect_line "FAIL bin/ffprobe: ELF ABI note 4.18.20 is above the oldest DSM 7.2 kernel 3.10.108"
	expect_line "5 failed, 0 warnings"
}

@test "a non-free licence warns for an interim build and fails for a release" {
	make_tree "$tree" x86_64 interim static 3.2.0
	fixture_notice interim GPL-3.0-or-later+nonfree >"$tree/pkg/share/licenses/ffmpeg/SOURCE.txt"
	inspect x86_64
	expect_status 0
	expect_line "WARN SOURCE.txt licence GPL-3.0-or-later+nonfree is non-free (not redistributable)"
	inspect --release x86_64
	expect_fail "SOURCE.txt licence GPL-3.0-or-later+nonfree is non-free (not redistributable)"
}

@test "a dynamically linked ffmpeg fails in test mode when the build is not interim" {
	make_tree "$tree" x86_64 selected dynamic none
	inspect x86_64
	expect_fail "bin/ffmpeg: dynamically linked (has a program interpreter)"
	expect_line "FAIL bin/ffprobe: dynamically linked (has a program interpreter)"
}

@test "a dynamically linked server binary always fails" {
	fake_elf x86_64 dynamic none >"$tree/pkg/bin/$FIXTURE_ID"
	chmod 0755 "$tree/pkg/bin/$FIXTURE_ID"
	make_tree_notice_interim
	inspect x86_64
	expect_fail "bin/$FIXTURE_ID: dynamically linked (has a program interpreter)"
}

make_tree_notice_interim() {
	fixture_notice interim GPL-3.0-or-later >"$tree/pkg/share/licenses/ffmpeg/SOURCE.txt"
}

@test "an ABI note above the family's kernel fails for a selected build" {
	make_tree "$tree" armv8 selected static 4.18.20
	inspect armv8
	expect_fail "bin/ffmpeg: ELF ABI note 4.18.20 is above the oldest DSM 7.2 kernel 4.4.302"
	make_tree "$tree" armv8 selected static 4.4.302
	inspect armv8
	expect_clean
}

@test "an unknown FFmpeg status and a notice with two Status lines fail" {
	fixture_notice unknown GPL-3.0-or-later >"$tree/pkg/share/licenses/ffmpeg/SOURCE.txt"
	inspect x86_64
	expect_fail "SOURCE.txt status is 'unknown', want selected"
	fixture_notice selected GPL-3.0-or-later >"$tree/pkg/share/licenses/ffmpeg/SOURCE.txt"
	echo "Status: interim" >>"$tree/pkg/share/licenses/ffmpeg/SOURCE.txt"
	inspect x86_64
	expect_fail "SOURCE.txt has 2 Status: and 1 Licence: lines, want one each"
}

# --- members ------------------------------------------------------------------------------

@test "a missing SOURCE.txt fails" {
	rm "$tree/pkg/share/licenses/ffmpeg/SOURCE.txt"
	inspect x86_64
	expect_fail "share/licenses/ffmpeg/SOURCE.txt is missing"
	expect_line "share/licenses/ffmpeg/SOURCE.txt(expected once, found 0)"
}

@test "a missing FFmpeg licence file fails" {
	rm "$tree/pkg/share/licenses/ffmpeg/LICENSE.md"
	inspect --release x86_64
	expect_fail "share/licenses/ffmpeg has no licence file besides SOURCE.txt"
	expect_line "1 failed, 0 warnings"
}

@test "--expected replaces the expected-member list" {
	local list="$BATS_TEST_TMPDIR/expected.txt"
	cp "$BATS_TEST_DIRNAME/expected-members.txt" "$list"
	echo "spk 0644 NOTES.txt" >>"$list"
	inspect --expected "$list" x86_64
	expect_fail "spk: members differ from the expected list: NOTES.txt(expected once, found 0)"
	echo notes >"$tree/spk/NOTES.txt"
	inspect --expected "$list" x86_64
	expect_clean
	inspect x86_64
	expect_fail "spk: members differ from the expected list: NOTES.txt(unexpected)"
}

@test "a removed bin/ffmpeg fails" {
	rm "$tree/pkg/bin/ffmpeg"
	inspect x86_64
	expect_fail "package: members differ from the expected list:"
	expect_line "bin/ffmpeg(expected once, found 0)"
}

@test "an unexpected member, a hidden file and a build leftover fail" {
	echo x >"$tree/pkg/bin/extra"
	echo x >"$tree/pkg/share/licenses/ffmpeg/.fetch-manifest"
	echo "ID='demo'" >"$tree/spk/build.env"
	mkdir -p "$tree/spk/SynoBuildConf" && echo x >"$tree/spk/SynoBuildConf/build"
	mkdir -p "$tree/spk/prebuilt/bin" && echo x >"$tree/spk/prebuilt/bin/demo"
	inspect x86_64
	expect_status 1
	expect_line "bin/extra(unexpected)"
	expect_line "share/licenses/ffmpeg/.fetch-manifest(build leftover or hidden file)"
	expect_line "build.env(build leftover or hidden file)"
	expect_line "SynoBuildConf/build(build leftover or hidden file)"
	expect_line "prebuilt/bin/demo(build leftover or hidden file)"
}

@test "a signature member fails" {
	echo fake >"$tree/spk/syno_signature.asc"
	inspect x86_64
	expect_fail "spk: a signature member is present"
}

@test "a script without the executable bit and a wizard file with it fail" {
	chmod 0644 "$tree/spk/scripts/start-stop-status"
	chmod 0755 "$tree/spk/WIZARD_UIFILES/install_uifile"
	inspect x86_64
	expect_status 1
	expect_line "scripts/start-stop-status(mode -rw-r--r--, want 0755)"
	expect_line "WIZARD_UIFILES/install_uifile(mode -rwxr-xr-x, want 0644)"
}

@test "links and special files fail before anything is extracted" {
	ln -s start-stop-status "$tree/spk/scripts/link"
	ln "$tree/spk/LICENSE" "$tree/spk/LICENSE.hard"
	mkfifo "$tree/spk/fifo"
	inspect x86_64
	expect_fail "spk: unsafe members, nothing extracted:"
	expect_line "scripts/link(link or special file: lrwxrwxrwx)"
	expect_line "LICENSE.hard(link or special file: hrw-r--r--)"
	expect_line "fifo(link or special file: prw-r--r--)"
	refute_line "INFO"
	expect_line "1 failed, 0 warnings"
}

@test "an absolute member and a .. member fail before anything is extracted" {
	echo outside >"$tree/outside"
	echo absolute >"$BATS_TEST_TMPDIR/absolute"
	pack_spk "$tree" "$out"
	(cd "$tree/spk" && tar rf "$out" -P ../outside "$BATS_TEST_TMPDIR/absolute")
	inspect_packed x86_64
	expect_fail "spk: unsafe members, nothing extracted:"
	expect_line "../outside(absolute or outside the archive)"
	expect_line "$BATS_TEST_TMPDIR/absolute(absolute or outside the archive)"
	refute_line "ok   spk: no link"
}

@test "a .. member inside package.tgz fails before the payload is extracted" {
	echo outside >"$BATS_TEST_TMPDIR/escape"
	repack_payload tar rf payload.tar -P ../escape
	pack_outer "$tree" "$out"
	inspect_packed x86_64
	expect_line "ok   spk: no link, special file, absolute name or .. component among the members"
	expect_fail "package.tgz: unsafe members, nothing extracted:"
	expect_line "../escape(absolute or outside the archive)"
}

@test "duplicate members fail, also those a pattern rule allows" {
	pack_spk "$tree" "$out"
	tar rf "$out" -C "$tree/spk" INFO
	inspect_packed x86_64
	expect_fail "spk: members differ from the expected list: INFO(duplicate, 2 times)"
	# A separate tar call stores the copy as a file; within one call GNU tar makes it a hard link.
	repack_payload tar rf payload.tar -C "$tree/pkg" share/licenses/ffmpeg/LICENSE.md
	pack_outer "$tree" "$out"
	inspect_packed x86_64
	expect_fail "package: members differ from the expected list:"
	expect_line "share/licenses/ffmpeg/LICENSE.md(duplicate, 2 times)"
}

@test "an archive tar cannot read is a finding (exit 1), not a usage error" {
	pack_spk "$tree" "$out"
	head -c 1000 "$out" >"$BATS_TEST_TMPDIR/truncated.spk"
	mv "$BATS_TEST_TMPDIR/truncated.spk" "$out"
	inspect_packed x86_64
	expect_fail "spk: tar cannot list the archive"
	expect_line "1 failed, 0 warnings"
	# cut inside the data of bin/demo (a cut inside a header block reads as the end of the
	# archive and shows up as missing members instead)
	repack_payload truncate -s 1100 payload.tar
	pack_outer "$tree" "$out"
	inspect_packed x86_64
	expect_fail "package.tgz: tar cannot list the archive"
}

@test "a base-256 size field is read big-endian, so the header walk sees every member" {
	pack_spk "$tree" "$out"
	base256_size "$out"
	inspect_packed x86_64
	expect_clean
	expect_line "info SPK tar extensions beyond plain ustar: base-256-size"
	long_name="$(printf 'n%.0s' {1..110})"
	echo x >"$tree/spk/$long_name"
	pack_spk "$tree" "$out"
	base256_size "$out"
	inspect_packed x86_64
	expect_status 1
	expect_line "info SPK tar extensions beyond plain ustar: base-256-size,gnu-longname"
}

@test "a gzip-compressed SPK and a gzip-compressed package.tgz fail" {
	pack_spk "$tree" "$out"
	(cd "$tree/spk" && tar czf "$out" INFO package.tgz)
	run sh "$INSPECT" --port "$FIXTURE_PORT" "$out" x86_64
	expect_fail "SPK is gzip-compressed, not an uncompressed tar"
	(cd "$tree/pkg" && tar czf "$tree/spk/package.tgz" bin port_conf share)
	# shellcheck disable=SC2046 # ls order on purpose
	(cd "$tree/spk" && tar cf "$out" $(ls))
	run sh "$INSPECT" --port "$FIXTURE_PORT" "$out" x86_64
	expect_fail "package.tgz is not xz-compressed (magic 1f8b"
}

@test "the tar format and extensions are recorded from the headers" {
	inspect x86_64
	expect_line "ok   SPK is an uncompressed tar (format gnu)"
	expect_line "ok   package.tgz holds a tar (format gnu)"
	pack_spk "$tree" "$out" --format=ustar
	run sh "$INSPECT" --port "$FIXTURE_PORT" "$out" x86_64
	expect_status 0
	expect_line "ok   SPK is an uncompressed tar (format posix)"
	pack_spk "$tree" "$out" --format=posix
	run sh "$INSPECT" --port "$FIXTURE_PORT" "$out" x86_64
	expect_line "ok   SPK is an uncompressed tar (format posix)"
	long_name="$(printf 'n%.0s' {1..110})"
	echo x >"$tree/spk/$long_name"
	inspect x86_64
	expect_status 1
	expect_line "info SPK tar extensions beyond plain ustar: gnu-longname"
}

# --- INFO ---------------------------------------------------------------------------------

@test "a wrong arch fails" {
	sed -i 's/^arch=.*/arch="armv8"/' "$tree/spk/INFO"
	inspect x86_64
	expect_fail 'INFO arch="armv8" does not match ^x86_64$'
}

@test "an adminurl line fails" {
	echo 'adminurl="/"' >>"$tree/spk/INFO"
	inspect x86_64
	expect_fail 'INFO has adminurl (adminurl="/")'
}

@test "a version part above 2^31-1, a bad version format and a wrong beta flag fail" {
	sed -i 's/^version=.*/version="0.1.0-2147483648"/' "$tree/spk/INFO"
	inspect x86_64
	expect_fail "INFO version 0.1.0-2147483648 has a part above 2147483647"
	sed -i 's/^version=.*/version="0.1.0"/' "$tree/spk/INFO"
	inspect x86_64
	expect_fail 'INFO version="0.1.0" does not match'
	fixture_info x86_64 1.2.3-9000 yes >"$tree/spk/INFO"
	inspect x86_64
	expect_fail 'INFO has beta (beta="yes")'
	fixture_info x86_64 1.2.3-1040 no >"$tree/spk/INFO"
	inspect x86_64
	expect_fail "INFO beta: present 0 times, want once"
}

@test "missing, duplicated and forbidden INFO keys fail" {
	sed -i '/^description_ger=/d' "$tree/spk/INFO"
	printf '%s\n' 'thirdparty="yes"' 'startable="no"' 'changelog="x"' 'adminport="9999"' >>"$tree/spk/INFO"
	inspect x86_64
	expect_fail "INFO description_ger: present 0 times, want once"
	expect_line "FAIL INFO has thirdparty"
	expect_line "FAIL INFO has startable"
	expect_line "FAIL INFO has changelog"
	expect_line "FAIL INFO adminport: present 2 times, want once"
}

@test "INFO values with shell-active characters, a wrong os_min_ver and a wrong port fail" {
	# shellcheck disable=SC2016 # the dollar sign must reach INFO unexpanded
	sed -i 's/^displayname=.*/displayname="Demo $HOME"/' "$tree/spk/INFO"
	sed -i 's/^os_min_ver=.*/os_min_ver="7.1-42962"/' "$tree/spk/INFO"
	inspect x86_64
	expect_fail 'INFO: lines that are not key="value": '
	expect_line 'FAIL INFO os_min_ver="7.1-42962" does not match ^7.2-64570$'
	run sh "$INSPECT" --port 8460 "$out" x86_64
	expect_fail 'INFO adminport="9999" does not match ^8460$'
	expect_line 'FAIL port_conf/demo.sc: dst.ports="8460/tcp"'
}

# --- payload, scripts, resources, icons, tokens ---------------------------------------------

@test "a wrong ELF machine fails" {
	fake_elf armv8 static 3.2.0 >"$tree/pkg/bin/ffprobe"
	chmod 0755 "$tree/pkg/bin/ffprobe"
	inspect x86_64
	expect_fail "bin/ffprobe: ELF class 2, machine 183, want 2 62 for x86_64"
}

@test "a binary that is not ELF fails" {
	printf '#!/bin/sh\n' >"$tree/pkg/bin/ffmpeg"
	chmod 0755 "$tree/pkg/bin/ffmpeg"
	inspect x86_64
	expect_fail "bin/ffmpeg is not a little-endian ELF file"
}

@test "a big-endian ELF fails" {
	FAKE_ELF_DATA=2 fake_elf x86_64 static 3.2.0 >"$tree/pkg/bin/ffprobe"
	inspect x86_64
	expect_fail "bin/ffprobe is not a little-endian ELF file"
}

@test "an ELF file that is not an executable fails" {
	FAKE_ELF_TYPE=1 fake_elf x86_64 static 3.2.0 >"$tree/pkg/bin/ffmpeg"
	inspect x86_64
	expect_fail "bin/ffmpeg: ELF type 1 is not an executable (2 or 3)"
	FAKE_ELF_TYPE=3 fake_elf x86_64 static 3.2.0 >"$tree/pkg/bin/ffmpeg"
	inspect x86_64
	expect_clean
	expect_line "ok   bin/ffmpeg: ELF type 3 (executable)"
}

@test "the ABI tag is found after a build-id note in 8-byte aligned note segments" {
	FAKE_ELF_NOTE_ALIGN=8 fake_elf armv8 static 4.18.20 >"$tree/pkg/bin/ffmpeg"
	FAKE_ELF_NOTE_ALIGN=8 fake_elf armv8 static 4.4.0 >"$tree/pkg/bin/ffprobe"
	FAKE_ELF_NOTE_ALIGN=8 fake_elf armv8 static none >"$tree/pkg/bin/$FIXTURE_ID"
	sed -i 's/^arch=.*/arch="armv8"/' "$tree/spk/INFO"
	inspect armv8
	expect_fail "bin/ffmpeg: ELF ABI note 4.18.20 is above the oldest DSM 7.2 kernel 4.4.302"
	expect_line "ok   bin/ffprobe: ELF ABI note 4.4.0 is at most the oldest DSM 7.2 kernel 4.4.302"
	expect_line "info bin/$FIXTURE_ID: no GNU ABI note"
}

@test "an ffmpeg without an ABI note warns in test and release mode" {
	make_tree "$tree" x86_64 selected static none
	inspect x86_64
	expect_status 0
	expect_line "WARN bin/ffmpeg: no GNU ABI note, kernel floor not checked here"
	expect_line "WARN bin/ffprobe: no GNU ABI note, kernel floor not checked here"
	expect_line "0 failed, 2 warnings"
	inspect --release x86_64
	expect_status 0
	expect_line "0 failed, 2 warnings"
}

@test "a leftover token fails and names the member" {
	sed -i "s/^\[demo\]/[@ID@]/" "$tree/pkg/port_conf/demo.sc"
	echo '# @DISPLAY_NAME@' >>"$tree/spk/scripts/postinst"
	inspect x86_64
	expect_fail "unreplaced @NAME@ tokens in: package.tgz:port_conf/demo.sc scripts/postinst"
	expect_line "FAIL port_conf/demo.sc: section [demo]"
}

@test "CR characters in a script, INFO or a JSON member fail" {
	printf '\r\n' >>"$tree/spk/scripts/preinst"
	printf 'maintainer_url="https://example.invalid/x"\r\n' >>"$tree/spk/INFO"
	inspect x86_64
	expect_fail "scripts with CR characters: scripts/preinst"
	expect_line 'FAIL INFO: lines that are not key="value"'
	expect_line "FAIL CR characters in: INFO"
}

@test "a script without the sh shebang fails" {
	printf '#!/bin/bash\nexit 0\n' >"$tree/spk/scripts/postupgrade"
	inspect x86_64
	expect_fail "scripts/postupgrade does not start with #!/bin/sh"
}

@test "a privilege file with root and broken JSON fail" {
	echo '{"defaults": {"run-as": "root"}}' >"$tree/spk/conf/privilege"
	echo '{' >"$tree/spk/WIZARD_UIFILES/uninstall_uifile_ger"
	inspect x86_64
	expect_fail "conf/privilege run-as is not package"
	expect_line "FAIL conf/privilege mentions root"
	expect_line "FAIL WIZARD_UIFILES/uninstall_uifile_ger parses as JSON"
}

@test "a service file without desc or with port forwarding fails" {
	printf '[demo]\ntitle="Demo"\nport_forward="yes"\ndst.ports="9999/tcp"\n' >"$tree/pkg/port_conf/demo.sc"
	inspect x86_64
	expect_fail "port_conf/demo.sc: desc"
	expect_line 'FAIL port_conf/demo.sc: port_forward="no"'
}

@test "a resource that names another protocol file fails" {
	sed -i 's|port_conf/demo.sc|port_conf/other.sc|' "$tree/spk/conf/resource"
	inspect x86_64
	expect_fail "conf/resource port-config names 'port_conf/other.sc', want port_conf/demo.sc"
}

@test "icons of the wrong size or type fail" {
	fake_png 128 >"$tree/spk/PACKAGE_ICON.PNG"
	echo "not a png" >"$tree/spk/PACKAGE_ICON_256.PNG"
	inspect x86_64
	expect_fail "PACKAGE_ICON.PNG is not a 64x64 PNG (128x128)"
	expect_line "FAIL PACKAGE_ICON_256.PNG is not a 256x256 PNG (not a PNG)"
}

# --- member digests and usage -----------------------------------------------------------------

@test "--members prints one digest line per file and two packs of one tree agree" {
	inspect --members x86_64
	expect_status 0
	local table_a table_b
	table_a="$(grep -E '^[0-9a-f]{64} ' <<<"$output")"
	expect_line " 755 spk:scripts/start-stop-status"
	expect_line " 644 spk:INFO"
	expect_line " 755 package.tgz:bin/ffmpeg"
	expect_line " 644 package.tgz:share/licenses/ffmpeg/SOURCE.txt"
	[ "$(wc -l <<<"$table_a")" -eq 25 ]
	sleep 1
	inspect --members x86_64
	table_b="$(grep -E '^[0-9a-f]{64} ' <<<"$output")"
	[ "$table_a" = "$table_b" ]
}

@test "TERM ends the inspection with exit 143 and removes the work directory" {
	pack_spk "$tree" "$out"
	local stubs="$BATS_TEST_TMPDIR/stubs" tmp="$BATS_TEST_TMPDIR/tmp"
	local marker="$BATS_TEST_TMPDIR/xz-started" pid tries=0 code=0
	mkdir -p "$stubs" "$tmp"
	# an xz that blocks, so the signal arrives while package.tgz is being decompressed
	printf '#!/bin/sh\n: >"%s"\nexec sleep 30\n' "$marker" >"$stubs/xz"
	chmod 0755 "$stubs/xz"
	# setsid: an own process group, so the signal reaches the script and the stub together
	TMPDIR="$tmp" PATH="$stubs:$PATH" setsid sh "$INSPECT" --port "$FIXTURE_PORT" "$out" x86_64 \
		>"$BATS_TEST_TMPDIR/log" 2>&1 &
	pid=$!
	while [ ! -e "$marker" ] && [ "$tries" -lt 100 ]; do
		sleep 0.1
		tries=$((tries + 1))
	done
	[ -e "$marker" ]
	kill -TERM -- "-$pid"
	wait "$pid" || code=$?
	[ "$code" -eq 143 ] || { cat "$BATS_TEST_TMPDIR/log" >&2; echo "exit $code" >&2; return 1; }
	[ -z "$(ls -A "$tmp")" ]
}

@test "usage errors exit 2" {
	run sh "$INSPECT"
	expect_status 2
	run sh "$INSPECT" "$out" mips
	expect_status 2
	run sh "$INSPECT" "$BATS_TEST_TMPDIR/missing.spk" x86_64
	expect_status 2
	pack_spk "$tree" "$out"
	run sh "$INSPECT" --port 0 "$out" x86_64
	expect_status 2
	run sh "$INSPECT" --bogus "$out" x86_64
	expect_status 2
	run sh "$INSPECT" --expected "$BATS_TEST_TMPDIR/missing.txt" "$out" x86_64
	expect_status 2
	run sh "$INSPECT" --port 65536 "$out" x86_64
	expect_status 2
}

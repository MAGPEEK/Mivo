#!/bin/bash
# Stages the placeholder test project of the toolkit walking skeleton (SPK-04) for one family:
# the package's toolkit files (SynoBuildConf/, INFO.sh), the fixture files of demo/, build.env
# with the family's ARCH, and three tiny static executables as server, ffmpeg and ffprobe.
# The real project is staged by spkstage (SPK-14); this script only stands in for it in tests.
#
#   stage-demo.sh <x86_64|armv8|armv7> <project-dir>
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
package_dir="$(cd "$here/../../package" && pwd)"

# Minimal static ELF executables that only call exit(0): ELF header, one PT_LOAD program header
# and the exit system call, loaded at 0x400000 (64-bit) or 0x10000 (32-bit). Bytes are
# little-endian; offsets and sizes are in the comments.
readonly ELF_HEADER_64="7f454c46020101000000000000000000"      # magic, ELFCLASS64, LSB, version 1
readonly ELF_X86_64="${ELF_HEADER_64}02003e000100000078004000000000004000000000000000\
0000000000000000000000004000380001000000000000000100000005000000\
0000000000000000000040000000000000004000000000008100000000000000\
81000000000000000010000000000000\
31ffb83c0000000f05"                                                  # xor edi,edi; mov eax,60; syscall (129 bytes)
readonly ELF_ARMV8="${ELF_HEADER_64}0200b7000100000078004000000000004000000000000000\
0000000000000000000000004000380001000000000000000100000005000000\
0000000000000000000040000000000000004000000000008400000000000000\
84000000000000000010000000000000\
000080d2a80b80d2010000d4"                                            # mov x0,#0; mov x8,#93; svc #0 (132 bytes)
readonly ELF_ARMV7="7f454c46010101000000000000000000\
020028000100000054000100340000000000000000040005340020000100000000000000\
0100000000000000000001000000010060000000600000000500000000100000\
0000a0e30170a0e3000000ef"                                            # EABI5 hard-float; mov r0,#0; mov r7,#1; svc #0 (96 bytes)

die() { printf 'stage-demo: %s\n' "$*" >&2; exit 1; }

elf_hex() {
	case "$1" in
		x86_64) echo "$ELF_X86_64" ;;
		armv8) echo "$ELF_ARMV8" ;;
		armv7) echo "$ELF_ARMV7" ;;
		*) return 1 ;;
	esac
}

# Writes hex digits as bytes; printf %b understands \xHH, including \x00.
write_hex() {
	printf '%b' "$(printf '%s' "$1" | sed 's/\(..\)/\\x\1/g')" >"$2"
}

main() {
	[ "$#" -eq 2 ] || die "usage: stage-demo.sh <x86_64|armv8|armv7> <project-dir>"
	local family="$1" project="$2" hex binary
	hex="$(elf_hex "$family")" || die "unknown family '$family'"
	[ ! -e "$project" ] || [ -z "$(ls -A "$project")" ] || die "$project exists and is not empty"

	mkdir -p "$project/SynoBuildConf" "$project/prebuilt/bin"
	cp "$package_dir/SynoBuildConf/depends" "$package_dir/SynoBuildConf/build" \
		"$package_dir/SynoBuildConf/install" "$project/SynoBuildConf/"
	cp "$package_dir/INFO.sh" "$project/"
	cp -R "$here/demo/." "$project/"
	printf "ARCH='%s'\n" "$family" >>"$project/build.env"
	for binary in demo ffmpeg ffprobe; do
		write_hex "$hex" "$project/prebuilt/bin/$binary"
		chmod 0755 "$project/prebuilt/bin/$binary"
	done
	chmod 0755 "$project/INFO.sh" "$project"/scripts/*
}

main "$@"

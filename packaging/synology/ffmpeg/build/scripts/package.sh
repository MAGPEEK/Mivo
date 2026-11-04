#!/bin/sh
# package.sh - check the binaries of one family and assemble its output directory.
#
#   package.sh <family> <build-dir> <sources-list> <source-archive-dir> <dest>
#
# <build-dir> holds ffmpeg, ffprobe, configuration.txt and config-facts.txt (build-ffmpeg.sh).
# The script refuses binaries that are not fully static, are not ET_EXEC files of the family's
# ELF class and machine, carry an ELF ABI note above the family's oldest DSM 7.2 kernel, or
# (armv7) contain VFPv4 instructions. <dest> receives ffmpeg, ffprobe (0755), the licence texts
# of everything linked and SOURCE.txt (0644). SOURCE.txt records versions, URLs and SHA-256 of
# every source and of both binaries, the configure line, the CPU-feature facts of config.h, the
# check results and the status "selected" (the status field of
# packaging/synology/ffmpeg/sources.lock).
#
# Every gate captures the tool's output first and fails when the tool itself fails: POSIX sh has
# no pipefail, so `readelf ... | grep -q` would pass a binary readelf could not read. Each
# capture ends with an explicit `|| exit 1` instead of relying on set -e, which shells such as
# BusyBox ash do not apply inside command substitutions; the gates run in the main shell and
# hand values over in variables, never through a command substitution of a gate.
#
# Environment: BUILDER_IMAGE (base image reference of the build) and HOST_PACKAGES (the Debian
# packages of the host stage), both recorded in SOURCE.txt.
# shellcheck source-path=SCRIPTDIR
set -eu

[ "$#" -eq 5 ] || {
	printf 'usage: %s <family> <build-dir> <sources-list> <source-archive-dir> <dest>\n' "$0" >&2
	exit 2
}
family=$1
build=$2
list=$3
archive_dir=$4
dest=$5
script_dir=$(dirname "$0")

# shellcheck source=families.sh
. "$script_dir/families.sh"
# shellcheck source=layout.sh
. "$script_dir/layout.sh"
# shellcheck source=recipe.sh
. "$script_dir/recipe.sh"
load_family "$family"

readonly ELF_TYPE=EXEC

fail() {
	printf 'package.sh: %s\n' "$*" >&2
	exit 1
}

sha256_of() { sha256sum "$1" | cut -d ' ' -f 1; }

# version_at_most reports whether dotted version $1 <= $2.
version_at_most() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n 1)" = "$1" ]; }

# readelf_of <option> <file>: readelf output, or the build fails.
readelf_of() {
	"$TRIPLET-readelf" "$1" "$2" || fail "readelf $1 failed on $(basename "$2")"
}

# header_field <header-output> <field>: the value of one "Field: value" line of readelf -h.
header_field() { printf '%s\n' "$1" | sed -n "s/^ *$2: *//p" | head -n 1; }

# check_identity fails unless the ELF header has the family's class and machine and the
# executable type (ET_EXEC: static, non-PIE; a static PIE would be DYN, BACKLOG "spk/ffmpeg build").
check_identity() {
	header=$(readelf_of -hW "$1") || exit 1
	class=$(header_field "$header" Class)
	type=$(header_field "$header" Type | cut -d ' ' -f 1)
	machine=$(header_field "$header" Machine)
	[ "$class" = "$ELF_CLASS" ] || fail "$(basename "$1") is $class, the family needs $ELF_CLASS"
	[ "$type" = "$ELF_TYPE" ] || fail "$(basename "$1") is of ELF type $type, not $ELF_TYPE"
	[ "$machine" = "$ELF_MACHINE" ] || fail "$(basename "$1") is built for '$machine', the family needs '$ELF_MACHINE'"
}

# check_static fails unless the binary has no interpreter and no shared-library dependency.
# Each output is assigned before it is matched: a failing readelf inside the case word would
# only end its subshell and leave an empty word, which matches nothing and passes.
check_static() {
	segments=$(readelf_of -lW "$1") || exit 1
	dynamic=$(readelf_of -dW "$1") || exit 1
	case "$segments" in *INTERP*) fail "$(basename "$1") has a PT_INTERP" ;; esac
	case "$dynamic" in *"(NEEDED)"*) fail "$(basename "$1") needs shared libraries" ;; esac
}

# check_abi_note sets note to the minimum kernel of the ELF ABI note ("none" without one) and
# fails when it is above the family's oldest DSM 7.2 kernel. The notes are assigned before the
# sed pipeline, which would report only its last command.
check_abi_note() {
	notes=$(readelf_of -nW "$1") || exit 1
	note=$(printf '%s\n' "$notes" | sed -n 's/.*ABI: *\([0-9.]*\).*/\1/p' | head -n 1)
	note=${note:-none}
	if [ "$note" != none ] && ! version_at_most "$note" "$OLDEST_KERNEL"; then
		fail "$(basename "$1") has ABI note $note > $OLDEST_KERNEL"
	fi
}

# count_instructions <pattern> <disassembly-file>: matching lines; grep's "no match" (1) is a
# count of 0, any other failure stops the build.
count_instructions() {
	status=0
	count=$(grep -cE "$1" "$2") || status=$?
	[ "$status" -le 1 ] || fail "cannot count instructions in $2"
	printf '%s' "$count"
}

# check_arm_instructions refuses fused multiply-add (VFPv4, which Cortex-A9 lacks) and counts
# NEON loads, a marker for the NEON code behind run-time detection.
check_arm_instructions() {
	tool=$1
	"$TRIPLET-objdump" -d "$2" >"$work/disassembly" || fail "objdump failed on $tool"
	vfpv4=$(count_instructions '[[:space:]]v(fma|fms|fnma|fnms)\.' "$work/disassembly") || exit 1
	[ "$vfpv4" -eq 0 ] || fail "$tool contains $vfpv4 VFPv4 instructions"
	neon=$(count_instructions '[[:space:]]vld1\.' "$work/disassembly") || exit 1
	build_attributes=$(readelf_of -A "$2") || exit 1
	attributes=$(printf '%s\n' "$build_attributes" | grep -E 'Tag_(CPU_arch|FP_arch|Advanced_SIMD_arch|ABI_VFP_args):' |
		sed 's/^ *//' | tr '\n' ';')
	printf '  %-8s VFPv4 instructions (vfma/vfms/vfnma/vfnms): 0; NEON loads (vld1, run-time detected): %s; %s\n' \
		"$tool" "$neon" "$attributes" >>"$checks"
}

# check_binary runs every check on one tool and appends the results to $checks.
check_binary() {
	tool=$1
	path="$build/$tool"
	[ -f "$path" ] || fail "missing $path"
	check_identity "$path"
	check_static "$path"
	check_abi_note "$path"
	printf '  %-8s %s %s %s, static (no PT_INTERP, no NEEDED), ELF ABI note: %s (limit %s), %s bytes\n' \
		"$tool" "$ELF_CLASS" "$ELF_TYPE" "$ELF_MACHINE" "$note" "$OLDEST_KERNEL" "$(wc -c <"$path")" >>"$checks"
	[ "$family" = armv7 ] || return 0
	check_arm_instructions "$tool" "$path"
}

# source_lines prints one line per pinned source: name version linked sha256 url.
source_lines() {
	grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$list" |
		awk '{ printf "  %-16s %-13s %-3s %s %s\n", $1, $2, $4, $6, $7 }'
}

# host_tools prints the installed version of every Debian package of the host stage, or
# "unknown" without a list: dpkg-query -W without package names would list every package.
host_tools() {
	if [ -z "${HOST_PACKAGES:-}" ]; then
		printf 'unknown'
		return 0
	fi
	# shellcheck disable=SC2086 # HOST_PACKAGES is a blank-separated list
	dpkg-query -W -f '${Package}=${Version} ' $HOST_PACKAGES 2>/dev/null || true
}

# require_sha256 <what> <value> fails unless the value is a lowercase SHA-256 in hex.
require_sha256() {
	case "$2" in
	*[!0-9a-f]* | '') fail "$1 is not a SHA-256: '$2'" ;;
	esac
	[ "${#2}" -eq 64 ] || fail "$1 is not a SHA-256: '$2'"
}

# read_source_archive sets archive_sha and archive_name from the one <name>.tar.sha256 of
# <source-archive-dir> (a line "<sha256>  <name>" naming a file of that directory); none,
# several, a malformed line or a missing archive fail.
read_source_archive() {
	sums=$(find "$archive_dir" -maxdepth 1 -type f -name '*.tar.sha256') || fail "cannot list $archive_dir"
	[ -n "$sums" ] || fail "no *.tar.sha256 in $archive_dir"
	[ "$(printf '%s\n' "$sums" | wc -l)" -eq 1 ] || fail "more than one *.tar.sha256 in $archive_dir"
	[ "$(wc -l <"$sums")" -eq 1 ] || fail "$(basename "$sums") must hold exactly one line"
	sum_line=$(cat "$sums") || fail "cannot read $sums"
	# sha256sum's format: hash, blank, "*" or blank (binary or text mode), file name.
	archive_sha=${sum_line%% *}
	require_sha256 "the source archive checksum in $(basename "$sums")" "$archive_sha"
	# A line without the separator (a hash alone) names no file: the name stays empty.
	case "$sum_line" in
	"$archive_sha  "?* | "$archive_sha *"?*) archive_name=${sum_line#"$archive_sha" ?} ;;
	*) archive_name= ;;
	esac
	case "$archive_name" in
	'' | */* | *' '*) fail "$(basename "$sums") names no archive file: '$archive_name'" ;;
	esac
	[ -f "$archive_dir/$archive_name" ] || fail "$(basename "$sums"): $archive_name is not a file in $archive_dir"
}

# write_source_notice computes every value first and fails on the first one it cannot get, so
# SOURCE.txt never carries an empty or multi-line value: a command inside the here-document
# would fail without stopping the script.
write_source_notice() {
	ffmpeg_version=$(awk '$1 == "ffmpeg" { print $2 }' "$list") || fail "cannot read $list"
	[ -n "$ffmpeg_version" ] && [ "$(printf '%s\n' "$ffmpeg_version" | wc -l)" -eq 1 ] ||
		fail "$list needs exactly one ffmpeg entry"
	version_text=$(version_label "$ffmpeg_version") || fail "no version label for FFmpeg $ffmpeg_version"
	read_source_archive
	ffmpeg_sha=$(sha256_of "$dest/ffmpeg") || fail "cannot hash $dest/ffmpeg"
	ffprobe_sha=$(sha256_of "$dest/ffprobe") || fail "cannot hash $dest/ffprobe"
	require_sha256 "the SHA-256 of ffmpeg" "$ffmpeg_sha"
	require_sha256 "the SHA-256 of ffprobe" "$ffprobe_sha"
	configuration="$build/configuration.txt"
	[ -f "$configuration" ] || fail "missing $configuration"
	[ "$(wc -l <"$configuration")" -eq 1 ] || fail "$configuration must hold exactly one line"
	configure_line=$(cat "$configuration") || fail "cannot read $configuration"
	[ -n "$configure_line" ] || fail "$configuration is empty"
	config_facts=$(sed 's/^/  /' "$build/config-facts.txt") || fail "cannot read $build/config-facts.txt"
	sources=$(source_lines) || fail "cannot list the sources of $list"
	check_results=$(cat "$checks") || fail "cannot read the check results"
	tools=$(host_tools)
	cat >"$dest/SOURCE.txt" <<EOF || fail "cannot write $dest/SOURCE.txt"
FFmpeg build bundled for the DSM architecture family $family

Version:              $version_text
Status:               selected
Licence:              GPL-3.0-or-later
Built by:             static FFmpeg recipe (packaging/synology/ffmpeg/build), task SPK-15
Target:               $TRIPLET, $ARCH_CFLAGS, musl (static)
ffmpeg SHA-256:       $ffmpeg_sha
ffprobe SHA-256:      $ffprobe_sha
Corresponding source: $archive_name
Source SHA-256:       $archive_sha

The corresponding source is attached to every GitHub Release that carries these binaries.
It contains the recipe and every source below.

Configuration:
  $configure_line

CPU features of FFmpeg's config.h (1 = compiled in, selected at run time where FFmpeg does so):
$config_facts

Sources (name, version, linked into the binaries, SHA-256, URL):
$sources

Build checks:
$check_results

Build host: ${BUILDER_IMAGE:-unknown}
Host tools: $tools
EOF
}

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
checks="$work/checks"
: >"$checks"
[ -f "$build/config-facts.txt" ] || fail "missing $build/config-facts.txt"
mkdir -p "$dest"
check_binary ffmpeg
check_binary ffprobe
for tool in ffmpeg ffprobe; do
	cp "$build/$tool" "$dest/$tool"
	chmod 0755 "$dest/$tool"
done
cp "$LICENSE_DIR"/* "$dest/"
write_source_notice
for file in "$dest"/*; do
	case "$(basename "$file")" in ffmpeg | ffprobe) ;; *) chmod 0644 "$file" ;; esac
done
printf 'package.sh: %s\n' "$dest"
cat "$dest/SOURCE.txt"

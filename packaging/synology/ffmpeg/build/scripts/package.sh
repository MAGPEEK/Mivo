#!/bin/sh
# package.sh - check the binaries of one family and assemble its output directory.
#
#   package.sh <family> <build-dir> <sources-list> <source-archive-dir> <dest>
#
# <build-dir> holds ffmpeg, ffprobe and configuration.txt (build-ffmpeg.sh). The script refuses
# binaries that are not fully static, carry an ELF ABI note above the family's oldest DSM 7.2
# kernel, or (armv7) contain VFPv4 instructions. <dest> receives ffmpeg, ffprobe (0755), the
# licence texts of everything linked and SOURCE.txt (0644). SOURCE.txt records versions, URLs and
# SHA-256 of every source and of both binaries, the configure line, the check results and the
# status "selected" (the status field of packaging/synology/ffmpeg/sources.lock).
#
# Environment: BUILDER_IMAGE (base image reference of the build, recorded in SOURCE.txt).
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

fail() {
	printf 'package.sh: %s\n' "$*" >&2
	exit 1
}

sha256_of() { sha256sum "$1" | cut -d ' ' -f 1; }

# version_at_most reports whether dotted version $1 <= $2.
version_at_most() { [ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n 1)" = "$1" ]; }

# check_static fails unless the binary has no interpreter and no shared-library dependency.
check_static() {
	if "$TRIPLET-readelf" -lW "$1" | grep -q 'INTERP'; then fail "$(basename "$1") has a PT_INTERP"; fi
	if "$TRIPLET-readelf" -dW "$1" | grep -q '(NEEDED)'; then fail "$(basename "$1") needs shared libraries"; fi
}

# abi_note prints the minimum kernel of the ELF ABI note, or "none".
abi_note() {
	note=$("$TRIPLET-readelf" -nW "$1" | sed -n 's/.*ABI: *\([0-9.]*\).*/\1/p' | head -n 1)
	printf '%s' "${note:-none}"
}

check_abi_note() {
	note=$(abi_note "$1")
	if [ "$note" != none ] && ! version_at_most "$note" "$OLDEST_KERNEL"; then
		fail "$(basename "$1") has ABI note $note > $OLDEST_KERNEL"
	fi
	printf '%s' "$note"
}

# vfpv4_count counts fused multiply-add instructions, which VFPv3 CPUs (Cortex-A9) lack.
vfpv4_count() {
	"$TRIPLET-objdump" -d "$1" | grep -cE '[[:space:]]v(fma|fms|fnma|fnms)\.' || true
}

# neon_count counts NEON loads, a marker for the NEON code behind run-time detection.
neon_count() {
	"$TRIPLET-objdump" -d "$1" | grep -cE '[[:space:]]vld1\.' || true
}

# check_binary runs every check on one tool and appends the results to $checks.
check_binary() {
	tool=$1
	path="$build/$tool"
	[ -f "$path" ] || fail "missing $path"
	check_static "$path"
	note=$(check_abi_note "$path")
	printf '  %-8s static (no PT_INTERP, no NEEDED), ELF ABI note: %s (limit %s), %s bytes\n' \
		"$tool" "$note" "$OLDEST_KERNEL" "$(wc -c <"$path")" >>"$checks"
	[ "$family" = armv7 ] || return 0
	vfpv4=$(vfpv4_count "$path")
	[ "$vfpv4" -eq 0 ] || fail "$tool contains $vfpv4 VFPv4 instructions"
	attributes=$("$TRIPLET-readelf" -A "$path" | grep -E 'Tag_(CPU_arch|FP_arch|Advanced_SIMD_arch|ABI_VFP_args):' |
		sed 's/^ *//' | tr '\n' ';')
	printf '  %-8s VFPv4 instructions (vfma/vfms/vfnma/vfnms): 0; NEON loads (vld1, run-time detected): %s; %s\n' \
		"$tool" "$(neon_count "$path")" "$attributes" >>"$checks"
}

# source_lines prints one line per pinned source: name version linked sha256 url.
source_lines() {
	grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$list" |
		awk '{ printf "  %-16s %-13s %-3s %s %s\n", $1, $2, $4, $6, $7 }'
}

host_tools() {
	dpkg-query -W -f '${Package}=${Version} ' gcc make meson ninja-build pkgconf 2>/dev/null || true
}

write_source_notice() {
	ffmpeg_version=$(awk '$1 == "ffmpeg" { print $2 }' "$list")
	archive_sha=$(cut -d ' ' -f 1 "$archive_dir"/*.tar.sha256)
	archive_name=$(sed 's/^[0-9a-f]*  *//' "$archive_dir"/*.tar.sha256)
	cat >"$dest/SOURCE.txt" <<EOF
FFmpeg build bundled for the DSM architecture family $family

Version:              $(version_label "$ffmpeg_version")
Status:               selected
Licence:              GPL-3.0-or-later
Built by:             static FFmpeg recipe (packaging/synology/ffmpeg/build), task SPK-15
Target:               $TRIPLET, $ARCH_CFLAGS, musl (static)
ffmpeg SHA-256:       $(sha256_of "$dest/ffmpeg")
ffprobe SHA-256:      $(sha256_of "$dest/ffprobe")
Corresponding source: $archive_name
Source SHA-256:       $archive_sha

The corresponding source is attached to every GitHub Release that carries these binaries.
It contains the recipe and every source below.

Configuration:
  $(cat "$build/configuration.txt")

Sources (name, version, linked into the binaries, SHA-256, URL):
$(source_lines)

Build checks:
$(cat "$checks")

Build host: ${BUILDER_IMAGE:-unknown}
Host tools: $(host_tools)
EOF
}

mkdir -p "$dest"
checks=$(mktemp)
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
rm -f "$checks"
printf 'package.sh: %s\n' "$dest"
cat "$dest/SOURCE.txt"

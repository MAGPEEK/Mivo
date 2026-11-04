# shellcheck shell=bash
# Fixture of inspect-spk.bats: a synthetic server SPK of a made-up package ("demo", port 9999),
# built from minimal ELF files, PNGs and INFO, so the inspection is tested without the toolkit
# and without the real binaries. The layout and modes follow toolkit/README.md "Staged
# project" and expected-members.txt.

readonly FIXTURE_ID=demo
readonly FIXTURE_PORT=9999
readonly FIXTURE_DSM_VERSION="1.2.3-1040"
readonly FIXTURE_INTERPRETER="/lib/ld.so"

# byte <value>: one byte, written through an octal escape.
byte() {
	local escape
	escape=$(printf '\\%03o' "$1")
	# shellcheck disable=SC2059 # the escape is the format on purpose
	printf "$escape"
}

# le <bytes> <value>: the value as little-endian bytes.
le() {
	local width=$1 value=$2 index
	for ((index = 0; index < width; index++)); do
		byte $((value & 255))
		value=$((value >> 8))
	done
}

# be <bytes> <value>: the value as big-endian bytes.
be() {
	local width=$1 value=$2 index
	for ((index = width - 1; index >= 0; index--)); do
		byte $(((value >> (8 * index)) & 255))
	done
}

# ELF facts of the fake binaries: program header and note types, the GNU build-id length.
readonly PT_INTERP=3
readonly PT_NOTE=4
readonly NT_GNU_ABI_TAG=1
readonly NT_GNU_BUILD_ID=3
readonly BUILD_ID_BYTES=20
readonly ABI_TAG_BYTES=16
readonly ARM_EABI5_HARD_FLOAT=$((0x05000400))

# align_up <value> <alignment>
align_up() {
	echo $((($1 + $2 - 1) / $2 * $2))
}

# fake_note_size <align> <descsz>: bytes of a note named "GNU" (12-byte header, name and
# descriptor each padded to the alignment, as glibc's ELF_NOTE_NEXT_OFFSET computes).
fake_note_size() {
	align_up $(($(align_up 16 "$1") + $2)) "$1"
}

# fake_note <align> <type> <descsz> <word...>: one note named "GNU" whose descriptor is the
# 32-bit words given (or 0xab bytes when none are given), padded to the alignment.
fake_note() {
	local align=$1 type=$2 descsz=$3 index desc_offset end
	shift 3
	desc_offset=$(align_up 16 "$align")
	end=$(fake_note_size "$align" "$descsz")
	le 4 4; le 4 "$descsz"; le 4 "$type"; printf 'GNU\000'
	for ((index = 16; index < desc_offset; index++)); do byte 0; done
	if [ "$#" -gt 0 ]; then
		for index in "$@"; do le 4 "$index"; done
	else
		for ((index = 0; index < descsz; index++)); do byte 171; done
	fi
	for ((index = desc_offset + descsz; index < end; index++)); do byte 0; done
}

# fake_elf <family> <static|dynamic> <none|major.minor.patch>: a minimal ELF executable of
# the family's class and machine on stdout. One PT_NOTE segment holds a GNU build-id note
# and, when an ABI version is given, the GNU ABI tag after it (as real binaries order them);
# a PT_INTERP segment follows when dynamic. Variables tune the negative cases:
# FAKE_ELF_DATA (EI_DATA, default 1 little-endian; 2 marks the file big-endian),
# FAKE_ELF_TYPE (e_type, default 2 ET_EXEC) and FAKE_ELF_NOTE_ALIGN (p_align of the note
# segment and the padding of its notes, 4 or 8; default 4).
fake_elf() {
	local family=$1 linkage=$2 abi=$3 class machine ehsize phentsize phnum=1 note_size flags=0
	local data=${FAKE_ELF_DATA:-1} type=${FAKE_ELF_TYPE:-2} align=${FAKE_ELF_NOTE_ALIGN:-4}
	case "$family" in
		x86_64) class=2 machine=62 ;;
		armv8) class=2 machine=183 ;;
		armv7) class=1 machine=40 flags=$ARM_EABI5_HARD_FLOAT ;;
		*) echo "fake_elf: unknown family $family" >&2 && return 1 ;;
	esac
	if [ "$class" -eq 2 ]; then ehsize=64 phentsize=56; else ehsize=52 phentsize=32; fi
	note_size=$(fake_note_size "$align" "$BUILD_ID_BYTES")
	[ "$abi" = none ] || note_size=$((note_size + $(fake_note_size "$align" "$ABI_TAG_BYTES")))
	[ "$linkage" = static ] || phnum=2
	local note_offset=$((ehsize + phnum * phentsize))
	printf '\177ELF'
	le 1 "$class"; le 1 "$data"; le 1 1; le 1 0; le 8 0
	le 2 "$type"; le 2 "$machine"; le 4 1
	fake_ehdr_rest "$class" "$ehsize" "$phentsize" "$phnum" "$flags"
	fake_phdr "$class" "$PT_NOTE" "$note_offset" "$note_size" "$align"
	[ "$linkage" = static ] ||
		fake_phdr "$class" "$PT_INTERP" $((note_offset + note_size)) $((${#FIXTURE_INTERPRETER} + 1)) 1
	fake_note "$align" "$NT_GNU_BUILD_ID" "$BUILD_ID_BYTES"
	if [ "$abi" != none ]; then
		local major=${abi%%.*} rest=${abi#*.} minor patch
		minor=${rest%%.*}
		patch=${rest#*.}
		fake_note "$align" "$NT_GNU_ABI_TAG" "$ABI_TAG_BYTES" 0 "$major" "$minor" "$patch"
	fi
	[ "$linkage" = static ] || printf '%s\000' "$FIXTURE_INTERPRETER"
}

# fake_ehdr_rest <class> <ehsize> <phentsize> <phnum> <flags>: the ELF header after e_version
# (entry, program and section header offsets, flags, sizes and counts).
fake_ehdr_rest() {
	local class=$1 ehsize=$2 phentsize=$3 phnum=$4 flags=$5
	if [ "$class" -eq 2 ]; then
		le 8 0; le 8 "$ehsize"; le 8 0; le 4 "$flags"
	else
		le 4 0; le 4 "$ehsize"; le 4 0; le 4 "$flags"
	fi
	le 2 "$ehsize"; le 2 "$phentsize"; le 2 "$phnum"; le 2 0; le 2 0; le 2 0
}

# fake_phdr <class> <type> <offset> <size> <align>: one program header.
fake_phdr() {
	if [ "$1" -eq 2 ]; then
		le 4 "$2"; le 4 5; le 8 "$3"; le 8 0; le 8 0; le 8 "$4"; le 8 "$4"; le 8 "$5"
	else
		le 4 "$2"; le 4 "$3"; le 4 0; le 4 0; le 4 "$4"; le 4 "$4"; le 4 5; le 4 "$5"
	fi
}

# fake_png <side>: a PNG header with an IHDR chunk of side x side pixels (no image data; the
# inspection reads only the dimensions).
fake_png() {
	printf '\211PNG\r\n\032\n'
	be 4 13
	printf 'IHDR'
	be 4 "$1"
	be 4 "$1"
	printf '\010\006\000\000\000'
	be 4 0
}

# fixture_info <family> [version] [beta yes|no]: the INFO of the fixture as the toolkit writes it.
fixture_info() {
	local family=$1 version=${2:-$FIXTURE_DSM_VERSION} beta=${3:-yes} arch
	case "$family" in armv7) arch="armv7 armada38x monaco" ;; *) arch=$family ;; esac
	cat <<EOF
package="$FIXTURE_ID"
version="$version"
maintainer="Example Maintainer"
maintainer_url="https://example.invalid/demo"
arch="$arch"
adminprotocol="http"
adminport="$FIXTURE_PORT"
dsmuidir="ui"
os_min_ver="7.2-64570"
EOF
	[ "$beta" = no ] || echo 'beta="yes"'
	cat <<'EOF'
description="Placeholder package of the inspection tests."
displayname="Demo Server"
description_ger="Platzhalterpaket der Inspektionstests."
support_move="no"
extractsize="40"
toolkit_version="72723"
create_time="20261031-20:54:28"
EOF
}

# fixture_notice <status> <licence>: SOURCE.txt of the FFmpeg build.
fixture_notice() {
	cat <<EOF
FFmpeg build bundled for the DSM architecture family of the fixture

Version:              9.9.9
Status:               $1
Licence:              $2
Corresponding source: https://example.invalid/ffmpeg-src.tar.xz
EOF
}

# Sizes of the main menu icons (DSM's {0} of the config's "icon").
readonly FIXTURE_DESKTOP_ICON_SIZES="16 24 32 48 64 72 256"
# Keys of the main menu entry as jq's `keys` lists them (sorted).
# shellcheck disable=SC2034 # used by inspect-spk.bats
readonly FIXTURE_DESKTOP_KEYS="advanceGrantPrivilege,allUsers,desc,grantPrivilege,icon,port,protocol,title,type,url"

# fixture_desktop_config: ui/config as spkmeta renders it for the fixture package.
fixture_desktop_config() {
	cat <<EOF
{
  ".url": {
    "com.examplemaintainer.$FIXTURE_ID": {
      "type": "url",
      "title": "Demo Server",
      "desc": "Opens the web app of the media server",
      "icon": "images/${FIXTURE_ID}_{0}.png",
      "protocol": "http",
      "port": "$FIXTURE_PORT",
      "url": "/",
      "allUsers": true,
      "grantPrivilege": "all",
      "advanceGrantPrivilege": true
    }
  }
}
EOF
}

# make_tree <dir> <family> <status> <static|dynamic> <abi>: the unpacked SPK below <dir>/spk
# with the payload below <dir>/pkg, every mode set explicitly.
make_tree() {
	local dir=$1 family=$2 status=$3 linkage=$4 abi=$5 script size
	local spk="$dir/spk" pkg="$dir/pkg"
	rm -rf "$dir"
	mkdir -p "$spk/conf" "$spk/scripts" "$spk/WIZARD_UIFILES" \
		"$pkg/bin" "$pkg/port_conf" "$pkg/share/licenses/ffmpeg" "$pkg/ui/images"
	fixture_desktop_config >"$pkg/ui/config"
	for size in $FIXTURE_DESKTOP_ICON_SIZES; do
		fake_png "$size" >"$pkg/ui/images/${FIXTURE_ID}_$size.png"
	done
	fixture_info "$family" >"$spk/INFO"
	echo "GNU GENERAL PUBLIC LICENSE (fixture)" >"$spk/LICENSE"
	fake_png 64 >"$spk/PACKAGE_ICON.PNG"
	fake_png 256 >"$spk/PACKAGE_ICON_256.PNG"
	echo '{"defaults": {"run-as": "package"}}' >"$spk/conf/privilege"
	printf '{"data-share": {"shares": [{"name": "{{wizard_media_share}}", %s}]}, %s}\n' \
		"\"permission\": {\"ro\": [\"$FIXTURE_ID\"]}" \
		"\"port-config\": {\"protocol-file\": \"port_conf/$FIXTURE_ID.sc\"}" >"$spk/conf/resource"
	for script in preinst postinst preuninst postuninst preupgrade postupgrade start-stop-status; do
		printf '#!/bin/sh\nexit 0\n' >"$spk/scripts/$script"
	done
	printf 'pkg_noop() { :; }\n' >"$spk/scripts/common.sh"
	printf "ID='%s'\nPORT='%s'\n" "$FIXTURE_ID" "$FIXTURE_PORT" >"$spk/scripts/package.env"
	for script in install_uifile install_uifile_ger uninstall_uifile uninstall_uifile_ger; do
		echo '[{"step_title": "Fixture", "items": []}]' >"$spk/WIZARD_UIFILES/$script"
	done
	fake_elf "$family" static none >"$pkg/bin/$FIXTURE_ID"
	fake_elf "$family" "$linkage" "$abi" >"$pkg/bin/ffmpeg"
	fake_elf "$family" "$linkage" "$abi" >"$pkg/bin/ffprobe"
	printf '%s\n' "[$FIXTURE_ID]" 'title="Demo Server"' 'desc="Demo Server media server"' \
		'port_forward="no"' "dst.ports=\"$FIXTURE_PORT/tcp\"" >"$pkg/port_conf/$FIXTURE_ID.sc"
	fixture_notice "$status" GPL-3.0-or-later >"$pkg/share/licenses/ffmpeg/SOURCE.txt"
	echo "licence text (fixture)" >"$pkg/share/licenses/ffmpeg/LICENSE.md"
	set_modes "$dir"
}

# set_modes <dir>: directories 0755, scripts and binaries 0755, everything else 0644.
set_modes() {
	find "$1" -type d -exec chmod 0755 {} +
	find "$1" -type f -exec chmod 0644 {} +
	chmod 0755 "$1"/spk/scripts/* "$1"/pkg/bin/*
}

# pack_spk <dir> <out.spk> [tar options for the outer archive]: package.tgz with xz and the
# outer tar in ls order, as pkg_make_package and pkg_make_spk of the toolkit do.
pack_spk() {
	local dir=$1
	# shellcheck disable=SC2012 # ls order on purpose, as pkg_make_package packs
	(cd "$dir/pkg" && ls | tar cJf "$dir/spk/package.tgz" -T /dev/stdin)
	pack_outer "$@"
}

# pack_outer <dir> <out.spk> [tar options]: only the outer tar, package.tgz as it is.
pack_outer() {
	local dir=$1 out=$2
	shift 2
	# shellcheck disable=SC2046 # ls order on purpose, as the toolkit packs
	(cd "$dir/spk" && tar cf "$out" "$@" $(ls))
}

# base256_size <tar> [header offset]: rewrites the size field of the header at the offset
# (default 0, the first member) in GNU base-256 form - 0x80, three zero bytes, then the
# size as eight big-endian bytes - and fixes the header checksum. GNU tar still reads the
# member; the form normally appears only for members of 8 GiB and more.
base256_size() {
	local tar=$1 header=${2:-0} size
	size=$((8#$(dd if="$tar" bs=1 skip=$((header + 124)) count=11 2>/dev/null)))
	{ byte 128; be 3 0; be 8 "$size"; } | patch_bytes "$tar" $((header + 124))
	fix_header_checksum "$tar" "$header"
}

# fix_header_checksum <tar> <header offset>: recomputes the checksum of a patched header.
fix_header_checksum() {
	local tar=$1 header=$2 sum
	printf '        ' | patch_bytes "$tar" $((header + 148))
	sum=$(od -An -v -tu1 -j "$header" -N 512 "$tar" |
		awk '{ for (i = 1; i <= NF; i++) s += $i } END { print s }')
	printf '%06o\000 ' "$sum" | patch_bytes "$tar" $((header + 148))
}

# patch_bytes <file> <offset>: overwrites the file at the offset with stdin.
patch_bytes() {
	dd of="$1" bs=1 seek="$2" conv=notrunc 2>/dev/null
}

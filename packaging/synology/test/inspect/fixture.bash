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

# fake_elf <family> <static|dynamic> <none|major.minor.patch>: a minimal little-endian ELF
# executable of the family's class and machine, with a PT_INTERP segment when dynamic and a
# GNU ABI tag note when an ABI version is given. Written to stdout.
fake_elf() {
	local family=$1 linkage=$2 abi=$3 class machine ehsize phentsize phnum=0 note_size=0
	case "$family" in
		x86_64) class=2 machine=62 ;;
		armv8) class=2 machine=183 ;;
		armv7) class=1 machine=40 ;;
		*) echo "fake_elf: unknown family $family" >&2 && return 1 ;;
	esac
	if [ "$class" -eq 2 ]; then ehsize=64 phentsize=56; else ehsize=52 phentsize=32; fi
	[ "$abi" = none ] || { phnum=$((phnum + 1)); note_size=32; }
	[ "$linkage" = static ] || phnum=$((phnum + 1))
	local note_offset=$((ehsize + phnum * phentsize)) interp_offset=$((ehsize + phnum * phentsize + note_size))
	printf '\177ELF'
	le 1 "$class"; le 1 1; le 1 1; le 1 0; le 8 0
	le 2 2; le 2 "$machine"; le 4 1
	if [ "$class" -eq 2 ]; then
		le 8 0; le 8 "$ehsize"; le 8 0; le 4 0; le 2 "$ehsize"; le 2 "$phentsize"; le 2 "$phnum"; le 2 0; le 2 0; le 2 0
	else
		le 4 0; le 4 "$ehsize"; le 4 0; le 4 $((0x05000400)); le 2 "$ehsize"; le 2 "$phentsize"; le 2 "$phnum"; le 2 0; le 2 0; le 2 0
	fi
	[ "$abi" = none ] || fake_phdr "$class" 4 "$note_offset" "$note_size"
	[ "$linkage" = static ] || fake_phdr "$class" 3 "$interp_offset" $((${#FIXTURE_INTERPRETER} + 1))
	if [ "$abi" != none ]; then
		local major=${abi%%.*} rest=${abi#*.} minor patch
		minor=${rest%%.*}
		patch=${rest#*.}
		le 4 4; le 4 16; le 4 1; printf 'GNU\000'; le 4 0; le 4 "$major"; le 4 "$minor"; le 4 "$patch"
	fi
	[ "$linkage" = static ] || printf '%s\000' "$FIXTURE_INTERPRETER"
}

# fake_phdr <class> <type> <offset> <size>: one program header.
fake_phdr() {
	if [ "$1" -eq 2 ]; then
		le 4 "$2"; le 4 5; le 8 "$3"; le 8 0; le 8 0; le 8 "$4"; le 8 "$4"; le 8 4
	else
		le 4 "$2"; le 4 "$3"; le 4 0; le 4 0; le 4 "$4"; le 4 "$4"; le 4 5; le 4 4
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

# make_tree <dir> <family> <status> <static|dynamic> <abi>: the unpacked SPK below <dir>/spk
# with the payload below <dir>/pkg, every mode set explicitly.
make_tree() {
	local dir=$1 family=$2 status=$3 linkage=$4 abi=$5 script
	local spk="$dir/spk" pkg="$dir/pkg"
	rm -rf "$dir"
	mkdir -p "$spk/conf" "$spk/scripts" "$spk/WIZARD_UIFILES" "$pkg/bin" "$pkg/port_conf" "$pkg/share/licenses/ffmpeg"
	fixture_info "$family" >"$spk/INFO"
	echo "GNU GENERAL PUBLIC LICENSE (fixture)" >"$spk/LICENSE"
	fake_png 64 >"$spk/PACKAGE_ICON.PNG"
	fake_png 256 >"$spk/PACKAGE_ICON_256.PNG"
	echo '{"defaults": {"run-as": "package"}}' >"$spk/conf/privilege"
	printf '{"data-share": {"shares": [{"name": "{{wizard_media_share}}", "permission": {"ro": ["%s"]}}]}, "port-config": {"protocol-file": "port_conf/%s.sc"}}\n' \
		"$FIXTURE_ID" "$FIXTURE_ID" >"$spk/conf/resource"
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
	printf '[%s]\ntitle="Demo Server"\ndesc="Demo Server media server"\nport_forward="no"\ndst.ports="%s/tcp"\n' \
		"$FIXTURE_ID" "$FIXTURE_PORT" >"$pkg/port_conf/$FIXTURE_ID.sc"
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
	local dir=$1 out=$2
	shift 2
	# shellcheck disable=SC2012 # ls order on purpose, as pkg_make_package packs
	(cd "$dir/pkg" && ls | tar cJf "$dir/spk/package.tgz" -T /dev/stdin)
	# shellcheck disable=SC2046 # ls order on purpose, as the toolkit packs
	(cd "$dir/spk" && tar cf "$out" "$@" $(ls))
}

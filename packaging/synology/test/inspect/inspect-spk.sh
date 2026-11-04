#!/bin/sh
# inspect-spk.sh - checks one server SPK the toolkit produced against ADR 0026 and its Notes
# (task SPK-10): outer archive and members, INFO, package.tgz with the binaries, scripts,
# resources, icons, leftovers from the build environment and the FFmpeg status.
#
#   inspect-spk.sh [--release] [--port N] [--expected FILE] [--members] <file.spk> <family>
#
#   family       x86_64, armv8 or armv7: decides the expected INFO arch, the ELF class and
#                machine of the binaries and the oldest DSM 7.2 kernel (ADR 0026 Notes)
#   --release    release mode: an interim or non-free FFmpeg build, a dynamically linked
#                ffmpeg/ffprobe and an ABI note above the family's kernel fail instead of
#                warning (ADR 0026 Notes: no release may contain an interim build)
#   --port N     the port INFO adminport and the firewall service file must name
#                (default 8460, config.DefaultPort)
#   --expected   the expected-member list (default expected-members.txt next to this script)
#   --members    also print one line per file of both archives: SHA-256, mode, archive:name
#                (for comparing two builds; the toolkit's packing is not byte-reproducible)
#
# Prints one line per check: "ok", "WARN" or "FAIL" followed by the check, "info" lines for
# facts that are recorded but not judged, and a summary. Exit 0 when nothing failed, 1 when a
# check failed, 2 on a usage error or a missing tool.
#
# Runs in a debian:stable-slim based container (Dockerfile in this directory: tar, xz, jq;
# how to run: ../../toolkit/README.md "Output verification"); POSIX sh, no bash. Needs tar
# (GNU), xz, od, dd, awk, sed, grep, sort, mktemp, sha256sum and jq. Nothing here names the
# product: the package name comes from INFO.
set -u

readonly EXIT_FAILURE=1
readonly EXIT_USAGE=2
readonly DEFAULT_PORT=8460
readonly OS_MIN_VER="7.2-64570"
# Largest number of an INFO version part (the guide: each part at most 2^31-1).
readonly MAX_VERSION_NUMBER=2147483647
# Build numbers below this encode a pre-release (OD-SPK-5: stage 1-3 alpha/beta/rc, 0 dev).
readonly FINAL_BUILD_NUMBER=9000
readonly PACKAGE_NAME_PATTERN='^[A-Za-z0-9][A-Za-z0-9._-]*$'
# ELF constants: program header types and e_machine values.
readonly PT_INTERP=3
readonly PT_NOTE=4
readonly NT_GNU_ABI_TAG=1
# Bytes of a tar header block and the offsets of its fields (POSIX ustar layout).
readonly TAR_BLOCK=512
readonly TAR_SIZE_OFFSET=124
readonly TAR_TYPEFLAG_OFFSET=156
readonly TAR_MAGIC_OFFSET=257
# Members of a text kind are checked for CR characters and leftover @NAME@ tokens.
readonly TOKEN_PATTERN='@[A-Z][A-Z0-9_]*@'
readonly LIFECYCLE_SCRIPTS="preinst postinst preuninst postuninst preupgrade postupgrade start-stop-status"
readonly ICON_SIZES="PACKAGE_ICON.PNG:64 PACKAGE_ICON_256.PNG:256"

script_dir=$(cd "$(dirname "$0")" && pwd)
readonly script_dir

release=no
port=$DEFAULT_PORT
expected_file="$script_dir/expected-members.txt"
print_members=no
spk=""
family=""
work=""
id=""
failures=0
warnings=0
checks=0
# The FFmpeg status of SOURCE.txt decides whether FFmpeg findings warn or fail (see
# ffmpeg_verdict); "" until SOURCE.txt was read.
ffmpeg_status=""
ffmpeg_licence=""

usage() {
	sed -n '2,/^set -u$/{/^set /d;s/^# \{0,1\}//;p}' "$0" >&2
	exit "$EXIT_USAGE"
}

die() {
	printf 'inspect-spk: %s\n' "$*" >&2
	exit "$EXIT_USAGE"
}

pass() {
	checks=$((checks + 1))
	printf 'ok   %s\n' "$*"
}

failed() {
	checks=$((checks + 1))
	failures=$((failures + 1))
	printf 'FAIL %s\n' "$*"
}

warn() {
	checks=$((checks + 1))
	warnings=$((warnings + 1))
	printf 'WARN %s\n' "$*"
}

info() { printf 'info %s\n' "$*"; }

# check <description> <command...>: ok when the command succeeds, FAIL otherwise.
check() {
	check_description=$1
	shift
	if "$@"; then pass "$check_description"; else failed "$check_description"; fi
}

# ffmpeg_verdict <description>: an FFmpeg finding warns while the packed build is interim and
# the run is not a release inspection; otherwise it fails (ADR 0026 Notes).
ffmpeg_verdict() {
	if [ "$release" = no ] && [ "$ffmpeg_status" = interim ]; then
		warn "$* (interim FFmpeg build, test build only)"
	else
		failed "$*"
	fi
}

cleanup() {
	[ -z "$work" ] || rm -rf "$work"
}

parse_args() {
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--release) release=yes ;;
			--members) print_members=yes ;;
			--port)
				[ "$#" -ge 2 ] || die "--port needs a value"
				port=$2
				shift
				;;
			--expected)
				[ "$#" -ge 2 ] || die "--expected needs a value"
				expected_file=$2
				shift
				;;
			-h | --help) usage ;;
			-*) die "unknown option $1" ;;
			*)
				if [ -z "$spk" ]; then spk=$1; elif [ -z "$family" ]; then family=$1; else die "too many arguments: $1"; fi
				;;
		esac
		shift
	done
	[ -n "$spk" ] && [ -n "$family" ] || usage
	[ -f "$spk" ] || die "$spk is not a file"
	[ -f "$expected_file" ] || die "expected-member list $expected_file not found"
	echo "$port" | grep -Eq '^[1-9][0-9]{0,4}$' && [ "$port" -le 65535 ] || die "--port must be 1-65535, not '$port'"
	case "$family" in x86_64 | armv8 | armv7) ;; *) die "family must be x86_64, armv8 or armv7, not '$family'" ;; esac
}

require_tools() {
	for tool in tar xz od dd awk sed grep sort mktemp sha256sum jq; do
		command -v "$tool" >/dev/null 2>&1 || die "$tool is required (run in the image of the Dockerfile next to this script)"
	done
}

# Facts of the family (ADR 0026 Notes): INFO arch, ELF class and machine, oldest DSM 7.2 kernel.
family_arch() {
	case "$1" in
		x86_64) echo "x86_64" ;;
		armv8) echo "armv8" ;;
		armv7) echo "armv7 armada38x monaco" ;;
	esac
}

family_elf() {
	case "$1" in
		x86_64) echo "2 62" ;;  # ELFCLASS64, EM_X86_64
		armv8) echo "2 183" ;;  # ELFCLASS64, EM_AARCH64
		armv7) echo "1 40" ;;   # ELFCLASS32, EM_ARM
	esac
}

family_kernel() {
	case "$1" in
		x86_64 | armv7) echo "3.10.108" ;;
		armv8) echo "4.4.302" ;;
	esac
}

# --- binary helpers ---------------------------------------------------------------------

# read_u <file> <offset> <bytes>: unsigned little-endian integer of up to 8 bytes.
read_u() {
	od -An -v -tu1 -j "$2" -N "$3" "$1" | awk 'BEGIN { p = 1 } { for (i = 1; i <= NF; i++) { v += $i * p; p *= 256 } } END { printf "%d", v }'
}

# read_hex <file> <offset> <bytes>: the bytes as lowercase hex without blanks.
read_hex() {
	od -An -v -tx1 -j "$2" -N "$3" "$1" | tr -d ' \n'
}

# read_text <file> <offset> <bytes>: the bytes as text up to the first NUL.
read_text() {
	dd if="$1" bs=1 skip="$2" count="$3" 2>/dev/null | tr -d '\000' | head -c "$3"
}

# octal <digits>: decimal value of an octal digit string (tar size fields).
octal() {
	echo "$1" | awk '{ for (i = 1; i <= length($0); i++) v = v * 8 + substr($0, i, 1); printf "%d", v }'
}

# --- tar format ---------------------------------------------------------------------------

# tar_format <file>: prints "<flavour> <extensions>" for an uncompressed tar: flavour gnu
# (magic "ustar  " NUL), posix (magic "ustar" NUL "00", ustar or pax) or unknown; extensions
# are the header types that go beyond plain ustar (gnu-longname, gnu-longlink, pax-extended,
# pax-global, base-256-size) or "none". Walks the header blocks, so every member is seen.
tar_format() {
	tar_file=$1
	tar_total=$(wc -c <"$tar_file")
	tar_offset=0
	tar_flavour=""
	tar_extensions=""
	while [ "$((tar_offset + TAR_BLOCK))" -le "$tar_total" ]; do
		tar_magic=$(read_hex "$tar_file" "$((tar_offset + TAR_MAGIC_OFFSET))" 8)
		case "$tar_magic" in
			7573746172202000) tar_this=gnu ;;
			7573746172003030) tar_this=posix ;;
			0000000000000000) break ;; # end-of-archive block
			*) tar_this=unknown ;;
		esac
		[ -n "$tar_flavour" ] && [ "$tar_flavour" != "$tar_this" ] && tar_flavour=mixed || tar_flavour=${tar_flavour:-$tar_this}
		tar_type=$(read_text "$tar_file" "$((tar_offset + TAR_TYPEFLAG_OFFSET))" 1)
		case "$tar_type" in
			L) tar_extensions="$tar_extensions gnu-longname" ;;
			K) tar_extensions="$tar_extensions gnu-longlink" ;;
			x) tar_extensions="$tar_extensions pax-extended" ;;
			g) tar_extensions="$tar_extensions pax-global" ;;
		esac
		tar_size_first=$(read_u "$tar_file" "$((tar_offset + TAR_SIZE_OFFSET))" 1)
		if [ "$tar_size_first" -ge 128 ]; then
			tar_extensions="$tar_extensions base-256-size"
			tar_size=$(read_u "$tar_file" "$((tar_offset + TAR_SIZE_OFFSET + 4))" 8)
		else
			tar_size=$(octal "$(read_text "$tar_file" "$((tar_offset + TAR_SIZE_OFFSET))" 11 | tr -d ' ')")
		fi
		tar_offset=$((tar_offset + TAR_BLOCK + (tar_size + TAR_BLOCK - 1) / TAR_BLOCK * TAR_BLOCK))
	done
	tar_extensions=$(echo "$tar_extensions" | tr ' ' '\n' | grep -v '^$' | sort -u | tr '\n' ',' | sed 's/,$//')
	echo "${tar_flavour:-empty} ${tar_extensions:-none}"
}

# --- member lists -------------------------------------------------------------------------

# list_members <tar> [xz]: "<mode> <owner> <name>" per member, "./" prefixes stripped.
list_members() {
	if [ "${2:-}" = xz ]; then tar -tvJf "$1"; else tar -tvf "$1"; fi |
		awk '{ name = $0; for (i = 1; i <= 5; i++) sub(/^[^ ]+ +/, "", name); sub(/^\.\//, "", name); sub(/ -> .*$/, "", name); print $1, $2, name }'
}

# mode_matches <mode-column> <expected 0755|0644>
mode_matches() {
	case "$1:$2" in
		-rwxr-xr-x:0755 | -rw-r--r--:0644 | drwxr-xr-x:0755) return 0 ;;
		*) return 1 ;;
	esac
}

# expected_rule <archive> <name>: prints "<mode> <rule>" of the first matching rule of the
# expected-member list, exact names first, then patterns; nothing when there is none.
expected_rule() {
	rule_archive=$1
	rule_name=$2
	rule_found=""
	while read -r rule_kind rule_mode rule_member; do
		[ "$rule_kind" = "$rule_archive" ] || continue
		rule_member=$(echo "$rule_member" | sed "s|<id>|$id|g")
		if [ "$rule_member" = "$rule_name" ]; then
			echo "$rule_mode $rule_member"
			return 0
		fi
		case "$rule_member" in
			*\**)
				# shellcheck disable=SC2254 # the rule is a pattern on purpose
				case "$rule_name" in $rule_member) [ -n "$rule_found" ] || rule_found="$rule_mode $rule_member" ;; esac
				;;
		esac
	done <"$expected_rules"
	[ -z "$rule_found" ] || echo "$rule_found"
}

# suspicious_name <name>: true for names that come from the build environment or escape the
# package directory.
suspicious_name() {
	case "$1" in
		/* | ../* | */../* | *..) return 0 ;;
		*SynoBuildConf* | build.env | *INFO.sh | prebuilt/* | .spkstage* | .fetch-manifest | */.fetch-manifest) return 0 ;;
		*_debug* | *.debug | *.dbg | *.orig | *.rej | *~) return 0 ;;
		.* | */.*) return 0 ;;
	esac
	return 1
}

# check_members <archive> <listing>: every member against the expected list (name, mode,
# type), every required rule present exactly once, nothing suspicious, no signature.
check_members() {
	members_archive=$1
	members_listing=$2
	members_problems=""
	members_seen="$work/seen-$members_archive"
	: >"$members_seen"
	while read -r member_mode _ member_name; do
		[ -n "$member_name" ] || continue
		case "$member_mode" in
			d* | -*) ;;
			*) members_problems="$members_problems ${member_name}(not a file or directory: $member_mode)"; continue ;;
		esac
		if suspicious_name "$member_name"; then
			members_problems="$members_problems ${member_name}(build leftover or unsafe name)"
			continue
		fi
		member_rule=$(expected_rule "$members_archive" "$member_name")
		if [ -z "$member_rule" ]; then
			members_problems="$members_problems ${member_name}(unexpected)"
			continue
		fi
		member_expected_mode=${member_rule%% *}
		case "$member_name" in */) member_expected_mode=0755 ;; esac
		mode_matches "$member_mode" "$member_expected_mode" ||
			members_problems="$members_problems ${member_name}(mode $member_mode, want $member_expected_mode)"
		echo "${member_rule#* }" >>"$members_seen"
	done <<EOF
$members_listing
EOF
	members_missing=$(while read -r rule_kind rule_mode rule_member; do
		[ "$rule_kind" = "$members_archive" ] || continue
		case "$rule_member" in *\**) continue ;; esac
		rule_member=$(echo "$rule_member" | sed "s|<id>|$id|g")
		rule_count=$(grep -cxF -- "$rule_member" "$members_seen")
		[ "$rule_count" -eq 1 ] || echo "${rule_member}(expected once, found $rule_count)"
	done <"$expected_rules" | tr '\n' ' ')
	if [ -z "$members_problems" ] && [ -z "$members_missing" ]; then
		pass "$members_archive: members, modes and types match the expected list"
	else
		failed "$members_archive: members differ from the expected list:$members_problems ${members_missing% }"
	fi
	if echo "$members_listing" | grep -qi 'signature'; then
		failed "$members_archive: a signature member is present (signing is deprecated since DSM 7)"
	else
		pass "$members_archive: no signature member"
	fi
	members_owners=$(echo "$members_listing" | awk '{ print $2 }' | sort -u | tr '\n' ' ')
	info "$members_archive: owners ${members_owners% }"
}

# --- the outer archive ----------------------------------------------------------------------

check_outer_archive() {
	outer_magic=$(read_hex "$spk" 0 6)
	case "$outer_magic" in
		1f8b*) failed "SPK is gzip-compressed, not an uncompressed tar"; exit "$EXIT_FAILURE" ;;
		fd377a585a00) failed "SPK is xz-compressed, not an uncompressed tar"; exit "$EXIT_FAILURE" ;;
	esac
	outer_format=$(tar_format "$spk")
	case "$outer_format" in
		gnu\ * | posix\ *) pass "SPK is an uncompressed tar (format ${outer_format%% *})" ;;
		*) failed "SPK is not a tar archive with ustar or GNU headers (${outer_format%% *})"; exit "$EXIT_FAILURE" ;;
	esac
	info "SPK tar extensions beyond plain ustar: ${outer_format#* }"
	outer_listing=$(list_members "$spk")
	info "SPK member order: $(echo "$outer_listing" | awk '{ print $3 }' | tr '\n' ' ')"
	mkdir -p "$work/spk"
	tar -xf "$spk" -C "$work/spk" || die "cannot extract $spk"
	[ -f "$work/spk/INFO" ] || { failed "INFO is missing"; exit "$EXIT_FAILURE"; }
	read_package_name
	check_members spk "$outer_listing"
}

# --- INFO -------------------------------------------------------------------------------

info_value() {
	sed -n "s/^$1=\"\(.*\)\"\$/\1/p" "$work/spk/INFO"
}

info_count() {
	grep -c "^$1=" "$work/spk/INFO"
}

# info_required <key> [pattern]: the key is present once; its value matches the extended
# regular expression when one is given.
info_required() {
	info_key=$1
	info_pattern=${2:-.}
	info_present=$(info_count "$info_key")
	if [ "$info_present" -ne 1 ]; then
		failed "INFO $info_key: present $info_present times, want once"
		return
	fi
	info_actual=$(info_value "$info_key")
	if echo "$info_actual" | grep -Eq "$info_pattern"; then
		pass "INFO $info_key=\"$info_actual\""
	else
		failed "INFO $info_key=\"$info_actual\" does not match $info_pattern"
	fi
}

info_absent() {
	if [ "$(info_count "$1")" -eq 0 ]; then pass "INFO has no $1"; else failed "INFO has $1 ($(grep "^$1=" "$work/spk/INFO" | head -n 1))"; fi
}

read_package_name() {
	id=$(info_value package | head -n 1)
	echo "$id" | grep -Eq "$PACKAGE_NAME_PATTERN" || { failed "INFO package=\"$id\" is not a package name"; exit "$EXIT_FAILURE"; }
}

# version_parts_in_range <a.b.c-d>: every number at most 2^31-1.
version_parts_in_range() {
	for version_part in $(echo "$1" | tr '.-' '  '); do
		[ "${#version_part}" -le 10 ] && [ "$version_part" -le "$MAX_VERSION_NUMBER" ] || return 1
	done
}

check_info() {
	info_bad_lines=$(grep -Env '^[a-z_]+="[^"\\$`]*"$' "$work/spk/INFO" | head -n 3 | tr '\n' ' ')
	if [ -z "$info_bad_lines" ]; then pass "INFO: every line is key=\"value\" without shell-active characters"; else failed "INFO: lines that are not key=\"value\": $info_bad_lines"; fi
	info_required package "$PACKAGE_NAME_PATTERN"
	info_required displayname '^.+$'
	info_required version '^[0-9]+\.[0-9]+\.[0-9]+-[0-9]+$'
	info_version=$(info_value version)
	if version_parts_in_range "$info_version"; then pass "INFO version parts are at most $MAX_VERSION_NUMBER"; else failed "INFO version $info_version has a part above $MAX_VERSION_NUMBER"; fi
	info_build=${info_version##*-}
	if [ "${info_build:-9999}" -lt "$FINAL_BUILD_NUMBER" ] 2>/dev/null; then
		info_required beta '^yes$'
	else
		info_absent beta
	fi
	info_required arch "^$(family_arch "$family")\$"
	info_required os_min_ver "^$OS_MIN_VER\$"
	info_required adminport "^$port\$"
	info_required adminprotocol '^http$'
	info_absent adminurl
	info_required description '^.+$'
	info_required description_ger '^.+$'
	info_required maintainer '^.+$'
	info_required support_move '^no$'
	info_required extractsize '^[1-9][0-9]*$'
	info_required toolkit_version '^[0-9]+$'
	info_required create_time '^[0-9]{8}-[0-9]{2}:[0-9]{2}:[0-9]{2}$'
	for info_key in thirdparty startable changelog; do
		info_absent "$info_key"
	done
}

# --- package.tgz ------------------------------------------------------------------------

check_package_tgz() {
	package_tgz="$work/spk/package.tgz"
	package_magic=$(read_hex "$package_tgz" 0 6)
	if [ "$package_magic" = fd377a585a00 ]; then pass "package.tgz is xz-compressed"; else failed "package.tgz is not xz-compressed (magic $package_magic)"; exit "$EXIT_FAILURE"; fi
	xz -dc "$package_tgz" >"$work/package.tar" || { failed "package.tgz does not decompress"; exit "$EXIT_FAILURE"; }
	package_format=$(tar_format "$work/package.tar")
	case "$package_format" in
		gnu\ * | posix\ *) pass "package.tgz holds a tar (format ${package_format%% *})" ;;
		*) failed "package.tgz does not hold a tar with ustar or GNU headers (${package_format%% *})"; exit "$EXIT_FAILURE" ;;
	esac
	info "package.tgz tar extensions beyond plain ustar: ${package_format#* }"
	package_listing=$(list_members "$work/package.tar")
	check_members package "$package_listing"
	mkdir -p "$work/pkg"
	tar -xf "$work/package.tar" -C "$work/pkg" || die "cannot extract package.tgz"
	package_kib=$(du -sk "$work/pkg" | cut -f1)
	info "package.tgz extracts to $package_kib KiB (INFO extractsize=\"$(info_value extractsize)\")"
}

# --- ELF binaries ---------------------------------------------------------------------------

# elf_facts <file>: prints "class machine type interp abi flags" (interp yes/no; abi is the
# GNU ABI tag as major.minor.patch or "none"); nothing when the file is not a little-endian ELF.
elf_facts() {
	elf=$1
	[ "$(read_hex "$elf" 0 4)" = 7f454c46 ] || return 0
	elf_class=$(read_u "$elf" 4 1)
	[ "$(read_u "$elf" 5 1)" -eq 1 ] || return 0
	elf_type=$(read_u "$elf" 16 2)
	elf_machine=$(read_u "$elf" 18 2)
	if [ "$elf_class" -eq 2 ]; then
		elf_phoff=$(read_u "$elf" 32 8); elf_phentsize=$(read_u "$elf" 54 2); elf_phnum=$(read_u "$elf" 56 2); elf_flags=$(read_u "$elf" 48 4)
	else
		elf_phoff=$(read_u "$elf" 28 4); elf_phentsize=$(read_u "$elf" 42 2); elf_phnum=$(read_u "$elf" 44 2); elf_flags=$(read_u "$elf" 36 4)
	fi
	elf_flags=$(printf '0x%08x' "$elf_flags")
	elf_interp=no
	elf_abi=none
	elf_index=0
	while [ "$elf_index" -lt "$elf_phnum" ]; do
		elf_ph=$((elf_phoff + elf_index * elf_phentsize))
		elf_ptype=$(read_u "$elf" "$elf_ph" 4)
		if [ "$elf_ptype" -eq "$PT_INTERP" ]; then elf_interp=yes; fi
		if [ "$elf_ptype" -eq "$PT_NOTE" ]; then
			if [ "$elf_class" -eq 2 ]; then
				elf_note_offset=$(read_u "$elf" "$((elf_ph + 8))" 8); elf_note_size=$(read_u "$elf" "$((elf_ph + 32))" 8)
			else
				elf_note_offset=$(read_u "$elf" "$((elf_ph + 4))" 4); elf_note_size=$(read_u "$elf" "$((elf_ph + 16))" 4)
			fi
			elf_tag=$(elf_abi_tag "$elf" "$elf_note_offset" "$elf_note_size")
			[ -z "$elf_tag" ] || elf_abi=$elf_tag
		fi
		elf_index=$((elf_index + 1))
	done
	echo "$elf_class $elf_machine $elf_type $elf_interp $elf_abi $elf_flags"
}

# elf_abi_tag <file> <offset> <size>: the GNU ABI tag of a note segment (NT_GNU_ABI_TAG,
# name "GNU"), as major.minor.patch; nothing when the segment has none.
elf_abi_tag() {
	note_pos=$2
	note_end=$(($2 + $3))
	while [ "$((note_pos + 12))" -le "$note_end" ]; do
		note_namesz=$(read_u "$1" "$note_pos" 4)
		note_descsz=$(read_u "$1" "$((note_pos + 4))" 4)
		note_type=$(read_u "$1" "$((note_pos + 8))" 4)
		[ "$note_namesz" -le 64 ] && [ "$note_descsz" -le 4096 ] || return 0
		note_name=$(read_text "$1" "$((note_pos + 12))" "$note_namesz")
		note_desc=$((note_pos + 12 + (note_namesz + 3) / 4 * 4))
		if [ "$note_name" = GNU ] && [ "$note_type" -eq "$NT_GNU_ABI_TAG" ] && [ "$note_descsz" -ge 16 ]; then
			echo "$(read_u "$1" "$((note_desc + 4))" 4).$(read_u "$1" "$((note_desc + 8))" 4).$(read_u "$1" "$((note_desc + 12))" 4)"
			return 0
		fi
		note_pos=$((note_desc + (note_descsz + 3) / 4 * 4))
	done
}

# version_at_most <a.b.c> <x.y.z>: true when the first kernel version is not newer.
version_at_most() {
	awk -v a="$1" -v b="$2" 'BEGIN { split(a, x, "."); split(b, y, "."); for (i = 1; i <= 3; i++) { if (x[i] + 0 < y[i] + 0) exit 0; if (x[i] + 0 > y[i] + 0) exit 1 } exit 0 }'
}

# check_binary <name> <kind>: kind "server" (always strict) or "ffmpeg" (ffmpeg_verdict).
check_binary() {
	binary="$work/pkg/bin/$1"
	binary_kind=$2
	binary_facts=$(elf_facts "$binary")
	if [ -z "$binary_facts" ]; then
		failed "bin/$1 is not a little-endian ELF file"
		return
	fi
	# shellcheck disable=SC2086 # the facts are six blank-separated words on purpose
	set -- "$1" "$binary_kind" $binary_facts
	# $3 class, $4 machine, $5 type, $6 interp, $7 abi, $8 flags
	binary_expected=$(family_elf "$family")
	if [ "$3 $4" = "$binary_expected" ]; then pass "bin/$1: ELF class $3, machine $4 (family $family)"; else failed "bin/$1: ELF class $3, machine $4, want $binary_expected for $family"; fi
	case "$5" in 2 | 3) pass "bin/$1: ELF type $5 (executable)" ;; *) failed "bin/$1: ELF type $5 is not an executable (2 or 3)" ;; esac
	if [ "$6" = no ]; then
		pass "bin/$1: statically linked (no program interpreter)"
	elif [ "$2" = ffmpeg ]; then
		ffmpeg_verdict "bin/$1: dynamically linked (has a program interpreter)"
	else
		failed "bin/$1: dynamically linked (has a program interpreter)"
	fi
	binary_kernel=$(family_kernel "$family")
	if [ "$7" = none ]; then
		info "bin/$1: no GNU ABI note (runs on any kernel the libc supports)"
	elif version_at_most "$7" "$binary_kernel"; then
		pass "bin/$1: ELF ABI note $7 is at most the oldest DSM 7.2 kernel $binary_kernel"
	elif [ "$2" = ffmpeg ]; then
		ffmpeg_verdict "bin/$1: ELF ABI note $7 is above the oldest DSM 7.2 kernel $binary_kernel"
	else
		failed "bin/$1: ELF ABI note $7 is above the oldest DSM 7.2 kernel $binary_kernel"
	fi
	[ "$family" != armv7 ] || info "bin/$1: ELF e_flags $8 (EABI version in the top byte; 0x400 = EF_ARM_ABI_FLOAT_HARD)"
}

# --- FFmpeg notice -----------------------------------------------------------------------

read_ffmpeg_notice() {
	notice="$work/pkg/share/licenses/ffmpeg/SOURCE.txt"
	if [ ! -f "$notice" ]; then
		failed "share/licenses/ffmpeg/SOURCE.txt is missing"
		ffmpeg_status=missing
		return
	fi
	notice_status_lines=$(grep -c '^Status:' "$notice")
	notice_licence_lines=$(grep -c '^Licence:' "$notice")
	if [ "$notice_status_lines" -eq 1 ] && [ "$notice_licence_lines" -eq 1 ]; then
		pass "SOURCE.txt has one Status: and one Licence: line"
	else
		failed "SOURCE.txt has $notice_status_lines Status: and $notice_licence_lines Licence: lines, want one each"
	fi
	ffmpeg_status=$(sed -n 's/^Status:[[:space:]]*\([^[:space:]]*\).*/\1/p' "$notice" | head -n 1)
	ffmpeg_licence=$(sed -n 's/^Licence:[[:space:]]*\([^[:space:]]*\).*/\1/p' "$notice" | head -n 1)
	info "FFmpeg build: version $(sed -n 's/^Version:[[:space:]]*//p' "$notice" | head -n 1), status ${ffmpeg_status:-none}, licence ${ffmpeg_licence:-none}"
}

check_ffmpeg_notice() {
	[ "$ffmpeg_status" != missing ] || return 0
	case "$ffmpeg_status" in
		selected) pass "SOURCE.txt status is selected" ;;
		interim) ffmpeg_verdict "SOURCE.txt status is interim" ;;
		*) failed "SOURCE.txt status is '$ffmpeg_status', want selected" ;;
	esac
	case "$ffmpeg_licence" in
		*nonfree*) ffmpeg_verdict "SOURCE.txt licence $ffmpeg_licence is non-free (not redistributable)" ;;
		GPL-*) pass "SOURCE.txt licence $ffmpeg_licence" ;;
		*) failed "SOURCE.txt licence '$ffmpeg_licence' is not a GPL identifier" ;;
	esac
	licence_files=$(find "$work/pkg/share/licenses/ffmpeg" -type f ! -name SOURCE.txt | wc -l)
	if [ "$licence_files" -ge 1 ]; then pass "share/licenses/ffmpeg has $licence_files licence file(s) besides SOURCE.txt"; else failed "share/licenses/ffmpeg has no licence file besides SOURCE.txt"; fi
}

# --- firewall service file --------------------------------------------------------------

check_service_file() {
	service="$work/pkg/port_conf/$id.sc"
	[ -f "$service" ] || { failed "port_conf/$id.sc is missing"; return; }
	check "port_conf/$id.sc: section [$id]" grep -qx "\[$id\]" "$service"
	check "port_conf/$id.sc: title" grep -Eq '^title="[^"]+"$' "$service"
	check "port_conf/$id.sc: desc" grep -Eq '^desc="[^"]+"$' "$service"
	check "port_conf/$id.sc: port_forward=\"no\"" grep -qx 'port_forward="no"' "$service"
	check "port_conf/$id.sc: dst.ports=\"$port/tcp\"" grep -qx "dst.ports=\"$port/tcp\"" "$service"
	resource_file=$(jq -r '."port-config"."protocol-file" // ""' "$work/spk/conf/resource" 2>/dev/null)
	if [ "$resource_file" = "port_conf/$id.sc" ]; then pass "conf/resource port-config names port_conf/$id.sc"; else failed "conf/resource port-config names '$resource_file', want port_conf/$id.sc"; fi
}

# --- scripts, resources, wizard, icons -----------------------------------------------------

check_scripts() {
	cr=$(printf '\r')
	scripts_with_cr=$(grep -l "$cr" "$work/spk/scripts"/* 2>/dev/null | sed "s|$work/spk/||" | tr '\n' ' ')
	if [ -z "$scripts_with_cr" ]; then pass "scripts/*: LF line endings"; else failed "scripts with CR characters: $scripts_with_cr"; fi
	for script in $LIFECYCLE_SCRIPTS; do
		[ -f "$work/spk/scripts/$script" ] || continue
		if head -n 1 "$work/spk/scripts/$script" | grep -q '^#!/bin/sh'; then pass "scripts/$script starts with #!/bin/sh"; else failed "scripts/$script does not start with #!/bin/sh"; fi
	done
}

json_valid() {
	jq -e . "$1" >/dev/null 2>&1
}

check_json_files() {
	for json in conf/privilege conf/resource "$work/spk/WIZARD_UIFILES"/*; do
		case "$json" in /*) json=${json#"$work/spk/"} ;; esac
		[ -f "$work/spk/$json" ] || { failed "$json is missing"; continue; }
		check "$json parses as JSON" json_valid "$work/spk/$json"
	done
	privilege="$work/spk/conf/privilege"
	[ -f "$privilege" ] || return 0
	if [ "$(jq -r '.defaults."run-as" // ""' "$privilege" 2>/dev/null)" = package ]; then pass "conf/privilege run-as is package"; else failed "conf/privilege run-as is not package"; fi
	if jq -e '[.. | strings | select(. == "root")] | length == 0' "$privilege" >/dev/null 2>&1; then pass "conf/privilege mentions no root"; else failed "conf/privilege mentions root"; fi
}

# png_size <file>: "<width>x<height>" of a PNG, nothing for another file type.
png_size() {
	[ "$(read_hex "$1" 0 8)" = 89504e470d0a1a0a ] || return 0
	od -An -v -tu1 -j 16 -N 8 "$1" | awk '{ printf "%dx%d", (($1 * 256 + $2) * 256 + $3) * 256 + $4, (($5 * 256 + $6) * 256 + $7) * 256 + $8 }'
}

check_icons() {
	for icon_size in $ICON_SIZES; do
		icon=${icon_size%%:*}
		icon_side=${icon_size#*:}
		[ -f "$work/spk/$icon" ] || { failed "$icon is missing"; continue; }
		icon_actual=$(png_size "$work/spk/$icon")
		if [ "$icon_actual" = "${icon_side}x${icon_side}" ]; then pass "$icon is a ${icon_side}x${icon_side} PNG"; else failed "$icon is not a ${icon_side}x${icon_side} PNG (${icon_actual:-not a PNG})"; fi
	done
}

# text_members <find actions...>: runs find over every text file of both archives (everything
# but the binaries and icons) with the given actions.
text_members() {
	find "$work/spk" "$work/pkg" -type f ! -name '*.PNG' ! -name package.tgz ! -path "$work/pkg/bin/*" "$@"
}

check_tokens() {
	tokens=$(text_members -exec grep -El "$TOKEN_PATTERN" {} + 2>/dev/null | sed "s|$work/spk/||;s|$work/pkg/|package.tgz:|" | LC_ALL=C sort | tr '\n' ' ')
	if [ -z "$tokens" ]; then pass "no unreplaced @NAME@ token in any text member"; else failed "unreplaced @NAME@ tokens in: $tokens"; fi
	cr=$(printf '\r')
	text_with_cr=$(find "$work/spk/conf" "$work/spk/WIZARD_UIFILES" "$work/pkg/port_conf" "$work/spk/INFO" -type f -exec grep -l "$cr" {} + 2>/dev/null | sed "s|$work/spk/||;s|$work/pkg/|package.tgz:|" | LC_ALL=C sort | tr '\n' ' ')
	if [ -z "$text_with_cr" ]; then pass "INFO, conf/*, WIZARD_UIFILES/* and port_conf/*: LF line endings"; else failed "CR characters in: $text_with_cr"; fi
}

# --- member digests --------------------------------------------------------------------------

print_member_digests() {
	(cd "$work/spk" && find . -type f ! -name package.tgz | sed 's|^\./||' | LC_ALL=C sort | while read -r digest_name; do
		printf '%s %s spk:%s\n' "$(sha256sum "$digest_name" | cut -d' ' -f1)" "$(stat -c %a "$digest_name")" "$digest_name"
	done)
	(cd "$work/pkg" && find . -type f | sed 's|^\./||' | LC_ALL=C sort | while read -r digest_name; do
		printf '%s %s package.tgz:%s\n' "$(sha256sum "$digest_name" | cut -d' ' -f1)" "$(stat -c %a "$digest_name")" "$digest_name"
	done)
}

main() {
	parse_args "$@"
	require_tools
	work=$(mktemp -d) || die "cannot create a work directory"
	trap cleanup EXIT INT TERM
	expected_rules="$work/rules"
	grep -Ev '^[[:space:]]*(#|$)' "$expected_file" >"$expected_rules"
	info "inspecting $spk for family $family ($([ "$release" = yes ] && echo release || echo test) mode, port $port)"
	check_outer_archive
	check_info
	check_package_tgz
	read_ffmpeg_notice
	check_binary "$id" server
	check_binary ffmpeg ffmpeg
	check_binary ffprobe ffmpeg
	check_ffmpeg_notice
	check_service_file
	check_scripts
	check_json_files
	check_icons
	check_tokens
	[ "$print_members" = no ] || print_member_digests
	printf 'inspect-spk: %s checks, %s failed, %s warnings (%s)\n' "$checks" "$failures" "$warnings" "$spk"
	[ "$failures" -eq 0 ] || exit "$EXIT_FAILURE"
}

main "$@"

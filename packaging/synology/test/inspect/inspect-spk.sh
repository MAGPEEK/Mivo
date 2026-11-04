#!/bin/sh
# inspect-spk.sh - checks one server SPK the toolkit produced against ADR 0026 and its Notes
# (task SPK-10): outer archive and members, INFO, package.tgz with the binaries, scripts,
# resources, icons, the DSM main menu entry (INFO dsmuidir, ui/config, ui/images), leftovers
# from the build environment and the FFmpeg status.
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
# check failed (also when an archive cannot be read or holds members that are unsafe to
# extract), 2 on a usage error or a missing tool. An archive with an unsafe member, or one
# tar cannot list, is not extracted at all; when tar lists an archive but fails while
# extracting it, part of it may already lie in the temporary work directory, which is removed
# on every exit. The outer archive is extracted before package.tgz inside it is judged.
#
# Runs in a debian:stable-slim based container (Dockerfile in this directory: tar, xz, jq;
# how to run: ../../toolkit/README.md "Output verification"); POSIX sh, no bash. Needs GNU
# tar, xz, jq, od, dd, awk, sed, grep, sort, uniq, find, du, cut, wc, tr, head, mkdir, rm,
# mktemp, sha256sum and dirname. Nothing here names the product: the package name comes from
# INFO.
#
# No "set -e": most commands here are checks whose non-zero exit is a finding, not an error
# (grep without a match, a missing member), so every exit status is handled where it occurs.
set -u

readonly EXIT_FAILURE=1
readonly EXIT_USAGE=2
# Shell convention: 128 + signal number (SIGINT 2, SIGTERM 15).
readonly EXIT_INTERRUPTED=130
readonly EXIT_TERMINATED=143
readonly DEFAULT_PORT=8460
readonly MAX_PORT=65535
readonly OS_MIN_VER="7.2-64570"
# Largest number of an INFO version part (the guide: each part at most 2^31-1).
readonly MAX_VERSION_NUMBER=2147483647
readonly MAX_VERSION_DIGITS=10
# Build numbers below this encode a pre-release (OD-SPK-5: stage 1-3 alpha/beta/rc, 0 dev).
readonly FINAL_BUILD_NUMBER=9000
readonly PACKAGE_NAME_PATTERN='^[A-Za-z0-9][A-Za-z0-9._-]*$'
readonly REQUIRED_TOOLS="tar xz jq od dd awk sed grep sort uniq find du cut wc tr head mkdir rm \
mktemp sha256sum dirname"
# ELF constants: program header types, note type, note alignments.
readonly PT_INTERP=3
readonly PT_NOTE=4
readonly NT_GNU_ABI_TAG=1
readonly NOTE_ALIGN_DEFAULT=4
readonly NOTE_ALIGN_WIDE=8
readonly NOTE_HEADER_BYTES=12
# Upper bounds of a note's name and descriptor; a larger value means a broken note segment.
readonly NOTE_MAX_NAME=64
readonly NOTE_MAX_DESC=4096
# Bytes of a tar header block and the offsets of its fields (POSIX ustar layout).
readonly TAR_BLOCK=512
readonly TAR_SIZE_OFFSET=124
readonly TAR_SIZE_BYTES=12
readonly TAR_TYPEFLAG_OFFSET=156
readonly TAR_MAGIC_OFFSET=257
# A numeric field whose first byte is 0x80 holds a positive big-endian base-256 number in
# the remaining bytes (GNU extension, used for members of 8 GiB and more); 0xff is negative.
readonly TAR_BASE256_POSITIVE=128
readonly TAR_BASE256_NEGATIVE=255
readonly TAR_BASE256_HIGH_BYTES=4
# Members of a text kind are checked for CR characters and leftover @NAME@ tokens.
readonly TOKEN_PATTERN='@[A-Z][A-Z0-9_]*@'
readonly LIFECYCLE_SCRIPTS="preinst postinst preuninst postuninst preupgrade postupgrade \
start-stop-status"
readonly SCRIPT_SHEBANG='#!/bin/sh'
readonly ICON_SIZES="PACKAGE_ICON.PNG:64 PACKAGE_ICON_256.PNG:256"
# The DSM main menu entry (DSM-NOTES section 16): INFO dsmuidir, its application config and
# the icon sizes DSM puts into the {0} of the config's "icon".
readonly DESKTOP_UI_DIR=ui
readonly DESKTOP_ICON_SIZES="16 24 32 48 64 72 256"
# Keys of the entry as spkmeta's desktopApp writes them, sorted as jq's `keys` lists them.
readonly DESKTOP_ENTRY_KEYS="advanceGrantPrivilege,allUsers,desc,grantPrivilege,icon,port,protocol,title,type,url"

release=no
port=$DEFAULT_PORT
# "" until resolve_expected_file sets the default next to this script.
expected_file=""
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

summary() {
	printf 'inspect-spk: %s checks, %s failed, %s warnings (%s)\n' \
		"$checks" "$failures" "$warnings" "$spk"
}

# abort <description>: a failure after which no further check makes sense (the archive is
# not readable or not safe to extract); prints the summary and exits 1.
abort() {
	failed "$*"
	summary
	exit "$EXIT_FAILURE"
}

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
			*) set_operand "$1" ;;
		esac
		shift
	done
	[ -n "$spk" ] && [ -n "$family" ] || usage
	[ -f "$spk" ] || die "$spk is not a file"
	if ! printf '%s\n' "$port" | grep -Eqx '[1-9][0-9]{0,4}' || [ "$port" -gt "$MAX_PORT" ]; then
		die "--port must be 1-$MAX_PORT, not '$port'"
	fi
	case "$family" in
		x86_64 | armv8 | armv7) ;;
		*) die "family must be x86_64, armv8 or armv7, not '$family'" ;;
	esac
}

# set_operand <word>: the first operand is the SPK, the second the family.
set_operand() {
	if [ -z "$spk" ]; then
		spk=$1
	elif [ -z "$family" ]; then
		family=$1
	else
		die "too many arguments: $1"
	fi
}

require_tools() {
	for tool in $REQUIRED_TOOLS; do
		command -v "$tool" >/dev/null 2>&1 ||
			die "$tool is required (run in the image of the Dockerfile next to this script)"
	done
}

# Facts of the family (ADR 0026 Notes): INFO arch, ELF class and machine, oldest DSM 7.2 kernel.
family_arch() {
	case "$1" in
		x86_64) printf '%s\n' "x86_64" ;;
		armv8) printf '%s\n' "armv8" ;;
		armv7) printf '%s\n' "armv7 armada38x monaco" ;;
	esac
}

family_elf() {
	case "$1" in
		x86_64) printf '%s\n' "2 62" ;;  # ELFCLASS64, EM_X86_64
		armv8) printf '%s\n' "2 183" ;;  # ELFCLASS64, EM_AARCH64
		armv7) printf '%s\n' "1 40" ;;   # ELFCLASS32, EM_ARM
	esac
}

family_kernel() {
	case "$1" in
		x86_64 | armv7) printf '%s\n' "3.10.108" ;;
		armv8) printf '%s\n' "4.4.302" ;;
	esac
}

# --- binary helpers ---------------------------------------------------------------------

# read_u <file> <offset> <bytes>: unsigned little-endian integer of up to 8 bytes.
read_u() {
	od -An -v -tu1 -j "$2" -N "$3" "$1" |
		awk 'BEGIN { p = 1 } { for (i = 1; i <= NF; i++) { v += $i * p; p *= 256 } }
			END { printf "%.0f", v }'
}

# read_be <file> <offset> <bytes>: unsigned big-endian integer (exact up to 2^53).
read_be() {
	od -An -v -tu1 -j "$2" -N "$3" "$1" |
		awk '{ for (i = 1; i <= NF; i++) v = v * 256 + $i } END { printf "%.0f", v }'
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
	printf '%s\n' "$1" |
		awk '{ for (i = 1; i <= length($0); i++) v = v * 8 + substr($0, i, 1); printf "%.0f", v }'
}

# file_size <file>: size in bytes.
file_size() {
	wc -c <"$1" | tr -d ' '
}

# --- tar format ---------------------------------------------------------------------------

# tar_member_size <file> <header offset>: the size field of a header; octal, or base-256
# (first byte 0x80, the other eleven bytes big-endian). Prints "-1" for a negative
# base-256 value, for one of 2^56 bytes or more (the high bytes +1..+4 not zero), which
# no member can have and the shell could not add, and for a first byte 0x81 to 0xfe: GNU tar
# writes only 0x80 and 0xff there, and such a byte is no octal digit either.
tar_member_size() {
	size_first=$(read_u "$1" "$(($2 + TAR_SIZE_OFFSET))" 1)
	if [ "$size_first" -gt "$TAR_BASE256_POSITIVE" ] && [ "$size_first" -lt "$TAR_BASE256_NEGATIVE" ]; then
		printf '%s' -1
	elif [ "$size_first" -eq "$TAR_BASE256_POSITIVE" ]; then
		if [ "$(read_hex "$1" "$(($2 + TAR_SIZE_OFFSET + 1))" "$TAR_BASE256_HIGH_BYTES")" = 00000000 ]
		then
			read_be "$1" "$(($2 + TAR_SIZE_OFFSET + 1 + TAR_BASE256_HIGH_BYTES))" \
				"$((TAR_SIZE_BYTES - 1 - TAR_BASE256_HIGH_BYTES))"
		else
			printf '%s' -1
		fi
	elif [ "$size_first" -eq "$TAR_BASE256_NEGATIVE" ]; then
		printf '%s' -1
	else
		octal "$(read_text "$1" "$(($2 + TAR_SIZE_OFFSET))" "$((TAR_SIZE_BYTES - 1))" | tr -d ' ')"
	fi
}

# tar_header_kind <file> <header offset>: gnu (magic "ustar  " NUL), posix ("ustar" NUL "00",
# ustar or pax), end (a zero block) or unknown.
tar_header_kind() {
	case "$(read_hex "$1" "$(($2 + TAR_MAGIC_OFFSET))" 8)" in
		7573746172202000) printf '%s\n' gnu ;;
		7573746172003030) printf '%s\n' posix ;;
		0000000000000000) printf '%s\n' end ;;
		*) printf '%s\n' unknown ;;
	esac
}

# tar_extension <file> <header offset>: the extension a header type stands for, if any.
tar_extension() {
	case "$(read_text "$1" "$(($2 + TAR_TYPEFLAG_OFFSET))" 1)" in
		L) printf '%s\n' gnu-longname ;;
		K) printf '%s\n' gnu-longlink ;;
		x) printf '%s\n' pax-extended ;;
		g) printf '%s\n' pax-global ;;
	esac
	[ "$(read_u "$1" "$(($2 + TAR_SIZE_OFFSET))" 1)" -lt "$TAR_BASE256_POSITIVE" ] ||
		printf '%s\n' base-256-size
}

# tar_format <file>: prints "<flavour> <extensions>" for an uncompressed tar: flavour gnu,
# posix, mixed, unknown or empty; extensions are the header types that go beyond plain ustar
# (gnu-longname, gnu-longlink, pax-extended, pax-global, base-256-size) or "none". Walks
# the header blocks, so every member is seen.
tar_format() {
	tar_file=$1
	tar_total=$(file_size "$tar_file")
	tar_offset=0
	tar_flavour=""
	tar_extensions=""
	while [ "$((tar_offset + TAR_BLOCK))" -le "$tar_total" ]; do
		tar_this=$(tar_header_kind "$tar_file" "$tar_offset")
		[ "$tar_this" != end ] || break
		if [ -z "$tar_flavour" ]; then
			tar_flavour=$tar_this
		elif [ "$tar_flavour" != "$tar_this" ]; then
			tar_flavour=mixed
		fi
		tar_extensions="$tar_extensions $(tar_extension "$tar_file" "$tar_offset")"
		tar_size=$(tar_member_size "$tar_file" "$tar_offset")
		[ "$tar_size" -ge 0 ] || { tar_flavour=unknown; break; }
		tar_offset=$((tar_offset + TAR_BLOCK + (tar_size + TAR_BLOCK - 1) / TAR_BLOCK * TAR_BLOCK))
	done
	# shellcheck disable=SC2086 # one extension per word on purpose
	tar_extensions=$(printf '%s\n' $tar_extensions | LC_ALL=C sort -u | tr '\n' ',' | sed 's/,$//')
	printf '%s %s\n' "${tar_flavour:-empty}" "${tar_extensions:-none}"
}

# --- member lists -------------------------------------------------------------------------

# list_members <tar> <out>: writes "<mode> <owner> <name>" per member to <out>, "./"
# prefixes and the link targets of hard and symbolic links stripped. Fails when tar cannot
# read the archive.
list_members() {
	tar -tvf "$1" >"$2.raw" || return 1
	awk '{
		name = $0
		for (i = 1; i <= 5; i++) sub(/^[^ ]+ +/, "", name)
		sub(/^\.\//, "", name)
		if ($1 ~ /^h/) sub(/ link to .*$/, "", name)
		if ($1 ~ /^l/) sub(/ -> .*$/, "", name)
		print $1, $2, name
	}' "$2.raw" >"$2"
}

# member_names <listing>: the name of every member, whole: everything after the mode and owner
# columns, so a name with blanks is not cut at the first one (as print_digests reads it).
member_names() {
	awk '{ name = $0; sub(/^[^ ]+ [^ ]+ /, "", name); print name }' "$1"
}

# file_member_names <listing>: member_names of the regular files only.
file_member_names() {
	awk '$1 ~ /^-/ { name = $0; sub(/^[^ ]+ [^ ]+ /, "", name); print name }' "$1"
}

# listing_mode <line>: the mode column of a listing line.
listing_mode() {
	printf '%s\n' "${1%% *}"
}

# listing_name <line>: the name of a listing line, everything after the mode and owner columns,
# blanks at either end included (read with IFS= and -r, so nothing is trimmed).
listing_name() {
	listing_rest=${1#* }
	printf '%s\n' "${listing_rest#* }"
}

# unsafe_members <listing>: one "name(reason)" per member that must not be extracted:
# anything but a file or directory (links, devices, FIFOs), an absolute name, a ".."
# component, or a name tar printed with escapes (control characters, backslashes).
unsafe_members() {
	while IFS= read -r unsafe_line; do
		unsafe_mode=$(listing_mode "$unsafe_line")
		unsafe_name=$(listing_name "$unsafe_line")
		case "$unsafe_mode" in
			d* | -*) ;;
			*) printf '%s(link or special file: %s) ' "$unsafe_name" "$unsafe_mode"; continue ;;
		esac
		case "$unsafe_name" in
			/* | .. | ../* | */../* | */..)
				printf '%s(absolute or outside the archive) ' "$unsafe_name"
				;;
			*\\*) printf '%s(name with escaped characters) ' "$unsafe_name" ;;
		esac
	done <"$1"
}

# read_archive <label> <tar> <listing> <dir>: lists the archive, refuses unsafe members and
# extracts it without owners and stored permissions (the modes are judged from the listing).
read_archive() {
	list_members "$2" "$3" || abort "$1: tar cannot list the archive"
	archive_unsafe=$(unsafe_members "$3")
	[ -z "$archive_unsafe" ] || abort "$1: unsafe members, nothing extracted: ${archive_unsafe% }"
	pass "$1: no link, special file, absolute name or .. component among the members"
	mkdir -p "$4" || die "cannot create $4"
	tar -xf "$2" -C "$4" --no-same-owner --no-same-permissions ||
		abort "$1: tar cannot extract the archive"
}

# mode_matches <mode-column> <expected 0755|0644>
mode_matches() {
	case "$1:$2" in
		-rwxr-xr-x:0755 | -rw-r--r--:0644 | drwxr-xr-x:0755) return 0 ;;
		*) return 1 ;;
	esac
}

# rule_name <member column>: the member column of a rule with "<id>" replaced.
rule_name() {
	printf '%s\n' "$1" | sed "s|<id>|$id|g"
}

# expected_rule <archive> <name>: prints "<mode> <rule>" of the first matching rule of the
# expected-member list, exact names first, then patterns; nothing when there is none.
expected_rule() {
	rule_archive=$1
	rule_name=$2
	rule_found=""
	while read -r rule_kind rule_mode rule_member; do
		[ "$rule_kind" = "$rule_archive" ] || continue
		rule_member=$(rule_name "$rule_member")
		if [ "$rule_member" = "$rule_name" ]; then
			printf '%s %s\n' "$rule_mode" "$rule_member"
			return 0
		fi
		case "$rule_member" in
			*\**)
				# shellcheck disable=SC2254 # the rule is a pattern on purpose
				case "$rule_name" in
					$rule_member) [ -n "$rule_found" ] || rule_found="$rule_mode $rule_member" ;;
				esac
				;;
		esac
	done <"$expected_rules"
	[ -z "$rule_found" ] || printf '%s\n' "$rule_found"
}

# suspicious_name <name>: true for names that come from the build environment.
suspicious_name() {
	case "$1" in
		*SynoBuildConf* | build.env | *INFO.sh | prebuilt/*) return 0 ;;
		.spkstage* | .fetch-manifest | */.fetch-manifest) return 0 ;;
		*_debug* | *.debug | *.dbg | *.orig | *.rej | *~) return 0 ;;
		.* | */.*) return 0 ;;
	esac
	return 1
}

# member_problem <archive> <mode> <name>: the problem of one member as "name(reason)", or
# nothing; records the matched rule in $members_seen.
member_problem() {
	if suspicious_name "$3"; then
		printf '%s(build leftover or hidden file)' "$3"
		return
	fi
	problem_rule=$(expected_rule "$1" "$3")
	if [ -z "$problem_rule" ]; then
		printf '%s(unexpected)' "$3"
		return
	fi
	problem_mode=${problem_rule%% *}
	case "$3" in */) problem_mode=0755 ;; esac
	mode_matches "$2" "$problem_mode" || printf '%s(mode %s, want %s)' "$3" "$2" "$problem_mode"
	printf '%s\n' "${problem_rule#* }" >>"$members_seen"
}

# missing_members <archive>: "rule(expected once, found 0)" for every exact rule no member
# matched (a second member of the same name is a duplicate, see duplicate_members).
missing_members() {
	while read -r rule_kind _ rule_member; do
		[ "$rule_kind" = "$1" ] || continue
		case "$rule_member" in *\**) continue ;; esac
		rule_member=$(rule_name "$rule_member")
		grep -qxF -- "$rule_member" "$members_seen" ||
			printf '%s(expected once, found 0) ' "$rule_member"
	done <"$expected_rules"
}

# duplicate_members <listing>: "name(duplicate, N times)" for every name stored more than
# once, whether an exact rule or a pattern allows it (tar keeps only the last copy).
duplicate_members() {
	member_names "$1" | LC_ALL=C sort | uniq -c |
		awk '$1 > 1 { count = $1; sub(/^ *[0-9]+ /, ""); printf "%s(duplicate, %d times) ", $0, count }'
}

# check_members <archive> <listing>: every member against the expected list (name, mode),
# every required rule present, no duplicate, nothing from the build, no signature.
check_members() {
	members_archive=$1
	members_listing=$2
	members_problems=""
	members_seen="$work/seen-$members_archive"
	: >"$members_seen"
	while IFS= read -r member_line; do
		member_mode=$(listing_mode "$member_line")
		member_name=$(listing_name "$member_line")
		[ -n "$member_name" ] || continue
		member_text=$(member_problem "$members_archive" "$member_mode" "$member_name")
		[ -z "$member_text" ] || members_problems="$members_problems $member_text"
	done <"$members_listing"
	members_problems="$members_problems $(missing_members "$members_archive")"
	members_problems="$members_problems $(duplicate_members "$members_listing")"
	members_problems=$(printf '%s\n' "$members_problems" | sed 's/^ *//;s/ *$//;s/  */ /g')
	if [ -z "$members_problems" ]; then
		pass "$members_archive: members, modes and types match the expected list"
	else
		failed "$members_archive: members differ from the expected list: $members_problems"
	fi
	if grep -qi 'signature' "$members_listing"; then
		failed "$members_archive: a signature member is present (signing is deprecated since DSM 7)"
	else
		pass "$members_archive: no signature member"
	fi
	members_owners=$(awk '{ print $2 }' "$members_listing" | LC_ALL=C sort -u | tr '\n' ' ')
	info "$members_archive: owners ${members_owners% }"
}

# --- the outer archive ----------------------------------------------------------------------

check_outer_archive() {
	case "$(read_hex "$spk" 0 6)" in
		1f8b*) abort "SPK is gzip-compressed, not an uncompressed tar" ;;
		fd377a585a00) abort "SPK is xz-compressed, not an uncompressed tar" ;;
	esac
	outer_format=$(tar_format "$spk")
	case "$outer_format" in
		gnu\ * | posix\ *) pass "SPK is an uncompressed tar (format ${outer_format%% *})" ;;
		*) abort "SPK is not a tar archive with ustar or GNU headers (${outer_format%% *})" ;;
	esac
	info "SPK tar extensions beyond plain ustar: ${outer_format#* }"
	outer_listing="$work/listing-spk"
	read_archive spk "$spk" "$outer_listing" "$work/spk"
	info "SPK member order: $(member_names "$outer_listing" | tr '\n' ' ')"
	[ -f "$work/spk/INFO" ] || abort "INFO is missing"
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
	if printf '%s\n' "$info_actual" | grep -Eq "$info_pattern"; then
		pass "INFO $info_key=\"$info_actual\""
	else
		failed "INFO $info_key=\"$info_actual\" does not match $info_pattern"
	fi
}

info_absent() {
	if [ "$(info_count "$1")" -eq 0 ]; then
		pass "INFO has no $1"
	else
		failed "INFO has $1 ($(grep "^$1=" "$work/spk/INFO" | head -n 1))"
	fi
}

read_package_name() {
	id=$(info_value package | head -n 1)
	printf '%s\n' "$id" | grep -Eq "$PACKAGE_NAME_PATTERN" ||
		abort "INFO package=\"$id\" is not a package name"
}

# version_parts_in_range <a.b.c-d>: every number at most 2^31-1.
version_parts_in_range() {
	for version_part in $(printf '%s\n' "$1" | tr '.-' '  '); do
		[ "${#version_part}" -le "$MAX_VERSION_DIGITS" ] &&
			[ "$version_part" -le "$MAX_VERSION_NUMBER" ] || return 1
	done
}

check_info_lines() {
	info_bad_lines=$(grep -Env '^[a-z_]+="[^"\\$`]*"$' "$work/spk/INFO" | head -n 3 | tr '\n' ' ')
	if [ -z "$info_bad_lines" ]; then
		pass "INFO: every line is key=\"value\" without shell-active characters"
	else
		failed "INFO: lines that are not key=\"value\": $info_bad_lines"
	fi
}

check_info_version() {
	info_required version '^[0-9]+\.[0-9]+\.[0-9]+-[0-9]+$'
	info_version=$(info_value version)
	if version_parts_in_range "$info_version"; then
		pass "INFO version parts are at most $MAX_VERSION_NUMBER"
	else
		failed "INFO version $info_version has a part above $MAX_VERSION_NUMBER"
	fi
	info_build=${info_version##*-}
	if [ "${info_build:-9999}" -lt "$FINAL_BUILD_NUMBER" ] 2>/dev/null; then
		info_required beta '^yes$'
	else
		info_absent beta
	fi
}

check_info() {
	check_info_lines
	info_required package "$PACKAGE_NAME_PATTERN"
	info_required displayname '^.+$'
	check_info_version
	info_required arch "^$(family_arch "$family")\$"
	info_required os_min_ver "^$OS_MIN_VER\$"
	info_required adminport "^$port\$"
	info_required adminprotocol '^http$'
	info_absent adminurl
	info_required dsmuidir "^$DESKTOP_UI_DIR\$"
	info_required dsmappname '^[A-Za-z0-9][A-Za-z0-9._-]*$'
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
	if [ "$package_magic" = fd377a585a00 ]; then
		pass "package.tgz is xz-compressed"
	else
		abort "package.tgz is not xz-compressed (magic $package_magic)"
	fi
	xz -dc "$package_tgz" >"$work/package.tar" || abort "package.tgz does not decompress"
	package_format=$(tar_format "$work/package.tar")
	case "$package_format" in
		gnu\ * | posix\ *) pass "package.tgz holds a tar (format ${package_format%% *})" ;;
		*) abort "package.tgz does not hold a tar with ustar or GNU headers (${package_format%% *})" ;;
	esac
	info "package.tgz tar extensions beyond plain ustar: ${package_format#* }"
	package_listing="$work/listing-package"
	read_archive package.tgz "$work/package.tar" "$package_listing" "$work/pkg"
	check_members package "$package_listing"
	package_kib=$(du -sk "$work/pkg" | cut -f1)
	info "package.tgz extracts to $package_kib KiB (INFO extractsize=\"$(info_value extractsize)\")"
}

# --- ELF binaries ---------------------------------------------------------------------------

# elf_header <file>: sets elf_class, elf_type, elf_machine, elf_flags, elf_phoff,
# elf_phentsize and elf_phnum; fails when the file is not a little-endian ELF.
elf_header() {
	[ "$(read_hex "$1" 0 4)" = 7f454c46 ] || return 1
	[ "$(read_u "$1" 5 1)" -eq 1 ] || return 1
	elf_class=$(read_u "$1" 4 1)
	elf_type=$(read_u "$1" 16 2)
	elf_machine=$(read_u "$1" 18 2)
	if [ "$elf_class" -eq 2 ]; then
		elf_phoff=$(read_u "$1" 32 8)
		elf_flags=$(read_u "$1" 48 4)
		elf_phentsize=$(read_u "$1" 54 2)
		elf_phnum=$(read_u "$1" 56 2)
	else
		elf_phoff=$(read_u "$1" 28 4)
		elf_flags=$(read_u "$1" 36 4)
		elf_phentsize=$(read_u "$1" 42 2)
		elf_phnum=$(read_u "$1" 44 2)
	fi
}

# elf_note_segment <file> <program header offset>: prints "offset size align" of a PT_NOTE
# program header (p_offset, p_filesz, p_align) for the class in $elf_class.
elf_note_segment() {
	if [ "$elf_class" -eq 2 ]; then
		printf '%s %s %s\n' "$(read_u "$1" "$(($2 + 8))" 8)" "$(read_u "$1" "$(($2 + 32))" 8)" \
			"$(read_u "$1" "$(($2 + 48))" 8)"
	else
		printf '%s %s %s\n' "$(read_u "$1" "$(($2 + 4))" 4)" "$(read_u "$1" "$(($2 + 16))" 4)" \
			"$(read_u "$1" "$(($2 + 28))" 4)"
	fi
}

# elf_facts <file>: prints "class machine type interp abi flags" (interp yes/no; abi is the
# GNU ABI tag as major.minor.patch or "none"); nothing when the file is not a little-endian ELF.
elf_facts() {
	elf=$1
	elf_header "$elf" || return 0
	elf_interp=no
	elf_abi=none
	elf_index=0
	while [ "$elf_index" -lt "$elf_phnum" ]; do
		elf_ph=$((elf_phoff + elf_index * elf_phentsize))
		elf_ptype=$(read_u "$elf" "$elf_ph" 4)
		[ "$elf_ptype" -ne "$PT_INTERP" ] || elf_interp=yes
		if [ "$elf_ptype" -eq "$PT_NOTE" ]; then
			# shellcheck disable=SC2046 # three numbers on purpose
			elf_tag=$(elf_abi_tag "$elf" $(elf_note_segment "$elf" "$elf_ph"))
			[ -z "$elf_tag" ] || elf_abi=$elf_tag
		fi
		elf_index=$((elf_index + 1))
	done
	printf '%s %s %s %s %s 0x%08x\n' "$elf_class" "$elf_machine" "$elf_type" "$elf_interp" \
		"$elf_abi" "$elf_flags"
}

# align_up <value> <alignment>
align_up() {
	printf '%s\n' "$((($1 + $2 - 1) / $2 * $2))"
}

# elf_abi_tag <file> <offset> <size> <align>: the GNU ABI tag of a note segment
# (NT_GNU_ABI_TAG, name "GNU"), as major.minor.patch; nothing when the segment has none.
# Name and descriptor are padded to the segment's alignment: 8 for p_align 8 (as glibc's
# ELF_NOTE_DESC_OFFSET/ELF_NOTE_NEXT_OFFSET and readelf do), else 4.
elf_abi_tag() {
	note_align=$NOTE_ALIGN_DEFAULT
	[ "$4" -ne "$NOTE_ALIGN_WIDE" ] || note_align=$NOTE_ALIGN_WIDE
	note_pos=$2
	note_end=$(($2 + $3))
	note_file_end=$(file_size "$1")
	[ "$note_end" -le "$note_file_end" ] || note_end=$note_file_end
	while [ "$((note_pos + NOTE_HEADER_BYTES))" -le "$note_end" ]; do
		note_namesz=$(read_u "$1" "$note_pos" 4)
		note_descsz=$(read_u "$1" "$((note_pos + 4))" 4)
		note_type=$(read_u "$1" "$((note_pos + 8))" 4)
		[ "$note_namesz" -le "$NOTE_MAX_NAME" ] && [ "$note_descsz" -le "$NOTE_MAX_DESC" ] || return 0
		note_name=$(read_text "$1" "$((note_pos + NOTE_HEADER_BYTES))" "$note_namesz")
		note_desc=$((note_pos + $(align_up "$((NOTE_HEADER_BYTES + note_namesz))" "$note_align")))
		if [ "$note_name" = GNU ] && [ "$note_type" -eq "$NT_GNU_ABI_TAG" ] &&
			[ "$note_descsz" -ge 16 ]; then
			printf '%s.%s.%s\n' "$(read_u "$1" "$((note_desc + 4))" 4)" \
				"$(read_u "$1" "$((note_desc + 8))" 4)" "$(read_u "$1" "$((note_desc + 12))" 4)"
			return 0
		fi
		note_pos=$((note_pos + $(align_up "$((note_desc - note_pos + note_descsz))" "$note_align")))
	done
}

# version_at_most <a.b.c> <x.y.z>: true when the first kernel version is not newer.
version_at_most() {
	awk -v a="$1" -v b="$2" 'BEGIN {
		split(a, x, "."); split(b, y, ".")
		for (i = 1; i <= 3; i++) { if (x[i] + 0 < y[i] + 0) exit 0; if (x[i] + 0 > y[i] + 0) exit 1 }
		exit 0
	}'
}

# binary_verdict <kind> <description>: FFmpeg findings follow ffmpeg_verdict, the server
# binary always fails.
binary_verdict() {
	if [ "$1" = ffmpeg ]; then ffmpeg_verdict "$2"; else failed "$2"; fi
}

# check_binary_abi <name> <kind> <abi>: the GNU ABI note against the family's oldest kernel.
# Without the note the kernel floor cannot be read from the file: the Go server carries
# none (recorded), and neither does a static musl FFmpeg (SPK-15; a warning, its kernel
# floor is checked by the SPK-11 smoke test under kernelsim).
check_binary_abi() {
	abi_kernel=$(family_kernel "$family")
	if [ "$3" = none ] && [ "$2" = server ]; then
		info "bin/$1: no GNU ABI note (runs on any kernel the libc supports)"
	elif [ "$3" = none ]; then
		warn "bin/$1: no GNU ABI note, kernel floor not checked here" \
			"(the SPK-11 smoke test runs it under kernelsim)"
	elif version_at_most "$3" "$abi_kernel"; then
		pass "bin/$1: ELF ABI note $3 is at most the oldest DSM 7.2 kernel $abi_kernel"
	else
		binary_verdict "$2" "bin/$1: ELF ABI note $3 is above the oldest DSM 7.2 kernel $abi_kernel"
	fi
}

# check_binary <name> <kind>: kind "server" (always strict) or "ffmpeg" (ffmpeg_verdict).
check_binary() {
	# a missing binary is reported by the member check already
	[ -f "$work/pkg/bin/$1" ] || return 0
	binary_facts=$(elf_facts "$work/pkg/bin/$1")
	if [ -z "$binary_facts" ]; then
		failed "bin/$1 is not a little-endian ELF file"
		return
	fi
	# shellcheck disable=SC2086 # the facts are six blank-separated words on purpose
	set -- "$1" "$2" $binary_facts
	# $3 class, $4 machine, $5 type, $6 interp, $7 abi, $8 flags
	binary_expected=$(family_elf "$family")
	if [ "$3 $4" = "$binary_expected" ]; then
		pass "bin/$1: ELF class $3, machine $4 (family $family)"
	else
		failed "bin/$1: ELF class $3, machine $4, want $binary_expected for $family"
	fi
	case "$5" in
		2 | 3) pass "bin/$1: ELF type $5 (executable)" ;;
		*) failed "bin/$1: ELF type $5 is not an executable (2 or 3)" ;;
	esac
	if [ "$6" = no ]; then
		pass "bin/$1: statically linked (no program interpreter)"
	else
		binary_verdict "$2" "bin/$1: dynamically linked (has a program interpreter)"
	fi
	check_binary_abi "$1" "$2" "$7"
	[ "$family" != armv7 ] ||
		info "bin/$1: ELF e_flags $8 (EABI version in the top byte; 0x400 = EF_ARM_ABI_FLOAT_HARD)"
}

# --- FFmpeg notice -----------------------------------------------------------------------

# notice_field <file> <key>: the first word after "<key>:".
notice_field() {
	sed -n "s/^$2:[[:space:]]*\([^[:space:]]*\).*/\1/p" "$1" | head -n 1
}

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
		failed "SOURCE.txt has $notice_status_lines Status: and $notice_licence_lines Licence:" \
			"lines, want one each"
	fi
	ffmpeg_status=$(notice_field "$notice" Status)
	ffmpeg_licence=$(notice_field "$notice" Licence)
	info "FFmpeg build: version $(notice_field "$notice" Version)," \
		"status ${ffmpeg_status:-none}, licence ${ffmpeg_licence:-none}"
}

check_ffmpeg_notice() {
	[ "$ffmpeg_status" != missing ] || return 0
	case "$ffmpeg_status" in
		selected) pass "SOURCE.txt status is selected" ;;
		interim) ffmpeg_verdict "SOURCE.txt status is interim" ;;
		*) failed "SOURCE.txt status is '$ffmpeg_status', want selected" ;;
	esac
	case "$ffmpeg_licence" in
		*nonfree*)
			ffmpeg_verdict "SOURCE.txt licence $ffmpeg_licence is non-free (not redistributable)"
			;;
		GPL-*) pass "SOURCE.txt licence $ffmpeg_licence" ;;
		*) failed "SOURCE.txt licence '$ffmpeg_licence' is not a GPL identifier" ;;
	esac
	licence_files=$(file_member_names "$work/listing-package" |
		awk 'index($0, "share/licenses/ffmpeg/") == 1 && $0 !~ /\/SOURCE\.txt$/' | wc -l | tr -d ' ')
	if [ "$licence_files" -ge 1 ]; then
		pass "share/licenses/ffmpeg has $licence_files licence file(s) besides SOURCE.txt"
	else
		failed "share/licenses/ffmpeg has no licence file besides SOURCE.txt"
	fi
}

# --- firewall service file --------------------------------------------------------------

check_service_file() {
	service="$work/pkg/port_conf/$id.sc"
	[ -f "$service" ] || { failed "port_conf/$id.sc is missing"; return; }
	check "port_conf/$id.sc: section [$id]" grep -qxF "[$id]" "$service"
	check "port_conf/$id.sc: title" grep -Eq '^title="[^"]+"$' "$service"
	check "port_conf/$id.sc: desc" grep -Eq '^desc="[^"]+"$' "$service"
	check "port_conf/$id.sc: port_forward=\"no\"" grep -qxF 'port_forward="no"' "$service"
	check "port_conf/$id.sc: dst.ports=\"$port/tcp\"" grep -qxF "dst.ports=\"$port/tcp\"" "$service"
	resource_file=$(jq -r '."port-config"."protocol-file" // ""' "$work/spk/conf/resource" 2>/dev/null)
	if [ "$resource_file" = "port_conf/$id.sc" ]; then
		pass "conf/resource port-config names port_conf/$id.sc"
	else
		failed "conf/resource port-config names '$resource_file', want port_conf/$id.sc"
	fi
}

# --- text members -------------------------------------------------------------------------

# list_files <out> <root...>: writes the regular files below the roots that exist to <out>,
# one per line (the member names were checked for escapes before extraction). A missing
# root is a missing member, which the member check reports.
list_files() {
	list_out=$1
	shift
	: >"$list_out"
	for list_root in "$@"; do
		[ -e "$list_root" ] || continue
		find "$list_root" -type f >>"$list_out" || abort "find cannot list $list_root"
	done
}

# files_matching <pattern> <file list>: sets matching_names to the files of the list that
# contain a match of the extended regular expression, as member names ("package.tgz:" for
# the payload), sorted and blank-separated. A file grep cannot read is a failure, not a
# silent "no match". Sets a variable instead of printing, so the failure counts.
files_matching() {
	: >"$work/matches"
	while IFS= read -r matching_file; do
		grep -Eq -- "$1" "$matching_file"
		case "$?" in
			0) printf '%s\n' "$matching_file" >>"$work/matches" ;;
			1) ;;
			*) failed "grep cannot read $matching_file" ;;
		esac
	done <"$2"
	matching_names=$(sed "s|^$work/spk/||;s|^$work/pkg/|package.tgz:|" "$work/matches" |
		LC_ALL=C sort | tr '\n' ' ')
	matching_names=${matching_names% }
}

check_scripts() {
	list_files "$work/files-scripts" "$work/spk/scripts"
	files_matching "$(printf '\r')" "$work/files-scripts"
	if [ -z "$matching_names" ]; then
		pass "scripts/*: LF line endings"
	else
		failed "scripts with CR characters: $matching_names"
	fi
	for script in $LIFECYCLE_SCRIPTS; do
		[ -f "$work/spk/scripts/$script" ] || continue
		if [ "$(head -n 1 "$work/spk/scripts/$script")" = "$SCRIPT_SHEBANG" ]; then
			pass "scripts/$script starts with $SCRIPT_SHEBANG"
		else
			failed "scripts/$script does not start with $SCRIPT_SHEBANG"
		fi
	done
}

# check_tokens: every text file of both archives (everything but the binaries, icons and
# package.tgz) without @NAME@ tokens; INFO, conf, wizard and service files without CR.
check_tokens() {
	list_files "$work/files-all" "$work/spk" "$work/pkg"
	awk -v binaries="$work/pkg/bin/" '!/\.(PNG|png)$/ && !/\/package\.tgz$/ && index($0, binaries) != 1' \
		"$work/files-all" >"$work/files-text"
	files_matching "$TOKEN_PATTERN" "$work/files-text"
	if [ -z "$matching_names" ]; then
		pass "no unreplaced @NAME@ token in any text member"
	else
		failed "unreplaced @NAME@ tokens in: $matching_names"
	fi
	list_files "$work/files-lf" "$work/spk/INFO" "$work/spk/conf" "$work/spk/WIZARD_UIFILES" \
		"$work/pkg/port_conf" "$work/pkg/$DESKTOP_UI_DIR/config"
	files_matching "$(printf '\r')" "$work/files-lf"
	if [ -z "$matching_names" ]; then
		pass "INFO, conf/*, WIZARD_UIFILES/*, port_conf/* and $DESKTOP_UI_DIR/config: LF line endings"
	else
		failed "CR characters in: $matching_names"
	fi
}

# --- resources, wizard, icons ---------------------------------------------------------------

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
	if [ "$(jq -r '.defaults."run-as" // ""' "$privilege" 2>/dev/null)" = package ]; then
		pass "conf/privilege run-as is package"
	else
		failed "conf/privilege run-as is not package"
	fi
	if jq -e '[.. | strings | select(. == "root")] | length == 0' "$privilege" >/dev/null 2>&1; then
		pass "conf/privilege mentions no root"
	else
		failed "conf/privilege mentions root"
	fi
}

# png_size <file>: "<width>x<height>" of a PNG, nothing for another file type.
png_size() {
	[ "$(read_hex "$1" 0 8)" = 89504e470d0a1a0a ] || return 0
	printf '%sx%s' "$(read_be "$1" 16 4)" "$(read_be "$1" 20 4)"
}

check_icons() {
	for icon_size in $ICON_SIZES; do
		icon=${icon_size%%:*}
		icon_side=${icon_size#*:}
		[ -f "$work/spk/$icon" ] || { failed "$icon is missing"; continue; }
		icon_actual=$(png_size "$work/spk/$icon")
		if [ "$icon_actual" = "${icon_side}x${icon_side}" ]; then
			pass "$icon is a ${icon_side}x${icon_side} PNG"
		else
			failed "$icon is not a ${icon_side}x${icon_side} PNG (${icon_actual:-not a PNG})"
		fi
	done
}

# --- DSM main menu entry ----------------------------------------------------------------------

# desktop_value <jq path below the entry>: one value of the single ".url" entry of ui/config
# as text ("" when absent).
desktop_value() {
	jq -r "[.\".url\"[]][0]$1 | if . == null then \"\" else tostring end" "$desktop_config" 2>/dev/null
}

# desktop_expect <key> <expected>: the entry's key has exactly the expected value.
desktop_expect() {
	desktop_actual=$(desktop_value ".\"$1\"")
	if [ "$desktop_actual" = "$2" ]; then
		pass "$DESKTOP_UI_DIR/config: $1 is \"$2\""
	else
		failed "$DESKTOP_UI_DIR/config: $1 is \"$desktop_actual\", want \"$2\""
	fi
}

# check_desktop_config: ui/config (the application config of INFO dsmuidir) declares one
# "url" entry that opens http://<host>:<adminport>/ for every user, titled with INFO
# displayname and using images/<id>_{0}.png as icon.
check_desktop_config() {
	desktop_config="$work/pkg/$DESKTOP_UI_DIR/config"
	# a missing member is reported by the member check already
	[ -f "$desktop_config" ] || return 0
	if ! json_valid "$desktop_config"; then
		failed "$DESKTOP_UI_DIR/config does not parse as JSON"
		return
	fi
	pass "$DESKTOP_UI_DIR/config parses as JSON"
	desktop_shape=$(jq -r '(keys | join(",")) + " " + ((.".url" // {}) | length | tostring)' \
		"$desktop_config" 2>/dev/null)
	if [ "$desktop_shape" = ".url 1" ]; then
		pass "$DESKTOP_UI_DIR/config: exactly one .url entry"
	else
		failed "$DESKTOP_UI_DIR/config: top-level keys and .url entries are '$desktop_shape', want '.url 1'"
		return
	fi
	desktop_expect type url
	desktop_expect title "$(info_value displayname)"
	desktop_expect protocol http
	desktop_expect port "$(info_value adminport)"
	desktop_expect url /
	desktop_expect allUsers true
	desktop_expect icon "images/${id}_{0}.png"
	desktop_expect grantPrivilege all
	desktop_expect advanceGrantPrivilege true
	check_desktop_keys
	check_desktop_app_name
}

# check_desktop_app_name: the entry's property name is INFO dsmappname; DSM integrates the
# entry into the main menu only then (guide "dsmappname"; DSM-NOTES section 16).
check_desktop_app_name() {
	desktop_key=$(jq -r '.".url" | keys | .[0]' "$desktop_config" 2>/dev/null)
	desktop_app_name=$(info_value dsmappname)
	if [ -n "$desktop_key" ] && [ "$desktop_key" = "$desktop_app_name" ]; then
		pass "$DESKTOP_UI_DIR/config: the entry key is INFO dsmappname"
	else
		failed "$DESKTOP_UI_DIR/config: entry key \"$desktop_key\" is not INFO dsmappname \"$desktop_app_name\""
	fi
}

# check_desktop_keys: the entry has exactly the keys spkmeta writes, so neither a key DSM would
# read differently nor a missing one (desc) slips through the value checks.
check_desktop_keys() {
	desktop_keys=$(jq -r '[.".url"[]][0] | keys | join(",")' "$desktop_config" 2>/dev/null)
	if [ "$desktop_keys" = "$DESKTOP_ENTRY_KEYS" ]; then
		pass "$DESKTOP_UI_DIR/config: the entry has the keys $DESKTOP_ENTRY_KEYS"
	else
		failed "$DESKTOP_UI_DIR/config: the entry has the keys $desktop_keys, want $DESKTOP_ENTRY_KEYS"
	fi
}

# check_desktop_icons: one PNG of each size DSM substitutes into the config's icon template.
check_desktop_icons() {
	for desktop_size in $DESKTOP_ICON_SIZES; do
		desktop_icon="$DESKTOP_UI_DIR/images/${id}_$desktop_size.png"
		[ -f "$work/pkg/$desktop_icon" ] || continue
		desktop_actual=$(png_size "$work/pkg/$desktop_icon")
		if [ "$desktop_actual" = "${desktop_size}x$desktop_size" ]; then
			pass "$desktop_icon is a ${desktop_size}x$desktop_size PNG"
		else
			failed "$desktop_icon is not a ${desktop_size}x$desktop_size PNG (${desktop_actual:-not a PNG})"
		fi
	done
}

# --- member digests --------------------------------------------------------------------------

# print_digests <label> <dir> <listing>: "<sha256> <mode> <label>:<name>" per file member,
# sorted by name; the mode is the one stored in the archive (extraction ignores it).
print_digests() {
	awk '$1 ~ /^-/ {
		name = $0; sub(/^[^ ]+ [^ ]+ /, "", name)
		mode = 0
		for (i = 2; i <= 10; i++) mode = mode * 2 + (substr($1, i, 1) !~ /^[-ST]$/)
		if (substr($1, 4, 1) ~ /[sS]/) mode += 2048
		if (substr($1, 7, 1) ~ /[sS]/) mode += 1024
		if (substr($1, 10, 1) ~ /[tT]/) mode += 512
		printf "%o %s\n", mode, name
	}' "$3" | LC_ALL=C sort -k2 | while IFS= read -r digest_line; do
		digest_mode=${digest_line%% *}
		digest_name=${digest_line#* }
		[ "$1:$digest_name" != spk:package.tgz ] || continue
		# The redirect and sha256sum are checked: a member that cannot be read gets no line
		# with an empty digest.
		if ! digest=$(sha256sum <"$2/$digest_name") || [ -z "$digest" ]; then
			printf 'unhashed %s:%s\n' "$1" "$digest_name"
			continue
		fi
		printf '%s %s %s:%s\n' "${digest%% *}" "$digest_mode" "$1" "$digest_name"
	done
}

# print_member_digests prints the digest lines of both archives; a member that cannot be
# hashed fails the inspection.
print_member_digests() {
	{
		print_digests spk "$work/spk" "$work/listing-spk"
		print_digests package.tgz "$work/pkg" "$work/listing-package"
	} >"$work/digests"
	grep -v '^unhashed ' "$work/digests"
	# A here-document, not a pipe: failed must count in this shell.
	while IFS= read -r unhashed_line; do
		[ -z "$unhashed_line" ] || failed "--members: cannot hash ${unhashed_line#unhashed }"
	done <<EOF
$(grep '^unhashed ' "$work/digests")
EOF
}

# read_expected_rules writes the rule lines of the expected-member list to $expected_rules.
# grep exits 1 when the list holds no rule and 2 when it cannot read it; both are usage
# errors, since every member would otherwise be judged against nothing.
read_expected_rules() {
	expected_rules="$work/rules"
	expected_status=0
	grep -Ev '^[[:space:]]*(#|$)' "$expected_file" >"$expected_rules" || expected_status=$?
	case "$expected_status" in
		0) ;;
		1) die "expected-member list $expected_file holds no rule" ;;
		*) die "cannot read the expected-member list $expected_file" ;;
	esac
}

# resolve_expected_file sets the default expected-member list (next to this script) unless
# --expected named one, and checks that it exists. Runs after require_tools: it needs dirname.
resolve_expected_file() {
	if [ -z "$expected_file" ]; then
		expected_file="$(cd "$(dirname "$0")" && pwd)/expected-members.txt" ||
			die "cannot find the directory of $0"
	fi
	[ -f "$expected_file" ] || die "expected-member list $expected_file not found"
}

main() {
	parse_args "$@"
	require_tools
	resolve_expected_file
	work=$(mktemp -d) || die "cannot create a work directory"
	# An interrupt leaves through exit, so the EXIT trap removes the work directory.
	trap cleanup EXIT
	trap 'exit "$EXIT_INTERRUPTED"' INT
	trap 'exit "$EXIT_TERMINATED"' TERM
	read_expected_rules
	mode_name="test"
	[ "$release" = no ] || mode_name="release"
	info "inspecting $spk for family $family ($mode_name mode, port $port)"
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
	check_desktop_config
	check_desktop_icons
	check_tokens
	[ "$print_members" = no ] || print_member_digests
	summary
	[ "$failures" -eq 0 ] || exit "$EXIT_FAILURE"
}

main "$@"

#!/bin/sh
# fetch.sh - obtain the pinned FFmpeg build of one DSM architecture family from the own build
# recipe (build/, task SPK-15), verify it against sources.lock and install ffmpeg, ffprobe, the
# licence files and SOURCE.txt into a directory (SPK-16; until then this script downloaded the
# interim prebuilts, README.md "Result").
#
#   fetch.sh <family> <dest-dir> [lock-file]
#
#   family     x86_64, armv8 or armv7 (the DSM "arch" family values, also the recipe targets)
#   dest-dir   created if missing; receives ffmpeg and ffprobe (mode 0755), the licence
#              files and SOURCE.txt (mode 0644). Files of an earlier fetch into the same
#              directory (listed in its .fetch-manifest) are removed first, so licence files
#              of another family or build never stay behind; other files are left alone.
#   lock-file  defaults to sources.lock next to this script (tests pass a modified copy)
#
# The build runs as `docker build --target <family>` of the recipe into a work directory
# (BuildKit caches it: the first build makes the toolchains, build/README.md "Build times",
# later ones take seconds). Nothing is written to dest-dir before the SHA-256 of ffmpeg and
# ffprobe and the Version:, Status: and Licence: lines of SOURCE.txt matched the lock entry.
# Whenever the script ends without success (a reported error, a command failing under set -e,
# a signal), the EXIT trap removes ffmpeg, ffprobe and partial copies from dest-dir, so a
# package can never carry binaries of an earlier or a rejected build.
#
# Exit codes: 0 success, 1 docker, copy or tool failure, 2 usage error, unknown family or
# malformed lock entry, 3 checksum or notice mismatch, 4 file missing in the build output.
#
# Needs: docker with BuildKit, sha256sum or shasum or openssl, mktemp. POSIX sh; runs on the
# host (Git Bash on Windows, Linux shells), not inside the SPK build: packaging/synology/
# Dockerfile takes the same recipe output from the image build.sh / build.ps1 make of it.
# Environment: FFMPEG_RECIPE_DIR (default: build/ next to this script).
set -eu
# Lock fields are split on purpose (word splitting) but never expanded as file name patterns.
set -f

readonly EXIT_FAILURE=1
readonly EXIT_USAGE=2
readonly EXIT_MISMATCH=3
readonly EXIT_MEMBER=4
readonly FIELD_COUNT=6
readonly SHA256_LENGTH=64
readonly LOG_PREFIX="fetch.sh:"
readonly MANIFEST=".fetch-manifest"
readonly NOTICE="SOURCE.txt"

# Git Bash would rewrite container paths in docker arguments into Windows paths.
MSYS_NO_PATHCONV=1
export MSYS_NO_PATHCONV

dest=""
work=""
ok=0
hasher=""

log() { printf '%s %s\n' "$LOG_PREFIX" "$*" >&2; }

# fail reports an error and exits with the given code; on_exit removes what must not stay.
fail() {
	code=$1
	shift
	log "error: $*"
	exit "$code"
}

# on_exit runs on every exit after the arguments were read: it deletes the work directory and,
# unless main finished (ok=1), the binaries a failed fetch must not leave behind in dest.
on_exit() {
	status=$?
	if [ "$ok" -ne 1 ]; then
		if [ -n "$dest" ] && [ -d "$dest" ]; then
			rm -f "$dest/ffmpeg" "$dest/ffprobe" "$dest/.ffmpeg.partial" "$dest/.ffprobe.partial"
		fi
		[ "$status" -ne 0 ] || status=$EXIT_FAILURE
	fi
	if [ -n "$work" ] && [ -d "$work" ]; then rm -rf "$work"; fi
	exit "$status"
}

usage() {
	printf 'usage: %s <x86_64|armv8|armv7> <dest-dir> [lock-file]\n' "$0" >&2
	exit "$EXIT_USAGE"
}

# require_tools picks the checksum tool before anything is built, because a failure inside a
# command substitution could not stop the script.
require_tools() {
	command -v docker >/dev/null 2>&1 || fail "$EXIT_FAILURE" "docker is not installed"
	if command -v sha256sum >/dev/null 2>&1; then
		hasher=sha256sum
	elif command -v shasum >/dev/null 2>&1; then
		hasher=shasum
	elif command -v openssl >/dev/null 2>&1; then
		hasher=openssl
	else
		fail "$EXIT_FAILURE" "no SHA-256 tool (sha256sum, shasum or openssl) is installed"
	fi
}

sha256_of() {
	case "$hasher" in
	sha256sum) sha256sum "$1" | cut -d ' ' -f 1 ;;
	shasum) shasum -a 256 "$1" | cut -d ' ' -f 1 ;;
	*) openssl dgst -sha256 -r "$1" | cut -d ' ' -f 1 ;;
	esac
}

# host_path converts a path for docker on Windows (C:/... form); cygpath exists only there.
host_path() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

check_sha256() {
	case "$1" in
	*[!0-9a-f]*) fail "$EXIT_USAGE" "lock field is not a lowercase SHA-256: $1" ;;
	esac
	[ "${#1}" -eq "$SHA256_LENGTH" ] || fail "$EXIT_USAGE" "lock field is not a SHA-256: $1"
}

# read_entry sets the entry_* variables from the single lock line of the family.
read_entry() {
	lock=$1
	wanted=$2
	matches=0
	while IFS= read -r line || [ -n "$line" ]; do
		case "$line" in '' | '#'*) continue ;; esac
		# Word splitting of the unquoted line is the lock format (blank-separated fields);
		# set -f keeps a field such as "*" from expanding to file names.
		# shellcheck disable=SC2086
		set -- $line
		[ "$1" = "$wanted" ] || continue
		[ "$#" -eq "$FIELD_COUNT" ] || fail "$EXIT_USAGE" "lock entry of $wanted has $# fields, expected $FIELD_COUNT"
		matches=$((matches + 1))
		entry_status=$2
		entry_version=$3
		entry_licence=$4
		entry_ffmpeg_sha256=$5
		entry_ffprobe_sha256=$6
	done <"$lock"
	[ "$matches" -eq 1 ] || fail "$EXIT_USAGE" "lock file has $matches entries for $wanted, expected 1"
}

validate_entry() {
	case "$entry_status" in
	selected) ;;
	interim) log "warning: the $family build is interim (not release-eligible, see README.md)" ;;
	*) fail "$EXIT_USAGE" "unknown status in lock entry of $family: $entry_status" ;;
	esac
	case "$entry_licence" in
	*nonfree*) log "warning: the $family build links non-free code and is not redistributable (see README.md)" ;;
	esac
	check_sha256 "$entry_ffmpeg_sha256"
	check_sha256 "$entry_ffprobe_sha256"
}

# build_family runs the recipe for the family into $work/out.
build_family() {
	log "building the $family FFmpeg with the recipe in $recipe (cached by BuildKit)"
	docker build --target "$family" --output "type=local,dest=$(host_path "$work/out")" "$(host_path "$recipe")" ||
		fail "$EXIT_FAILURE" "docker build of the $family FFmpeg failed"
	for name in ffmpeg ffprobe "$NOTICE"; do
		[ -f "$work/out/$name" ] || fail "$EXIT_MEMBER" "$name missing in the build output"
	done
}

# notice_value prints the single value of a "Field:" line of SOURCE.txt: everything after the
# colon, trimmed, so a word such as "nonfree" cannot hide behind the first one. A notice with no
# or several such lines is refused, since a second line could hide behind the first.
notice_value() {
	lines=$(grep -c "^$1:" "$work/out/$NOTICE" || true)
	[ "$lines" -eq 1 ] || fail "$EXIT_MISMATCH" "$NOTICE has $lines '$1:' lines, expected 1"
	sed -n "s/^$1:[[:space:]]*//p" "$work/out/$NOTICE" | sed 's/[[:space:]]*$//'
}

# verify_output compares the built binaries and the notice with the lock entry.
verify_output() {
	for tool in ffmpeg ffprobe; do
		if [ "$tool" = ffmpeg ]; then expected=$entry_ffmpeg_sha256; else expected=$entry_ffprobe_sha256; fi
		actual=$(sha256_of "$work/out/$tool")
		[ "$actual" = "$expected" ] ||
			fail "$EXIT_MISMATCH" "checksum mismatch for $tool: expected $expected, got $actual (recipe changed? README.md \"Updating a pin\")"
		log "sha256 ok: $tool $actual"
	done
	for field in Version Status Licence; do
		case "$field" in
		Version) expected=$entry_version ;;
		Status) expected=$entry_status ;;
		*) expected=$entry_licence ;;
		esac
		actual=$(notice_value "$field")
		[ "$actual" = "$expected" ] || fail "$EXIT_MISMATCH" "$NOTICE says $field: '$actual', the lock entry '$expected'"
	done
}

# remove_previous_fetch deletes the files an earlier fetch listed in the manifest; names with a
# slash or a leading dot are skipped, so a tampered manifest cannot reach outside dest.
remove_previous_fetch() {
	[ -f "$dest/$MANIFEST" ] || return 0
	while IFS= read -r name || [ -n "$name" ]; do
		case "$name" in '' | */* | .*) continue ;; esac
		rm -f "$dest/$name"
	done <"$dest/$MANIFEST"
	rm -f "$dest/$MANIFEST"
}

# copy_file copies one file to an exact target name with a mode. A directory in the way would
# make cp copy into it, so it is refused instead.
copy_file() {
	[ ! -d "$2" ] || fail "$EXIT_FAILURE" "a directory is in the way: $2"
	cp "$1" "$2" || fail "$EXIT_FAILURE" "cannot copy $(basename "$1") to $2"
	chmod "$3" "$2" || fail "$EXIT_FAILURE" "cannot set the mode of $2"
}

# install_into copies the verified files into dest; the binaries arrive under temporary names
# and are renamed last, so an interrupted copy leaves no executable ffmpeg or ffprobe. Every
# other regular file of the build output (licence texts, SOURCE.txt) is installed read-only.
install_into() {
	mkdir -p "$dest" || fail "$EXIT_FAILURE" "cannot create $dest"
	remove_previous_fetch
	for tool in ffmpeg ffprobe; do
		[ ! -d "$dest/$tool" ] || fail "$EXIT_FAILURE" "a directory is in the way: $dest/$tool"
		copy_file "$work/out/$tool" "$dest/.$tool.partial" 0755
	done
	installed="ffmpeg ffprobe"
	# The recipe writes plain file names without blanks, so splitting the list is safe.
	names=$(cd "$work/out" && ls) || fail "$EXIT_FAILURE" "cannot list the build output"
	for name in $names; do
		case "$name" in ffmpeg | ffprobe) continue ;; esac
		[ -f "$work/out/$name" ] || fail "$EXIT_FAILURE" "unexpected entry in the build output: $name"
		copy_file "$work/out/$name" "$dest/$name" 0644
		installed="$installed $name"
	done
	if ! mv -f "$dest/.ffmpeg.partial" "$dest/ffmpeg" || ! mv -f "$dest/.ffprobe.partial" "$dest/ffprobe"; then
		fail "$EXIT_FAILURE" "cannot install the binaries into $dest"
	fi
	# shellcheck disable=SC2086 # one name per line; names have no blanks
	printf '%s\n' $installed >"$dest/$MANIFEST" || fail "$EXIT_FAILURE" "cannot write $dest/$MANIFEST"
}

main() {
	[ "$#" -ge 2 ] && [ "$#" -le 3 ] || usage
	family=$1
	dest=$2
	lock=${3:-"$(dirname "$0")/sources.lock"}
	recipe=${FFMPEG_RECIPE_DIR:-"$(dirname "$0")/build"}
	trap on_exit EXIT
	trap 'fail "$EXIT_FAILURE" "interrupted"' HUP INT TERM
	case "$family" in
	x86_64 | armv8 | armv7) ;;
	*) fail "$EXIT_USAGE" "unknown family: $family (expected x86_64, armv8 or armv7)" ;;
	esac
	[ -r "$lock" ] || fail "$EXIT_USAGE" "lock file not readable: $lock"
	[ -f "$recipe/Dockerfile" ] || fail "$EXIT_USAGE" "recipe not found: $recipe/Dockerfile"
	read_entry "$lock" "$family"
	validate_entry
	require_tools

	work=$(mktemp -d "${TMPDIR:-/tmp}/ffmpeg-fetch.XXXXXX") || fail "$EXIT_FAILURE" "mktemp failed"
	build_family
	verify_output
	install_into
	ok=1
	log "installed FFmpeg $entry_version ($family, $entry_status) into $dest"
}

main "$@"

#!/bin/sh
# fetch.sh - download the pinned FFmpeg build of one DSM architecture family, verify it and
# extract ffmpeg, ffprobe, the licence files and a source notice (SPK-02, ADR 0026 section 4).
#
#   fetch.sh <family> <dest-dir> [lock-file]
#
#   family     x86_64, armv8 or armv7 (the DSM "arch" family values)
#   dest-dir   created if missing; receives ffmpeg and ffprobe (mode 0755), the licence
#              files and SOURCE.txt (mode 0644). Files of an earlier fetch into the same
#              directory (listed in its .fetch-manifest) are removed first, so licence files
#              of another family or build never stay behind; other files are left alone.
#   lock-file  defaults to sources.lock next to this script (tests pass a modified copy)
#
# Nothing is written to dest-dir before every download matched its SHA-256 and every member
# was found. Whenever the script ends without success (a reported error, a command failing
# under set -e, a signal), the EXIT trap removes ffmpeg, ffprobe and partial copies from
# dest-dir, so a build can never pack binaries of an earlier or a rejected fetch.
#
# Exit codes: 0 success, 1 download, extraction or tool failure, 2 usage error, unknown family
# or malformed lock entry, 3 checksum mismatch, 4 member missing in a verified archive.
#
# Needs: curl or wget, sha256sum or shasum or openssl, tar, xz (for .tar.xz) or gzip (for
# .tar.gz), mktemp. POSIX sh; runs outside the toolkit chroot (SPK-07 prebuild stage).
set -eu
# Lock fields are split on purpose (word splitting) but never expanded as file name patterns.
set -f

readonly EXIT_FAILURE=1
readonly EXIT_USAGE=2
readonly EXIT_CHECKSUM=3
readonly EXIT_MEMBER=4
readonly FIELD_COUNT=10
readonly LOG_PREFIX="fetch.sh:"
readonly MANIFEST=".fetch-manifest"

dest=""
work=""
ok=0
downloader=""
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

# require_tools picks the download and checksum tools before anything is fetched, because a
# failure inside a command substitution could not stop the script.
require_tools() {
	if command -v curl >/dev/null 2>&1; then
		downloader=curl
	elif command -v wget >/dev/null 2>&1; then
		downloader=wget
	else
		fail "$EXIT_FAILURE" "neither curl nor wget is installed"
	fi
	if command -v sha256sum >/dev/null 2>&1; then
		hasher=sha256sum
	elif command -v shasum >/dev/null 2>&1; then
		hasher=shasum
	elif command -v openssl >/dev/null 2>&1; then
		hasher=openssl
	else
		fail "$EXIT_FAILURE" "no SHA-256 tool (sha256sum, shasum or openssl) is installed"
	fi
	command -v tar >/dev/null 2>&1 || fail "$EXIT_FAILURE" "tar is not installed"
}

download() {
	if [ "$downloader" = curl ]; then
		curl -fsSL --proto '=https' --retry 3 -o "$2" "$1"
	else
		wget -q -O "$2" "$1"
	fi
}

sha256_of() {
	case "$hasher" in
	sha256sum) sha256sum "$1" | cut -d ' ' -f 1 ;;
	shasum) shasum -a 256 "$1" | cut -d ' ' -f 1 ;;
	*) openssl dgst -sha256 -r "$1" | cut -d ' ' -f 1 ;;
	esac
}

# fetch_verified downloads url to target and stops unless its SHA-256 is expected.
fetch_verified() {
	url=$1
	expected=$2
	target=$3
	log "downloading $url"
	download "$url" "$target" || fail "$EXIT_FAILURE" "download failed: $url"
	actual=$(sha256_of "$target")
	if [ "$actual" != "$expected" ]; then
		fail "$EXIT_CHECKSUM" "checksum mismatch for $url: expected $expected, got $actual"
	fi
	log "sha256 ok: $actual"
}

# extract_members unpacks the named members of a verified archive into a directory.
extract_members() {
	archive=$1
	into=$2
	shift 2
	case "$archive" in
	*.tar.xz) decompress="xz" ;;
	*.tar.gz | *.tgz) decompress="gzip" ;;
	*) fail "$EXIT_USAGE" "unsupported archive type: $archive" ;;
	esac
	command -v "$decompress" >/dev/null 2>&1 || fail "$EXIT_FAILURE" "$decompress is not installed"
	mkdir -p "$into" || fail "$EXIT_FAILURE" "cannot create $into"
	if ! "$decompress" -dc "$archive" | (cd "$into" && tar -xf - "$@"); then
		fail "$EXIT_MEMBER" "member missing in $(basename "$archive"): $*"
	fi
	for member in "$@"; do
		[ -f "$into/$member" ] || fail "$EXIT_MEMBER" "member missing in $(basename "$archive"): $member"
	done
}

# member_path joins an archive directory field and a file name ("." is the archive root).
member_path() {
	if [ "$1" = "." ]; then printf '%s' "$2"; else printf '%s/%s' "$1" "$2"; fi
}

# check_relative refuses empty and absolute paths, parent references and names that tar would
# read as an option.
check_relative() {
	case "$1" in
	'' | /* | .. | ../* | */.. | */../* | -*) fail "$EXIT_USAGE" "lock field is not a safe relative path: $1" ;;
	esac
}

check_sha256() {
	case "$1" in
	*[!0-9a-f]*) fail "$EXIT_USAGE" "lock field is not a lowercase SHA-256: $1" ;;
	esac
	[ "${#1}" -eq 64 ] || fail "$EXIT_USAGE" "lock field is not a SHA-256: $1"
}

check_https() {
	case "$1" in
	https://*) ;;
	*) fail "$EXIT_USAGE" "lock field is not an https URL: $1" ;;
	esac
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
		entry_archive_url=$5
		entry_archive_sha256=$6
		entry_archive_dir=$7
		entry_source_url=$8
		entry_source_sha256=$9
		shift 9
		entry_licence_files=$1
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
	check_https "$entry_archive_url"
	check_https "$entry_source_url"
	check_sha256 "$entry_archive_sha256"
	check_sha256 "$entry_source_sha256"
	check_relative "$entry_archive_dir"
}

# collect_licences extracts every licence file of the entry into $work/licences, fetching the
# source archive only when a licence lives there.
collect_licences() {
	archive=$1
	archive_licences=""
	source_licences=""
	old_ifs=$IFS
	IFS=,
	# shellcheck disable=SC2086 # split the comma-separated field (no globbing: set -f)
	set -- $entry_licence_files
	IFS=$old_ifs
	for spec in "$@"; do
		path=${spec#*:}
		check_relative "$path"
		case "$spec" in
		archive:?*) archive_licences="$archive_licences $path" ;;
		source:?*) source_licences="$source_licences $path" ;;
		*) fail "$EXIT_USAGE" "licence file entry needs archive: or source: and a path: $spec" ;;
		esac
	done
	[ -n "$archive_licences$source_licences" ] || fail "$EXIT_USAGE" "lock entry of $family names no licence file"
	mkdir -p "$work/licences" || fail "$EXIT_FAILURE" "cannot create $work/licences"
	if [ -n "$archive_licences" ]; then
		# shellcheck disable=SC2086 # paths without blanks (lock format), no globbing (set -f)
		extract_members "$archive" "$work/archive-licences" $archive_licences
		for path in $archive_licences; do
			cp "$work/archive-licences/$path" "$work/licences/" || fail "$EXIT_FAILURE" "cannot stage $path"
		done
	fi
	if [ -n "$source_licences" ]; then
		source_archive="$work/$(basename "$entry_source_url")"
		fetch_verified "$entry_source_url" "$entry_source_sha256" "$source_archive"
		# shellcheck disable=SC2086 # as above
		extract_members "$source_archive" "$work/source-licences" $source_licences
		for path in $source_licences; do
			cp "$work/source-licences/$path" "$work/licences/" || fail "$EXIT_FAILURE" "cannot stage $path"
		done
	fi
}

# write_source_notice records where the binaries come from; the status and licence lines let a
# package inspection spot an interim or non-free build.
write_source_notice() {
	ffmpeg_sha256=$(sha256_of "$work/bin/$(member_path "$entry_archive_dir" ffmpeg)")
	ffprobe_sha256=$(sha256_of "$work/bin/$(member_path "$entry_archive_dir" ffprobe)")
	cat >"$work/SOURCE.txt" <<EOF
FFmpeg build bundled for the DSM architecture family $family

Version:              $entry_version
Status:               $entry_status
Licence:              $entry_licence
Binary archive:       $entry_archive_url
Archive SHA-256:      $entry_archive_sha256
ffmpeg SHA-256:       $ffmpeg_sha256
ffprobe SHA-256:      $ffprobe_sha256
Corresponding source: $entry_source_url
Source SHA-256:       $entry_source_sha256

The corresponding source is also attached to every GitHub Release that carries the package.
EOF
	case "$entry_licence" in
	*nonfree*)
		cat >>"$work/SOURCE.txt" <<EOF

Licence note: this build links code that FFmpeg classifies as non-free (libfdk-aac). Under
FFmpeg's own licensing rules such a build is not redistributable.
EOF
		;;
	esac
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
# and are renamed last, so an interrupted copy leaves no executable ffmpeg or ffprobe.
install_into() {
	mkdir -p "$dest" || fail "$EXIT_FAILURE" "cannot create $dest"
	remove_previous_fetch
	for tool in ffmpeg ffprobe; do
		[ ! -d "$dest/$tool" ] || fail "$EXIT_FAILURE" "a directory is in the way: $dest/$tool"
		copy_file "$work/bin/$(member_path "$entry_archive_dir" "$tool")" "$dest/.$tool.partial" 0755
	done
	cp "$work/SOURCE.txt" "$work/licences/SOURCE.txt" || fail "$EXIT_FAILURE" "cannot stage SOURCE.txt"
	installed="ffmpeg ffprobe"
	# Staged licence names come from the lock (no blanks), so splitting the list is safe.
	names=$(cd "$work/licences" && ls) || fail "$EXIT_FAILURE" "cannot list the staged licence files"
	for name in $names; do
		copy_file "$work/licences/$name" "$dest/$name" 0644
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
	trap on_exit EXIT
	trap 'fail "$EXIT_FAILURE" "interrupted"' HUP INT TERM
	case "$family" in
	x86_64 | armv8 | armv7) ;;
	*) fail "$EXIT_USAGE" "unknown family: $family (expected x86_64, armv8 or armv7)" ;;
	esac
	[ -r "$lock" ] || fail "$EXIT_USAGE" "lock file not readable: $lock"
	read_entry "$lock" "$family"
	validate_entry
	require_tools

	work=$(mktemp -d "${TMPDIR:-/tmp}/ffmpeg-fetch.XXXXXX") || fail "$EXIT_FAILURE" "mktemp failed"

	archive="$work/$(basename "$entry_archive_url")"
	fetch_verified "$entry_archive_url" "$entry_archive_sha256" "$archive"
	extract_members "$archive" "$work/bin" \
		"$(member_path "$entry_archive_dir" ffmpeg)" "$(member_path "$entry_archive_dir" ffprobe)"
	collect_licences "$archive"
	write_source_notice
	install_into
	ok=1
	log "installed FFmpeg $entry_version ($family, $entry_status) into $dest"
}

main "$@"

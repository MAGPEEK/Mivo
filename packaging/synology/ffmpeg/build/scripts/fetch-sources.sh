#!/bin/sh
# fetch-sources.sh - download or verify the pinned inputs of the static FFmpeg build (SPK-15).
#
#   fetch-sources.sh download <list> <dest> <cache>
#       downloads every entry of <list> to <dest>/<stage>/<file>; a file already in <cache>
#       (named by its SHA-256) is reused, so a changed pin downloads only what changed
#   fetch-sources.sh verify <list> <dir>
#       checks that <dir>/<stage>/ holds exactly the listed files with the listed SHA-256 and
#       writes <dir>/<stage>/versions.env (NAME_VERSION and NAME_FILE of that stage's entries)
#
# The Dockerfile always runs "verify" before a build stage sees a source, also when the
# sources come from an unpacked source archive instead of the network (README.md).
#
# Exit codes: 0 success, 1 download or tool failure, 2 usage error or malformed list,
# 3 checksum mismatch, 4 missing or unlisted file.
set -eu

readonly EXIT_FAILURE=1
readonly EXIT_USAGE=2
readonly EXIT_CHECKSUM=3
readonly EXIT_FILES=4
readonly FIELD_COUNT=7
readonly SHA256_LENGTH=64
readonly STAGES="host toolchain libs ffmpeg"

work=""

log() { printf 'fetch-sources.sh: %s\n' "$*" >&2; }

fail() {
	code=$1
	shift
	log "error: $*"
	exit "$code"
}

cleanup() {
	if [ -n "$work" ]; then rm -rf "$work"; fi
}

usage() {
	printf 'usage: %s download <list> <dest> <cache> | verify <list> <dir>\n' "$0" >&2
	exit "$EXIT_USAGE"
}

sha256_of() { sha256sum "$1" | cut -d ' ' -f 1; }

# check_entry validates the fields of one list line (passed as positional parameters).
check_entry() {
	[ "$#" -eq "$FIELD_COUNT" ] || fail "$EXIT_USAGE" "entry '$1' has $# fields, expected $FIELD_COUNT"
	case " $STAGES " in *" $3 "*) ;; *) fail "$EXIT_USAGE" "entry '$1': unknown stage '$3'" ;; esac
	case "$4" in yes | no) ;; *) fail "$EXIT_USAGE" "entry '$1': linked must be yes or no, not '$4'" ;; esac
	case "$5" in */* | .* | '') fail "$EXIT_USAGE" "entry '$1': '$5' is not a plain file name" ;; esac
	case "$6" in *[!0-9a-f]*) fail "$EXIT_USAGE" "entry '$1': '$6' is not a lowercase SHA-256" ;; esac
	[ "${#6}" -eq "$SHA256_LENGTH" ] || fail "$EXIT_USAGE" "entry '$1': '$6' is not a SHA-256"
	case "$7" in https://*) ;; *) fail "$EXIT_USAGE" "entry '$1': '$7' is not an https URL" ;; esac
}

# read_entries writes the list without comments and empty lines to $work/entries, so the
# loops below run in the main shell and fail() ends the script.
read_entries() {
	work=$(mktemp -d)
	trap cleanup EXIT
	grep -v -e '^[[:space:]]*#' -e '^[[:space:]]*$' "$list" >"$work/entries" || true
	[ -s "$work/entries" ] || fail "$EXIT_USAGE" "the list has no entries: $list"
}

download_entry() {
	name=$1 stage=$3 file=$5 expected=$6 url=$7
	cached="$cache/$expected"
	if [ ! -f "$cached" ] || [ "$(sha256_of "$cached")" != "$expected" ]; then
		log "downloading $url"
		partial="$work/download"
		# --retry-all-errors: upstream servers (ffmpeg.org, ftp.gnu.org) occasionally refuse
		# connections for a while; the checksum below still decides what is accepted.
		curl -fsSL --proto '=https' --connect-timeout 30 --retry 5 --retry-delay 10 --retry-all-errors \
			-o "$partial" "$url" || fail "$EXIT_FAILURE" "download failed: $url"
		actual=$(sha256_of "$partial")
		[ "$actual" = "$expected" ] ||
			fail "$EXIT_CHECKSUM" "checksum mismatch for $name ($url): expected $expected, got $actual"
		mv -f "$partial" "$cached"
	fi
	mkdir -p "$dest/$stage"
	cp "$cached" "$dest/$stage/$file"
	log "ok $stage/$file"
}

download_all() {
	command -v curl >/dev/null 2>&1 || fail "$EXIT_FAILURE" "curl is not installed"
	mkdir -p "$dest" "$cache"
	read_entries
	while IFS= read -r line; do
		# Word splitting of the line is the list format (blank-separated fields).
		# shellcheck disable=SC2086
		set -- $line
		check_entry "$@"
		download_entry "$@"
	done <"$work/entries"
}

# variable_prefix turns a component name into the prefix of its versions.env variables.
variable_prefix() { printf '%s' "$1" | tr 'a-z-' 'A-Z_'; }

verify_entry() {
	name=$1 version=$2 stage=$3 file=$5 expected=$6
	path="$dir/$stage/$file"
	[ -f "$path" ] || fail "$EXIT_FILES" "missing source of $name: $stage/$file"
	actual=$(sha256_of "$path")
	[ "$actual" = "$expected" ] ||
		fail "$EXIT_CHECKSUM" "checksum mismatch for $stage/$file: expected $expected, got $actual"
	prefix=$(variable_prefix "$name")
	printf '%s_VERSION=%s\n%s_FILE=%s\n' "$prefix" "$version" "$prefix" "$file" >>"$dir/$stage/versions.env"
	printf '%s/%s\n' "$stage" "$file" >>"$work/listed"
	log "verified $stage/$file"
}

verify_all() {
	[ -d "$dir" ] || fail "$EXIT_FILES" "source directory not found: $dir"
	read_entries
	for stage in $STAGES; do
		mkdir -p "$dir/$stage"
		rm -f "$dir/$stage/versions.env"
	done
	: >"$work/listed"
	while IFS= read -r line; do
		# shellcheck disable=SC2086 # the list format, as above
		set -- $line
		check_entry "$@"
		verify_entry "$@"
	done <"$work/entries"
	# An unlisted file would enter the source archive without a pin.
	sort "$work/listed" >"$work/listed.sorted"
	(cd "$dir" && find . -type f ! -name versions.env | sed 's|^\./||' | sort) >"$work/present"
	unlisted=$(comm -13 "$work/listed.sorted" "$work/present")
	[ -z "$unlisted" ] || fail "$EXIT_FILES" "files not in the list: $unlisted"
}

main() {
	[ "$#" -ge 1 ] || usage
	mode=$1
	shift
	case "$mode" in
	download)
		[ "$#" -eq 3 ] || usage
		list=$1 dest=$2 cache=$3
		;;
	verify)
		[ "$#" -eq 2 ] || usage
		list=$1 dir=$2
		;;
	*) usage ;;
	esac
	[ -r "$list" ] || fail "$EXIT_USAGE" "list not readable: $list"
	if [ "$mode" = download ]; then download_all; else verify_all; fi
}

main "$@"

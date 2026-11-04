#!/bin/bash
# Fake `docker` for build-scripts.bats and fetch.bats: stands in for the steps of build.sh /
# build.ps1 and for the recipe build of ffmpeg/fetch.sh without Docker, so the tests reach the
# safety checks of the scripts in seconds. The tests copy it to <tmp>/bin/docker and put that
# directory first in PATH.
#
#   docker build -q -t <image> <dir>              toolkit image: prints a fake image ID
#   docker build -t <image> <dir>                 step 0, the FFmpeg recipe image: prints nothing
#   docker build --target <family> --output <csv> <recipe>
#                                                 fetch.sh: writes one family's recipe output
#   docker build ... --output <csv> <context>     step 1: writes a staged project per family and
#                                                 ffmpeg-source/ with the source archive
#   docker run ... -v <dir>:/project:ro -v <dir>:/output <image> <family>
#                                                 step 2: writes <ID>-<family>-<DSM>.spk
#
# Every call is appended to $FAKE_DOCKER_LOG, one line per call with the arguments separated by
# blanks. The fake FFmpeg binaries have a fixed content, "fake <tool> <family>\n" (the tests
# compute the pins of their sources.lock from it); the fake source archive holds "fake source\n".
# Behaviour switches (environment):
#   FAKE_FFMPEG_STATUS    Status of the staged SOURCE.txt (default selected)
#   FAKE_FFMPEG_LICENCE   Licence of the staged SOURCE.txt (default GPL-3.0-or-later)
#   FAKE_FFMPEG_VERSION   Version of the staged SOURCE.txt (default 1.0-static1)
#   FAKE_FFMPEG_CHANGED   family whose ffmpeg gets another content ("fake ffmpeg <family> changed")
#   FAKE_FFMPEG_OMIT      file name left out of every FFmpeg output directory
#   FAKE_SOURCE_MISMATCH  non-empty: the staged SOURCE.txt names another source archive hash
#   FAKE_NOTICE_EXTRA     extra line appended to every staged SOURCE.txt
#   FAKE_TOOLKIT_FAIL     family whose toolkit run fails like run.sh ("[spk-toolkit] failed:")
#   FAKE_ID               package name in the staged build.env (default demo)
set -euo pipefail

readonly FAMILIES=(x86_64 armv8 armv7)
readonly SOURCE_CONTENT="fake source"

printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:-/dev/null}"

sha256_of_text() { printf '%s\n' "$1" | sha256sum | cut -d' ' -f1; }

# spkmeta's mapping (internal/spkmeta/version.go): stage x 1000 + pre-release number x 10 +
# package revision, four digits.
dsm_version() {
	local version="$1" revision="$2" core="${1%%-*}" pre="" stage=9 number=0
	[ "$core" = "$version" ] || pre="${version#*-}"
	case "$pre" in
		"") ;;
		dev) stage=0 ;;
		alpha.*) stage=1 number="${pre#alpha.}" ;;
		beta.*) stage=2 number="${pre#beta.}" ;;
		rc.*) stage=3 number="${pre#rc.}" ;;
	esac
	printf '%s-%04d' "$core" "$((stage * 1000 + number * 10 + revision))"
}

# Turns the --output value type=local,dest=<path> or type=local,"dest=<path>" into <path>.
output_dir() {
	local field="${1#type=local,}"
	if [[ "$field" == \"*\" ]]; then
		field="${field#\"}"
		field="${field%\"}"
		field="${field//\"\"/\"}"
	fi
	printf '%s' "${field#dest=}"
}

ffmpeg_content() {
	local tool="$1" family="$2"
	if [ "$tool" = ffmpeg ] && [ "${FAKE_FFMPEG_CHANGED:-}" = "$family" ]; then
		printf 'fake ffmpeg %s changed' "$family"
	else
		printf 'fake %s %s' "$tool" "$family"
	fi
}

# write_ffmpeg_output <dir> <family> writes what the recipe's family target produces:
# ffmpeg, ffprobe, a licence text and SOURCE.txt.
write_ffmpeg_output() {
	local directory="$1" family="$2" tool source_sha
	mkdir -p "$directory"
	for tool in ffmpeg ffprobe; do
		[ "${FAKE_FFMPEG_OMIT:-}" = "$tool" ] || printf '%s\n' "$(ffmpeg_content "$tool" "$family")" >"$directory/$tool"
	done
	[ "${FAKE_FFMPEG_OMIT:-}" = COPYING.GPLv3 ] || echo "GNU GPL v3" >"$directory/COPYING.GPLv3"
	source_sha="$(sha256_of_text "$SOURCE_CONTENT")"
	[ -z "${FAKE_SOURCE_MISMATCH:-}" ] || source_sha="$(sha256_of_text "another archive")"
	[ "${FAKE_FFMPEG_OMIT:-}" = SOURCE.txt ] || {
		echo "FFmpeg build bundled for the DSM architecture family $family"
		echo
		echo "Version:              ${FAKE_FFMPEG_VERSION:-1.0-static1}"
		echo "Status:               ${FAKE_FFMPEG_STATUS:-selected}"
		echo "Licence:              ${FAKE_FFMPEG_LICENCE:-GPL-3.0-or-later}"
		echo "ffmpeg SHA-256:       $(sha256_of_text "$(ffmpeg_content ffmpeg "$family")")"
		echo "ffprobe SHA-256:      $(sha256_of_text "$(ffmpeg_content ffprobe "$family")")"
		echo "Corresponding source: ffmpeg-${FAKE_FFMPEG_VERSION:-1.0-static1}-source.tar"
		echo "Source SHA-256:       $source_sha"
		[ -z "${FAKE_NOTICE_EXTRA:-}" ] || echo "$FAKE_NOTICE_EXTRA"
	} >"$directory/SOURCE.txt"
}

stage_family() {
	local project="$1/$2"
	mkdir -p "$project/prebuilt/bin"
	{
		echo "ID='${FAKE_ID:-demo}'"
		echo "VERSION='$3'"
		echo "DSM_VERSION='$(dsm_version "$3" "$4")'"
		echo "ARCH='$2'"
	} >"$project/build.env"
	write_ffmpeg_output "$project/prebuilt/share/licenses/ffmpeg" "$2"
	for tool in ffmpeg ffprobe; do
		[ ! -f "$project/prebuilt/share/licenses/ffmpeg/$tool" ] || mv "$project/prebuilt/share/licenses/ffmpeg/$tool" "$project/prebuilt/bin/$tool"
	done
}

write_source_archive() {
	local directory="$1/ffmpeg-source" name="ffmpeg-${FAKE_FFMPEG_VERSION:-1.0-static1}-source.tar"
	mkdir -p "$directory"
	printf '%s\n' "$SOURCE_CONTENT" >"$directory/$name"
	printf '%s  %s\n' "$(sha256_of_text "$SOURCE_CONTENT")" "$name" >"$directory/$name.sha256"
}

fake_build() {
	local output="" version="0.0.0-dev" revision=0 family target="" quiet=no
	while [ "$#" -gt 0 ]; do
		case "$1" in
			-q) quiet=yes ;;
			--output) output="$(output_dir "$2")" && shift ;;
			--target) target="$2" && shift ;;
			--build-arg)
				case "$2" in
					VERSION=*) version="${2#VERSION=}" ;;
					PACKAGE_REVISION=*) revision="${2#PACKAGE_REVISION=}" ;;
				esac
				shift
				;;
		esac
		shift
	done
	if [ -z "$output" ]; then
		[ "$quiet" = no ] || echo "sha256:fake"
		return
	fi
	if [ -n "$target" ]; then
		write_ffmpeg_output "$output" "$target"
		return
	fi
	for family in "${FAMILIES[@]}"; do
		stage_family "$output" "$family" "$version" "$revision"
	done
	write_source_archive "$output"
}

fake_run() {
	local project="" output="" family="${!#}" id dsm
	while [ "$#" -gt 0 ]; do
		if [ "$1" = -v ]; then
			case "$2" in
				*:/project:ro) project="${2%:/project:ro}" ;;
				*:/output) output="${2%:/output}" ;;
			esac
			shift
		fi
		shift
	done
	if [ "${FAKE_TOOLKIT_FAIL:-}" = "$family" ]; then
		echo "[spk-toolkit] failed: fake toolkit failure for $family" >&2
		exit 1
	fi
	id="$(sed -n "s/^ID='\(.*\)'$/\1/p" "$project/build.env")"
	dsm="$(sed -n "s/^DSM_VERSION='\(.*\)'$/\1/p" "$project/build.env")"
	echo "fake SPK of $family" >"$output/$id-$family-$dsm.spk"
}

case "${1:-}" in
	build) shift && fake_build "$@" ;;
	run) shift && fake_run "$@" ;;
	*) echo "fake docker: unsupported command: $*" >&2 && exit 2 ;;
esac

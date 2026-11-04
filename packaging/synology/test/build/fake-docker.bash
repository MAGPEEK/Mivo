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
#   docker build -f <dir>/Dockerfile ... --output <csv> <context>
#                                                 step 1: writes what the FFmpeg image holds and
#                                                 fake server binaries to a temporary directory
#                                                 and runs the real <dir>/stage.sh on them, with
#                                                 the --build-arg values as its environment and
#                                                 the spkstage of PATH (the tests install
#                                                 fake-spkstage.bash); stage.sh's output becomes
#                                                 the --output directory, its failure step 1's
#   docker run ... -v <dir>:/project:ro -v <dir>:/output <image> <family>
#                                                 step 2: writes <ID>-<family>-<DSM>.spk
#
# Every call is appended to $FAKE_DOCKER_LOG, one line per call with the arguments separated by
# blanks. The fake FFmpeg binaries have a fixed content, "fake <tool> <family>\n" (the tests
# compute the pins of their sources.lock from it); the fake source archive holds "fake source\n".
# Behaviour switches (environment; fake-spkstage.bash has its own):
#   FAKE_FFMPEG_STATUS    Status of the staged SOURCE.txt (default selected)
#   FAKE_FFMPEG_LICENCE   Licence of the staged SOURCE.txt (default GPL-3.0-or-later)
#   FAKE_FFMPEG_VERSION   Version of the staged SOURCE.txt (default 1.0-static1)
#   FAKE_FFMPEG_CHANGED   family whose ffmpeg gets another content ("fake ffmpeg <family> changed")
#   FAKE_FFMPEG_OMIT      file name left out of every FFmpeg output directory
#   FAKE_SOURCE_MISMATCH  non-empty: the staged SOURCE.txt names another source archive hash
#   FAKE_NOTICE_EXTRA     extra line appended to every staged SOURCE.txt
#   FAKE_STAGE_TEST_MODE  non-empty: step 1 runs stage.sh with RELEASE=false whatever build.sh
#                         asked for, so the tests reach the checks build.sh makes after step 1
#   FAKE_TOOLKIT_FAIL     family whose toolkit run fails like run.sh ("[spk-toolkit] failed:")
#   FAKE_TOOLKIT_SILENT   family whose toolkit run succeeds but writes no SPK
set -euo pipefail

readonly FAMILIES=(x86_64 armv8 armv7)
readonly SOURCE_CONTENT="fake source"

printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:-/dev/null}"

sha256_of_text() { printf '%s\n' "$1" | sha256sum | cut -d' ' -f1; }

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

write_source_archive() {
	local directory="$1" name="ffmpeg-${FAKE_FFMPEG_VERSION:-1.0-static1}-source.tar"
	mkdir -p "$directory"
	printf '%s\n' "$SOURCE_CONTENT" >"$directory/$name"
	printf '%s  %s\n' "$(sha256_of_text "$SOURCE_CONTENT")" "$name" >"$directory/$name.sha256"
}

# stage_step <dockerfile> <output> <build-arg>...: step 1 through the real stage.sh.
stage_step() {
	local stage_script="${1%/*}/stage.sh" output="$2" prebuilt family assignment
	local -a environment=()
	shift 2
	for assignment in "$@"; do
		case "$assignment" in
			RELEASE=*) [ -z "${FAKE_STAGE_TEST_MODE:-}" ] || assignment=RELEASE=false ;;
		esac
		environment+=("$assignment")
	done
	prebuilt="$(mktemp -d)"
	for family in "${FAMILIES[@]}"; do
		write_ffmpeg_output "$prebuilt/ffmpeg/$family" "$family"
		mkdir -p "$prebuilt/server/$family"
		echo "fake server $family" >"$prebuilt/server/$family/server"
	done
	write_source_archive "$prebuilt/ffmpeg/source"
	# BuildKit exports nothing from a failed build.
	if ! env "${environment[@]}" bash "$stage_script" "$prebuilt" /src "$prebuilt/stage"; then
		rm -rf "$prebuilt"
		echo "ERROR: process \"bash packaging/synology/stage.sh\" did not complete successfully" >&2
		return 1
	fi
	mkdir -p "$output"
	cp -R "$prebuilt/stage/." "$output/"
	rm -rf "$prebuilt"
}

fake_build() {
	local output="" dockerfile="" target="" quiet=no
	local -a build_args=()
	while [ "$#" -gt 0 ]; do
		case "$1" in
			-q) quiet=yes ;;
			-f) dockerfile="$2" && shift ;;
			--output) output="$(output_dir "$2")" && shift ;;
			--target) target="$2" && shift ;;
			--build-arg) build_args+=("$2") && shift ;;
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
	stage_step "$dockerfile" "$output" "${build_args[@]}"
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
	[ "${FAKE_TOOLKIT_SILENT:-}" != "$family" ] || return 0
	id="$(sed -n "s/^ID='\(.*\)'$/\1/p" "$project/build.env")"
	dsm="$(sed -n "s/^DSM_VERSION='\(.*\)'$/\1/p" "$project/build.env")"
	echo "fake SPK of $family" >"$output/$id-$family-$dsm.spk"
}

case "${1:-}" in
	build) shift && fake_build "$@" ;;
	run) shift && fake_run "$@" ;;
	*) echo "fake docker: unsupported command: $*" >&2 && exit 2 ;;
esac

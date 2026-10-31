#!/bin/bash
# Fake `docker` for build-scripts.bats: stands in for the two steps of build.sh / build.ps1
# without Docker, so the tests reach the safety checks of the scripts in seconds. The tests copy
# it to <tmp>/bin/docker and put that directory first in PATH.
#
#   docker build -q -t <image> <dir>              toolkit image: prints a fake image ID
#   docker build ... --output <csv> <context>     step 1: writes a staged project per family
#   docker run ... -v <dir>:/project:ro -v <dir>:/output <image> <family>
#                                                 step 2: writes <ID>-<family>-<DSM>.spk
#
# Every call is appended to $FAKE_DOCKER_LOG, one line per call with the arguments separated by
# blanks. Behaviour switches (environment):
#   FAKE_FFMPEG_STATUS   Status of the staged SOURCE.txt (default selected)
#   FAKE_FFMPEG_LICENCE  Licence of the staged SOURCE.txt (default GPL-3.0-or-later)
#   FAKE_NOTICE_EXTRA    extra line appended to every staged SOURCE.txt
#   FAKE_TOOLKIT_FAIL    family whose toolkit run fails like run.sh ("[spk-toolkit] failed:")
#   FAKE_ID              package name in the staged build.env (default demo)
set -euo pipefail

readonly FAMILIES=(x86_64 armv8 armv7)

printf '%s\n' "$*" >>"${FAKE_DOCKER_LOG:-/dev/null}"

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

stage_family() {
	local project="$1/$2" licences
	licences="$project/prebuilt/share/licenses/ffmpeg"
	mkdir -p "$licences"
	{
		echo "ID='${FAKE_ID:-demo}'"
		echo "VERSION='$3'"
		echo "DSM_VERSION='$(dsm_version "$3" "$4")'"
		echo "ARCH='$2'"
	} >"$project/build.env"
	{
		echo "FFmpeg build bundled for the DSM architecture family $2"
		echo
		echo "Status:               ${FAKE_FFMPEG_STATUS:-selected}"
		echo "Licence:              ${FAKE_FFMPEG_LICENCE:-GPL-3.0-or-later}"
		[ -z "${FAKE_NOTICE_EXTRA:-}" ] || echo "$FAKE_NOTICE_EXTRA"
	} >"$licences/SOURCE.txt"
}

fake_build() {
	local output="" version="0.0.0-dev" revision=0 family
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--output) output="$(output_dir "$2")" && shift ;;
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
		echo "sha256:fake"
		return
	fi
	for family in "${FAMILIES[@]}"; do
		stage_family "$output" "$family" "$version" "$revision"
	done
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

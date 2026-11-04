#!/bin/bash
# Fake `spkstage` for stage.bats and, through fake-docker.bash, for build-scripts.bats: stages
# what ../../stage.sh needs to judge from a staged project, without the templates. The tests
# copy it to <tmp>/bin/spkstage and put that directory first in PATH.
#
#   spkstage -family F -version V -package-revision N -source-date-epoch E -binary B
#            -ffmpeg-dir D -repo R -out O
#
# Writes O/build.env (ID, VERSION, DSM_VERSION with spkmeta's mapping, ARCH), copies D/ffmpeg
# and D/ffprobe to O/prebuilt/bin and every other file of D to
# O/prebuilt/share/licenses/ffmpeg, as the real spkstage does. Behaviour switches (environment):
#   FAKE_ID                    package name in build.env (default demo)
#   FAKE_SPKSTAGE_WARN         family for which it prints "spkstage: warning: ..." on stderr
#   FAKE_SPKSTAGE_FAIL         family for which it fails
#   FAKE_REVISION_SKEW         non-empty: DSM_VERSION encodes package revision N+1
set -euo pipefail

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

family="" version="" revision="" ffmpeg_dir="" out=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		-family) family="$2" ;;
		-version) version="$2" ;;
		-package-revision) revision="$2" ;;
		-ffmpeg-dir) ffmpeg_dir="$2" ;;
		-out) out="$2" ;;
		-source-date-epoch | -binary | -repo) ;;
		*) echo "spkstage: unknown option $1" >&2 && exit 2 ;;
	esac
	shift 2
done
if [ "${FAKE_SPKSTAGE_FAIL:-}" = "$family" ]; then
	echo "spkstage: fake failure for $family" >&2
	exit 1
fi
[ -z "${FAKE_REVISION_SKEW:-}" ] || revision=$((revision + 1))
mkdir -p "$out/prebuilt/bin" "$out/prebuilt/share/licenses/ffmpeg"
{
	echo "ID='${FAKE_ID:-demo}'"
	echo "VERSION='$version'"
	echo "DSM_VERSION='$(dsm_version "$version" "$revision")'"
	echo "ARCH='$family'"
} >"$out/build.env"
for file in "$ffmpeg_dir"/*; do
	case "${file##*/}" in
		ffmpeg | ffprobe) cp "$file" "$out/prebuilt/bin/" ;;
		*) cp "$file" "$out/prebuilt/share/licenses/ffmpeg/" ;;
	esac
done
if [ "${FAKE_SPKSTAGE_WARN:-}" = "$family" ]; then
	echo "spkstage: warning: fake warning for $family" >&2
fi

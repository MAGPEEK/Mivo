#!/bin/bash
# stage.sh - the "stage" stage of step 1 (Dockerfile next to this file; ADR 0026 section 2,
# tasks SPK-07 and SPK-16): stages one Package Toolkit project per architecture family with
# spkstage, copies the FFmpeg corresponding source archive and writes build-info.txt.
#
#   stage.sh <prebuilt-dir> <repo-dir> <output-dir>
#
#   prebuilt-dir  server/<family>/server (the server binaries), ffmpeg/<family>/ (ffmpeg,
#                 ffprobe, licence texts, SOURCE.txt) and ffmpeg/source/ (the corresponding
#                 source archive *-source.tar and its .sha256)
#   repo-dir      the repository root spkstage reads the templates from (spkstage -repo)
#   output-dir    receives <family>/, ffmpeg-source/ and build-info.txt
#
# Environment: the build arguments of the Dockerfile, VERSION, REVISION, PACKAGE_REVISION,
# SOURCE_DATE_EPOCH, RELEASE (true or false), NODE_IMAGE, GO_IMAGE and FFMPEG_IMAGE; spkstage
# is taken from PATH. Tested with a fake spkstage by test/build/stage.bats.
#
# A release (RELEASE=true) refuses the default version 0.0.0-dev, an empty REVISION, an FFmpeg
# build whose SOURCE.txt is not "Status: selected" or whose licence is non-free, and any
# spkstage warning (interim builds, dynamically linked FFmpeg); BACKLOG "spk release", ADR 0026
# section 4. The check reads the SOURCE.txt that was staged, so it holds whatever supplies
# FFmpeg. In both modes a SOURCE.txt needs exactly one Status: and one Licence: line, so neither
# value can hide behind a second line. build.sh / build.ps1 check the staged result again.
set -euo pipefail

readonly FAMILIES=(x86_64 armv8 armv7)
readonly DEV_VERSION="0.0.0-dev"
readonly SOURCE_ARCHIVE_PATTERN="*-source.tar"
readonly NOTICE_PATH="prebuilt/share/licenses/ffmpeg/SOURCE.txt"

die() {
	echo "stage: $*" >&2
	exit 1
}

# warnings_file holds spkstage's stderr of the family being staged; the EXIT trap removes it
# however the stage ends, also when die leaves from inside stage_family.
warnings_file=""
remove_warnings_file() {
	[ -z "$warnings_file" ] || rm -f "$warnings_file"
}
trap remove_warnings_file EXIT

# notice_value <notice> <field> prints the single value of a "Field:" line, trimmed, so a word
# such as "nonfree" cannot hide behind the first one; a notice with no or several such lines
# is refused.
notice_value() {
	local count
	count="$(grep -c "^$2:" "$1" || true)"
	[ "$count" = 1 ] || die "${1#"$output_dir"/}: SOURCE.txt needs exactly one $2: line (has $count)"
	LC_ALL=C sed -n "s/^$2:[[:space:]]*//p" "$1" | LC_ALL=C sed 's/[[:space:]]*$//'
}

check_arguments() {
	case "${RELEASE:-}" in true | false) ;; *) die "RELEASE must be true or false, not '${RELEASE:-}'" ;; esac
	[ "$RELEASE" = false ] || [ "${VERSION:-}" != "$DEV_VERSION" ] || die "a release needs VERSION, not the default $DEV_VERSION"
	[ "$RELEASE" = false ] || [ -n "${REVISION:-}" ] || die "a release needs REVISION, the git commit it is built from"
}

write_build_info() {
	{
		echo "version=${VERSION:-}"
		echo "revision=${REVISION:-}"
		echo "package_revision=${PACKAGE_REVISION:-}"
		echo "source_date_epoch=${SOURCE_DATE_EPOCH:-}"
		echo "release=$RELEASE"
		echo "node_image=${NODE_IMAGE:-}"
		echo "go_image=${GO_IMAGE:-}"
		echo "ffmpeg_image=${FFMPEG_IMAGE:-}"
	} >"$output_dir/build-info.txt"
}

# copy_source_archive copies the one corresponding source archive with its .sha256; build.sh /
# build.ps1 move it next to the SPKs and check it against every SOURCE.txt.
copy_source_archive() {
	local archives=()
	mapfile -t archives < <(find "$prebuilt_dir/ffmpeg/source" -maxdepth 1 -type f -name "$SOURCE_ARCHIVE_PATTERN" 2>/dev/null)
	[ "${#archives[@]}" = 1 ] || die "the FFmpeg image holds ${#archives[@]} corresponding source archives in /source, expected 1"
	[ -f "${archives[0]}.sha256" ] || die "the FFmpeg image holds no ${archives[0]##*/}.sha256"
	mkdir -p "$output_dir/ffmpeg-source"
	cp "${archives[0]}" "${archives[0]}.sha256" "$output_dir/ffmpeg-source/"
}

# stage_family runs spkstage for family $1 and judges the staged FFmpeg.
stage_family() {
	local family="$1" warnings notice status licence
	remove_warnings_file
	warnings_file="$(mktemp)"
	warnings="$warnings_file"
	if ! spkstage -family "$family" -version "$VERSION" -package-revision "$PACKAGE_REVISION" \
		-source-date-epoch "$SOURCE_DATE_EPOCH" -binary "$prebuilt_dir/server/$family/server" \
		-ffmpeg-dir "$prebuilt_dir/ffmpeg/$family" -repo "$repo_dir" -out "$output_dir/$family" 2>"$warnings"; then
		cat "$warnings" >&2
		die "spkstage failed for $family"
	fi
	cat "$warnings" >&2
	notice="$output_dir/$family/$NOTICE_PATH"
	[ -f "$notice" ] || die "$family: spkstage staged no $NOTICE_PATH"
	status="$(notice_value "$notice" Status)" || exit 1
	licence="$(notice_value "$notice" Licence)" || exit 1
	echo "ffmpeg_$family=${status:-unknown} ${licence:-unknown}" >>"$output_dir/build-info.txt"
	if [ "$RELEASE" = true ]; then
		[ "$status" = selected ] || die "release refused: the FFmpeg build of $family has status '${status:-none}', not 'selected' (interim builds are for local test builds only)"
		case "$licence" in *nonfree*) die "release refused: the FFmpeg build of $family is non-free ($licence) and not redistributable" ;; esac
		! grep -q 'warning:' "$warnings" || die "release refused: spkstage warned for $family (see above)"
	elif [ "$status" != selected ]; then
		echo "stage: WARNING: $family uses an FFmpeg build with status '${status:-none}'; this is a test build, not for release or distribution" >&2
	fi
	remove_warnings_file
	warnings_file=""
}

main() {
	[ "$#" -eq 3 ] || {
		echo "usage: stage.sh <prebuilt-dir> <repo-dir> <output-dir>" >&2
		exit 2
	}
	prebuilt_dir="$1"
	repo_dir="$2"
	output_dir="$3"
	check_arguments
	mkdir -p "$output_dir"
	write_build_info
	copy_source_archive
	for family in "${FAMILIES[@]}"; do
		stage_family "$family"
	done
}

main "$@"

#!/bin/bash
# build.sh - builds the server SPKs of all three architecture families (x86_64, armv8, armv7)
# from a clean checkout with nothing but Docker and bash >= 4 (ADR 0026 section 2, task
# SPK-07). build.ps1 is the PowerShell twin with the same steps, options and output.
#
#   build.sh [options] [version]
#
#   version                  server version, e.g. 0.1.0-alpha.2 (default 0.0.0-dev); checked
#                            against the version scheme of spkmeta before anything runs
#   --release                release build: refuses interim or non-free FFmpeg builds, FFmpeg
#                            binaries that differ from the pins of ffmpeg/sources.lock, spkstage
#                            warnings, the default version and (with git) a dirty working tree
#                            (also SPK_BUILD_MODE=release; the default on CI hosts)
#   --test                   local test build (also SPK_BUILD_MODE=test; the default elsewhere):
#                            an interim or unpinned FFmpeg is packed with a loud warning and
#                            dist/ gets NOT-FOR-RELEASE.txt
#   --revision REV           git commit recorded in build-info.txt (default: git rev-parse HEAD,
#                            empty without git)
#   --package-revision N     packaging-only revision 0-9 (default 0)
#   --source-date-epoch N    file time of the staged projects (default: $SOURCE_DATE_EPOCH, else
#                            the commit time of HEAD, else 0)
#   --privilege MODE         privileges of the toolkit container (also SPK_PRIVILEGE):
#                              sys-admin           --cap-add SYS_ADMIN (default; the tested
#                                                  setting of ADR 0026 Notes)
#                              sys-admin-apparmor  --cap-add SYS_ADMIN --security-opt
#                                                  apparmor=unconfined (Linux hosts with
#                                                  AppArmor such as GitHub-hosted Ubuntu
#                                                  runners; not verified yet)
#                              privileged          --privileged (fallback only)
#   -h, --help               this text
#
# Empty values: an option given with an empty value is refused; an empty environment variable
# counts as unset. A host counts as CI when CI is set to anything but empty, "false" or "0".
#
# Environment: SPK_TOOLKIT_VOLUME (default spk-toolkit) and SPK_TOOLKIT_IMAGE (default
# spk-toolkit:dev) as in toolkit/test.sh; SPK_FFMPEG_IMAGE (default spk-ffmpeg-static:dev) names
# the local image of the FFmpeg recipe; NODE_IMAGE and GO_IMAGE are passed to the Dockerfile
# when set; BUILDKIT_PROGRESS=plain shows every log line of steps 0 and 1.
#
# Step 0 (docker build of ffmpeg/build, unprivileged): the own static FFmpeg of every family and
# its corresponding source archive -> the local image SPK_FFMPEG_IMAGE (cached by BuildKit; the
# first build takes 10-14 minutes, ffmpeg/build/README.md). Step 1 (docker build of Dockerfile,
# unprivileged): web app, server binaries, FFmpeg from that image and the staged projects ->
# dist/stage/<family>/, the source archive -> dist/. The staged ffmpeg and ffprobe must match
# the pins of ffmpeg/sources.lock, the archive the Source SHA-256 of every SOURCE.txt. Step 2
# (docker run of the toolkit image with CAP_SYS_ADMIN, one family after the other because the
# toolkit volume serialises runs): packs each project -> dist/<id>-<version>-<family>.spk
# (package revision N >= 1: <id>-<version>-r<N>-<family>.spk), then dist/SHA256SUMS over the
# SPKs and the source archive. The first error stops the build.
set -euo pipefail

readonly FAMILIES=(x86_64 armv8 armv7)
readonly DEV_VERSION="0.0.0-dev"
readonly SUMS_NAME="SHA256SUMS"
readonly MARKER_NAME="NOT-FOR-RELEASE.txt"
readonly NOTICE_PATH="prebuilt/share/licenses/ffmpeg/SOURCE.txt"
readonly FFMPEG_BIN_PATH="prebuilt/bin"
readonly SOURCE_STAGE_NAME="ffmpeg-source"
readonly SOURCE_ARCHIVE_SUFFIX="-source.tar"
readonly LOCK_FIELD_COUNT=6
readonly SHA256_PATTERN='^[0-9a-f]{64}$'
readonly PACKAGE_NAME_PATTERN='^[A-Za-z0-9][A-Za-z0-9._-]*$'
# The server versions spkmeta maps (internal/spkmeta/version.go): SemVer core without leading
# zeros, optionally -alpha.N, -beta.N or -rc.N (N 0-99), or the development version.
readonly VERSION_PATTERN='^(0\.0\.0-dev|(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})\.(0|[1-9][0-9]{0,9})(-(alpha|beta|rc)\.(0|[1-9][0-9]?))?)$'
# DSM version of spkmeta: <core>-<build>, the build number ends with the package revision.
readonly DSM_VERSION_PATTERN='^[0-9]+\.[0-9]+\.[0-9]+-[0-9]{4,}$'

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly here
repo_root="$(cd "$here/../.." && pwd)"
readonly repo_root
readonly DIST="$here/dist"
readonly STAGE="$DIST/stage"
readonly SOURCES_LOCK="$here/ffmpeg/sources.lock"
readonly FFMPEG_RECIPE="$here/ffmpeg/build"
readonly VOLUME="${SPK_TOOLKIT_VOLUME:-spk-toolkit}"
readonly IMAGE="${SPK_TOOLKIT_IMAGE:-spk-toolkit:dev}"
readonly FFMPEG_IMAGE="${SPK_FFMPEG_IMAGE:-spk-ffmpeg-static:dev}"

# Git Bash would rewrite container paths such as /project into Windows paths.
export MSYS_NO_PATHCONV=1

version="$DEV_VERSION"
mode=""
revision=""
revision_set=no
package_revision=0
source_date_epoch="${SOURCE_DATE_EPOCH:-}"
privilege="${SPK_PRIVILEGE:-sys-admin}"
privilege_options=()
interim_families=""
notice_status=""
notice_licence=""
notice_version=""
notice_source_sha256=""
source_archive=""
source_archive_sha256=""
# The sources.lock entry per family (read_lock): status, version, licence, ffmpeg and ffprobe SHA-256.
declare -A lock_status lock_version lock_licence lock_ffmpeg lock_ffprobe

log() { printf '[spk-build] %s\n' "$*" >&2; }
die() {
	printf '[spk-build] failed: %s\n' "$*" >&2
	exit 1
}

usage() {
	sed -n '2,/^set -euo pipefail$/{/^set /d;s/^# \{0,1\}//;p}' "${BASH_SOURCE[0]}"
	exit "${1:-0}"
}

# Docker on Windows needs C:/... paths for bind mounts; cygpath exists only in Git Bash.
host_path() {
	if command -v cygpath >/dev/null 2>&1; then cygpath -m "$1"; else printf '%s' "$1"; fi
}

# Runs git in the repository; fails without git. Git for Windows is a native program and needs
# the C:/... form, which MSYS_NO_PATHCONV would otherwise withhold.
repo_git() {
	command -v git >/dev/null 2>&1 || return 1
	git -C "$(host_path "$repo_root")" "$@" 2>/dev/null
}

require_value() { [ "$#" -ge 2 ] && [ -n "$2" ] || die "$1 needs a value"; }

parse_args() {
	local positional=()
	while [ "$#" -gt 0 ]; do
		case "$1" in
			--release) set_mode release ;;
			--test) set_mode test ;;
			--revision) require_value "$@" && revision="$2" && revision_set=yes && shift ;;
			--package-revision) require_value "$@" && package_revision="$2" && shift ;;
			--source-date-epoch) require_value "$@" && source_date_epoch="$2" && shift ;;
			--privilege) require_value "$@" && privilege="$2" && shift ;;
			-h | --help) usage 0 ;;
			-*) log "unknown option $1" && usage 2 ;;
			*) positional+=("$1") ;;
		esac
		shift
	done
	[ "${#positional[@]}" -le 1 ] || die "expected at most one version, got: ${positional[*]}"
	[ "${#positional[@]}" -eq 0 ] || version="${positional[0]}"
}

set_mode() {
	[ -z "$mode" ] || [ "$mode" = "$1" ] || die "--release and --test exclude each other"
	mode="$1"
}

is_ci_host() {
	local ci="${CI:-}"
	ci="${ci,,}"
	[ -n "$ci" ] && [ "$ci" != false ] && [ "$ci" != 0 ]
}

# A build is a release build when asked for, and by default on CI hosts: anything a CI run
# uploads can be handed out, so it must never carry an interim FFmpeg unless the workflow says
# --test explicitly.
resolve_mode() {
	[ -n "$mode" ] || mode="${SPK_BUILD_MODE:-}"
	if [ -z "$mode" ]; then
		if is_ci_host; then mode="release"; else mode="test"; fi
	fi
	case "$mode" in release | test) ;; *) die "SPK_BUILD_MODE must be release or test, not '$mode'" ;; esac
}

resolve_inputs() {
	[[ "$version" =~ $VERSION_PATTERN ]] ||
		die "version '$version' is not <major>.<minor>.<patch>[-alpha.N|-beta.N|-rc.N] or $DEV_VERSION"
	[[ "$package_revision" =~ ^[0-9]$ ]] || die "--package-revision must be 0-9, not '$package_revision'"
	if [ "$revision_set" = no ]; then revision="$(repo_git rev-parse HEAD)" || revision=""; fi
	[ -n "$source_date_epoch" ] || source_date_epoch="$(repo_git log -1 --format=%ct)" || source_date_epoch=""
	source_date_epoch="${source_date_epoch:-0}"
	[[ "$source_date_epoch" =~ ^[0-9]+$ ]] || die "SOURCE_DATE_EPOCH must be seconds since 1970, not '$source_date_epoch'"
	case "$privilege" in
		sys-admin) privilege_options=(--cap-add SYS_ADMIN) ;;
		sys-admin-apparmor) privilege_options=(--cap-add SYS_ADMIN --security-opt apparmor=unconfined) ;;
		privileged) privilege_options=(--privileged) ;;
		*) die "--privilege must be sys-admin, sys-admin-apparmor or privileged, not '$privilege'" ;;
	esac
}

# Reads the one sources.lock entry of every family (format in the lock's header) and lists the
# families whose entry is interim; a malformed lock stops the build before anything runs.
read_lock() {
	local line fields=() family
	[ -f "$SOURCES_LOCK" ] || die "$SOURCES_LOCK not found"
	while IFS= read -r line || [ -n "$line" ]; do
		[[ "$line" =~ ^[[:space:]]*(#|$) ]] && continue
		read -ra fields <<<"$line"
		family="${fields[0]}"
		[ "${#fields[@]}" -eq "$LOCK_FIELD_COUNT" ] ||
			die "ffmpeg/sources.lock: the entry of $family has ${#fields[@]} fields, expected $LOCK_FIELD_COUNT"
		[ -z "${lock_status[$family]:-}" ] || die "ffmpeg/sources.lock has two entries for $family"
		[[ "${fields[1]}" =~ ^(selected|interim)$ ]] || die "ffmpeg/sources.lock: unknown status '${fields[1]}' for $family"
		[[ "${fields[4]}" =~ $SHA256_PATTERN ]] && [[ "${fields[5]}" =~ $SHA256_PATTERN ]] ||
			die "ffmpeg/sources.lock: the entry of $family has no lowercase SHA-256 for ffmpeg and ffprobe"
		lock_status[$family]="${fields[1]}"
		lock_version[$family]="${fields[2]}"
		lock_licence[$family]="${fields[3]}"
		lock_ffmpeg[$family]="${fields[4]}"
		lock_ffprobe[$family]="${fields[5]}"
	done <"$SOURCES_LOCK"
	for family in "${FAMILIES[@]}"; do
		[ -n "${lock_status[$family]:-}" ] || die "ffmpeg/sources.lock has no entry for $family"
		[ "${lock_status[$family]}" != interim ] || interim_families="$interim_families $family"
	done
	interim_families="${interim_families# }"
}

interim_banner() {
	printf '%s\n' \
		"==========================================================================================" \
		"WARNING: TEST BUILD WITH INTERIM FFMPEG - NOT FOR RELEASE OR DISTRIBUTION" \
		"  ffmpeg/sources.lock has interim entries for: $interim_families" \
		"  Interim builds miss criteria of ADR 0026 section 4 (static, licence, kernel); the" \
		"  release build (--release) refuses them." \
		"==========================================================================================" >&2
}

# A release must be rebuildable from its commit, so uncommitted or untracked files are refused
# (dist/ is ignored by packaging/synology/.gitignore). Without git or outside a checkout there
# is nothing to compare against.
check_clean_tree() {
	local changes
	repo_git rev-parse --is-inside-work-tree >/dev/null || return 0
	changes="$(repo_git status --porcelain)" || die "git status failed in $repo_root"
	[ -z "$changes" ] || die "release refused: the working tree has uncommitted changes:
$(head -n 10 <<<"$changes")"
}

check_release_inputs() {
	if [ "$mode" = release ]; then
		[ "$version" != "$DEV_VERSION" ] || die "a release build needs a version, not the default $DEV_VERSION"
		[ -z "$interim_families" ] ||
			die "release refused: ffmpeg/sources.lock has interim FFmpeg entries for $interim_families"
		check_clean_tree
	elif [ -n "$interim_families" ]; then
		interim_banner
	fi
}

clean_dist() {
	rm -rf "$STAGE"
	rm -f "$DIST"/*.spk "$DIST"/*"$SOURCE_ARCHIVE_SUFFIX" "$DIST/$SUMS_NAME" "$DIST/$MARKER_NAME"
	mkdir -p "$DIST"
}

# Step 0: the FFmpeg recipe's "all" target as a local image, which step 1 copies from. BuildKit
# keys every stage by its inputs, so an unchanged recipe is loaded from the cache in seconds.
build_ffmpeg_image() {
	local started=$SECONDS
	log "step 0: FFmpeg (own static build of ffmpeg/build, cached by BuildKit) -> $FFMPEG_IMAGE"
	docker build -t "$FFMPEG_IMAGE" "$(host_path "$FFMPEG_RECIPE")" || die "step 0 (FFmpeg recipe build) failed"
	log "step 0 finished in $((SECONDS - started))s"
}

build_args() {
	local release=false name
	[ "$mode" = test ] || release=true
	printf '%s\n' --build-arg "VERSION=$version" --build-arg "REVISION=$revision" \
		--build-arg "PACKAGE_REVISION=$package_revision" \
		--build-arg "SOURCE_DATE_EPOCH=$source_date_epoch" --build-arg "RELEASE=$release" \
		--build-arg "FFMPEG_IMAGE=$FFMPEG_IMAGE"
	for name in NODE_IMAGE GO_IMAGE; do
		[ -z "${!name:-}" ] || printf '%s\n' --build-arg "$name=${!name}"
	done
}

stage_projects() {
	local args=() dest started=$SECONDS
	mapfile -t args < <(build_args)
	# --output is a CSV list: the quoted field keeps a comma in the path from splitting it.
	dest="$(host_path "$STAGE")"
	log "step 1: prebuild and stage ($mode build, version $version)"
	docker build -f "$(host_path "$here/Dockerfile")" "${args[@]}" \
		--output "type=local,\"dest=${dest//\"/\"\"}\"" "$(host_path "$repo_root")" ||
		die "step 1 (docker build) failed"
	log "step 1 finished in $((SECONDS - started))s"
}

# Prints one value of a staged build.env (lines NAME='literal', checked by spkstage).
build_env_value() {
	sed -n "s/^$2='\([^']*\)'\$/\1/p" "$1/build.env"
}

# Prints the single value of a "Field:" line of a staged FFmpeg SOURCE.txt: everything after the
# colon, trimmed, so a word such as "nonfree" cannot hide behind the first one. A notice with no
# or several such lines is refused, since a second line could hide behind the first.
notice_value() {
	local notice="$1" field="$2" lines value
	lines="$(grep -c "^$field:" "$notice" || true)"
	[ "$lines" = 1 ] || die "$notice needs exactly one $field: line (has $lines)"
	value="$(sed -n "s/^$field:[[:space:]]*//p" "$notice" | sed 's/[[:space:]]*$//')"
	[ -n "$value" ] || die "$notice has an empty $field: value"
	printf '%s' "$value"
}

# Sets the notice_* variables from a staged FFmpeg SOURCE.txt.
read_notice() {
	local notice="$1"
	[ -f "$notice" ] || die "$notice missing in the staged project"
	notice_status="$(notice_value "$notice" Status)"
	notice_licence="$(notice_value "$notice" Licence)"
	notice_version="$(notice_value "$notice" Version)"
	notice_source_sha256="$(notice_value "$notice" "Source SHA-256")"
}

# Prints the differences between a family's staged FFmpeg and its sources.lock entry: the SHA-256
# of the staged ffmpeg and ffprobe and the version and licence of SOURCE.txt must be the pinned
# ones, so an SPK carries exactly the binaries that were verified under emulation. Runs in a
# command substitution, so it only prints; check_staged has already required the files (a die
# here would end the subshell, not the build).
pin_differences() {
	local family="$1" project="$STAGE/$1" tool hash expected
	for tool in ffmpeg ffprobe; do
		hash="$(sha256_of "$project/$FFMPEG_BIN_PATH/$tool")"
		if [ "$tool" = ffmpeg ]; then expected="${lock_ffmpeg[$family]}"; else expected="${lock_ffprobe[$family]}"; fi
		[ "$hash" = "$expected" ] || printf ' %s' "$tool"
	done
	[ "$notice_version" = "${lock_version[$family]}" ] || printf ' version %s (pinned %s)' "$notice_version" "${lock_version[$family]}"
	[ "$notice_licence" = "${lock_licence[$family]}" ] || printf ' licence %s (pinned %s)' "$notice_licence" "${lock_licence[$family]}"
}

# Reads what was staged, not what sources.lock says: the release check holds for any stage
# that supplies FFmpeg. Test builds list the families in NOT-FOR-RELEASE.txt.
check_staged() {
	local family project tool not_eligible="" unpinned="" differences
	find_source_archive
	for family in "${FAMILIES[@]}"; do
		project="$STAGE/$family"
		[ -f "$project/build.env" ] || die "step 1 staged no project for $family ($project/build.env)"
		for tool in ffmpeg ffprobe; do
			[ -f "$project/$FFMPEG_BIN_PATH/$tool" ] || die "$project/$FFMPEG_BIN_PATH/$tool missing in the staged project"
		done
		read_notice "$project/$NOTICE_PATH"
		if [ "$notice_status" != selected ] || [[ "$notice_licence" == *nonfree* ]]; then
			not_eligible="$not_eligible $family ($notice_status, $notice_licence)"
		fi
		differences="$(pin_differences "$family")"
		[ -z "$differences" ] || unpinned="$unpinned $family (${differences# })"
		[ "$source_archive_sha256" = "$notice_source_sha256" ] ||
			die "the staged source archive $source_archive ($source_archive_sha256) is not the one SOURCE.txt of $family names ($notice_source_sha256)"
	done
	if [ "$mode" = release ]; then
		[ -z "$not_eligible" ] || die "release refused: FFmpeg builds not release-eligible:$not_eligible"
		[ -z "$unpinned" ] || die "release refused: FFmpeg builds differ from the pins of ffmpeg/sources.lock:$unpinned"
		return
	fi
	[ -z "$unpinned" ] || log "WARNING: FFmpeg builds differ from the pins of ffmpeg/sources.lock (recipe changed? update the pins on purpose):$unpinned"
	write_marker "$not_eligible" "$unpinned"
}

# Sets source_archive and its SHA-256: step 1 stages exactly one corresponding source archive,
# which must be the one every SOURCE.txt names (Source SHA-256), so the archive next to the
# SPKs is the source of exactly their binaries.
find_source_archive() {
	local archives=()
	mapfile -t archives < <(cd "$STAGE/$SOURCE_STAGE_NAME" 2>/dev/null && LC_ALL=C ls -1 -- *"$SOURCE_ARCHIVE_SUFFIX" 2>/dev/null)
	[ "${#archives[@]}" -eq 1 ] || die "step 1 staged ${#archives[@]} FFmpeg source archives in $STAGE/$SOURCE_STAGE_NAME, expected 1"
	source_archive="${archives[0]}"
	source_archive_sha256="$(sha256_of "$STAGE/$SOURCE_STAGE_NAME/$source_archive")"
}

write_marker() {
	{
		echo "These SPKs come from a TEST BUILD (build.sh/build.ps1 without --release / -Release)."
		echo "Do not publish, upload or hand them out; build again with --release for that."
		[ -z "$1" ] || echo "FFmpeg builds that are not release-eligible (status, licence):$1"
		[ -z "$2" ] || echo "FFmpeg builds that differ from the pins of ffmpeg/sources.lock:$2"
	} >"$DIST/$MARKER_NAME"
}

# Moves the corresponding source archive next to the SPKs (its .sha256 sidecar stays in
# stage/; SHA256SUMS covers the archive).
collect_source_archive() {
	mv -f "$STAGE/$SOURCE_STAGE_NAME/$source_archive" "$DIST/$source_archive" || die "cannot move $source_archive to $DIST"
	log "FFmpeg corresponding source: $source_archive"
}

# Prints the release file name of a packed project: <id>-<version>-<family>.spk, with -r<N>
# before the family for package revision N >= 1. N is read back from the staged DSM version
# (spkmeta: the last digit of the build number) and must be the revision that was asked for.
spk_name() {
	local family="$1" project="$STAGE/$1" id server_version dsm_version staged_revision suffix=""
	id="$(build_env_value "$project" ID)"
	server_version="$(build_env_value "$project" VERSION)"
	dsm_version="$(build_env_value "$project" DSM_VERSION)"
	[[ "$id" =~ $PACKAGE_NAME_PATTERN ]] || die "staged build.env of $family has no valid ID ('$id')"
	[ "$server_version" = "$version" ] || die "staged build.env of $family has VERSION '$server_version', not '$version'"
	[[ "$dsm_version" =~ $DSM_VERSION_PATTERN ]] || die "staged build.env of $family has no valid DSM_VERSION ('$dsm_version')"
	staged_revision="${dsm_version: -1}"
	[ "$staged_revision" = "$package_revision" ] ||
		die "staged DSM_VERSION $dsm_version of $family encodes package revision $staged_revision, not $package_revision"
	[ "$staged_revision" = 0 ] || suffix="-r$staged_revision"
	printf '%s %s %s\n' "$id-$family-$dsm_version.spk" "$id-$server_version$suffix-$family.spk" "$id"
}

# Packs one family with the toolkit and renames the toolkit's <id>-<family>-<dsm>.spk.
pack_family() {
	local family="$1" packed target id started=$SECONDS
	read -r packed target id <<<"$(spk_name "$family")"
	[ -n "$id" ] || die "cannot name the SPK of $family"
	log "step 2: packing $family with the toolkit"
	docker run --rm "${privilege_options[@]}" -v "$VOLUME:/toolkit" \
		-v "$(host_path "$STAGE/$family"):/project:ro" -v "$(host_path "$DIST"):/output" \
		"$IMAGE" "$family" || die "toolkit run for $family failed"
	[ -f "$DIST/$packed" ] || die "the toolkit did not write $packed"
	mv -f "$DIST/$packed" "$DIST/$target"
	log "wrote $target in $((SECONDS - started))s"
}

sha256_of() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum <"$1" | cut -d' ' -f1
	else
		shasum -a 256 <"$1" | cut -d' ' -f1
	fi
}

# SHA256SUMS in the format of `sha256sum` (hash, two blanks, name) over the SPKs and the FFmpeg
# source archive, sorted by name, LF endings.
write_sums() {
	local name hash names=() spks=() lines=""
	mapfile -t spks < <(cd "$DIST" && LC_ALL=C ls -1 -- *.spk)
	[ "${#spks[@]}" -eq "${#FAMILIES[@]}" ] || die "expected ${#FAMILIES[@]} SPKs in $DIST, found ${#spks[@]}"
	mapfile -t names < <(cd "$DIST" && LC_ALL=C ls -1 -- *.spk *"$SOURCE_ARCHIVE_SUFFIX")
	for name in "${names[@]}"; do
		hash="$(sha256_of "$DIST/$name")"
		[[ "$hash" =~ $SHA256_PATTERN ]] || die "cannot compute the SHA-256 of $name"
		lines="$lines$hash  $name"$'\n'
	done
	printf '%s' "$lines" >"$DIST/$SUMS_NAME"
	cat "$DIST/$SUMS_NAME" >&2
}

main() {
	local started=$SECONDS family
	parse_args "$@"
	resolve_mode
	resolve_inputs
	read_lock
	check_release_inputs
	command -v docker >/dev/null 2>&1 || die "docker not found"
	clean_dist
	log "building the toolkit image $IMAGE"
	docker build -q -t "$IMAGE" "$(host_path "$here/toolkit")" >/dev/null || die "toolkit image build failed"
	build_ffmpeg_image
	stage_projects
	check_staged
	collect_source_archive
	for family in "${FAMILIES[@]}"; do
		pack_family "$family"
	done
	write_sums
	log "done in $((SECONDS - started))s ($mode build): $(host_path "$DIST")"
	[ "$mode" = release ] || [ -z "$interim_families" ] || interim_banner
}

main "$@"

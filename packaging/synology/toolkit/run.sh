#!/bin/bash
# Entrypoint of the toolkit container (ADR 0026 section 2, task SPK-04): packs one staged
# project into one SPK for one architecture family with Synology's Package Toolkit
# pkgscripts-ng.
#
#   spk-toolkit-run <family> [project-dir] [output-dir]
#
#   family       x86_64 | armv8 | armv7
#   project-dir  staged toolkit project (build.env, INFO.sh, SynoBuildConf/, ...), default /project
#   output-dir   receives the SPK, default /output
#
# Steps: clone pkgscripts-ng at the pinned commit into the toolkit volume (once), download the
# platform tarballs and run EnvDeploy (once per platform), copy the project to source/<ID>,
# run PkgCreate.py and copy the resulting SPK to the output directory. Everything heavy stays
# in $TOOLKIT_ROOT, a named Docker volume. README.md next to this file explains the details.
set -euo pipefail

readonly PKGSCRIPTS_REPOSITORY="https://github.com/SynologyOpenSource/pkgscripts-ng.git"
readonly PKGSCRIPTS_BRANCH="DSM7.2"
# "Release for DSM7.2.2", 2025-05-28: the head of branch DSM7.2 when SPK-04 was written.
readonly PKGSCRIPTS_COMMIT="e4a8d54209a31cef21b7636ad58b1b0e86b5b201"
# Environment version passed to EnvDeploy and PkgCreate.py (-v); the toolkit names the
# tarballs and chroots after it (ds.<platform>-7.2).
readonly DSM_TOOLKIT_VERSION="7.2"

TOOLKIT_ROOT="${TOOLKIT_ROOT:-/toolkit}"
readonly TOOLKIT_ROOT
readonly PKGSCRIPTS_DIR="$TOOLKIT_ROOT/pkgscripts-ng"
readonly TARBALL_DIR="$TOOLKIT_ROOT/toolkit_tarballs"
readonly STATE_DIR="$TOOLKIT_ROOT/.state"
readonly RESULT_DIR="$TOOLKIT_ROOT/result_spk"
TARBALL_LOCK="${TARBALL_LOCK:-/usr/local/share/spk-toolkit/tarballs.sha256}"
readonly TARBALL_LOCK

log() { printf '[spk-toolkit] %s\n' "$*" >&2; }
die() { printf '[spk-toolkit] failed: %s\n' "$*" >&2; exit 1; }

usage() {
	cat >&2 <<'EOF'
usage: spk-toolkit-run <x86_64|armv8|armv7> [project-dir] [output-dir]
  project-dir defaults to /project, output-dir to /output.
  TOOLKIT_PLATFORM=<platform> overrides the platform of the family (for trying candidates).
EOF
	exit 2
}

# The representative DSM 7.2 platform per family (README "Platforms"). Any platform of the
# family produces an SPK for the whole family; the INFO arch value comes from build.env.
default_platform() {
	case "$1" in
		x86_64) echo "v1000" ;;
		armv8) echo "rtd1619b" ;;
		armv7) echo "alpine" ;;
		*) return 1 ;;
	esac
}

# Prints the seconds since the given SECONDS value, for the timing lines of the README.
elapsed() { echo "$((SECONDS - $1))s"; }

ensure_toolkit() {
	if [ ! -d "$PKGSCRIPTS_DIR/.git" ]; then
		log "cloning pkgscripts-ng ($PKGSCRIPTS_BRANCH) into the toolkit volume"
		git clone --quiet --branch "$PKGSCRIPTS_BRANCH" --single-branch \
			"$PKGSCRIPTS_REPOSITORY" "$PKGSCRIPTS_DIR"
	fi
	if [ "$(git -C "$PKGSCRIPTS_DIR" rev-parse HEAD)" != "$PKGSCRIPTS_COMMIT" ]; then
		if ! git -C "$PKGSCRIPTS_DIR" cat-file -e "$PKGSCRIPTS_COMMIT^{commit}" 2>/dev/null; then
			git -C "$PKGSCRIPTS_DIR" fetch --quiet origin "$PKGSCRIPTS_BRANCH"
		fi
		git -C "$PKGSCRIPTS_DIR" checkout --quiet --force --detach "$PKGSCRIPTS_COMMIT"
	fi
	# Edits inside the clone would change what the toolkit does without changing the commit.
	if ! git -C "$PKGSCRIPTS_DIR" diff --quiet HEAD; then
		log "local changes in the pkgscripts-ng clone, restoring the pinned commit"
		git -C "$PKGSCRIPTS_DIR" checkout --quiet --force --detach "$PKGSCRIPTS_COMMIT"
	fi
	git -C "$PKGSCRIPTS_DIR" merge-base --is-ancestor "$PKGSCRIPTS_COMMIT" \
		"origin/$PKGSCRIPTS_BRANCH" || die "commit $PKGSCRIPTS_COMMIT is not on $PKGSCRIPTS_BRANCH"
	log "pkgscripts-ng at $PKGSCRIPTS_COMMIT"
}

# EnvDeploy always downloads the 1.1 GiB base tarball together with the platform tarballs and
# cannot skip files that are already there (EnvDeploy:65-69, pkgdeploy.py:191-194). The
# toolkit's own downloader is therefore called for the missing files only; EnvDeploy -D then
# deploys from the cache. A marker per download makes an interrupted download count as missing.
download_tarballs() {
	local platform="$1" need_base=no need_platform=no started=$SECONDS
	[ -f "$STATE_DIR/base_env-$DSM_TOOLKIT_VERSION.downloaded" ] || need_base=yes
	[ -f "$STATE_DIR/ds.$platform-$DSM_TOOLKIT_VERSION.downloaded" ] || need_platform=yes
	if [ "$need_base" = no ] && [ "$need_platform" = no ]; then
		log "tarballs of $platform cached, nothing to download"
		return
	fi
	log "downloading tarballs (base: $need_base, $platform: $need_platform)"
	python3 - "$PKGSCRIPTS_DIR/include/python" "$DSM_TOOLKIT_VERSION" "$platform" \
		"$need_base" "$need_platform" <<'PYTHON'
import sys
sys.path.insert(0, sys.argv[1])
from pkgdeploy import ToolkitDownloader, DownloadDir
from toolkit import TarballManager

version, platform, need_base, need_platform = sys.argv[2:6]
downloader = ToolkitDownloader(version, TarballManager(version, DownloadDir))
if need_base == "yes":
    downloader.download_base_tarball(quiet=True)
if need_platform == "yes":
    downloader.download_platform_tarball(platform, quiet=True)
PYTHON
	mkdir -p "$STATE_DIR"
	touch "$STATE_DIR/base_env-$DSM_TOOLKIT_VERSION.downloaded" \
		"$STATE_DIR/ds.$platform-$DSM_TOOLKIT_VERSION.downloaded"
	log "download finished in $(elapsed "$started"); tarballs: $(du -sxh "$TARBALL_DIR" | cut -f1)"
}

# Checks the tarballs of a platform against tarballs.sha256. A mismatching file is deleted
# together with its download marker, so the next run downloads it again.
verify_tarballs() {
	local platform="$1" file expected actual marker started=$SECONDS
	[ -f "$TARBALL_LOCK" ] || die "checksum file $TARBALL_LOCK missing"
	for file in "base_env-$DSM_TOOLKIT_VERSION.txz" "ds.$platform-$DSM_TOOLKIT_VERSION.dev.txz" \
		"ds.$platform-$DSM_TOOLKIT_VERSION.env.txz"; do
		expected="$(awk -v name="$file" '$1 !~ /^#/ && $2 == name { print $1 }' "$TARBALL_LOCK")"
		[ -n "$expected" ] || die "no pinned SHA-256 for $file in tarballs.sha256; check the download and add it"
		actual="$(sha256sum "$TARBALL_DIR/$file" | cut -d' ' -f1)"
		if [ "$actual" != "$expected" ]; then
			marker="$STATE_DIR/ds.$platform-$DSM_TOOLKIT_VERSION.downloaded"
			[ "${file#base_env}" = "$file" ] || marker="$STATE_DIR/base_env-$DSM_TOOLKIT_VERSION.downloaded"
			rm -f "$TARBALL_DIR/$file" "$marker"
			die "SHA-256 of $file is $actual, pinned $expected; file deleted (README \"Pinned tarballs\")"
		fi
	done
	log "tarballs of $platform match tarballs.sha256 ($(elapsed "$started"))"
}

ensure_environment() {
	local platform="$1" chroot="$2" marker deployed_with started
	marker="$STATE_DIR/ds.$platform-$DSM_TOOLKIT_VERSION.deployed"
	if [ -f "$marker" ] && [ -f "$chroot/PkgVersion" ]; then
		deployed_with="$(cut -d' ' -f1 "$marker")"
		if [ "$deployed_with" = "$PKGSCRIPTS_COMMIT" ]; then
			log "environment ds.$platform-$DSM_TOOLKIT_VERSION deployed, EnvDeploy skipped"
			return
		fi
		log "environment was deployed by toolkit commit $deployed_with; deploying again"
	fi
	download_tarballs "$platform"
	verify_tarballs "$platform"
	started=$SECONDS
	log "EnvDeploy -v $DSM_TOOLKIT_VERSION -p $platform -D"
	(cd "$PKGSCRIPTS_DIR" && ./EnvDeploy -v "$DSM_TOOLKIT_VERSION" -p "$platform" -D -q)
	[ -f "$chroot/PkgVersion" ] || die "EnvDeploy did not create $chroot"
	printf '%s %s\n' "$PKGSCRIPTS_COMMIT" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >"$marker"
	log "deployed in $(elapsed "$started"); environment: $(du -sxh "$chroot" | cut -f1)"
}

# build.env is sourced by this script (as root, outside the chroot), by INFO.sh and by
# SynoBuildConf/*, so before anything sources it every line must be empty, a comment or one
# assignment of a single-quoted literal without any quote inside: NAME='value' (README
# "Staged project"; the generator spkmeta emits exactly this). Nothing in such a file can
# expand or run.
readonly BUILD_ENV_LINE="^([A-Z_][A-Z0-9_]*='[^']*'|[[:space:]]*(#.*)?)$"

check_build_env_grammar() {
	local bad
	bad="$(LC_ALL=C grep -nvE "$BUILD_ENV_LINE" "$1" | cut -c1-80)" || true
	[ -z "$bad" ] || die "build.env lines other than NAME='literal' assignments or comments:
$bad"
}

# The fields of build.env (README "Staged project"; spkmeta's Build.BuildEnv writes exactly
# these). Any other name could override a variable of INFO.sh, SynoBuildConf/* or the toolkit
# that sources the file, so it is refused; each field must be assigned exactly once, because a
# second assignment would silently win over the first.
readonly BUILD_ENV_FIELDS="ID DISPLAY_NAME DESCRIPTION DESCRIPTION_GER MAINTAINER MAINTAINER_URL VERSION DSM_VERSION BETA OS_MIN_VER ARCH PORT"

check_build_env_fields() {
	local name field seen=" " unknown="" duplicate="" missing=""
	while IFS= read -r name; do
		case " $BUILD_ENV_FIELDS " in
			*" $name "*) ;;
			*) unknown="$unknown $name"; continue ;;
		esac
		case "$seen" in
			*" $name "*) duplicate="$duplicate $name" ;;
			*) seen="$seen$name " ;;
		esac
	done < <(sed -n "s/^\([A-Z_][A-Z0-9_]*\)='.*/\1/p" "$1")
	[ -z "$unknown" ] || die "build.env fields other than $BUILD_ENV_FIELDS:$unknown"
	[ -z "$duplicate" ] || die "build.env assigns these fields more than once:$duplicate"
	for field in $BUILD_ENV_FIELDS; do
		case "$seen" in *" $field "*) ;; *) missing="$missing $field" ;; esac
	done
	[ -z "$missing" ] || die "build.env lacks these fields:$missing"
}

# Refuses CR characters in every text file of the project that a shell reads or DSM parses:
# INFO.sh, build.env, SynoBuildConf/*, scripts/*, conf/*, the firewall service file and the
# wizard scripts. bash in the chroot and DSM's sh would take the CR for part of a command or
# value.
check_line_endings() {
	local project="$1" files=() crlf
	mapfile -t files < <(
		find "$project/INFO.sh" "$project/build.env" "$project/SynoBuildConf" "$project/scripts" \
			"$project/conf" "$project/prebuilt/port_conf" -type f 2>/dev/null
		find "$project/WIZARD_UIFILES" -type f -name '*.sh' 2>/dev/null
	)
	[ "${#files[@]}" -gt 0 ] || return 0
	crlf="$(grep -l $'\r' "${files[@]}")" || true
	[ -n "$crlf" ] || return 0
	crlf="${crlf//"$project/"/}"
	die "CRLF line endings in ${crlf//$'\n'/, }; bash in the chroot and DSM cannot read them"
}

# Reads one variable of build.env in a subshell, so its values never leak into this script.
build_env_value() {
	# shellcheck source=/dev/null
	(set +u && . "$1" && printf '%s' "${!2:-}")
}

# Copies the project into source/<ID> and prints <ID>. Bind mounts from Windows show every
# file as 0777, so modes are not trusted here; SynoBuildConf/install sets them explicitly.
stage_project() {
	local project="$1" id dest
	[ -f "$project/build.env" ] || die "$project/build.env not found (is the project staged?)"
	check_line_endings "$project"
	check_build_env_grammar "$project/build.env"
	check_build_env_fields "$project/build.env"
	id="$(build_env_value "$project/build.env" ID)"
	[[ "$id" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || die "build.env ID '$id' is not a package name"
	# The chroot's Python 3.4 reads SynoBuildConf/depends with an ASCII locale; a non-ASCII byte
	# makes it list no project, and PkgCreate.py then builds and packs nothing.
	if LC_ALL=C grep -lq '[^[:print:][:space:]]' "$project"/SynoBuildConf/* 2>/dev/null; then
		die "non-ASCII characters in $project/SynoBuildConf; the toolkit cannot read them"
	fi
	dest="$TOOLKIT_ROOT/source/$id"
	rm -rf "$dest"
	mkdir -p "$dest"
	cp -R "$project/." "$dest/"
	echo "$id"
}

# Prints the end of the chroot's build and install logs, where the toolkit records the real
# reason of a failure (PkgCreate.py itself only names the failed stage).
show_logs() {
	local chroot="$1" id="$2" log_file
	for log_file in "$chroot/logs/$id.build" "$chroot/logs/$id.install"; do
		[ -f "$log_file" ] || continue
		log "--- last lines of ${log_file#"$chroot"} ---"
		tail -n 25 "$log_file" >&2
	done
}

pack() {
	local platform="$1" chroot="$2" id="$3" started=$SECONDS
	rm -rf "$RESULT_DIR"
	log "PkgCreate.py -v $DSM_TOOLKIT_VERSION -p $platform -c $id"
	if ! (cd "$PKGSCRIPTS_DIR" && python3 ./PkgCreate.py -v "$DSM_TOOLKIT_VERSION" -p "$platform" -c "$id"); then
		show_logs "$chroot" "$id"
		die "PkgCreate.py failed for $id on $platform"
	fi
	log "packed in $(elapsed "$started")"
}

# Copies the one release SPK to the output directory and prints its path. The toolkit runs
# the install stage twice (with and without --with-debug, PkgCreate.py:161-167); our install
# script skips the debug pass, so a *_debug.spk would mean a foreign install script.
collect() {
	local output="$1" spks=() spk
	mapfile -t spks < <(find "$RESULT_DIR" -type f -name '*.spk' ! -name '*_debug.spk')
	[ "${#spks[@]}" -eq 1 ] || die "expected one SPK in $RESULT_DIR, found ${#spks[@]}"
	mkdir -p "$output"
	spk="$output/$(basename "${spks[0]}")"
	cp "${spks[0]}" "$spk"
	# On a Linux host the output belongs to the caller, not to root of the container.
	chown --reference="$output" "$spk" 2>/dev/null || true
	echo "$spk"
}

# PkgCreate.py and EnvDeploy mount /proc into the chroot (exec_env.py:66) and never unmount
# it. The mount ends with the container anyway; unmounting is the documented clean exit
# (README of pkgscripts-ng, "Remove Environment").
CHROOT_TO_UNMOUNT=""
unmount_proc() {
	[ -n "$CHROOT_TO_UNMOUNT" ] || return 0
	if mountpoint -q "$CHROOT_TO_UNMOUNT/proc"; then
		umount "$CHROOT_TO_UNMOUNT/proc" || log "could not unmount $CHROOT_TO_UNMOUNT/proc"
	fi
}

main() {
	[ "$#" -ge 1 ] && [ "$#" -le 3 ] || usage
	local family="$1" project="${2:-/project}" output="${3:-/output}"
	local platform chroot id spk started=$SECONDS
	platform="$(default_platform "$family")" || die "unknown family '$family'"
	platform="${TOOLKIT_PLATFORM:-$platform}"
	chroot="$TOOLKIT_ROOT/build_env/ds.$platform-$DSM_TOOLKIT_VERSION"
	[ -d "$project" ] || die "project directory $project not found"

	mkdir -p "$TOOLKIT_ROOT"
	# One run at a time per volume: source/ and result_spk/ are shared between platforms.
	exec 9>"$TOOLKIT_ROOT/.lock"
	flock 9
	CHROOT_TO_UNMOUNT="$chroot"
	trap unmount_proc EXIT

	log "family $family, platform $platform"
	ensure_toolkit
	ensure_environment "$platform" "$chroot"
	id="$(stage_project "$project")"
	pack "$platform" "$chroot" "$id"
	spk="$(collect "$output")"
	log "wrote ${spk#"$output"/} in $(elapsed "$started")"
	tar -tvf "$spk" >&2
}

main "$@"

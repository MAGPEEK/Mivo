# shellcheck shell=sh source-path=SCRIPTDIR
# shellcheck disable=SC2034 # the PKG_* variables are used by the scripts that source this file
# Shared code of the package scripts: the brand values of package.env and the package paths of
# ADR 0026 section 6, derived from the directories DSM passes to every script.
#
# Where DSM keeps the scripts at run time is not documented (DSM-NOTES section 5), so a script
# loads this file from its own directory, never from a fixed path:
#
#   case "$0" in */*) PKG_SCRIPT_DIR=${0%/*} ;; *) PKG_SCRIPT_DIR=. ;; esac
#   . "$PKG_SCRIPT_DIR/common.sh"
#   pkg_load_package_env && pkg_require_dsm_environment && pkg_derive_paths
#
# The file holds only assignments and function definitions and reads package.env only inside
# pkg_load_package_env, so it still works when the staging step pastes it into a script (the
# fallback of SPK-14 if DSM drops extra files in scripts/). POSIX sh only: DSM's /bin/sh is
# unconfirmed (BusyBox ash or bash, DSM-NOTES section 5).

# Mode of the directories the package creates for the server (ADR 0026 section 6).
PKG_DIR_MODE=0750
# Files the package and the server create are not readable by others, matching PKG_DIR_MODE.
PKG_FILE_UMASK=027
# Name of the server's configuration file in its config directory (config.FileName).
PKG_CONFIG_FILE_NAME=config.yaml
# Value of <ENV_PREFIX>_INSTALL_KIND (config.InstallKindSynology).
PKG_INSTALL_KIND=synology
# DSM's directory of the installed packages; /var/packages/<id>/shares/<share> was the link
# Package Center created for each share of 0.1.0-alpha.2's data-share resource (DSM-NOTES
# section 7, unused since SPK-17).
PKG_DSM_PACKAGES_DIR=/var/packages
# Directory that holds DSM's volumes (/volume1, /volume2, ...): "" stands for /. The test-only
# PKG_TEST_VOLUMES_ROOT (packaging/synology/test; DSM never sets it) moves it into a test tree.
PKG_VOLUMES_ROOT=${PKG_TEST_VOLUMES_ROOT:-}

# pkg_error prints a message on stderr. DSM appends the script output to
# /var/log/packages/<id>.log.
pkg_error() {
	printf '%s\n' "$*" >&2
}

# pkg_localized prints $2 (German) when DSM runs in German, else $1 (English). DSM passes its
# language as SYNOPKG_DSM_LANGUAGE (DSM-NOTES section 5).
pkg_localized() {
	case "${SYNOPKG_DSM_LANGUAGE:-}" in
	ger) printf '%s\n' "$2" ;;
	*) printf '%s\n' "$1" ;;
	esac
}

# pkg_tell_user shows a message in Package Center's dialog: DSM reads $SYNOPKG_TEMP_LOGFILE
# after a script exits non-zero (DSM-NOTES section 5). The message also goes to stderr, so the
# package log keeps it.
pkg_tell_user() {
	pkg_error "$*"
	if [ -n "${SYNOPKG_TEMP_LOGFILE:-}" ]; then
		printf '%s\n' "$*" >>"$SYNOPKG_TEMP_LOGFILE" 2>/dev/null || :
	fi
}

# pkg_load_package_env reads ID, DISPLAY_NAME, ENV_PREFIX and PORT from package.env next to the
# calling script (rendered by the staging step) and checks the values that become paths and
# variable names.
pkg_load_package_env() {
	if [ ! -r "$PKG_SCRIPT_DIR/package.env" ]; then
		pkg_error "$PKG_SCRIPT_DIR/package.env is missing; reinstall the package"
		return 1
	fi
	# Not followed: package.env exists only in the staged package (spkstage writes it).
	# shellcheck source=/dev/null
	. "$PKG_SCRIPT_DIR/package.env"
	pkg_check_package_env
}

# pkg_check_package_env rejects values that would break a path, the package user or the
# firewall section name (ID: the pattern of spkmeta's Package.Validate, no underscore because
# the .sc section allows none) or an environment variable name (ENV_PREFIX); the staging step
# renders them from internal/platform/brand.
pkg_check_package_env() {
	case "${ID:-}" in
	'' | [!a-z0-9]* | *[!a-z0-9.-]* | *[.-] | *[.-][.-]*)
		pkg_error "package.env: invalid ID '${ID:-}'"
		return 1
		;;
	esac
	case "${ENV_PREFIX:-}" in
	'' | [!A-Z]* | *[!A-Z0-9_]*)
		pkg_error "package.env: invalid ENV_PREFIX '${ENV_PREFIX:-}'"
		return 1
		;;
	esac
	if [ -z "${DISPLAY_NAME:-}" ]; then
		pkg_error "package.env: DISPLAY_NAME is empty"
		return 1
	fi
}

# pkg_require_absolute_path fails unless the value $2 of the DSM variable named $1 is an
# absolute path.
pkg_require_absolute_path() {
	case "$2" in
	/*) return 0 ;;
	'') pkg_error "$1 is not set; the package scripts run only under DSM's Package Center" ;;
	*) pkg_error "$1 is not an absolute path: $2" ;;
	esac
	return 1
}

# pkg_require_dsm_environment checks the package directories DSM exports to every script
# (guide "Package Filesystem Hierarchy Standard"; DSM 7.0-40759 or newer has all four).
pkg_require_dsm_environment() {
	pkg_require_absolute_path SYNOPKG_PKGDEST "${SYNOPKG_PKGDEST:-}" &&
		pkg_require_absolute_path SYNOPKG_PKGVAR "${SYNOPKG_PKGVAR:-}" &&
		pkg_require_absolute_path SYNOPKG_PKGTMP "${SYNOPKG_PKGTMP:-}" &&
		pkg_require_absolute_path SYNOPKG_PKGHOME "${SYNOPKG_PKGHOME:-}"
}

# pkg_derive_paths sets the package paths of ADR 0026 section 6. Persistent state lives in
# var, which DSM never removes; transcode output lives in tmp, which DSM wipes on upgrade and
# uninstall (OD-SPK-7 A).
pkg_derive_paths() {
	PKG_BINARY=$SYNOPKG_PKGDEST/bin/$ID
	PKG_FFMPEG=$SYNOPKG_PKGDEST/bin/ffmpeg
	PKG_FFPROBE=$SYNOPKG_PKGDEST/bin/ffprobe
	PKG_CONFIG_DIR=$SYNOPKG_PKGVAR
	PKG_CONFIG_FILE=$PKG_CONFIG_DIR/$PKG_CONFIG_FILE_NAME
	PKG_DATA_DIR=$SYNOPKG_PKGVAR/data
	PKG_CACHE_DIR=$SYNOPKG_PKGVAR/cache
	PKG_TRANSCODE_DIR=$SYNOPKG_PKGTMP/transcode
	# preupgrade records the targets of alpha.2's share links here for postupgrade.
	PKG_SHARE_RECORD_FILE=$SYNOPKG_PKGVAR/share-links-before-upgrade.tsv
	PKG_LOG_FILE=$SYNOPKG_PKGVAR/$ID.log
	PKG_PID_FILE=$SYNOPKG_PKGVAR/$ID.pid
	# The action lock of start-stop-status; in tmp, which DSM wipes on upgrade and uninstall.
	PKG_LOCK_DIR=$SYNOPKG_PKGTMP/$ID.lock
	PKG_HOME_DIR=$SYNOPKG_PKGHOME
}

# pkg_export_server_environment exports the settings the package owns (ADR 0018 names: brand
# prefix plus key path). The environment wins over config.yaml, so port, server.lan_only and
# log settings are deliberately not set here: they stay with config.yaml. The install kind and
# the browse roots belong to the package like the paths; libraries are managed in the app.
pkg_export_server_environment() {
	export "${ENV_PREFIX}_CONFIG_DIR=$PKG_CONFIG_DIR"
	export "${ENV_PREFIX}_PATHS_DATA=$PKG_DATA_DIR"
	export "${ENV_PREFIX}_PATHS_CACHE=$PKG_CACHE_DIR"
	export "${ENV_PREFIX}_PATHS_TRANSCODE=$PKG_TRANSCODE_DIR"
	export "${ENV_PREFIX}_TOOLS_FFMPEG=$PKG_FFMPEG"
	export "${ENV_PREFIX}_TOOLS_FFPROBE=$PKG_FFPROBE"
	export "${ENV_PREFIX}_INSTALL_KIND=$PKG_INSTALL_KIND"
	export "${ENV_PREFIX}_BROWSE_ROOTS=$(pkg_browse_roots)"
	export HOME="$PKG_HOME_DIR"
}

# pkg_browse_roots prints the existing volume directories (/volume<N> and /volumeUSB<N>, N digits
# only) joined by ":", sorted, empty when there are none. The folder browser starts there; a
# shared folder is the unit of access on DSM. Evaluated at each start, so a new volume shows up.
pkg_browse_roots() {
	pkg_roots_list=''
	for pkg_root_dir in "$PKG_VOLUMES_ROOT"/volume[0-9]* "$PKG_VOLUMES_ROOT"/volumeUSB[0-9]*; do
		pkg_root_number=${pkg_root_dir##*/volume}
		pkg_root_number=${pkg_root_number#USB}
		case "$pkg_root_number" in
		'' | *[!0-9]*) continue ;;
		esac
		if [ -d "$pkg_root_dir" ]; then
			pkg_roots_list=$pkg_roots_list${pkg_roots_list:+:}${pkg_root_dir#"$PKG_VOLUMES_ROOT"}
		fi
	done
	printf '%s\n' "$pkg_roots_list"
}

# pkg_create_server_dirs creates the data, cache and transcode directories that are missing,
# with mode PKG_DIR_MODE. Existing directories keep their mode and content.
pkg_create_server_dirs() {
	for pkg_server_dir in "$PKG_DATA_DIR" "$PKG_CACHE_DIR" "$PKG_TRANSCODE_DIR"; do
		if [ ! -d "$pkg_server_dir" ]; then
			# No -p: the parents are DSM's var and tmp directories, which DSM creates.
			mkdir -m "$PKG_DIR_MODE" "$pkg_server_dir" || return 1
		fi
	done
}

# pkg_remove_directory_contents removes every entry of directory $1, hidden ones included, and
# keeps the directory. rm -r does not follow symbolic links, so nothing outside $1 is removed.
# Each entry that cannot be removed is named on stderr; the function then fails.
pkg_remove_directory_contents() {
	pkg_remove_status=0
	for pkg_remove_entry in "$1"/* "$1"/.[!.]* "$1"/..?*; do
		if [ -e "$pkg_remove_entry" ] || [ -L "$pkg_remove_entry" ]; then
			if ! rm -rf -- "$pkg_remove_entry"; then
				pkg_error "cannot remove $pkg_remove_entry"
				pkg_remove_status=1
			fi
		fi
	done
	return "$pkg_remove_status"
}

# pkg_volume_directory_kind prints "var" or "home" when the path $1 is /volume<N>/@appdata/<id>
# or /volume<N>/@apphome/<id> of this package: the volume is exactly one path component right
# below PKG_VOLUMES_ROOT, "volume" and a number (N digits only), so no deeper directory of that
# shape and no other name passes.
pkg_volume_directory_kind() {
	pkg_volume_rest=${1#"$PKG_VOLUMES_ROOT"/volume}
	if [ "$pkg_volume_rest" = "$1" ]; then
		return 1
	fi
	# What follows "volume" up to the next "/": the volume number, digits only.
	case "${pkg_volume_rest%%/*}" in
	'' | *[!0-9]*) return 1 ;;
	esac
	case "${pkg_volume_rest#*/}" in
	"$pkg_volume_rest") return 1 ;;
	@appdata/"$ID") printf '%s\n' var ;;
	@apphome/"$ID") printf '%s\n' home ;;
	*) return 1 ;;
	esac
}

# pkg_package_directory_kind prints "var" or "home" when the absolute path $1 has the shape of
# this package's var or home directory as DSM lays them out (DSM-NOTES section 8): the link
# /var/packages/<id>/var or /var/packages/<id>/home, or the real directory on a volume,
# /volume<N>/@appdata/<id> or /volume<N>/@apphome/<id> (pkg_volume_directory_kind). Paths with
# "." or ".." elements or "//" fail; one trailing "/" is ignored.
pkg_package_directory_kind() {
	pkg_kind_path=${1%/}
	case "$pkg_kind_path/" in
	*/./* | */../* | *//*) return 1 ;;
	esac
	case "$pkg_kind_path" in
	"$PKG_DSM_PACKAGES_DIR/$ID/var") printf '%s\n' var ;;
	"$PKG_DSM_PACKAGES_DIR/$ID/home") printf '%s\n' home ;;
	*) pkg_volume_directory_kind "$pkg_kind_path" ;;
	esac
}

# pkg_resolve_package_directory prints the resolved path (every link followed, cd -P) of the
# existing directory $2 when both $2 and its resolved path have the shape of this package's
# directory of kind $1 (var or home, pkg_package_directory_kind). It guards every deletion:
# a link that points elsewhere, a path of another package or of the other kind, and anything
# that is not a directory fail. Callers delete below the printed path, not below $2, so a link
# changed after the check cannot redirect the deletion.
pkg_resolve_package_directory() {
	[ "$(pkg_package_directory_kind "$2")" = "$1" ] || return 1
	pkg_resolved_dir=$(CDPATH='' cd -P "$2" 2>/dev/null && pwd -P) || return 1
	[ "$(pkg_package_directory_kind "$pkg_resolved_dir")" = "$1" ] || return 1
	printf '%s\n' "$pkg_resolved_dir"
}

# pkg_is_volume_path succeeds when $1 is a path below /volume<N>/ or /volumeUSB<N>/ (N digits
# only), the shape of a share's real location; it is what the upgrade accepts as a target.
pkg_is_volume_path() {
	pkg_volume_check=${1#/volume}
	if [ "$pkg_volume_check" = "$1" ]; then
		return 1
	fi
	pkg_volume_check=${pkg_volume_check#USB}
	pkg_volume_number=${pkg_volume_check%%/*}
	case "$pkg_volume_number" in
	'' | *[!0-9]*) return 1 ;;
	esac
	# Something must follow the volume directory, and no "." or ".." element.
	case "$pkg_volume_check" in
	"$pkg_volume_number" | "$pkg_volume_number"/ | */./* | */../* | */. | */.. | *//*) return 1 ;;
	esac
}

# pkg_share_link_prefix prints the start of the path of every DSM link to a share of this
# package (DSM-NOTES section 7), the library paths 0.1.0-alpha.2 wrote to config.yaml. Only the
# upgrade (preupgrade, postupgrade) uses it: the install wizard asks for no shared folder any
# more (SPK-17), so the shared-folder naming rules are gone.
pkg_share_link_prefix() {
	printf '%s\n' "$PKG_DSM_PACKAGES_DIR/$ID/shares/"
}

# pkg_yaml_quote prints $1 as a YAML double-quoted scalar. Only "\" and """ need an escape there;
# callers reject control characters first, which double quotes would carry as raw bytes.
pkg_yaml_quote() {
	printf '"%s"\n' "$(printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"
}

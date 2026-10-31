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

# pkg_error prints a message on stderr. DSM appends the script output to
# /var/log/packages/<id>.log.
pkg_error() {
	printf '%s\n' "$*" >&2
}

# pkg_load_package_env reads ID, DISPLAY_NAME, ENV_PREFIX and PORT from package.env next to the
# calling script (rendered by the staging step) and checks the values that become paths and
# variable names.
pkg_load_package_env() {
	if [ ! -r "$PKG_SCRIPT_DIR/package.env" ]; then
		pkg_error "$PKG_SCRIPT_DIR/package.env is missing; reinstall the package"
		return 1
	fi
	# Not followed: in the repository package.env is a template with tokens instead of values.
	# shellcheck source=/dev/null
	. "$PKG_SCRIPT_DIR/package.env"
	pkg_check_package_env
}

# pkg_check_package_env rejects values that would break a path (ID) or an environment variable
# name (ENV_PREFIX); the staging step renders them from internal/platform/brand.
pkg_check_package_env() {
	case "${ID:-}" in
	'' | [!A-Za-z0-9]* | *[!A-Za-z0-9._-]*)
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
	PKG_DATA_DIR=$SYNOPKG_PKGVAR/data
	PKG_CACHE_DIR=$SYNOPKG_PKGVAR/cache
	PKG_TRANSCODE_DIR=$SYNOPKG_PKGTMP/transcode
	PKG_LOG_FILE=$SYNOPKG_PKGVAR/$ID.log
	PKG_PID_FILE=$SYNOPKG_PKGVAR/$ID.pid
	PKG_HOME_DIR=$SYNOPKG_PKGHOME
}

# pkg_export_server_environment exports the settings the package owns (ADR 0018 names: brand
# prefix plus key path). The environment wins over config.yaml, so port, server.lan_only, log
# settings and libraries are deliberately not set here: they stay with config.yaml.
pkg_export_server_environment() {
	export "${ENV_PREFIX}_CONFIG_DIR=$PKG_CONFIG_DIR"
	export "${ENV_PREFIX}_PATHS_DATA=$PKG_DATA_DIR"
	export "${ENV_PREFIX}_PATHS_CACHE=$PKG_CACHE_DIR"
	export "${ENV_PREFIX}_PATHS_TRANSCODE=$PKG_TRANSCODE_DIR"
	export "${ENV_PREFIX}_TOOLS_FFMPEG=$PKG_FFMPEG"
	export "${ENV_PREFIX}_TOOLS_FFPROBE=$PKG_FFPROBE"
	export HOME="$PKG_HOME_DIR"
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

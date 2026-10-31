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
# DSM's directory of the installed packages; /var/packages/<id>/shares/<share> is the link
# Package Center creates for each share of the data-share resource (DSM-NOTES section 7).
PKG_DSM_PACKAGES_DIR=/var/packages
# Longest shared-folder name DSM accepts, in characters (DSM-NOTES section 9).
PKG_SHARE_NAME_MAX_CHARACTERS=32
# Characters DSM forbids in shared-folder names, as a grep bracket expression (DSM-NOTES
# section 9). Inside brackets "]" must come first and "\" is literal.
PKG_SHARE_NAME_FORBIDDEN='[]!"#%&'\''()*+,/:;<=>?@[\^`{}|~]'

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
	'' | [!a-z0-9]* | *[!a-z0-9.-]*)
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

# pkg_is_package_directory succeeds when $1 is an existing directory that belongs to this
# package: an absolute path with the package ID as one of its elements, as both
# /var/packages/<id>/var and its target /volumeN/@appdata/<id> have. It guards every deletion.
pkg_is_package_directory() {
	case "$1" in
	/*/"$ID" | /*/"$ID"/*) ;;
	*) return 1 ;;
	esac
	case "/$1/" in
	*/../* | */./*) return 1 ;;
	esac
	[ -d "$1" ]
}

# pkg_character_count prints the number of characters of the UTF-8 text $1: the bytes that are
# not UTF-8 continuation bytes, independent of the locale DSM runs the script in.
pkg_character_count() {
	printf '%s' "$1" | LC_ALL=C tr -d '\200-\277' | wc -c | tr -d ' '
}

# pkg_check_share_name succeeds when $1 follows DSM's shared-folder naming rules (DSM-NOTES
# section 9), the rules the install wizard's validator implements. A name that passes contains
# no path separator and no control character, so it is safe in a path and in config.yaml.
pkg_check_share_name() {
	case "$1" in
	'' | . | .. | [-\ ]* | *' ' | \$* | *\$?* | *[[:cntrl:]]*) return 1 ;;
	esac
	if printf '%s' "$1" | LC_ALL=C grep -q -- "$PKG_SHARE_NAME_FORBIDDEN"; then
		return 1
	fi
	[ "$(pkg_character_count "$1")" -le "$PKG_SHARE_NAME_MAX_CHARACTERS" ]
}

# pkg_share_link prints the path of DSM's link to share $1 (DSM-NOTES section 7). It is the
# library root: it survives moving the share to another volume (ADR 0026 section 7).
pkg_share_link() {
	printf '%s\n' "$PKG_DSM_PACKAGES_DIR/$ID/shares/$1"
}

# pkg_yaml_quote prints $1 as a YAML double-quoted scalar. Only "\" and """ need an escape there;
# callers reject control characters first, which double quotes would carry as raw bytes.
pkg_yaml_quote() {
	printf '"%s"\n' "$(printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"
}

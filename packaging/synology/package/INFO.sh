#!/bin/bash
# Writes the package's INFO file to stdout (ADR 0026 sections 2, 5 and 8).
# SynoBuildConf/install runs it inside the toolkit chroot as `bash ./INFO.sh > INFO`. Every
# value comes from build.env, which the staging step generates from the brand and the version;
# nothing here names the product.
#
# pkg_dump_info (pkgscripts-ng include/pkg_util.sh) prints the variables of a fixed key list
# that are non-empty. Keys outside that list are appended after it (README "INFO keys").
# Keep this file ASCII: the chroot's tools run with an ASCII locale (README "Pitfalls").
# shellcheck disable=SC2153 # the upper-case variables are defined by build.env

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

info_fail() {
	echo "Error: INFO.sh: $*" >&2
	exit 1
}

# pkg_dump_info prints `key="value"` with an unquoted echo: a double quote, backslash, dollar
# sign or backquote would break the INFO syntax, glob characters could expand against the
# working directory and runs of spaces collapse. Such values are rejected instead, and so is
# an apostrophe: the build.env contract has no way to write one (README "Staged project").
require_info_value() {
	local name="$1" value="${!1:-}"
	[ -n "$value" ] || info_fail "build.env sets no $name"
	case "$value" in
		*[\"\'\\\$\`*?[]* | *"  "* | *$'\n'* | *$'\t'*)
			info_fail "build.env $name contains characters INFO cannot carry: $value" ;;
	esac
}

[ -f "$script_dir/build.env" ] || info_fail "build.env missing next to INFO.sh"
# shellcheck source=/dev/null
. "$script_dir/build.env"
for build_env_name in ID DISPLAY_NAME DESCRIPTION DESCRIPTION_GER MAINTAINER MAINTAINER_URL \
	DSM_VERSION OS_MIN_VER ARCH BETA PORT; do
	require_info_value "$build_env_name"
done
case "$BETA" in yes | no) ;; *) info_fail "build.env BETA must be yes or no, not '$BETA'" ;; esac

# Formats DSM and the toolkit rely on. DSM_VERSION also ends up in the SPK file name and path.
[[ "$ID" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || info_fail "build.env ID '$ID' is not a package name"
[[ "$DSM_VERSION" =~ ^[0-9]+(\.[0-9]+)*-[0-9]+$ ]] ||
	info_fail "build.env DSM_VERSION '$DSM_VERSION' is not <numbers separated by dots>-<build>"
[[ "$OS_MIN_VER" =~ ^[0-9]+\.[0-9]+-[0-9]+$ ]] ||
	info_fail "build.env OS_MIN_VER '$OS_MIN_VER' is not <major>.<minor>-<build>"
if ! [[ "$PORT" =~ ^[1-9][0-9]{0,4}$ ]] || [ "$PORT" -gt 65535 ]; then
	info_fail "build.env PORT '$PORT' is not a TCP port (1-65535)"
fi

# The INFO arch is ARCH from build.env: the family, so one SPK covers every model of the
# family, for armv7 followed by the DSM 7.2 platforms whose arch value stands outside every
# family (README "Platforms"; ADR 0026 Notes: arch="armv7 armada38x monaco").
family="${ARCH%% *}"
case "$family" in x86_64 | armv8 | armv7) ;; *) info_fail "build.env ARCH '$ARCH' does not start with a family (x86_64, armv8, armv7)" ;; esac
for arch_word in ${ARCH#"$family"}; do
	case "$family:$arch_word" in
		armv7:armada38x | armv7:monaco) ;;
		*) info_fail "build.env ARCH '$ARCH': '$arch_word' is not an arch value for family $family" ;;
	esac
done

# shellcheck source=/dev/null
. /pkgscripts-ng/include/pkg_util.sh

# The toolkit's family of the chroot platform must equal the family; otherwise binaries of one
# family would be packed in another family's environment.
toolkit_family="$(pkg_get_platform_family)" || info_fail "the toolkit cannot name the family of this environment"
[ "$toolkit_family" = "$family" ] ||
	info_fail "build.env ARCH=$ARCH, but this environment belongs to family '$toolkit_family'"

# Variables read by pkg_dump_info by name.
# shellcheck disable=SC2034
{
	package="$ID"
	version="$DSM_VERSION"
	displayname="$DISPLAY_NAME"
	description="$DESCRIPTION"
	description_ger="$DESCRIPTION_GER"
	maintainer="$MAINTAINER"
	maintainer_url="$MAINTAINER_URL"
	os_min_ver="$OS_MIN_VER"
	arch="$ARCH"
	adminport="$PORT"
	adminprotocol="http"
	# No adminurl: DSM opens adminprotocol://<host>:adminport/adminurl, so "/" would give "//"
	# (ADR 0026 Notes, "adminurl is omitted").
}
# pkg_dump_info skips empty variables: a release writes no beta key at all.
# shellcheck disable=SC2034
if [ "$BETA" = yes ]; then beta="yes"; else beta=""; fi

pkg_dump_info
# Not in pkg_dump_info's key list; written explicitly so the intent is visible in INFO
# (ADR 0026 section 10: moving the package to another volume stays off).
echo 'support_move="no"'

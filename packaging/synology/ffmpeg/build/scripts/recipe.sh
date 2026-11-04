# recipe.sh - identity of this recipe's output (sourced, POSIX sh).
#
# Only the FFmpeg, package and source-archive stages read this file, so changing it never
# rebuilds the toolchains or libraries.

# shellcheck shell=sh

# RECIPE_REVISION counts changes of the configuration or the recipe for one FFmpeg release; it
# becomes FFmpeg's --extra-version ("ffmpeg version 9.0.2-static2"), part of the source archive
# name and the version field of packaging/synology/ffmpeg/sources.lock. Bump it with every
# change that alters the binaries (pins, flags, configuration) and update the lock's checksums.
# Edits that leave the binaries alone (documentation, checks) change only the source archive,
# whose SHA-256 every build records in its SOURCE.txt.
readonly RECIPE_REVISION=2
# ARCHIVE_EPOCH is the modification time of every member of the source archive (2026-10-31
# 00:00 UTC), so the archive does not depend on when or where it was built.
# shellcheck disable=SC2034 # used by make-source-archive.sh
readonly ARCHIVE_EPOCH=1793404800

# version_label prints the build's version from the FFmpeg release.
version_label() { printf '%s-static%s' "$1" "$RECIPE_REVISION"; }

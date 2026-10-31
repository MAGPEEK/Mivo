# layout.sh - directories shared by the build scripts (sourced, POSIX sh).
#
# Every family builds below the same fixed paths in its own Docker stage, so paths that end up
# in the binaries (FFmpeg's configure line) are identical in every build (reproducibility).

# shellcheck shell=sh

readonly BUILD_ROOT=/build
# Licence texts of everything linked, collected by the stage that unpacks each component.
# shellcheck disable=SC2034 # used by the scripts that source this file
readonly LICENSE_DIR="$BUILD_ROOT/licenses"

#!/bin/sh
# make-source-archive.sh - bundle the corresponding source of the static FFmpeg build.
#
#   make-source-archive.sh <sources> <recipe> <out> <ffmpeg-version>
#
# <sources> holds the verified inputs below <stage>/ (fetch-sources.sh verify), <recipe> this
# build directory. Writes <out>/ffmpeg-<label>-source.tar and <out>/ffmpeg-<label>-source.tar.sha256.
# The archive holds every source the binaries are built from (FFmpeg, x264, dav1d, zlib, musl,
# gcc for libgcc), the tools that build them, and the recipe with musl-cross-make's patches: the
# "Corresponding Source" of GPL-3.0 section 1, attached to each GitHub Release (section 6).
#
# The archive is reproducible: fixed member order, owner, modes and modification time. The
# tarballs inside are already compressed, so the archive itself is not.
# shellcheck source-path=SCRIPTDIR
set -eu

[ "$#" -eq 4 ] || {
	printf 'usage: %s <sources> <recipe> <out> <ffmpeg-version>\n' "$0" >&2
	exit 2
}
sources=$1
recipe=$2
out=$3
ffmpeg_version=$4
script_dir=$(dirname "$0")

# shellcheck source=recipe.sh
. "$script_dir/recipe.sh"

name="ffmpeg-$(version_label "$ffmpeg_version")-source"
stage=$(mktemp -d)
top="$stage/$name"
mkdir -p "$top/sources" "$top/recipe"
cp -R "$recipe/." "$top/recipe/"
for directory in "$sources"/*/; do
	group=$(basename "$directory")
	mkdir -p "$top/sources/$group"
	find "$directory" -maxdepth 1 -type f ! -name versions.env -exec cp {} "$top/sources/$group/" \;
done

cat >"$top/README.txt" <<EOF
Corresponding source of the static FFmpeg $(version_label "$ffmpeg_version") build

recipe/   the Docker recipe that produced the binaries (start with recipe/README.md)
sources/  every input it downloads, as listed with its SHA-256 in recipe/sources.list

Rebuild without downloading the sources (base image and Debian build tools still come from
the network):

  docker build --build-context sources=sources --target all --output type=local,dest=out recipe

The recipe verifies every file in sources/ against recipe/sources.list before it builds.
EOF

# Modes do not depend on the checkout (Windows checkouts mark every file executable).
find "$top" -type d -exec chmod 0755 {} +
find "$top" -type f -exec chmod 0644 {} +
find "$top" -type f -name '*.sh' -exec chmod 0755 {} +

mkdir -p "$out"
tar --sort=name --owner=0 --group=0 --numeric-owner --mtime="@$ARCHIVE_EPOCH" --format=gnu \
	-C "$stage" -cf "$out/$name.tar" "$name"
(cd "$out" && sha256sum "$name.tar" >"$name.tar.sha256")
rm -rf "$stage"
printf 'make-source-archive.sh: %s\n' "$(cat "$out/$name.tar.sha256")"

#!/bin/sh
# ffmpeg-capabilities.sh - checks that an FFmpeg build has what disc and photo support need
# (task M3-16j, ADR 0029) and reports the optional HEIC/AVIF decoders.
#
#   ffmpeg-capabilities.sh <ffmpeg>
#
# One script for every build the project ships: the Docker image runs it at build time, the SPK
# verification (ffmpeg/build/verify/checks.sh) and inspect-spk.sh --capabilities run it on the
# bundled binaries, and postinst runs it on the NAS, where DSM appends the output to the package
# log /var/log/packages/<id>.log. The lists below are the single place that names the needs.
#
# Prints one line per capability on stdout:
#   required <kind> <name>: ok | MISSING      (kind: demuxer, protocol, decoder)
#   optional <name>: available | not available
# then "summary: required capabilities present" or "summary: required capabilities missing:
# <list>". Exit 0 when every required capability is present, 1 when one is missing, 2 on a
# usage error or when ffmpeg cannot list its components (or a listing outlasts the timeout).
#
# The demuxers come from "ffmpeg -demuxers", the demuxing half of "ffmpeg -formats" without its
# flag column. POSIX sh, no product name: the script also runs inside the SPK under DSM.
set -u

readonly EXIT_MISSING=1
readonly EXIT_USAGE=2
# Folder discs (ADR 0029): a Blu-ray's concat list, the joined VOBs (mpeg = program stream)
# and the m2ts clips (mpegts); the photo renderer reads single images through image pipes.
readonly REQUIRED_DEMUXERS="concat mpeg mpegts"
# file: every input; concat: the joined VOBs of a DVD title (concat:file:a|file:b); subfile: a
# byte range of a disc image, read without mounting (subfile,,start,A,end,B,,:file:<image>,
# M3-16g).
readonly REQUIRED_PROTOCOLS="file concat subfile"
# Photo thumbnails (M3-15d): JPEG, PNG, WebP, GIF and TIFF are decoded by FFmpeg itself.
readonly REQUIRED_DECODERS="mjpeg png webp gif tiff"
# Seconds one ffmpeg listing may take before it counts as failed. postinst runs this script on
# the NAS, where a hanging binary would block the installation; FFMPEG_CAPABILITIES_TIMEOUT
# overrides it (the tests use 1).
readonly LISTING_TIMEOUT_SECONDS="${FFMPEG_CAPABILITIES_TIMEOUT:-30}"

if [ "$#" -ne 1 ]; then
	printf 'usage: %s <ffmpeg>\n' "$0" >&2
	exit "$EXIT_USAGE"
fi
ffmpeg=$1

# run_ffmpeg <arguments...>: ffmpeg -hide_banner with the arguments, limited by timeout(1) when
# the system has one (DSM's base system does not document it). timeout starts ffmpeg as its
# direct child, so both GNU and BusyBox timeout stop it, not only a shell around it.
run_ffmpeg() {
	if command -v timeout >/dev/null 2>&1; then
		timeout "$LISTING_TIMEOUT_SECONDS" "$ffmpeg" -hide_banner "$@"
	else
		"$ffmpeg" -hide_banner "$@"
	fi
}

# component_names <listing option>: the names of a component listing (-demuxers, -decoders),
# one per line. The rows follow a dashed separator line; the first field is the flag column,
# a lone "d" after it marks a device (FFmpeg 7 flags "D.d"), and aliases are comma-separated.
# The device rule takes a second field "d" for the flag, never for a name: FFmpeg has no
# component called "d", and a row of one would need a third field to be misread.
component_names() {
	run_ffmpeg "$1" 2>/dev/null | awk '
		seen { name = ($2 == "d" && NF >= 3) ? $3 : $2; n = split(name, names, ","); for (i = 1; i <= n; i++) print names[i] }
		/^ *-+ *$/ { seen = 1 }'
}

# input_protocols: the names of the "Input:" section of ffmpeg -protocols, one per line.
input_protocols() {
	run_ffmpeg -protocols 2>/dev/null | awk '/^Input:/ { input = 1; next } /^Output:/ { input = 0 } input { print $1 }'
}

demuxers=$(component_names -demuxers)
decoders=$(component_names -decoders)
protocols=$(input_protocols)
if [ -z "$demuxers" ] || [ -z "$decoders" ] || [ -z "$protocols" ]; then
	printf 'ffmpeg-capabilities.sh: %s cannot list its demuxers, decoders and protocols\n' "$ffmpeg" >&2
	exit "$EXIT_USAGE"
fi

# has <list> <name>: the newline-separated list contains the name.
has() {
	printf '%s\n' "$1" | grep -qx -- "$2"
}

missing=""

# require <kind> <list> <names...>
require() {
	kind=$1
	list=$2
	shift 2
	for name in "$@"; do
		if has "$list" "$name"; then
			printf 'required %s %s: ok\n' "$kind" "$name"
		else
			printf 'required %s %s: MISSING\n' "$kind" "$name"
			missing="$missing $kind:$name"
		fi
	done
}

# report <name> <condition...>: prints whether the optional capability is available.
report() {
	name=$1
	shift
	if "$@"; then state=available; else state="not available"; fi
	printf 'optional %s: %s\n' "$name" "$state"
}

# heif_decodable: HEIF has no decoder of its own; FFmpeg reads the items with the mov demuxer
# and decodes them with hevc (grid images need FFmpeg 7.1 or newer, reported by -version).
heif_decodable() { has "$demuxers" mov && has "$decoders" hevc; }

printf 'ffmpeg capabilities of %s: %s\n' "$ffmpeg" "$(run_ffmpeg -version 2>/dev/null | sed -n 1p)"
# The lists are split into names on purpose.
# shellcheck disable=SC2086
{
	require demuxer "$demuxers" $REQUIRED_DEMUXERS
	require protocol "$protocols" $REQUIRED_PROTOCOLS
	require decoder "$decoders" $REQUIRED_DECODERS
}
report "hevc decoder" has "$decoders" hevc
report "heif (mov demuxer and hevc decoder)" heif_decodable
report "libdav1d decoder (AVIF, AV1)" has "$decoders" libdav1d

if [ -n "$missing" ]; then
	printf 'summary: required capabilities missing:%s\n' "$missing"
	exit "$EXIT_MISSING"
fi
printf 'summary: required capabilities present\n'

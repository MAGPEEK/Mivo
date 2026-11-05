#!/bin/sh
# fake-ffmpeg.sh - stands in for ffmpeg in the tests of ffmpeg-capabilities.sh and postinst
# (task M3-16j): answers -version, -demuxers, -decoders and -protocols in the layout of FFmpeg
# 7 to 9 (flag column, a lone "d" for devices, comma-separated aliases, "Input:"/"Output:").
#
# Environment: FAKE_FFMPEG_MISSING  blank-separated component names left out of every listing
#              FAKE_FFMPEG_SILENT   when set, prints nothing (a binary that lists nothing)
#              FAKE_FFMPEG_HANG     when set, hangs for 60 s (a binary the timeout must stop);
#                                   exec, so the process timeout(1) started is the one sleeping
set -u

[ -z "${FAKE_FFMPEG_SILENT:-}" ] || exit 0
[ -z "${FAKE_FFMPEG_HANG:-}" ] || exec sleep 60

# row <name> <line>: prints the listing row unless the name is to be left out.
row() {
	for missing in ${FAKE_FFMPEG_MISSING:-}; do
		[ "$missing" != "$1" ] || return 0
	done
	printf '%s\n' "$2"
}

for argument in "$@"; do
	case "$argument" in
	-version)
		printf 'ffmpeg version 9.9-fake Copyright (c) 2000-2026 the FFmpeg developers\n'
		exit 0
		;;
	-demuxers)
		printf 'File formats:\n D. = Demuxing supported\n .E = Muxing supported\n ..d = Is a device\n ---\n'
		row alsa ' D d alsa            ALSA audio input'
		row concat ' D   concat          Virtual concatenation script'
		row mov ' D   mov,mp4,m4a,3gp,3g2,mj2 QuickTime / MOV'
		row mpeg ' D   mpeg            MPEG-PS (MPEG-2 Program Stream)'
		row mpegts ' D   mpegts          MPEG-TS (MPEG-2 Transport Stream)'
		exit 0
		;;
	-decoders)
		printf 'Decoders:\n V..... = Video\n ------\n'
		for decoder in mjpeg png webp gif tiff hevc libdav1d; do row "$decoder" " V....D $decoder    $decoder decoder"; done
		exit 0
		;;
	-protocols)
		# Output lists concat too: only the Input: section counts.
		printf 'Supported file protocols:\nInput:\n'
		for protocol in concat file pipe subfile; do row "$protocol" "  $protocol"; done
		printf 'Output:\n  concat\n  file\n'
		exit 0
		;;
	esac
done
exit 0

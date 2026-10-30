package main

import (
	"slices"
	"strconv"
	"time"
)

// Seek clips are long enough to seek in and have keyframes at a fixed interval, so a seek
// test knows where a remux can start and how far a transcode has to decode (ADR 0025).
const (
	seekClipDuration     = 30 * time.Second
	seekKeyframeInterval = 2 * time.Second
	// seekCaseSuffix ends the name of every seek case.
	seekCaseSuffix = "-long"
	// mpegVideoSceneCutOff disables the scene-change I-frames of FFmpeg's MPEG-1/2 encoders;
	// it is a score threshold, so 0 would not turn them off. FFmpeg's own messages name
	// this value.
	mpegVideoSceneCutOff = 1000000000
)

// seekCases returns one clip that the server remuxes (H.264/AAC in Matroska) and one it
// transcodes (MPEG-2/MP2 in MPEG-TS).
func seekCases() []mediaCase {
	source := standardSource().withDuration(seekClipDuration)

	return []mediaCase{
		{
			Name: "video-mkv-h264-aac" + seekCaseSuffix, Category: categoryVideo, Output: "video/h264_aac_long.mkv",
			Source: source, Muxer: "matroska", Encoders: []string{"libx264", "aac"},
			Args:   slices.Concat(x264Args(), x264FixedGOPArgs(), audioArgs("aac")),
			Expect: expect("mkv", probeMatroska, h264Video(), audioOf("aac")),
		},
		{
			Name: "video-ts-mpeg2-mp2" + seekCaseSuffix, Category: categoryVideo, Output: "video/mpeg2_mp2_long.ts",
			Source: source, Muxer: "mpegts", Encoders: []string{"mpeg2video", "mp2"},
			Args:   slices.Concat(simpleVideoArgs("mpeg2video"), mpegVideoFixedGOPArgs(), audioArgs("mp2")),
			Expect: expect("mpegts", probeMPEGTS, mpeg2Video(), audioOf("mp2")),
		},
	}
}

// seekGOPFrames is the number of frames between two keyframes of a seek clip.
func seekGOPFrames() string {
	return strconv.Itoa(int(seekKeyframeInterval.Seconds()) * defaultFrameRate)
}

// x264FixedGOPArgs makes x264 place a keyframe exactly every seekKeyframeInterval: no
// shorter GOPs and no extra keyframes at scene cuts.
func x264FixedGOPArgs() []string {
	frames := seekGOPFrames()

	return []string{"-g", frames, "-keyint_min", frames, "-sc_threshold", "0"}
}

// mpegVideoFixedGOPArgs is the MPEG-2 equivalent; the encoder has no minimum GOP length.
func mpegVideoFixedGOPArgs() []string {
	return []string{"-g", seekGOPFrames(), "-sc_threshold", strconv.Itoa(mpegVideoSceneCutOff)}
}

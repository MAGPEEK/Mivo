package main

import (
	"slices"
	"time"
)

const (
	// cueLength and cueGap place two cues inside the 2 s clip.
	cueLength = 900 * time.Millisecond
	cueGap    = 100 * time.Millisecond

	languageEnglish = "eng"
	languageGerman  = "deu"

	codecSubRip = "subrip"
	codecASS    = "ass"
	codecWebVTT = "webvtt"
)

// subtitleCases returns sidecar and embedded text subtitle cases and the bitmap subtitle
// formats FFmpeg cannot produce from text.
func subtitleCases() []mediaCase {
	return slices.Concat(
		sidecarSubtitleCases(),
		embeddedSubtitleCases(),
		subtitleGaps(),
	)
}

// subtitleCues returns two cues with the given texts, both inside clipDuration.
func subtitleCues(first, second string) []cue {
	return []cue{
		{Start: 0, End: cueLength, Text: first},
		{Start: cueLength + cueGap, End: 2*cueLength + cueGap, Text: second},
	}
}

func englishCues() []cue { return subtitleCues("Generated subtitle one", "Generated subtitle two") }

func germanCues() []cue {
	return subtitleCues("Erzeugter Untertitel eins", "Erzeugter Untertitel zwei")
}

// sidecarSubtitleCases writes SRT, WebVTT and ASS files next to a video file with the same
// base name, the layout media servers use to find external subtitles.
func sidecarSubtitleCases() []mediaCase {
	cues := englishCues()

	return []mediaCase{
		{
			Name: "subtitle-sidecar-srt-vtt-ass", Category: categorySubtitle, Output: "subtitle/sidecar.mkv",
			Source: standardSource(), Muxer: "matroska", Encoders: []string{"libx264", "aac"},
			Sidecars: []sidecarFile{
				{Output: "subtitle/sidecar.srt", Content: formatSRT(cues), ProbeFormat: "srt", Codec: codecSubRip},
				{Output: "subtitle/sidecar.vtt", Content: formatWebVTT(cues), ProbeFormat: "webvtt", Codec: codecWebVTT},
				{
					Output: "subtitle/sidecar.ass", Content: formatASS(cues, defaultWidth, defaultHeight),
					ProbeFormat: "ass", Codec: codecASS,
				},
			},
			Args:   slices.Concat(x264Args(), audioArgs("aac")),
			Expect: expect("mkv", probeMatroska, h264Video(), audioOf("aac")),
		},
	}
}

func embeddedSubtitleCases() []mediaCase {
	english := trackExpectation{Language: languageEnglish, Default: true}
	secondary := trackExpectation{Language: languageEnglish}
	// The MKV sets subtitle dispositions, so it marks its video and audio default as well.
	video := defaultTrack()
	audio := defaultTrack()

	mkv := expect("mkv", probeMatroska, h264Video(), audioOf("aac"))
	mkv.Video.Track = &video
	mkv.Audio[0].Track = &audio
	mkv.Subtitles = []subtitleExpectation{{Codec: codecSubRip, Track: &english}, {Codec: codecASS, Track: &secondary}}

	mp4 := expect("mp4", probeISOBMFF, h264Video(), audioOf("aac"))
	mp4.Subtitles = []subtitleExpectation{{Codec: "mov_text", Track: &english}}

	return []mediaCase{
		{
			Name: "subtitle-mkv-srt-ass", Category: categorySubtitle, Output: "subtitle/h264_aac_srt_ass.mkv",
			Source: standardSource(), Muxer: "matroska", Encoders: []string{"libx264", "aac", "srt", "ass"},
			Inputs: []textInput{englishSRTInput("english.srt"), {
				FileName: "english.ass", Format: "ass", Role: roleSubtitle,
				Content: formatASS(englishCues(), defaultWidth, defaultHeight),
			}},
			Args: slices.Concat(x264Args(), audioArgs("aac"), []string{"-c:s:0", "srt", "-c:s:1", "ass"},
				trackArgs("v:0", video), trackArgs("a:0", audio),
				trackArgs("s:0", english), trackArgs("s:1", secondary)),
			Expect: mkv,
		},
		{
			// MP4 only carries 3GPP timed text (mov_text) as a text subtitle format.
			Name: "subtitle-mp4-movtext", Category: categorySubtitle, Output: "subtitle/h264_aac_mov_text.mp4",
			Source: standardSource(), Muxer: "mp4", Encoders: []string{"libx264", "aac", "mov_text"},
			Inputs: []textInput{englishSRTInput("english.srt")},
			Args: slices.Concat(x264Args(), audioArgs("aac"), []string{"-c:s", "mov_text"},
				trackArgs("s:0", english)),
			Expect: mp4,
		},
	}
}

func englishSRTInput(fileName string) textInput {
	return textInput{FileName: fileName, Format: "srt", Role: roleSubtitle, Content: formatSRT(englishCues())}
}

// subtitleGaps lists bitmap subtitle formats: FFmpeg has encoders for some of them, but no
// way to render text into the bitmaps they need (ADR 0021).
func subtitleGaps() []mediaCase {
	return []mediaCase{
		knownGap("subtitle-sup-pgs", "subtitle/pgs.sup",
			expectation{Container: "sup", ProbeFormat: "sup", Subtitles: []subtitleExpectation{{Codec: "hdmv_pgs_subtitle"}}},
			"ffmpeg has no PGS (Blu-ray) subtitle encoder"),
		knownGap("subtitle-vobsub", "subtitle/vobsub.sub",
			expectation{Container: "vobsub", ProbeFormat: "vobsub", Subtitles: []subtitleExpectation{{Codec: "dvd_subtitle"}}},
			"ffmpeg cannot render text into the bitmaps its VobSub (dvdsub) encoder needs"),
		knownGap("subtitle-ts-dvb", "subtitle/dvb_subtitle.ts",
			expectation{Container: "mpegts", ProbeFormat: probeMPEGTS, Subtitles: []subtitleExpectation{{Codec: "dvb_subtitle"}}},
			"ffmpeg cannot render text into the bitmaps its DVB subtitle encoder needs"),
	}
}

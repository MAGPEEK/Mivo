package main

import (
	"slices"
	"strconv"
)

const (
	// commentaryAudioTracks is the number of audio streams of the commentary case.
	commentaryAudioTracks = 3
	// commentaryTitle is the track name players and the track selection recognise.
	commentaryTitle = "Commentary"

	codecPGS         = "hdmv_pgs_subtitle"
	codecDVDSubtitle = "dvd_subtitle"
)

// trackSelectionCases are the inputs of the milestone-4 track and subtitle selection (task
// M4-03): styled ASS next to plain SRT, an audio commentary next to stereo and 5.1 tracks,
// and the two bitmap subtitle formats FFmpeg can mux from the PGS stream the generator
// writes itself (disc_pgs.go).
func trackSelectionCases() []mediaCase {
	return []mediaCase{
		styledSubtitleCase(),
		commentaryCase(),
		bitmapSubtitleCase(casePGS, "tracks/h264_aac_pgs.mkv", codecPGS, nil),
		// dvdsub converts bitmap to bitmap, the only bitmap subtitle encoder that Matroska
		// carries besides the PGS copy.
		bitmapSubtitleCase(caseDVDSubtitle, "tracks/h264_aac_dvdsub.mkv", codecDVDSubtitle, []string{"dvdsub"}),
	}
}

// styledSubtitleCase has a styled ASS track first and default, so a session that converts it
// to WebVTT loses the styling, and the same cues as plain SRT second.
func styledSubtitleCase() mediaCase {
	video := defaultTrack()
	audio := defaultTrack()
	styled := trackExpectation{Language: languageEnglish, Title: "Styled", Default: true}
	plain := trackExpectation{Language: languageEnglish}

	expected := expect("mkv", probeMatroska, h264Video(), audioOf("aac"))
	expected.Video.Track = &video
	expected.Audio[0].Track = &audio
	expected.Subtitles = []subtitleExpectation{{Codec: codecASS, Track: &styled}, {Codec: codecSubRip, Track: &plain}}

	return mediaCase{
		Name: caseStyledSubtitles, Category: categoryTracks, Output: "tracks/h264_aac_ass_styled_srt.mkv",
		Source: standardSource(), Muxer: matroskaMuxer, Encoders: []string{"libx264", "aac", "ass", "srt"},
		Inputs: []textInput{
			{
				FileName: "styled.ass", Format: "ass", Role: roleSubtitle,
				Content: formatStyledASS(englishCues(), defaultWidth, defaultHeight),
			},
			englishSRTInput("english.srt"),
		},
		Args: slices.Concat(x264Args(), audioArgs("aac"), []string{"-c:s:0", "ass", "-c:s:1", "srt"},
			trackArgs("v:0", video), trackArgs("a:0", audio),
			trackArgs("s:0", styled), trackArgs("s:1", plain)),
		Expect: expected,
	}
}

// commentaryCase has English stereo (default), German 5.1 and an English stereo commentary.
// The three tones differ in frequency, so a listener hears which track a session mapped.
func commentaryCase() mediaCase {
	video := defaultTrack()
	english := trackExpectation{Language: languageEnglish, Default: true}
	german := trackExpectation{Language: languageGerman}
	commentary := trackExpectation{Language: languageEnglish, Title: commentaryTitle}

	source := surroundSource(surround51Channels)
	source.AudioTracks = commentaryAudioTracks

	expected := expect("mkv", probeMatroska, h264Video(), nil)
	expected.Video.Track = &video
	expected.Audio = []audioExpectation{
		{Codec: "aac", Channels: stereoChannels, Track: &english},
		{Codec: "aac", Track: &german},
		{Codec: "aac", Channels: stereoChannels, Track: &commentary},
	}

	stereo := strconv.Itoa(stereoChannels)

	return mediaCase{
		Name: caseCommentary, Category: categoryTracks, Output: "tracks/h264_aac_eng_deu51_commentary.mkv",
		Source: source, Muxer: matroskaMuxer, Encoders: []string{"libx264", "aac"},
		Args: slices.Concat(x264Args(), audioArgs("aac"), []string{"-ac:a:0", stereo, "-ac:a:2", stereo},
			trackArgs("v:0", video), trackArgs("a:0", english), trackArgs("a:1", german),
			trackArgs("a:2", commentary)),
		Expect: expected,
	}
}

// bitmapSubtitleCase is the standard clip with one bitmap subtitle track. Without a
// subtitle encoder the generated PGS stream is copied as it is.
func bitmapSubtitleCase(name, output, codec string, subtitleEncoders []string) mediaCase {
	video := defaultTrack()
	audio := defaultTrack()
	subtitles := trackExpectation{Language: languageEnglish}

	expected := expect("mkv", probeMatroska, h264Video(), audioOf("aac"))
	expected.Video.Track = &video
	expected.Audio[0].Track = &audio
	expected.Subtitles = []subtitleExpectation{{Codec: codec, Track: &subtitles}}

	input := textInput{
		FileName: subtitleFileName, Format: subtitleDemuxer, Role: roleSubtitle,
		Content: string(pgsSubtitles(pgsFrame{width: defaultWidth, height: defaultHeight}, clipBitmapCues())),
	}
	subtitleCodec := "copy"

	if len(subtitleEncoders) > 0 {
		subtitleCodec = subtitleEncoders[0]
		// PGS ends a cue with a second display set instead of a duration. Without this option
		// the re-encoded packets have none and Matroska stores the largest block duration,
		// which makes the file four million seconds long.
		input.Options = []string{"-fix_sub_duration"}
	}

	return mediaCase{
		Name: name, Category: categoryTracks, Output: output,
		Source: standardSource(), Muxer: matroskaMuxer,
		Encoders: slices.Concat([]string{"libx264", "aac"}, subtitleEncoders),
		Inputs:   []textInput{input},
		Args: slices.Concat(x264Args(), audioArgs("aac"), []string{"-c:s", subtitleCodec},
			trackArgs("v:0", video), trackArgs("a:0", audio), trackArgs("s:0", subtitles)),
		Expect: expected,
	}
}

// clipBitmapCues are the times of the text cues, so text and bitmap clips show subtitles at
// the same positions of the 2 s clip.
func clipBitmapCues() []bitmapCue {
	cues := make([]bitmapCue, 0, len(englishCues()))
	for _, text := range englishCues() {
		cues = append(cues, bitmapCue{start: text.Start, end: text.End})
	}

	return cues
}

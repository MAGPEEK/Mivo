package main

import "slices"

// multiTrackAudioTracks is the number of audio languages in the multi-track case.
const multiTrackAudioTracks = 2

// multiTrackCases returns a Matroska file with English and German audio and subtitles, for
// track selection: English audio is the default track and the German subtitles are forced.
func multiTrackCases() []mediaCase {
	englishAudio := trackExpectation{Language: languageEnglish, Default: true}
	germanAudio := trackExpectation{Language: languageGerman}
	englishSubtitles := trackExpectation{Language: languageEnglish}
	germanSubtitles := trackExpectation{Language: languageGerman, Forced: true}

	source := standardSource()
	source.AudioTracks = multiTrackAudioTracks

	expected := expect("mkv", probeMatroska, h264Video(), nil)
	expected.Audio = []audioExpectation{
		{Codec: "aac", Track: &englishAudio},
		{Codec: "aac", Track: &germanAudio},
	}
	expected.Subtitles = []subtitleExpectation{
		{Codec: codecSubRip, Track: &englishSubtitles},
		{Codec: codecSubRip, Track: &germanSubtitles},
	}

	return []mediaCase{
		{
			Name: "multitrack-mkv-eng-deu", Category: categoryMultiTrack, Output: "multitrack/h264_aac2_srt2.mkv",
			Source: source, Muxer: "matroska", Encoders: []string{"libx264", "aac", "srt"},
			Inputs: []textInput{
				englishSRTInput("english.srt"),
				{FileName: "german.srt", Format: "srt", Role: roleSubtitle, Content: formatSRT(germanCues())},
			},
			Args: slices.Concat(x264Args(), audioArgs("aac"), []string{"-c:s", "srt"},
				trackArgs("a:0", englishAudio), trackArgs("a:1", germanAudio),
				trackArgs("s:0", englishSubtitles), trackArgs("s:1", germanSubtitles)),
			Expect: expected,
		},
	}
}

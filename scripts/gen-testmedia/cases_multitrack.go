package main

import (
	"slices"
	"strconv"
)

// multiTrackAudioTracks is the number of audio languages in the multi-track case.
const multiTrackAudioTracks = 2

// multiTrackCases returns a Matroska file with English and German audio and subtitles, for
// track selection: the video and the English audio are the default tracks and the German
// subtitles are forced. Two release-style files follow (germanDLCase, defaultSecondCase).
func multiTrackCases() []mediaCase {
	video := defaultTrack()
	englishAudio := trackExpectation{Language: languageEnglish, Default: true}
	germanAudio := trackExpectation{Language: languageGerman}
	englishSubtitles := trackExpectation{Language: languageEnglish}
	germanSubtitles := trackExpectation{Language: languageGerman, Forced: true}

	source := standardSource()
	source.AudioTracks = multiTrackAudioTracks

	expected := expect("mkv", probeMatroska, h264Video(), nil)
	expected.Video.Track = &video
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
				trackArgs("v:0", video), trackArgs("a:0", englishAudio), trackArgs("a:1", germanAudio),
				trackArgs("s:0", englishSubtitles), trackArgs("s:1", germanSubtitles)),
			Expect: expected,
		},
		germanDLCase(),
		defaultSecondCase(),
	}
}

// germanDLCase mirrors a "German.DL" release: German DTS 5.1 first and default, English AC-3
// 2.0 second; the different channel counts show which track a session mapped.
func germanDLCase() mediaCase {
	video := defaultTrack()
	germanAudio := trackExpectation{Language: languageGerman, Default: true}
	englishAudio := trackExpectation{Language: languageEnglish}

	source := surroundSource(surround51Channels)
	source.AudioTracks = multiTrackAudioTracks

	expected := expect("mkv", probeMatroska, h264Video(), nil)
	expected.Video.Track = &video
	expected.Audio = []audioExpectation{
		{Codec: "dts", Track: &germanAudio},
		{Codec: "ac3", Channels: stereoChannels, Track: &englishAudio},
	}

	return mediaCase{
		Name: "multitrack-mkv-dts-ac3-deu-eng", Category: categoryMultiTrack,
		Output: "multitrack/h264_dts_ac3_deu_eng.mkv",
		Source: source, Muxer: "matroska", Encoders: []string{"libx264", "dca", "ac3"},
		Args: slices.Concat(x264Args(), []string{"-c:a:0", "dca", "-c:a:1", "ac3", "-ac:a:1", strconv.Itoa(stereoChannels)}, experimentalEncoders(),
			trackArgs("v:0", video), trackArgs("a:0", germanAudio), trackArgs("a:1", englishAudio)),
		Expect: expected,
	}
}

// defaultSecondCase is an MP4 whose first audio track (AC-3) is not the default one; browsers
// still play the first track of the original file, so the default AAC track must be remuxed.
func defaultSecondCase() mediaCase {
	englishAudio := trackExpectation{Language: languageEnglish}
	germanAudio := trackExpectation{Language: languageGerman, Default: true}

	source := standardSource()
	source.AudioTracks = multiTrackAudioTracks

	expected := expect("mp4", probeISOBMFF, h264Video(), nil)
	expected.Audio = []audioExpectation{
		{Codec: "ac3", Track: &englishAudio},
		{Codec: "aac", Track: &germanAudio},
	}

	return mediaCase{
		Name: "multitrack-mp4-ac3-aac-default-second", Category: categoryMultiTrack,
		Output: "multitrack/h264_ac3_aac_default_second.mp4",
		Source: source, Muxer: "mp4", Encoders: []string{"libx264", "ac3", "aac"},
		Args: slices.Concat(x264Args(), []string{"-c:a:0", "ac3", "-c:a:1", "aac"},
			trackArgs("a:0", englishAudio), trackArgs("a:1", germanAudio)),
		Expect: expected,
	}
}

package main

import (
	"maps"
	"slices"
	"strconv"
	"time"
)

const (
	// audioDuration is long enough for two audiobook chapters and for seeking tests.
	audioDuration = 5 * time.Second
	// cdSampleRate and hiResSampleRate are the CD and the common high-resolution rate.
	cdSampleRate    = 44100
	hiResSampleRate = 96000
	bitDepth16      = 16
	bitDepth24      = 24
	// chapterLength splits the audiobook into two equal chapters.
	chapterLength = audioDuration / 2

	probeOgg = "ogg"
)

// audioCases returns the music and audiobook cases and the audio formats FFmpeg cannot write.
func audioCases() []mediaCase {
	return slices.Concat(
		losslessAudioCases(),
		lossyAudioCases(),
		pcmAudioCases(),
		audiobookCases(),
		audioGaps(),
	)
}

// audioSource is a 5 s stereo sine tone without video.
func audioSource(sampleRate int) sourceSpec {
	return sourceSpec{SampleRate: sampleRate, Channels: stereoChannels, Duration: audioDuration}
}

func losslessAudioCases() []mediaCase {
	return []mediaCase{
		{
			Name: "audio-flac-16bit-44k", Category: categoryAudio, Output: "audio/flac_16bit_44k.flac",
			Source: audioSource(cdSampleRate), Muxer: "flac", Encoders: []string{"flac"},
			Args:   slices.Concat(audioArgs("flac"), []string{"-sample_fmt", "s16"}),
			Expect: expect("flac", "flac", nil, losslessAudio("flac", bitDepth16)),
		},
		{
			// FLAC stores 24-bit samples in 32-bit containers; bits_per_raw_sample says 24.
			Name: "audio-flac-24bit-96k", Category: categoryAudio, Output: "audio/flac_24bit_96k.flac",
			Source: audioSource(hiResSampleRate), Muxer: "flac", Encoders: []string{"flac"},
			Args: slices.Concat(audioArgs("flac"),
				[]string{"-sample_fmt", "s32", "-bits_per_raw_sample", strconv.Itoa(bitDepth24)}),
			Expect: expect("flac", "flac", nil, losslessAudio("flac", bitDepth24)),
		},
		{
			// The ipod muxer writes the MP4 flavour Apple uses for .m4a and .m4b files.
			Name: "audio-m4a-alac", Category: categoryAudio, Output: "audio/alac.m4a",
			Source: audioSource(defaultSampleRate), Muxer: "ipod", Encoders: []string{"alac"},
			Args:   audioArgs("alac"),
			Expect: expect("m4a", probeISOBMFF, nil, losslessAudio("alac", bitDepth16)),
		},
		{
			Name: "audio-wv-wavpack", Category: categoryAudio, Output: "audio/wavpack.wv",
			Source: audioSource(defaultSampleRate), Muxer: "wv", Encoders: []string{"wavpack"},
			Args:   audioArgs("wavpack"),
			Expect: expect("wv", "wv", nil, losslessAudio("wavpack", bitDepth16)),
		},
	}
}

func lossyAudioCases() []mediaCase {
	return []mediaCase{
		{
			Name: "audio-mp3", Category: categoryAudio, Output: "audio/mp3.mp3",
			Source: audioSource(defaultSampleRate), Muxer: "mp3", Encoders: []string{"libmp3lame"},
			Args:   audioArgs("libmp3lame"),
			Expect: expect("mp3", "mp3", nil, audioOf("mp3")),
		},
		{
			Name: "audio-m4a-aac", Category: categoryAudio, Output: "audio/aac.m4a",
			Source: audioSource(defaultSampleRate), Muxer: "ipod", Encoders: []string{"aac"},
			Args:   audioArgs("aac"),
			Expect: expect("m4a", probeISOBMFF, nil, audioOf("aac")),
		},
		{
			Name: "audio-opus", Category: categoryAudio, Output: "audio/opus.opus",
			Source: audioSource(defaultSampleRate), Muxer: "opus", Encoders: []string{"libopus"},
			Args:   audioArgs("libopus"),
			Expect: expect("ogg", probeOgg, nil, audioOf("opus")),
		},
		{
			Name: "audio-ogg-vorbis", Category: categoryAudio, Output: "audio/vorbis.ogg",
			Source: audioSource(defaultSampleRate), Muxer: "ogg", Encoders: []string{"libvorbis"},
			Args:   audioArgs("libvorbis"),
			Expect: expect("ogg", probeOgg, nil, audioOf("vorbis")),
		},
	}
}

func pcmAudioCases() []mediaCase {
	return []mediaCase{
		{
			Name: "audio-wav-pcm", Category: categoryAudio, Output: "audio/pcm_s16le.wav",
			Source: audioSource(defaultSampleRate), Muxer: "wav", Encoders: []string{"pcm_s16le"},
			Args:   audioArgs("pcm_s16le"),
			Expect: expect("wav", "wav", nil, audioOf("pcm_s16le")),
		},
		{
			Name: "audio-aiff-pcm", Category: categoryAudio, Output: "audio/pcm_s16be.aiff",
			Source: audioSource(defaultSampleRate), Muxer: "aiff", Encoders: []string{"pcm_s16be"},
			Args:   audioArgs("pcm_s16be"),
			Expect: expect("aiff", "aiff", nil, audioOf("pcm_s16be")),
		},
	}
}

// audiobookCases builds an M4B audiobook with two chapters and title, artist and album tags.
func audiobookCases() []mediaCase {
	chapters := audiobookChapters()
	tags := audiobookTags()

	expected := expect("m4b", probeISOBMFF, nil, audioOf("aac"))
	expected.Tags = tags

	for _, part := range chapters {
		expected.Chapters = append(expected.Chapters, chapterExpectation{
			Title: part.Title, StartSeconds: part.Start.Seconds(), EndSeconds: part.End.Seconds(),
		})
	}

	return []mediaCase{
		{
			Name: "audio-m4b-aac-chapters", Category: categoryAudio, Output: "audio/aac_chapters.m4b",
			Source: audioSource(defaultSampleRate), Muxer: "ipod", Encoders: []string{"aac"},
			Inputs: []textInput{{
				FileName: "chapters.ffmeta", Format: "ffmetadata", Role: roleChapters,
				Content: formatFFMetadata(chapters),
			}},
			Args:   slices.Concat(audioArgs("aac"), metadataArgs(tags)),
			Expect: expected,
		},
	}
}

func audiobookChapters() []chapter {
	return []chapter{
		{Title: "Chapter 1", Start: 0, End: chapterLength},
		{Title: "Chapter 2", Start: chapterLength, End: audioDuration},
	}
}

// audiobookTags are neutral on purpose: the product name lives in one place only (CLAUDE.md).
func audiobookTags() map[string]string {
	return map[string]string{
		"title":  "Generated Test Book",
		"artist": "Generated Narrator",
		"album":  "Generated Test Corpus",
	}
}

// metadataArgs sets container-level tags, sorted by key so the arguments are stable.
func metadataArgs(tags map[string]string) []string {
	var args []string

	for _, key := range slices.Sorted(maps.Keys(tags)) {
		args = append(args, "-metadata", key+"="+tags[key])
	}

	return args
}

// audioGaps lists audio formats FFmpeg can read but not write (ADR 0021).
func audioGaps() []mediaCase {
	return []mediaCase{
		knownGap("audio-ape", "audio/monkeys_audio.ape",
			expect("ape", "ape", nil, audioOf("ape")),
			"ffmpeg has no Monkey's Audio (APE) encoder"),
		knownGap("audio-dsf-dsd", "audio/dsd64.dsf",
			expect("dsf", "dsf", nil, audioOf("dsd_lsbf_planar")),
			"ffmpeg has no DSD encoder or DSF muxer"),
		knownGap("audio-dff-dsd", "audio/dsd64.dff",
			expect("dff", "iff", nil, audioOf("dsd_msbf")),
			"ffmpeg has no DSD encoder or DSDIFF (DFF) muxer"),
	}
}

func losslessAudio(codec string, bitDepth int) *audioExpectation {
	return &audioExpectation{Codec: codec, BitDepth: bitDepth}
}

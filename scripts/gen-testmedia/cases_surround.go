package main

import (
	"path"
	"slices"
)

// Channel counts of the surround layouts typical releases carry.
const (
	surround51Channels = 6
	surround71Channels = 8
)

// experimentalEncoders lets FFmpeg use its DTS and TrueHD encoders, which it marks as
// experimental; the clips only need to be decodable, not good.
func experimentalEncoders() []string {
	return []string{"-strict", "-2"}
}

// libopusSurroundArgs encodes Opus with more than two channels: mapping family 1 is the
// Vorbis channel order of RFC 7845, which Matroska and MP4 carry.
func libopusSurroundArgs() []string {
	return []string{"-c:a", "libopus", "-mapping_family", "1"}
}

// surroundSource is the standard clip with the given number of audio channels.
func surroundSource(channels int) sourceSpec {
	return standardSource().withAudio(defaultSampleRate, channels)
}

// surroundCases cover the audio of typical releases (Dolby, DTS, lossless and multichannel
// AAC) in Matroska and MP4, so playback can prove every one reaches the browser with sound.
func surroundCases() []mediaCase {
	return slices.Concat(dolbyDTSCases(), multichannelCases())
}

// dolbyDTSCases are the codecs no browser decodes: AC-3, E-AC-3, DTS and TrueHD in 5.1.
// FFmpeg's E-AC-3 and TrueHD encoders write at most 5.1, so 7.1 is covered by multichannelCases.
func dolbyDTSCases() []mediaCase {
	return []mediaCase{
		surroundCase("video-mkv-h264-ac3-51", "video/h264_ac3_51.mkv", "ac3", audioArgs("ac3")),
		surroundCase("video-mkv-h264-eac3-51", "video/h264_eac3_51.mkv", "eac3", audioArgs("eac3")),
		surroundCase("video-mp4-h264-eac3-51", "video/h264_eac3_51.mp4", "eac3", audioArgs("eac3")),
		surroundCase("video-mkv-h264-dts-51", "video/h264_dts_51.mkv", "dca",
			slices.Concat(audioArgs("dca"), experimentalEncoders())),
		surroundCase("video-mkv-h264-truehd-51", "video/h264_truehd_51.mkv", "truehd",
			slices.Concat(audioArgs("truehd"), experimentalEncoders())),
	}
}

// multichannelCases are codecs browsers decode, in layouts up to 7.1, plus PCM and MP3.
func multichannelCases() []mediaCase {
	pcm := surroundCase("video-mkv-h264-pcm", "video/h264_pcm.mkv", "pcm_s16le", audioArgs("pcm_s16le"))
	pcm.Source = standardSource()

	mp3 := surroundCase("video-mp4-h264-mp3", "video/h264_mp3.mp4", "libmp3lame", audioArgs("libmp3lame"))
	mp3.Source = standardSource()

	flac := surroundCase("video-mkv-h264-flac-71", "video/h264_flac_71.mkv", "flac", audioArgs("flac"))
	flac.Source = surroundSource(surround71Channels)

	aac71 := surroundCase("video-mp4-h264-aac-71", "video/h264_aac_71.mp4", "aac", audioArgs("aac"))
	aac71.Source = surroundSource(surround71Channels)

	return []mediaCase{
		surroundCase("video-mkv-h264-opus-51", "video/h264_opus_51.mkv", "libopus", libopusSurroundArgs()),
		surroundCase("video-mp4-h264-aac-51", "video/h264_aac_51.mp4", "aac", audioArgs("aac")),
		aac71, flac, pcm, mp3,
	}
}

// surroundCase is an H.264 clip with one 5.1 audio stream of encoder in the container that
// output's extension names. The expected codec is ffprobe's name of the encoder's output.
func surroundCase(name, output, encoder string, audio []string) mediaCase {
	muxer, container, probeFormat := "matroska", "mkv", probeMatroska
	if path.Ext(output) == ".mp4" {
		muxer, container, probeFormat = "mp4", "mp4", probeISOBMFF
	}

	return mediaCase{
		Name: name, Category: categoryVideo, Output: output,
		Source: surroundSource(surround51Channels), Muxer: muxer, Encoders: []string{"libx264", encoder},
		Args:   slices.Concat(x264Args(), audio),
		Expect: expect(container, probeFormat, h264Video(), audioOf(probedAudioCodec(encoder))),
	}
}

// probedAudioCodec maps an FFmpeg encoder to the codec name ffprobe reports for its output.
func probedAudioCodec(encoder string) string {
	switch encoder {
	case "dca":
		return "dts"
	case "libopus":
		return "opus"
	case "libmp3lame":
		return "mp3"
	default:
		return encoder
	}
}

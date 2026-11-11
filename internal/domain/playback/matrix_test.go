package playback_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// matrixRow is one file of the format matrix with the outcome for each reference profile
// on a server with full capabilities and a client with MSE HLS. The other combinations are
// derived by the rules of ADR 0023 (see expectedFor). Rows named after a corpus case mirror
// scripts/gen-testmedia; M2-33 keeps its own table keyed by the same names and must agree.
type matrixRow struct {
	name        string
	description media.Description
	request     playback.Request
	chromium    outcome
	firefox     outcome
	// subtitle is the delivery of the plan's subtitle; empty means off.
	subtitle playback.SubtitleDelivery
}

func both(name string, description media.Description, expected outcome) matrixRow {
	return matrixRow{name: name, description: description, chromium: expected, firefox: expected}
}

// standardTranscode is the outcome of a file whose container and both codecs are foreign to
// the reference profiles (MPEG-2, WMV, RealMedia, ...).
func standardTranscode() outcome {
	return transcode(transcoded, transcoded, containerReason(), videoCodecReason(), audioCodecReason())
}

func corpusVideoRows() []matrixRow {
	return []matrixRow{
		both("video-mkv-h264-aac", clip(media.ContainerMKV, h264High(0), audio(1, "aac")),
			remux(containerReason())),
		both("video-mkv-hevc10-ac3", clip(media.ContainerMKV, hevcMain10(0), audio(1, "ac3")),
			standardTranscode()),
		both("video-mkv-hevc10-hdr10-ac3", clip(media.ContainerMKV, hevcHDR10(0), audio(1, "ac3")),
			standardTranscode().withWarnings(hdrWarning())),
		both("video-mp4-h264-aac-faststart", clip(media.ContainerMP4, h264High(0), audio(1, "aac")),
			directPlay()),
		both("video-m4v-h264-aac", clip(media.ContainerM4V, h264High(0), audio(1, "aac")),
			directPlay()),
		both("video-mov-h264-pcm", clip(media.ContainerMOV, h264High(0), audio(1, "pcm_s16le")),
			transcode(copied, transcoded, containerReason(), audioCodecReason())),
		both("video-3gp-h263-aac", clip(media.Container3GP, simpleVideo(0, "h263"), audio(1, "aac")),
			transcode(transcoded, copied, containerReason(), videoCodecReason())),
		both("video-avi-xvid-mp3", clip(media.ContainerAVI, simpleVideo(0, "mpeg4"), audio(1, "mp3")),
			transcode(transcoded, copied, containerReason(), videoCodecReason())),
		both("video-avi-divx-mp3", clip(media.ContainerAVI, simpleVideo(0, "mpeg4"), audio(1, "mp3")),
			transcode(transcoded, copied, containerReason(), videoCodecReason())),
		both("video-ts-mpeg2-mp2", clip(media.ContainerMPEGTS, simpleVideo(0, "mpeg2video"), audio(1, "mp2")),
			standardTranscode()),
		both("video-m2ts-h264-ac3", clip(media.ContainerM2TS, h264High(0), audio(1, "ac3")),
			transcode(copied, transcoded, containerReason(), audioCodecReason())),
		both("video-mts-h264-ac3", clip(media.ContainerM2TS, h264High(0), audio(1, "ac3")),
			transcode(copied, transcoded, containerReason(), audioCodecReason())),
		both("video-webm-vp9-opus", clip(media.ContainerWebM, simpleVideo(0, "vp9"), audio(1, "opus")),
			directPlay()),
		both("video-flv-h264-aac", clip(media.ContainerFLV, h264High(0), audio(1, "aac")),
			remux(containerReason())),
		{
			name:        "video-ogv-theora-vorbis",
			description: clip(media.ContainerOGG, simpleVideo(0, "theora"), audio(1, "vorbis")),
			chromium:    standardTranscode(),
			firefox:     directPlay(),
		},
		both("video-webm-av1-opus", clip(media.ContainerWebM, simpleVideo(0, "av1"), audio(1, "opus")),
			directPlay()),
		both("video-mpg-mpeg2-mp2", clip(media.ContainerMPEG, simpleVideo(0, "mpeg2video"), audio(1, "mp2")),
			standardTranscode()),
		both("video-vob-mpeg2-ac3", clip(media.ContainerVOB, simpleVideo(0, "mpeg2video"), audio(1, "ac3")),
			standardTranscode()),
		both("video-wmv-wmv2-wmav2", clip(media.ContainerASF, simpleVideo(0, "wmv2"), audio(1, "wmav2")),
			standardTranscode()),
		both("video-mxf-mpeg2-pcm", clip(media.ContainerMXF, simpleVideo(0, "mpeg2video"), audio(1, "pcm_s16le")),
			standardTranscode()),
		both("video-rm-rv20-ra144", clip(media.ContainerRM, simpleVideo(0, "rv20"), audio(1, "ra_144")),
			standardTranscode()),
		both("video-rmvb-rv20-ra144", clip(media.ContainerRM, simpleVideo(0, "rv20"), audio(1, "ra_144")),
			standardTranscode()),
		// A known gap of the corpus (not generated); the row documents the expected mode.
		both("video-rmvb-rv40-cook", clip(media.ContainerRM, simpleVideo(0, "rv40"), audio(1, "cook")),
			standardTranscode()),
		both("video-mkv-h264-aac-long", clip(media.ContainerMKV, h264High(0), audio(1, "aac")),
			remux(containerReason())),
		both("video-ts-mpeg2-mp2-long", clip(media.ContainerMPEGTS, simpleVideo(0, "mpeg2video"), audio(1, "mp2")),
			standardTranscode()),
	}
}

// corpusSurroundRows cover the audio of typical releases: Dolby, DTS and lossless surround
// tracks the browsers cannot decode are re-encoded to AAC, and more than 5.1 channels are
// never copied (see TestDecideAudioChannels).
func corpusSurroundRows() []matrixRow {
	matroskaAudio := transcode(copied, transcoded, containerReason(), audioCodecReason())

	return []matrixRow{
		both("video-mkv-h264-ac3-51", clip(media.ContainerMKV, h264High(0), surround(1, "ac3", surround51)),
			matroskaAudio),
		both("video-mkv-h264-eac3-51", clip(media.ContainerMKV, h264High(0), surround(1, "eac3", surround51)),
			matroskaAudio),
		both("video-mp4-h264-eac3-51", clip(media.ContainerMP4, h264High(0), surround(1, "eac3", surround51)),
			transcode(copied, transcoded, audioCodecReason())),
		both("video-mkv-h264-dts-51", clip(media.ContainerMKV, h264High(0), surround(1, "dts", surround51)),
			matroskaAudio),
		both("video-mkv-h264-truehd-51", clip(media.ContainerMKV, h264High(0), surround(1, "truehd", surround51)),
			matroskaAudio),
		both("video-mkv-h264-flac-71", clip(media.ContainerMKV, h264High(0), surround(1, "flac", surround71)),
			transcode(copied, transcoded, containerReason(), audioChannelsReason())),
		both("video-mkv-h264-opus-51", clip(media.ContainerMKV, h264High(0), surround(1, "opus", surround51)),
			remux(containerReason())),
		both("video-mkv-h264-pcm", clip(media.ContainerMKV, h264High(0), audio(1, "pcm_s16le")),
			matroskaAudio),
		both("video-mp4-h264-aac-51", clip(media.ContainerMP4, h264High(0), surround(1, "aac", surround51)),
			directPlay()),
		both("video-mp4-h264-aac-71", clip(media.ContainerMP4, h264High(0), surround(1, "aac", surround71)),
			transcode(copied, transcoded, audioChannelsReason())),
		both("video-mp4-h264-mp3", clip(media.ContainerMP4, h264High(0), audio(1, "mp3")),
			directPlay()),
	}
}

func corpusTrackRows() []matrixRow {
	multiTrack := clip(media.ContainerMKV,
		asDefault(h264High(0)), asDefault(audio(1, "aac")), audio(2, "aac"),
		subtitle(3, "subrip"), subtitle(4, "subrip"))

	germanAudio := both("multitrack-mkv-eng-deu (German audio)", multiTrack,
		remux(containerReason(), audioTrackReason()))
	germanAudio.request = playback.Request{AudioStreamIndex: audioIndex(2)}

	englishAudio := both("multitrack-mkv-eng-deu (English audio requested)", multiTrack,
		remux(containerReason()))
	englishAudio.request = playback.Request{AudioStreamIndex: audioIndex(1)}

	return []matrixRow{
		both("subtitle-sidecar-srt-vtt-ass", clip(media.ContainerMKV, h264High(0), audio(1, "aac")),
			remux(containerReason())),
		both("subtitle-mkv-srt-ass", clip(media.ContainerMKV,
			asDefault(h264High(0)), asDefault(audio(1, "aac")),
			asDefault(subtitle(2, "subrip")), subtitle(3, "ass")),
			remux(containerReason())),
		both("subtitle-mp4-movtext", clip(media.ContainerMP4, h264High(0), audio(1, "aac"), subtitle(2, "mov_text")),
			directPlay()),
		both("multitrack-mkv-eng-deu", multiTrack,
			remux(containerReason())),
		germanAudio,
		englishAudio,
		// A "German.DL" release: German DTS 5.1 first and default, English AC-3 2.0 second.
		both("multitrack-mkv-dts-ac3-deu-eng", clip(media.ContainerMKV, asDefault(h264High(0)),
			asDefault(surround(1, "dts", surround51)), audio(2, "ac3")),
			transcode(copied, transcoded, containerReason(), audioCodecReason())),
		// Browsers play the first track (AC-3, silent); the default AAC track is remuxed.
		both("multitrack-mp4-ac3-aac-default-second", clip(media.ContainerMP4, asDefault(h264High(0)),
			audio(1, "ac3"), asDefault(audio(2, "aac"))),
			remux(audioTrackReason())),
	}
}

// syntheticRows cover rules the corpus has no file for.
func syntheticRows() []matrixRow {
	coverArt := media.Stream{
		Index: 0, Kind: media.StreamKindVideo, Codec: "mjpeg", Width: 600, Height: 600,
		AttachedPicture: true, Default: true,
	}

	h264High10 := video(0, "h264", "yuv420p10le", 10)
	h264High10.Profile = "High 10"

	uhd := hevcHDR10(0)
	uhd.Width, uhd.Height = 3840, 2160

	vp9Profile2 := video(0, "vp9", "yuv420p10le", 10)
	vp9Profile2.Profile = "Profile 2"

	// Neither ffprobe nor the pixel format tells the depth: a bit-depth limit fails closed.
	h264UnknownDepth := video(0, "h264", "", 0)
	h264UnknownDepth.Profile = "High"
	vp9UnknownDepth := video(0, "vp9", "", 0)

	secondAudio := both("mp4 with a second audio track selected",
		clip(media.ContainerMP4, h264High(0), asDefault(audio(1, "aac")), audio(2, "aac")),
		remux(audioTrackReason()))
	secondAudio.request = playback.Request{AudioStreamIndex: audioIndex(2)}

	return []matrixRow{
		both("mp4 with cover art before the video",
			clip(media.ContainerMP4, coverArt, h264High(1), audio(2, "aac")), directPlay()),
		secondAudio,
		both("mp4 with 10-bit H.264",
			clip(media.ContainerMP4, h264High10, audio(1, "aac")),
			transcode(transcoded, copied, videoProfileReason(), videoBitDepthReason())),
		both("webm with 10-bit VP9", clip(media.ContainerWebM, vp9Profile2, audio(1, "opus")), directPlay()),
		both("mp4 with H.264 of unknown bit depth",
			clip(media.ContainerMP4, h264UnknownDepth, audio(1, "aac")),
			transcode(transcoded, copied, videoBitDepthReason())),
		both("webm with VP9 of unknown bit depth",
			clip(media.ContainerWebM, vp9UnknownDepth, audio(1, "opus")),
			transcode(transcoded, copied, videoBitDepthReason())),
		both("mkv with 4K HDR10 HEVC",
			clip(media.ContainerMKV, uhd, audio(1, "eac3")),
			standardTranscode().withWarnings(hdrWarning(), downscaleWarning())),
		both("unknown container", clip(media.ContainerUnknown, h264High(0), audio(1, "aac")),
			remux(containerReason())),
	}
}

// subtitleRows are the subtitle deliveries of ADR 0030 §3 for a requested track: no subtitle,
// SubRip and ASS (sidecar, the mode of the file is kept) and PGS (burn-in, the video is
// re-encoded and the audio still copied), each in a container the browsers play directly and
// in one they get remuxed. Every matrix client renders WebVTT.
func subtitleRows() []matrixRow {
	formats := []struct {
		name, codec string
		delivery    playback.SubtitleDelivery
	}{
		{name: "SRT", codec: "subrip", delivery: playback.SubtitleSidecar},
		{name: "ASS", codec: "ass", delivery: playback.SubtitleSidecar},
		{name: "PGS", codec: pgsCodec, delivery: playback.SubtitleBurnIn},
	}

	rows := []matrixRow{
		both("subtitle off in mp4", clip(media.ContainerMP4, h264High(0), audio(1, "aac")), directPlay()),
		both("subtitle off in mkv", clip(media.ContainerMKV, h264High(0), audio(1, "aac")), remux(containerReason())),
	}

	for _, entry := range formats {
		inMP4, inMKV := directPlay(), remux(containerReason())
		if entry.delivery == playback.SubtitleBurnIn {
			inMP4 = transcode(transcoded, copied, burnInReason())
			inMKV = transcode(transcoded, copied, containerReason(), burnInReason())
		}

		mp4 := both("subtitle "+entry.name+" in mp4",
			clip(media.ContainerMP4, h264High(0), audio(1, "aac"), subtitle(2, entry.codec)), inMP4)
		mkv := both("subtitle "+entry.name+" in mkv",
			clip(media.ContainerMKV, h264High(0), audio(1, "aac"), subtitle(2, entry.codec)), inMKV)

		for _, row := range []matrixRow{mp4, mkv} {
			row.request = trackRequest("s:2")
			row.subtitle = entry.delivery
			rows = append(rows, row)
		}
	}

	return rows
}

// expectedFor derives the outcome for a client without HLS or a server without encoders:
// without HLS only direct play remains (ADR 0023), and without encoders every transcode
// fails with transcode_unavailable (ADR 0025).
func expectedFor(row matrixRow, profileName string, capabilities playback.ServerCapabilities,
	delivery playback.HLSDelivery,
) outcome {
	expected := row.chromium
	if profileName == "firefox" {
		expected = row.firefox
	}

	switch {
	case delivery == playback.HLSDeliveryNone && expected.mode != playback.ModeDirectPlay:
		return failure(playback.ErrNoCompatibleFormat)
	case capabilities.H264Encoder == "" && expected.mode == playback.ModeTranscode:
		return failure(playback.ErrTranscodeUnavailable)
	default:
		return expected
	}
}

func TestDecideFormatMatrix(t *testing.T) {
	t.Parallel()

	profiles := map[string]playback.ClientProfile{
		"chromium": playbacktest.ChromiumDesktop(),
		"firefox":  playbacktest.FirefoxDesktop(),
	}
	capabilities := map[string]playback.ServerCapabilities{
		"full":        playbacktest.FullCapabilities(),
		"no encoders": playbacktest.NoEncoders(),
	}
	deliveries := []playback.HLSDelivery{playback.HLSDeliveryMSE, playback.HLSDeliveryNone}

	for _, row := range matrixRows() {
		for profileName, profile := range profiles {
			for capabilitiesName, capability := range capabilities {
				for _, delivery := range deliveries {
					name := row.name + "/" + profileName + "/" + capabilitiesName + "/hls " + string(delivery)
					client := profile
					client.HLS.Delivery = delivery
					client.SubtitleFormats = webVTT().formats

					t.Run(name, func(t *testing.T) {
						t.Parallel()

						plan, err := playback.Decide(playback.Input{
							Description:  row.description,
							Profile:      client,
							Capabilities: capability,
							Request:      row.request,
							Policy:       playback.Policy{MaxTranscodeHeight: referenceMaxHeight},
						})

						want := expectedFor(row, profileName, capability, delivery)
						assertOutcome(t, plan, err, want)
						assertSubtitleDelivery(t, plan, want, row.subtitle)
					})
				}
			}
		}
	}
}

func matrixRows() []matrixRow {
	rows := corpusVideoRows()
	rows = append(rows, corpusSurroundRows()...)
	rows = append(rows, corpusTrackRows()...)
	rows = append(rows, subtitleRows()...)

	return append(rows, syntheticRows()...)
}

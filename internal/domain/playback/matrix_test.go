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

func corpusTrackRows() []matrixRow {
	multiTrack := clip(media.ContainerMKV,
		asDefault(h264High(0)), asDefault(audio(1, "aac")), audio(2, "aac"),
		subtitle(3, "subrip"), subtitle(4, "subrip"))

	germanAudio := both("multitrack-mkv-eng-deu (German audio)", multiTrack,
		remux(containerReason(), audioTrackReason()).withWarnings(subtitlesWarning()))
	germanAudio.request = playback.Request{AudioStreamIndex: audioIndex(2)}

	englishAudio := both("multitrack-mkv-eng-deu (English audio requested)", multiTrack,
		remux(containerReason()).withWarnings(subtitlesWarning()))
	englishAudio.request = playback.Request{AudioStreamIndex: audioIndex(1)}

	return []matrixRow{
		both("subtitle-sidecar-srt-vtt-ass", clip(media.ContainerMKV, h264High(0), audio(1, "aac")),
			remux(containerReason())),
		both("subtitle-mkv-srt-ass", clip(media.ContainerMKV,
			asDefault(h264High(0)), asDefault(audio(1, "aac")),
			asDefault(subtitle(2, "subrip")), subtitle(3, "ass")),
			remux(containerReason()).withWarnings(subtitlesWarning())),
		both("subtitle-mp4-movtext", clip(media.ContainerMP4, h264High(0), audio(1, "aac"), subtitle(2, "mov_text")),
			directPlay(subtitlesWarning())),
		both("multitrack-mkv-eng-deu", multiTrack,
			remux(containerReason()).withWarnings(subtitlesWarning())),
		germanAudio,
		englishAudio,
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

					t.Run(name, func(t *testing.T) {
						t.Parallel()

						plan, err := playback.Decide(playback.Input{
							Description:  row.description,
							Profile:      client,
							Capabilities: capability,
							Request:      row.request,
							Policy:       playback.Policy{MaxTranscodeHeight: referenceMaxHeight},
						})

						assertOutcome(t, plan, err, expectedFor(row, profileName, capability, delivery))
					})
				}
			}
		}
	}
}

func matrixRows() []matrixRow {
	rows := corpusVideoRows()
	rows = append(rows, corpusTrackRows()...)

	return append(rows, syntheticRows()...)
}

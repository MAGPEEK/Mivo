package playback_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// referenceInput plays the description with the Chromium profile on a full server.
func referenceInput(description media.Description) playback.Input {
	return playback.Input{
		Description:  description,
		Profile:      playbacktest.ChromiumDesktop(),
		Capabilities: playbacktest.FullCapabilities(),
		Policy:       playback.Policy{MaxTranscodeHeight: referenceMaxHeight},
	}
}

func mustDecide(t *testing.T, input playback.Input) playback.Plan {
	t.Helper()

	plan, err := playback.Decide(input)
	if err != nil {
		t.Fatalf("Decide() error = %v, want nil", err)
	}

	return plan
}

func assertStreams(t *testing.T, plan playback.Plan, want ...playback.StreamDecision) {
	t.Helper()

	if !slices.Equal(plan.Streams, want) {
		t.Errorf("Streams =\n%+v\nwant\n%+v", plan.Streams, want)
	}
}

func TestDecidePlanShapes(t *testing.T) {
	t.Parallel()

	copiedVideo := playback.StreamDecision{
		Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionCopy,
		SourceCodec: "h264", TargetCodec: "h264", TargetWidth: clipWidth, TargetHeight: clipHeight,
	}
	copiedAudio := playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionCopy,
		SourceCodec: "aac", TargetCodec: "aac", TargetChannels: clipChannels,
	}

	tests := []struct {
		name        string
		description media.Description
		mode        playback.Mode
		delivery    playback.Delivery
		target      media.Container
		streams     []playback.StreamDecision
	}{
		{
			name:        "direct play serves the original container",
			description: clip(media.ContainerM4V, h264High(0), audio(1, "aac")),
			mode:        playback.ModeDirectPlay, delivery: playback.DeliveryFile, target: media.ContainerM4V,
			streams: []playback.StreamDecision{copiedVideo, copiedAudio},
		},
		{
			name:        "remux delivers fMP4 HLS",
			description: clip(media.ContainerMKV, h264High(0), audio(1, "aac")),
			mode:        playback.ModeRemux, delivery: playback.DeliveryHLS, target: media.ContainerMP4,
			streams: []playback.StreamDecision{copiedVideo, copiedAudio},
		},
		{
			name:        "transcode re-encodes to H.264 and AAC stereo",
			description: clip(media.ContainerMPEGTS, simpleVideo(0, "mpeg2video"), audio(1, "mp2")),
			mode:        playback.ModeTranscode, delivery: playback.DeliveryHLS, target: media.ContainerMP4,
			streams: []playback.StreamDecision{
				{
					Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionTranscode,
					SourceCodec: "mpeg2video", TargetCodec: playback.TranscodeVideoCodec,
					TargetWidth: clipWidth, TargetHeight: clipHeight, Encoder: playback.EncoderLibx264,
				},
				{
					Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionTranscode,
					SourceCodec: "mp2", TargetCodec: playback.TranscodeAudioCodec,
					TargetChannels: playback.TranscodeAudioChannels, Encoder: playback.EncoderAAC,
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan := mustDecide(t, referenceInput(test.description))

			if plan.Mode != test.mode || plan.Delivery != test.delivery ||
				plan.SourceContainer != test.description.Container || plan.TargetContainer != test.target {
				t.Errorf("plan = %s/%s %s→%s, want %s/%s %s→%s", plan.Mode, plan.Delivery,
					plan.SourceContainer, plan.TargetContainer,
					test.mode, test.delivery, test.description.Container, test.target)
			}

			assertStreams(t, plan, test.streams...)
		})
	}
}

// targetSize is the expected output size of a transcoded video.
type targetSize struct {
	width, height int
	downscaled    bool
	scale         bool
}

func TestDecideTranscodeSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		width, height int
		maxHeight     int
		want          targetSize
	}{
		{name: "UHD to 1080p", width: 3840, height: 2160, maxHeight: 1080, want: targetSize{1920, 1080, true, true}},
		{name: "at the cap", width: 1920, height: 1080, maxHeight: 1080, want: targetSize{1920, 1080, false, false}},
		{name: "never upscaled", width: 320, height: 240, maxHeight: 1080, want: targetSize{320, 240, false, false}},
		{name: "anamorphic width kept", width: 1440, height: 1080, maxHeight: 1080, want: targetSize{1440, 1080, false, false}},
		{name: "cinemascope rounds to an even width", width: 4096, height: 1716, maxHeight: 1080, want: targetSize{2578, 1080, true, true}},
		{name: "portrait", width: 720, height: 1280, maxHeight: 1080, want: targetSize{608, 1080, true, true}},
		{name: "odd source width when downscaled", width: 1281, height: 2160, maxHeight: 720, want: targetSize{428, 720, true, true}},
		{name: "odd height rounded down", width: 720, height: 405, maxHeight: 1080, want: targetSize{720, 404, false, true}},
		{name: "odd width rounded down", width: 721, height: 480, maxHeight: 1080, want: targetSize{720, 480, false, true}},
		{name: "odd width and height rounded down", width: 853, height: 479, maxHeight: 0, want: targetSize{852, 478, false, true}},
		{name: "unknown width", width: 0, height: 2160, maxHeight: 1080, want: targetSize{0, 1080, true, true}},
		{name: "unknown height", width: 3840, height: 0, maxHeight: 1080, want: targetSize{3840, 0, false, false}},
		{name: "no cap", width: 3840, height: 2160, maxHeight: 0, want: targetSize{3840, 2160, false, false}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			source := simpleVideo(0, "mpeg2video")
			source.Width, source.Height = test.width, test.height
			input := referenceInput(clip(media.ContainerMPEGTS, source))
			input.Policy.MaxTranscodeHeight = test.maxHeight

			plan := mustDecide(t, input)
			decision, _ := plan.Video()
			assertTargetSize(t, plan, decision, test.want)
		})
	}
}

func assertTargetSize(t *testing.T, plan playback.Plan, decision playback.StreamDecision, want targetSize) {
	t.Helper()

	got := targetSize{decision.TargetWidth, decision.TargetHeight, decision.Downscaled, decision.ScaleRequired}
	if got != want {
		t.Errorf("target = %+v, want %+v", got, want)
	}

	if hasWarning := slices.Contains(plan.Warnings, downscaleWarning()); hasWarning != want.downscaled {
		t.Errorf("Warnings = %v, want video_downscaled %t", plan.Warnings, want.downscaled)
	}
}

// A 720x405 web video (16:9 at 405 lines) is common; libx264 cannot encode its odd height.
func TestDecideTranscodesOddSizedVP9ToEvenSize(t *testing.T) {
	t.Parallel()

	source := simpleVideo(0, "vp9")
	source.Width, source.Height = 720, 405

	input := referenceInput(clip(media.ContainerMKV, source, audio(1, "opus")))
	input.Profile.HLS.VideoCodecs = []string{"h264"}

	plan := mustDecide(t, input)
	decision, _ := plan.Video()

	if decision.Action != playback.ActionTranscode {
		t.Fatalf("video action = %q, want transcode", decision.Action)
	}

	assertTargetSize(t, plan, decision, targetSize{720, 404, false, true})
}

func TestDecideCopyKeepsOddSize(t *testing.T) {
	t.Parallel()

	source := simpleVideo(0, "vp9")
	source.Width, source.Height = 720, 405

	plan := mustDecide(t, referenceInput(clip(media.ContainerMKV, source, audio(1, "opus"))))
	decision, _ := plan.Video()

	assertTargetSize(t, plan, decision, targetSize{720, 405, false, false})
}

func TestPolicyValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		maxHeight int
		valid     bool
	}{
		{name: "no cap", maxHeight: 0, valid: true},
		{name: "1080p", maxHeight: 1080, valid: true},
		{name: "720p", maxHeight: 720, valid: true},
		{name: "negative", maxHeight: -2},
		{name: "odd", maxHeight: 719},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := playback.Policy{MaxTranscodeHeight: test.maxHeight}.Validate()
			if test.valid != (err == nil) || (err != nil && !errors.Is(err, domain.ErrInvalid)) {
				t.Errorf("Validate() = %v, want valid %t or domain.ErrInvalid", err, test.valid)
			}
		})
	}
}

func TestDecideCopiesLargeVideoWithoutScaling(t *testing.T) {
	t.Parallel()

	uhd := h264High(0)
	uhd.Width, uhd.Height = 3840, 2160

	plan := mustDecide(t, referenceInput(clip(media.ContainerMKV, uhd, audio(1, "ac3"))))
	decision, _ := plan.Video()

	if decision.Action != playback.ActionCopy || decision.TargetHeight != 2160 || decision.Downscaled {
		t.Errorf("video = %+v, want a copy at 2160 lines", decision)
	}

	if len(plan.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", plan.Warnings)
	}
}

func TestDecideHDRWarnings(t *testing.T) {
	t.Parallel()

	untagged := hevcHDR10(0)
	untagged.HDR = media.HDRFormatNone

	hevcClient := playbacktest.ChromiumDesktop()
	hevcClient.HLS.VideoCodecs = append(hevcClient.HLS.VideoCodecs, "hevc")

	tests := []struct {
		name    string
		stream  media.Stream
		profile playback.ClientProfile
		want    outcome
	}{
		{
			name: "HDR transcoded", stream: hevcHDR10(0), profile: playbacktest.ChromiumDesktop(),
			want: standardTranscode().withWarnings(hdrWarning()),
		},
		{
			name: "HDR classified from colour tags", stream: untagged, profile: playbacktest.ChromiumDesktop(),
			want: standardTranscode().withWarnings(hdrWarning()),
		},
		{
			name: "HDR copied keeps its signal", stream: hevcHDR10(0), profile: hevcClient,
			want: transcode(copied, transcoded, containerReason(), audioCodecReason()),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(clip(media.ContainerMKV, test.stream, audio(1, "ac3")))
			input.Profile = test.profile

			plan, err := playback.Decide(input)
			assertOutcome(t, plan, err, test.want)
		})
	}
}

func TestDecideCodecLimits(t *testing.T) {
	t.Parallel()

	withProfile := func(profile string, bitDepth int, pixelFormat string) media.Stream {
		stream := video(0, "h264", pixelFormat, bitDepth)
		stream.Profile = profile

		return stream
	}

	tests := []struct {
		name   string
		stream media.Stream
		limits []playback.CodecLimit
		want   outcome
	}{
		{
			name: "bit depth falls back to the pixel format", stream: withProfile("High", 0, "yuv420p10le"),
			want: transcode(transcoded, copied, videoBitDepthReason()),
		},
		{
			name: "unknown bit depth fails closed", stream: withProfile("High", 0, ""),
			want: transcode(transcoded, copied, videoBitDepthReason()),
		},
		{
			name: "unknown bit depth passes without a bit-depth limit", stream: withProfile("High", 0, ""),
			limits: []playback.CodecLimit{{Codec: "h264", Profiles: []string{"High"}}},
			want:   directPlay(),
		},
		{
			name: "unknown profile passes", stream: withProfile("", 8, "yuv420p"),
			want: directPlay(),
		},
		{
			name: "profile names compare case-insensitively", stream: withProfile("high", 8, "yuv420p"),
			want: directPlay(),
		},
		{
			name: "unlisted profile", stream: withProfile("High 4:4:4 Predictive", 8, "yuv444p"),
			want: transcode(transcoded, copied, videoProfileReason()),
		},
		{
			name: "every limit entry of a codec must hold", stream: h264High(0),
			limits: []playback.CodecLimit{{Codec: "h264", MaxBitDepth: 8}, {Codec: "h264", Profiles: []string{"Main"}}},
			want:   failure(playback.ErrNoCompatibleFormat),
		},
		{
			name: "limits of other codecs do not apply", stream: withProfile("High 10", 10, "yuv420p10le"),
			limits: []playback.CodecLimit{{Codec: "hevc", MaxBitDepth: 8}},
			want:   directPlay(),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(clip(media.ContainerMP4, test.stream, audio(1, "aac")))
			if test.limits != nil {
				input.Profile.CodecLimits = test.limits
			}

			plan, err := playback.Decide(input)
			assertOutcome(t, plan, err, test.want)
		})
	}
}

func TestDecideMergesDirectPlayEntriesOfOneContainer(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMP4, h264High(0), audio(1, "aac")))
	input.Profile.DirectPlay = []playback.DirectPlayProfile{
		{Container: media.DirectPlayMP4, VideoCodecs: []string{"h264"}, AudioCodecs: []string{"opus"}},
		{Container: media.DirectPlayMP4, VideoCodecs: []string{"vp9"}, AudioCodecs: []string{"aac"}},
	}

	plan, err := playback.Decide(input)
	assertOutcome(t, plan, err, directPlay())
}

func TestDecideFilesWithOneKindOfStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description media.Description
		want        outcome
	}{
		{
			name:        "audio only, direct play",
			description: clip(media.ContainerMP4, audio(0, "aac")),
			want:        outcome{mode: playback.ModeDirectPlay, audio: copied},
		},
		{
			name:        "audio only, remux",
			description: clip(media.ContainerMKV, audio(0, "flac")),
			want:        outcome{mode: playback.ModeRemux, audio: copied, reasons: []playback.Reason{containerReason()}},
		},
		{
			name:        "audio only, transcode",
			description: clip(media.ContainerMKV, audio(0, "dts")),
			want: outcome{
				mode: playback.ModeTranscode, audio: transcoded,
				reasons: []playback.Reason{containerReason(), audioCodecReason()},
			},
		},
		{
			name:        "video only, direct play",
			description: clip(media.ContainerWebM, simpleVideo(0, "vp9")),
			want:        outcome{mode: playback.ModeDirectPlay, video: copied},
		},
		{
			name:        "video only, transcode",
			description: clip(media.ContainerMPEGTS, simpleVideo(0, "mpeg2video")),
			want: outcome{
				mode: playback.ModeTranscode, video: transcoded,
				reasons: []playback.Reason{containerReason(), videoCodecReason()},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan, err := playback.Decide(referenceInput(test.description))
			assertOutcome(t, plan, err, test.want)
		})
	}
}

func TestDecideSelectsCoverArtFreeVideoAndRequestedAudio(t *testing.T) {
	t.Parallel()

	coverArt := media.Stream{Index: 0, Kind: media.StreamKindVideo, Codec: "png", AttachedPicture: true, Default: true}
	input := referenceInput(clip(media.ContainerMP4,
		coverArt, h264High(1), asDefault(audio(2, "aac")), audio(3, "mp3")))
	input.Request.AudioStreamIndex = audioIndex(3)

	plan := mustDecide(t, input)
	videoDecision, _ := plan.Video()
	audioDecision, _ := plan.Audio()

	if videoDecision.SourceIndex != 1 || audioDecision.SourceIndex != 3 || audioDecision.SourceCodec != "mp3" {
		t.Errorf("selected video %d and audio %d (%s), want video 1 and audio 3 (mp3)",
			videoDecision.SourceIndex, audioDecision.SourceIndex, audioDecision.SourceCodec)
	}
}

func TestDecideNativeHLSBehavesLikeMSE(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMKV, h264High(0), audio(1, "aac")))
	input.Profile.HLS.Delivery = playback.HLSDeliveryNative

	plan, err := playback.Decide(input)
	assertOutcome(t, plan, err, remux(containerReason()))
}

func TestDecideServerAndClientLimits(t *testing.T) {
	t.Parallel()

	withoutAACEncoder := playbacktest.FullCapabilities()
	withoutAACEncoder.AACEncoder = ""

	withoutH264Encoder := playbacktest.FullCapabilities()
	withoutH264Encoder.H264Encoder = ""

	withoutHLSH264 := playbacktest.ChromiumDesktop()
	withoutHLSH264.HLS.VideoCodecs = []string{"vp9"}

	withoutHLSAAC := playbacktest.ChromiumDesktop()
	withoutHLSAAC.HLS.AudioCodecs = []string{"opus"}

	baselineOnly := playbacktest.ChromiumDesktop()
	baselineOnly.CodecLimits = []playback.CodecLimit{{Codec: "h264", Profiles: []string{"Baseline"}}}

	mpeg2 := clip(media.ContainerMPEGTS, simpleVideo(0, "mpeg2video"), audio(1, "aac"))
	pcm := clip(media.ContainerMOV, h264High(0), audio(1, "pcm_s16le"))
	xvid := clip(media.ContainerAVI, simpleVideo(0, "mpeg4"), audio(1, "mp3"))
	mp2 := clip(media.ContainerMPEGTS, h264High(0), audio(1, "mp2"))
	mpeg2Mp2 := clip(media.ContainerMPEGTS, simpleVideo(0, "mpeg2video"), audio(1, "mp2"))

	tests := []struct {
		name         string
		description  media.Description
		profile      playback.ClientProfile
		capabilities playback.ServerCapabilities
		want         outcome
	}{
		{
			name: "no FFmpeg still plays directly", description: clip(media.ContainerMP4, h264High(0), audio(1, "aac")),
			profile: playbacktest.ChromiumDesktop(), want: directPlay(),
		},
		{
			name: "no FFmpeg cannot remux", description: clip(media.ContainerMKV, h264High(0), audio(1, "aac")),
			profile: playbacktest.ChromiumDesktop(), want: failure(playback.ErrTranscodeUnavailable),
		},
		{
			name: "missing audio encoder", description: pcm, profile: playbacktest.ChromiumDesktop(),
			capabilities: withoutAACEncoder, want: failure(playback.ErrTranscodeUnavailable),
		},
		{
			name: "missing audio encoder does not matter for a video transcode", description: xvid,
			profile: playbacktest.ChromiumDesktop(), capabilities: withoutAACEncoder,
			want: transcode(transcoded, copied, containerReason(), videoCodecReason()),
		},
		{
			name: "missing video encoder", description: xvid, profile: playbacktest.ChromiumDesktop(),
			capabilities: withoutH264Encoder, want: failure(playback.ErrTranscodeUnavailable),
		},
		{
			name: "client cannot play H.264 in HLS", description: mpeg2, profile: withoutHLSH264,
			capabilities: playbacktest.FullCapabilities(), want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name: "client cannot play the H.264 High profile", description: mpeg2, profile: baselineOnly,
			capabilities: playbacktest.FullCapabilities(), want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name: "client cannot play AAC in HLS", description: mp2, profile: withoutHLSAAC,
			capabilities: playbacktest.FullCapabilities(), want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name: "a client limit wins over a missing encoder", description: mpeg2, profile: withoutHLSH264,
			capabilities: playbacktest.NoEncoders(), want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name: "a client limit wins over missing FFmpeg", description: mpeg2, profile: withoutHLSH264,
			want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name: "an audio client limit wins over a missing video encoder", description: mpeg2Mp2,
			profile: withoutHLSAAC, capabilities: withoutH264Encoder, want: failure(playback.ErrNoCompatibleFormat),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(test.description)
			input.Profile = test.profile
			input.Capabilities = test.capabilities

			plan, err := playback.Decide(input)
			assertOutcome(t, plan, err, test.want)
		})
	}
}

func TestDecideRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	description := clip(media.ContainerMP4, h264High(0), asDefault(audio(1, "aac")), subtitle(2, "mov_text"))

	tests := []struct {
		name   string
		modify func(*playback.Input)
	}{
		{name: "invalid profile", modify: func(input *playback.Input) { input.Profile.HLS.Delivery = "" }},
		{name: "invalid description", modify: func(input *playback.Input) { input.Description.Duration = -time.Second }},
		{name: "negative start", modify: func(input *playback.Input) { input.Request.StartPosition = -time.Second }},
		{name: "start at the end", modify: func(input *playback.Input) { input.Request.StartPosition = clipDuration }},
		{name: "negative height cap", modify: func(input *playback.Input) { input.Policy.MaxTranscodeHeight = -1 }},
		{name: "odd height cap", modify: func(input *playback.Input) { input.Policy.MaxTranscodeHeight = 719 }},
		{name: "unknown audio index", modify: func(input *playback.Input) { input.Request.AudioStreamIndex = audioIndex(9) }},
		{name: "audio index of a video stream", modify: func(input *playback.Input) {
			input.Request.AudioStreamIndex = audioIndex(0)
		}},
		{name: "audio index of a subtitle", modify: func(input *playback.Input) {
			input.Request.AudioStreamIndex = audioIndex(2)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(description)
			test.modify(&input)

			_, err := playback.Decide(input)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Decide() error = %v, want domain.ErrInvalid", err)
			}

			// A wrong audio index is a bad request, never a missing resource (HTTP 404).
			if errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Decide() error = %v also wraps domain.ErrNotFound", err)
			}
		})
	}
}

func TestDecideAcceptsBoundaryRequests(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description media.Description
		start       time.Duration
	}{
		{name: "start just before the end", description: clip(media.ContainerMP4, h264High(0)), start: clipDuration - time.Millisecond},
		{name: "unknown duration", description: media.Description{Container: media.ContainerMP4, Streams: []media.Stream{h264High(0)}}, start: time.Hour},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(test.description)
			input.Request.StartPosition = test.start

			mustDecide(t, input)
		})
	}
}

func TestDecideRejectsFilesWithoutAudioAndVideo(t *testing.T) {
	t.Parallel()

	coverArt := media.Stream{Index: 0, Kind: media.StreamKindVideo, Codec: "mjpeg", AttachedPicture: true}

	_, err := playback.Decide(referenceInput(clip(media.ContainerMKV, coverArt, subtitle(1, "subrip"))))

	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("Decide() error = %v, want domain.ErrUnsupported", err)
	}

	if errors.Is(err, playback.ErrNoCompatibleFormat) || errors.Is(err, playback.ErrTranscodeUnavailable) {
		t.Errorf("Decide() error = %v, want neither client nor server code", err)
	}
}

func TestFailureCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		wantCode playback.FailureCode
		wantOK   bool
	}{
		{name: "no compatible format", err: playback.ErrNoCompatibleFormat, wantCode: "no_compatible_format", wantOK: true},
		{name: "transcode unavailable", err: playback.ErrTranscodeUnavailable, wantCode: "transcode_unavailable", wantOK: true},
		{name: "wrapped", err: errors.Join(errors.New("context"), playback.ErrTranscodeUnavailable), wantCode: "transcode_unavailable", wantOK: true},
		{name: "other unsupported", err: domain.ErrUnsupported},
		{name: "nil", err: nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			code, ok := playback.FailureCodeOf(test.err)
			if code != test.wantCode || ok != test.wantOK {
				t.Errorf("FailureCodeOf() = %q, %t, want %q, %t", code, ok, test.wantCode, test.wantOK)
			}

			if test.wantOK && !errors.Is(test.err, domain.ErrUnsupported) {
				t.Errorf("%v does not wrap domain.ErrUnsupported", test.err)
			}
		})
	}
}

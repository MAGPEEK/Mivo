package playback_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// Sample rates and sample depths of the audio sources and limits.
const (
	rate8k       = 8000
	rate44k      = 44100
	rate48k      = 48000
	rate88k      = 88200
	rate96k      = 96000
	rate176k     = 176400
	rate192k     = 192000
	dsd64Rate    = 352800
	dsd64Rate48k = 384000
	unknownRate  = 0
	depth16      = 16
	depth24      = 24
	depth32      = 32
	// depthBelowFLAC is a limit the lossless target cannot meet (FLAC here is 16 or 24 bit).
	depthBelowFLAC = 12
	unknownDepth   = 0
	sourceValue    = 0
)

func dsdReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonDSDToPCM, Scope: playback.ScopeAudio}
}

func hiResReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonHiResDownsampled, Scope: playback.ScopeAudio}
}

// sampled is a stereo audio stream with the given sample rate and sample depth.
func sampled(codec string, sampleRate, bitDepth int) media.Stream {
	stream := audio(1, codec)
	stream.SampleRate, stream.BitDepth = sampleRate, bitDepth

	return stream
}

// audioLimits are the profile fields of ADR 0030 Notes (M4-02b); 0 leaves a field out.
type audioLimits struct {
	channels, sampleRate, bitDepth int
}

// audioCase is one audio stream next to a video Chromium plays, so every reason is about audio.
type audioCase struct {
	name   string
	stream media.Stream
	limits audioLimits
	// lossyOnly removes FLAC from the client's HLS codecs; noFLACEncoder from the server.
	lossyOnly     bool
	noFLACEncoder bool
	want          playback.StreamDecision
	wantReasons   []playback.Reason
}

func copiedAudio(stream media.Stream) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: stream.Index, Action: copied,
		SourceCodec: stream.Codec, TargetCodec: stream.Codec, TargetChannels: stream.Channels,
	}
}

func toAAC(stream media.Stream, channels, sampleRate int) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: stream.Index, Action: transcoded,
		SourceCodec: stream.Codec, TargetCodec: playback.TranscodeAudioCodec,
		TargetChannels: channels, TargetSampleRate: sampleRate, Encoder: playback.EncoderAAC,
	}
}

func toFLAC(stream media.Stream, sampleRate, bitDepth int) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: stream.Index, Action: transcoded,
		SourceCodec: stream.Codec, TargetCodec: playback.TranscodeLosslessAudioCodec,
		TargetChannels: stream.Channels, TargetSampleRate: sampleRate, TargetBitDepth: bitDepth,
		Encoder: playback.EncoderFLAC,
	}
}

func runAudioCases(t *testing.T, tests []audioCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(clip(media.ContainerMP4, h264High(0), test.stream))
			input.Capabilities.FLACEncoder = playback.EncoderFLAC
			input.Profile.MaxAudioChannels = test.limits.channels
			input.Profile.MaxAudioSampleRate = test.limits.sampleRate
			input.Profile.MaxAudioBitDepth = test.limits.bitDepth

			if test.lossyOnly {
				input.Profile.HLS.AudioCodecs = []string{"aac", "mp3", "opus"}
			}

			if test.noFLACEncoder {
				input.Capabilities.FLACEncoder = ""
			}

			plan := mustDecide(t, input)

			if got, _ := plan.Audio(); got != test.want {
				t.Errorf("audio =\n%+v\nwant\n%+v", got, test.want)
			}

			if !slices.Equal(plan.Reasons, test.wantReasons) {
				t.Errorf("Reasons = %v, want %v", plan.Reasons, test.wantReasons)
			}
		})
	}
}

// TestDecideAudioChannelLimit: a profile that plays six channels keeps 5.1 when the audio is
// re-encoded, a smaller limit gets a stereo downmix, and a profile without the field keeps the
// server's default of 5.1 (milestone 2).
func TestDecideAudioChannelLimit(t *testing.T) {
	t.Parallel()

	dts51, dts71 := surround(1, "dts", surround51), surround(1, "dts", surround71)
	dtsStereo, dtsUnknown := surround(1, "dts", stereoChannels), surround(1, "dts", unknownChannels)
	aac51, flac71 := surround(1, "aac", surround51), surround(1, "flac", surround71)
	codec, channels := []playback.Reason{audioCodecReason()}, []playback.Reason{audioChannelsReason()}

	runAudioCases(t, []audioCase{
		{name: "5.1 without a limit stays 5.1", stream: dts51, want: toAAC(dts51, surround51, sourceValue), wantReasons: codec},
		{
			name: "5.1 for a 5.1 client stays 5.1", stream: dts51, limits: audioLimits{channels: surround51},
			want: toAAC(dts51, surround51, sourceValue), wantReasons: codec,
		},
		{
			name: "7.1 for a 7.1 client becomes AAC 5.1", stream: dts71, limits: audioLimits{channels: surround71},
			want: toAAC(dts71, surround51, sourceValue), wantReasons: codec,
		},
		{
			name: "5.1 for a stereo client is downmixed", stream: dts51, limits: audioLimits{channels: stereoChannels},
			want: toAAC(dts51, stereoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "5.1 for a five-channel client is downmixed to stereo", stream: dts51, limits: audioLimits{channels: 5},
			want: toAAC(dts51, stereoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "5.1 for a mono client becomes mono", stream: dts51, limits: audioLimits{channels: monoChannels},
			want: toAAC(dts51, monoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "stereo for a mono client becomes mono", stream: dtsStereo, limits: audioLimits{channels: monoChannels},
			want: toAAC(dtsStereo, monoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "stereo for a 5.1 client stays stereo", stream: dtsStereo, limits: audioLimits{channels: surround51},
			want: toAAC(dtsStereo, stereoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "unknown layout for a 5.1 client becomes stereo", stream: dtsUnknown, limits: audioLimits{channels: surround51},
			want: toAAC(dtsUnknown, stereoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "playable 5.1 above the client's limit is downmixed", stream: aac51, limits: audioLimits{channels: stereoChannels},
			want: toAAC(aac51, stereoChannels, sourceValue), wantReasons: channels,
		},
		{
			name: "playable 5.1 within the client's limit is copied", stream: aac51, limits: audioLimits{channels: surround51},
			want: copiedAudio(aac51),
		},
		{
			name: "playable 7.1 is passed through to a 7.1 client", stream: flac71, limits: audioLimits{channels: surround71},
			want: copiedAudio(flac71),
		},
		{
			name: "playable 7.1 without a limit becomes 5.1", stream: flac71,
			want: toAAC(flac71, surround51, sourceValue), wantReasons: channels,
		},
	})
}

// TestDecideDSD: DSD is never passed through. Clients that play FLAC get 24-bit PCM at 88.2 or
// 96 kHz (the rate of the source's family), others AAC.
func TestDecideDSD(t *testing.T) {
	t.Parallel()

	dsd64 := sampled("dsd_lsbf_planar", dsd64Rate, 1)
	dsd64At48k := sampled("dsd_msbf", dsd64Rate48k, 1)
	dsdUnknownRate := sampled("dsd_lsbf", unknownRate, unknownDepth)
	reasons := []playback.Reason{dsdReason()}

	runAudioCases(t, []audioCase{
		{name: "DSD64 becomes FLAC 24 bit at 88.2 kHz", stream: dsd64, want: toFLAC(dsd64, rate88k, depth24), wantReasons: reasons},
		{
			name: "DSD of the 48 kHz family becomes 96 kHz", stream: dsd64At48k,
			want: toFLAC(dsd64At48k, rate96k, depth24), wantReasons: reasons,
		},
		{
			name: "DSD of unknown rate is treated as DSD64", stream: dsdUnknownRate,
			want: toFLAC(dsdUnknownRate, rate88k, depth24), wantReasons: reasons,
		},
		{
			name: "the client's sample-rate limit applies", stream: dsd64, limits: audioLimits{sampleRate: rate48k},
			want: toFLAC(dsd64, rate44k, depth24), wantReasons: reasons,
		},
		{
			name: "a limit above 96 kHz does not raise the DSD target", stream: dsd64, limits: audioLimits{sampleRate: rate192k},
			want: toFLAC(dsd64, rate88k, depth24), wantReasons: reasons,
		},
		{
			name: "the client's bit-depth limit applies", stream: dsd64, limits: audioLimits{bitDepth: depth16},
			want: toFLAC(dsd64, rate88k, depth16), wantReasons: reasons,
		},
		{
			name: "a client without FLAC gets AAC", stream: dsd64, lossyOnly: true,
			want: toAAC(dsd64, stereoChannels, rate44k), wantReasons: reasons,
		},
		{
			name: "a server without a FLAC encoder sends AAC", stream: dsd64, noFLACEncoder: true,
			want: toAAC(dsd64, stereoChannels, rate44k), wantReasons: reasons,
		},
		{
			name: "a bit-depth limit below FLAC gets AAC", stream: dsd64, limits: audioLimits{bitDepth: depthBelowFLAC},
			want: toAAC(dsd64, stereoChannels, rate44k), wantReasons: reasons,
		},
	})
}

// TestDecideDSDIsNeverCopied: even a client that lists a DSD codec gets PCM, because no
// HLS segment carries DSD.
func TestDecideDSDIsNeverCopied(t *testing.T) {
	t.Parallel()

	stream := sampled("dsd_lsbf_planar", dsd64Rate, 1)
	input := referenceInput(clip(media.ContainerMP4, h264High(0), stream))
	input.Capabilities.FLACEncoder = playback.EncoderFLAC
	input.Profile.DirectPlay[0].AudioCodecs = append(input.Profile.DirectPlay[0].AudioCodecs, stream.Codec)
	input.Profile.HLS.AudioCodecs = append(slices.Clone(input.Profile.HLS.AudioCodecs), stream.Codec)

	plan, err := playback.Decide(input)

	assertOutcome(t, plan, err, transcode(copied, transcoded, dsdReason()))
}

// TestDecideHiResAudio: audio above the profile's sample rate or sample depth is resampled to
// the nearest rate of its family; lossless sources stay lossless for clients that play FLAC.
func TestDecideHiResAudio(t *testing.T) {
	t.Parallel()

	flac96, flac192 := sampled("flac", rate96k, depth24), sampled("flac", rate192k, depth24)
	flac176, flac48 := sampled("flac", rate176k, depth24), sampled("flac", rate48k, depth24)
	flac96At16, flacUnknownDepth := sampled("flac", rate96k, depth16), sampled("flac", rate48k, unknownDepth)
	pcm96, pcm32 := sampled("pcm_s24le", rate96k, depth24), sampled("pcm_s32le", rate192k, depth32)
	aac96, pcmUnknownRate := sampled("aac", rate96k, unknownDepth), sampled("pcm_s16le", unknownRate, depth16)
	hiRes := []playback.Reason{hiResReason()}
	codec, codecAndHiRes := []playback.Reason{audioCodecReason()}, []playback.Reason{audioCodecReason(), hiResReason()}

	runAudioCases(t, []audioCase{
		{name: "no limit copies hi-res audio", stream: flac96, want: copiedAudio(flac96)},
		{
			name: "audio at the limits is copied", stream: flac96, limits: audioLimits{sampleRate: rate96k, bitDepth: depth24},
			want: copiedAudio(flac96),
		},
		{
			name: "unknown depth passes a bit-depth limit", stream: flacUnknownDepth, limits: audioLimits{bitDepth: depth16},
			want: copiedAudio(flacUnknownDepth),
		},
		{
			name: "96 kHz above a 48 kHz limit", stream: flac96, limits: audioLimits{sampleRate: rate48k},
			want: toFLAC(flac96, rate48k, depth24), wantReasons: hiRes,
		},
		{
			name: "192 kHz above a 96 kHz limit", stream: flac192, limits: audioLimits{sampleRate: rate96k},
			want: toFLAC(flac192, rate96k, depth24), wantReasons: hiRes,
		},
		{
			name: "176.4 kHz stays in the 44.1 kHz family", stream: flac176, limits: audioLimits{sampleRate: rate96k},
			want: toFLAC(flac176, rate88k, depth24), wantReasons: hiRes,
		},
		{
			name: "no rate of the family fits the limit", stream: flac96, limits: audioLimits{sampleRate: rate44k},
			want: toFLAC(flac96, rate44k, depth24), wantReasons: hiRes,
		},
		{
			name: "the lowest limit", stream: flac96, limits: audioLimits{sampleRate: rate8k},
			want: toFLAC(flac96, rate8k, depth24), wantReasons: hiRes,
		},
		{
			name: "24 bit above a 16-bit limit keeps its rate", stream: flac48, limits: audioLimits{bitDepth: depth16},
			want: toFLAC(flac48, sourceValue, depth16), wantReasons: hiRes,
		},
		{
			name: "16-bit audio stays 16 bit", stream: flac96At16, limits: audioLimits{sampleRate: rate48k},
			want: toFLAC(flac96At16, rate48k, depth16), wantReasons: hiRes,
		},
		{
			name: "a bit-depth limit below FLAC gets AAC", stream: flac48, limits: audioLimits{bitDepth: depthBelowFLAC},
			want: toAAC(flac48, stereoChannels, sourceValue), wantReasons: hiRes,
		},
		{
			name: "hi-res PCM the client cannot decode becomes FLAC", stream: pcm96,
			limits: audioLimits{sampleRate: rate48k, bitDepth: depth16},
			want:   toFLAC(pcm96, rate48k, depth16), wantReasons: codecAndHiRes,
		},
		{
			name: "32-bit PCM becomes 24-bit FLAC", stream: pcm32, limits: audioLimits{sampleRate: rate96k},
			want: toFLAC(pcm32, rate96k, depth24), wantReasons: codecAndHiRes,
		},
		{
			name: "hi-res PCM for a client without FLAC becomes AAC", stream: pcm96, lossyOnly: true,
			limits: audioLimits{sampleRate: rate48k},
			want:   toAAC(pcm96, stereoChannels, rate48k), wantReasons: codecAndHiRes,
		},
		{
			// AAC is never encoded above 48 kHz; that is the encoder's limit, not the client's,
			// so it is no hires_downsampled.
			name: "PCM without a limit becomes AAC at 48 kHz at most", stream: pcm32,
			want: toAAC(pcm32, stereoChannels, rate48k), wantReasons: codec,
		},
		{
			name: "PCM of unknown rate keeps what the decoder finds", stream: pcmUnknownRate,
			limits: audioLimits{sampleRate: rate48k},
			want:   toAAC(pcmUnknownRate, stereoChannels, sourceValue), wantReasons: codec,
		},
		{
			name: "lossy audio above the limit stays lossy", stream: aac96, limits: audioLimits{sampleRate: rate48k},
			want: toAAC(aac96, stereoChannels, rate48k), wantReasons: hiRes,
		},
	})
}

// TestDecideAudioTargetNeedsAnHLSCodec: when neither FLAC nor AAC plays inside HLS, there is
// nothing the server could send.
func TestDecideAudioTargetNeedsAnHLSCodec(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMP4, h264High(0), sampled("dsd_lsbf_planar", dsd64Rate, 1)))
	input.Capabilities = playbacktest.NoEncoders()
	input.Profile.HLS.AudioCodecs = []string{"opus"}

	plan, err := playback.Decide(input)

	assertOutcome(t, plan, err, failure(playback.ErrNoCompatibleFormat))
}

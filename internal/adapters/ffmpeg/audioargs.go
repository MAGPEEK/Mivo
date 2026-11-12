package ffmpeg

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Audio targets of ADR 0030 section 4: AAC for lossy output, FLAC (PCM in fMP4) for DSD and
// hi-res audio that stays lossless.
const (
	flacCodec = "flac"
	// dsdCodecPrefix starts FFmpeg's DSD decoder names (dsd_lsbf, dsd_msbf and the planar
	// forms). The decoder already decimates DSD64 to 352.8 kHz PCM; aresample does the rest.
	dsdCodecPrefix = "dsd_"
	// dsdFallbackSampleRate is where a DSD stream lands when the plan names no rate: DSD64
	// holds nothing audible above 48 kHz, and a higher rate only stores ultrasonic noise.
	dsdFallbackSampleRate = 96000
	// minAudioSampleRate and maxAudioSampleRate bound a planned target rate; anything else is
	// a bug in the plan, not something to hand to FFmpeg.
	minAudioSampleRate = 8000
	maxAudioSampleRate = 768000
	// flac encodes 16-bit samples from s16 and 24-bit samples from s32 (FFmpeg writes 24 bits
	// for s32 input unless told otherwise).
	flacSampleFormat16 = "s16"
	flacSampleFormat24 = "s32"
	losslessDepth16    = 16
	losslessDepth24    = 24
	// soxrResampler is the aresample option value; it needs an FFmpeg built with libsoxr.
	soxrResampler = "soxr"
	// soxrBuildFlag is the configure flag listed by `ffmpeg -buildconf`.
	soxrBuildFlag  = "--enable-libsoxr"
	buildConfQuery = "-buildconf"
)

// HasSoxResampler reports whether the ffmpeg executable at ffmpegPath was built with libsoxr,
// the high-quality resampler HLSJob.SoxResampler selects. A failed query is an error; the
// caller then uses the default resampler.
func HasSoxResampler(ctx context.Context, ffmpegPath string) (bool, error) {
	ffmpeg := tool{name: ffmpegToolName, executable: ffmpegPath}

	output, err := ffmpeg.output(ctx, []string{hideBannerFlag, buildConfQuery})
	if err != nil {
		return false, fmt.Errorf("detect soxr resampler: ffmpeg %s: %w", buildConfQuery, err)
	}

	return buildConfigurationHasSoxr(output), nil
}

// buildConfigurationHasSoxr matches the configure flag as a whole word: "--enable-libsoxr"
// must not be confused with a flag that only starts like it.
func buildConfigurationHasSoxr(output []byte) bool {
	return slices.Contains(strings.Fields(string(output)), soxrBuildFlag)
}

// audioArguments copies the audio or encodes it to the planned target: AAC, or FLAC for
// DSD and hi-res audio, with the planned channel count, sample rate and bit depth.
func audioArguments(decision playback.StreamDecision, settings encoderSettings) ([]string, error) {
	if decision.Action == playback.ActionCopy {
		if decision.TargetCodec == aacCodec {
			return []string{"-c:a", copyCodec, "-bsf:a", adtsToMP4Bitstream}, nil
		}

		return []string{"-c:a", copyCodec}, nil
	}

	if !isAudioTarget(decision) {
		return nil, fmt.Errorf("audio: no arguments for encoder %q to %q: %w",
			decision.Encoder, decision.TargetCodec, domain.ErrUnsupported)
	}

	if decision.TargetChannels <= 0 {
		return nil, fmt.Errorf("audio: invalid channel count %d: %w", decision.TargetChannels, domain.ErrInvalid)
	}

	arguments := []string{"-c:a", decision.Encoder, "-ac", strconv.Itoa(decision.TargetChannels)}

	if decision.TargetCodec == playback.TranscodeAudioCodec {
		arguments = append(arguments, "-b:a", aacBitRate(decision.TargetChannels))
	}

	filter, err := resampleFilter(decision, settings.soxResampler)
	if err != nil {
		return nil, err
	}

	if filter != "" {
		arguments = append(arguments, "-af", filter)
	}

	if decision.TargetCodec != flacCodec {
		return arguments, nil
	}

	format, err := flacSampleFormat(decision.TargetBitDepth)
	if err != nil {
		return nil, err
	}

	return append(arguments, "-sample_fmt", format), nil
}

// isAudioTarget accepts the two encoder and codec pairs this adapter produces.
func isAudioTarget(decision playback.StreamDecision) bool {
	aac := decision.Encoder == playback.EncoderAAC && decision.TargetCodec == playback.TranscodeAudioCodec
	lossless := decision.Encoder == playback.EncoderFLAC && decision.TargetCodec == flacCodec

	return aac || lossless
}

// resampleFilter returns the aresample filter of a planned rate, or "" when the rate of the
// source is kept. A DSD source always needs one: its decoded rate is 352.8 kHz and up.
func resampleFilter(decision playback.StreamDecision, soxr bool) (string, error) {
	rate := decision.TargetSampleRate
	if rate == 0 && strings.HasPrefix(decision.SourceCodec, dsdCodecPrefix) {
		rate = dsdFallbackSampleRate
	}

	if rate == 0 {
		return "", nil
	}

	if rate < minAudioSampleRate || rate > maxAudioSampleRate {
		return "", fmt.Errorf("audio: sample rate %d outside %d..%d: %w",
			rate, minAudioSampleRate, maxAudioSampleRate, domain.ErrInvalid)
	}

	filter := "aresample=" + strconv.Itoa(rate)
	if soxr {
		filter += ":resampler=" + soxrResampler
	}

	return filter, nil
}

// flacSampleFormat maps the planned depth to the sample format FLAC encodes it from.
func flacSampleFormat(bitDepth int) (string, error) {
	switch bitDepth {
	case losslessDepth16:
		return flacSampleFormat16, nil
	case losslessDepth24:
		return flacSampleFormat24, nil
	default:
		return "", fmt.Errorf("audio: FLAC bit depth %d is neither %d nor %d: %w",
			bitDepth, losslessDepth16, losslessDepth24, domain.ErrInvalid)
	}
}

// aacBitRate scales with the layout: 5.1 at the stereo rate would starve every channel.
func aacBitRate(channels int) string {
	return strconv.FormatInt(aacBitsPerSecond(channels)/bitsPerKilobit, 10) + "k"
}

func aacBitsPerSecond(channels int) int64 {
	if channels > playback.TranscodeStereoChannels {
		return aacSurroundBitsPerSecond
	}

	return aacStereoBitsPerSecond
}

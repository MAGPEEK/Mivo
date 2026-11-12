package ffmpeg_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// audioOnlyJob is an HLS job over one transcoded audio stream (a music file or a film's track).
func audioOnlyJob(t *testing.T, audio playback.StreamDecision, soxResampler bool) ffmpeg.HLSJob {
	t.Helper()

	return ffmpeg.HLSJob{
		Plan:      hlsPlan(playback.ModeTranscode, audio),
		InputPath: hlsAbsolutePath(t, "media", "album", "track.flac"), OutputDirectory: hlsAbsolutePath(t, "session"),
		SoxResampler: soxResampler,
	}
}

func lossyAudio(index, channels, sampleRate int) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: index, Action: playback.ActionTranscode,
		SourceCodec: "ac3", TargetCodec: "aac", TargetChannels: channels, TargetSampleRate: sampleRate,
		Encoder: playback.EncoderAAC,
	}
}

func losslessAudio(sourceCodec string, channels, sampleRate, bitDepth int) playback.StreamDecision {
	return playback.StreamDecision{
		Kind: media.StreamKindAudio, SourceIndex: 0, Action: playback.ActionTranscode,
		SourceCodec: sourceCodec, TargetCodec: "flac", TargetChannels: channels, TargetSampleRate: sampleRate,
		TargetBitDepth: bitDepth, Encoder: playback.EncoderFLAC,
	}
}

// TestHLSArgumentsAudioGolden pins the audio options of every target the plan can produce.
func TestHLSArgumentsAudioGolden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		job  func(t *testing.T) ffmpeg.HLSJob
		want []string
	}{
		{
			name: "AAC 5.1 keeps six channels",
			job:  func(t *testing.T) ffmpeg.HLSJob { return audioOnlyJob(t, lossyAudio(1, 6, 0), false) },
			want: []string{"-map", "0:1", "-c:a", "aac", "-ac", "6", "-b:a", "384k"},
		},
		{
			name: "AAC stereo downmix",
			job:  func(t *testing.T) ffmpeg.HLSJob { return audioOnlyJob(t, lossyAudio(2, 2, 0), false) },
			want: []string{"-map", "0:2", "-c:a", "aac", "-ac", "2", "-b:a", "192k"},
		},
		{
			name: "AAC hi-res downsampled with the default resampler",
			job:  func(t *testing.T) ffmpeg.HLSJob { return audioOnlyJob(t, lossyAudio(0, 2, 48000), false) },
			want: []string{"-map", "0:0", "-c:a", "aac", "-ac", "2", "-b:a", "192k", "-af", "aresample=48000"},
		},
		{
			name: "AAC hi-res downsampled with soxr",
			job:  func(t *testing.T) ffmpeg.HLSJob { return audioOnlyJob(t, lossyAudio(0, 2, 48000), true) },
			want: []string{
				"-map", "0:0", "-c:a", "aac", "-ac", "2", "-b:a", "192k", "-af", "aresample=48000:resampler=soxr",
			},
		},
		{
			name: "FLAC 24-bit 96 kHz to 48 kHz",
			job: func(t *testing.T) ffmpeg.HLSJob {
				return audioOnlyJob(t, losslessAudio("flac", 2, 48000, 24), false)
			},
			want: []string{
				"-map", "0:0", "-c:a", "flac", "-ac", "2", "-af", "aresample=48000", "-sample_fmt", "s32",
			},
		},
		{
			name: "FLAC 16-bit without resampling",
			job:  func(t *testing.T) ffmpeg.HLSJob { return audioOnlyJob(t, losslessAudio("alac", 2, 0, 16), true) },
			want: []string{"-map", "0:0", "-c:a", "flac", "-ac", "2", "-sample_fmt", "s16"},
		},
		{
			name: "FLAC 5.1 stays six channels",
			job:  func(t *testing.T) ffmpeg.HLSJob { return audioOnlyJob(t, losslessAudio("truehd", 6, 0, 24), false) },
			want: []string{"-map", "0:0", "-c:a", "flac", "-ac", "6", "-sample_fmt", "s32"},
		},
		{
			name: "DSD to 96 kHz FLAC with soxr",
			job: func(t *testing.T) ffmpeg.HLSJob {
				return audioOnlyJob(t, losslessAudio("dsd_lsbf_planar", 2, 96000, 24), true)
			},
			want: []string{
				"-map", "0:0", "-c:a", "flac", "-ac", "2", "-af", "aresample=96000:resampler=soxr", "-sample_fmt", "s32",
			},
		},
		{
			name: "DSD without a planned rate falls back to 96 kHz",
			job: func(t *testing.T) ffmpeg.HLSJob {
				return audioOnlyJob(t, losslessAudio("dsd_msbf", 2, 0, 24), false)
			},
			want: []string{
				"-map", "0:0", "-c:a", "flac", "-ac", "2", "-af", "aresample=96000", "-sample_fmt", "s32",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ffmpeg.HLSArguments(test.job(t))
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			start := slices.Index(got, "-map")
			end := slices.Index(got, "-f")

			// The seek and the output muxer are covered by the per-plan-shape test; the maps
			// and codec options sit between them, around the -sn/-dn/metadata options.
			if filtered := dropNeutralOptions(got[start:end]); !slices.Equal(filtered, test.want) {
				t.Errorf("audio arguments = %q, want %q", filtered, test.want)
			}
		})
	}
}

// dropNeutralOptions removes the options every HLS job carries (-sn -dn and the dropped
// metadata and chapters) so the golden lists show only what the audio plan decides.
func dropNeutralOptions(arguments []string) []string {
	neutral := map[string]int{"-sn": 1, "-dn": 1, "-map_metadata": 2, "-map_chapters": 2}

	var kept []string

	for index := 0; index < len(arguments); index++ {
		if width, ok := neutral[arguments[index]]; ok {
			index += width - 1

			continue
		}

		kept = append(kept, arguments[index])
	}

	return kept
}

func TestHLSArgumentsRejectsAudioItCannotProduce(t *testing.T) {
	t.Parallel()

	withTarget := func(decision playback.StreamDecision, mutate func(*playback.StreamDecision)) playback.StreamDecision {
		mutate(&decision)

		return decision
	}

	tests := []struct {
		name  string
		audio playback.StreamDecision
		want  error
	}{
		{"FLAC without a bit depth", losslessAudio("flac", 2, 0, 0), domain.ErrInvalid},
		{"FLAC with 20 bit", losslessAudio("flac", 2, 0, 20), domain.ErrInvalid},
		{"negative sample rate", lossyAudio(0, 2, -1), domain.ErrInvalid},
		{"absurd sample rate", lossyAudio(0, 2, 10_000_000), domain.ErrInvalid},
		{"no channels", losslessAudio("flac", 0, 0, 24), domain.ErrInvalid},
		{"negative stream index", lossyAudio(-1, 2, 0), domain.ErrInvalid},
		{
			"FLAC target with the AAC encoder", withTarget(losslessAudio("flac", 2, 0, 24),
				func(d *playback.StreamDecision) { d.Encoder = playback.EncoderAAC }),
			domain.ErrUnsupported,
		},
		{
			"AAC target with the FLAC encoder", withTarget(lossyAudio(0, 2, 0),
				func(d *playback.StreamDecision) { d.Encoder = playback.EncoderFLAC }),
			domain.ErrUnsupported,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := ffmpeg.HLSArguments(audioOnlyJob(t, test.audio, false))
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}

			if got != nil {
				t.Errorf("arguments %q returned with an error", got)
			}
		})
	}
}

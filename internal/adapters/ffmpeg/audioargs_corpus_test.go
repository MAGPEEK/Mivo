package ffmpeg_test

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	audioCorpusDSDPCMRate   = 88200
	audioCorpusLimitedRate  = 48000
	audioCorpusSurround     = 6
	audioCorpusDSDRate      = 2822400
	audioCorpusDSFBlockSize = 4096
	// audioCorpusDSDSilence is the DSD idle pattern (a 0x69 byte decodes to silence).
	audioCorpusDSDSilence  = 0x69
	audioCorpusDSDDuration = 2
)

// audioCorpusRun is one source decided with the lossless target available and produced as HLS.
type audioCorpusRun struct {
	name         string
	profile      playback.ClientProfile
	wantCodec    string
	wantChannels int
	// wantRate is the sample rate of the output; 0 skips the check.
	wantRate int
}

func hiResLimitedProfile() playback.ClientProfile {
	profile := playbacktest.ChromiumDesktop()
	profile.MaxAudioSampleRate = audioCorpusLimitedRate

	return profile
}

// TestHLSAudioArgumentsProduceTheirPlanWithFFmpeg runs the audio plans of M4-04c through the
// local FFmpeg: 5.1 stays 5.1, 7.1 FLAC is downmixed to the default six channels (as AAC: nothing
// lossless is at stake), hi-res FLAC
// is downsampled, and the first segment probes as planned, with and without soxr.
func TestHLSAudioArgumentsProduceTheirPlanWithFFmpeg(t *testing.T) {
	tools, manifest := requireHLSTools(t)

	soxr, err := ffmpeg.HasSoxResampler(t.Context(), tools.ffmpegPath)
	if err != nil {
		t.Fatalf("HasSoxResampler: %v", err)
	}

	runs := []audioCorpusRun{
		{name: "video-mkv-h264-ac3-51", profile: playbacktest.ChromiumDesktop(), wantCodec: "aac", wantChannels: audioCorpusSurround},
		{name: "video-mkv-h264-flac-71", profile: playbacktest.ChromiumDesktop(), wantCodec: "aac", wantChannels: audioCorpusSurround},
		{
			name: "audio-flac-24bit-96k", profile: hiResLimitedProfile(), wantCodec: "flac", wantChannels: 2,
			wantRate: audioCorpusLimitedRate,
		},
	}

	resamplers := map[string]bool{"default": false}
	if soxr {
		resamplers["soxr"] = true
	}

	for _, run := range runs {
		for resampler, useSoxr := range resamplers {
			t.Run(run.name+"/"+resampler, func(t *testing.T) {
				t.Parallel()

				source := corpusPath(t, corpusCaseByName(t, manifest, run.name).File)
				tools.runAudio(t, source, run, useSoxr)
			})
		}
	}
}

// TestHLSAudioArgumentsDecodeDSD writes a DSD64 DSF (FFmpeg cannot, ADR 0021: the corpus lists
// DSD as a known gap) and checks that it plays as 88.2 kHz FLAC (the plan's rate for DSD64).
func TestHLSAudioArgumentsDecodeDSD(t *testing.T) {
	tools, _ := requireHLSTools(t)
	source := filepath.Join(t.TempDir(), "silence.dsf")

	if err := os.WriteFile(source, buildDSF(2, audioCorpusDSDDuration), 0o600); err != nil {
		t.Fatalf("write DSF: %v", err)
	}

	tools.runAudio(t, source, audioCorpusRun{
		profile: playbacktest.ChromiumDesktop(), wantCodec: "flac", wantChannels: 2, wantRate: audioCorpusDSDPCMRate,
	}, false)
}

// runAudio decides source with the FLAC encoder available, produces it and probes the output.
func (tools hlsTools) runAudio(t *testing.T, source string, run audioCorpusRun, soxr bool) {
	t.Helper()

	description, err := tools.prober.Probe(t.Context(), source)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	capabilities := playbacktest.FullCapabilities()
	capabilities.FLACEncoder = playback.EncoderFLAC

	plan, err := playback.Decide(playback.Input{
		Description: description, Profile: run.profile, Capabilities: capabilities,
		Policy: playback.Policy{MaxTranscodeHeight: hlsMaxHeight},
	})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}

	audio, _ := plan.Audio()
	if audio.TargetCodec != run.wantCodec || audio.TargetChannels != run.wantChannels {
		t.Fatalf("planned audio = %s with %d channels, want %s with %d",
			audio.TargetCodec, audio.TargetChannels, run.wantCodec, run.wantChannels)
	}

	directory := t.TempDir()
	hlsRunFFmpeg(t, tools.ffmpegPath, ffmpeg.HLSJob{
		Plan: plan, InputPath: source, OutputDirectory: directory, SoxResampler: soxr,
	})

	segments := hlsAssertPlaylist(t, directory)
	joined := hlsJoin(t, directory, segments[0])

	output, err := tools.prober.Probe(t.Context(), joined)
	if err != nil {
		t.Fatalf("Probe(first segment): %v", err)
	}

	hlsAssertAudio(t, output, audio)

	stream, _ := output.DefaultAudio()
	if run.wantRate != 0 && stream.SampleRate != run.wantRate {
		t.Errorf("output sample rate = %d, want %d", stream.SampleRate, run.wantRate)
	}

	if output.Duration <= 0 || output.Duration > description.Duration+hlsDurationTolerance*time.Duration(2) {
		t.Errorf("output duration = %s for a %s source", output.Duration, description.Duration)
	}
}

// buildDSF returns a DSD64 DSF file of idle (silent) samples: the DSD, fmt and data chunks
// of the DSF 1.0 specification, little endian, one block of audioCorpusDSFBlockSize bytes per
// channel in turn.
func buildDSF(channels, seconds uint32) []byte {
	const (
		headerChunkSize = 28
		formatChunkSize = 52
		chunkHeaderSize = 12
		stereoType      = 2
		bitsPerSample   = 1
		bitsPerByte     = 8
	)

	samplesPerChannel := audioCorpusDSDRate * seconds
	bytesPerChannel := samplesPerChannel / bitsPerByte
	blocks := (bytesPerChannel + audioCorpusDSFBlockSize - 1) / audioCorpusDSFBlockSize
	data := bytes.Repeat([]byte{audioCorpusDSDSilence}, int(blocks*audioCorpusDSFBlockSize*channels))

	var file bytes.Buffer

	write := func(values ...any) {
		for _, value := range values {
			_ = binary.Write(&file, binary.LittleEndian, value)
		}
	}

	total := uint64(headerChunkSize + formatChunkSize + chunkHeaderSize + len(data))

	file.WriteString("DSD ")
	write(uint64(headerChunkSize), total, uint64(0))
	file.WriteString("fmt ")
	write(uint64(formatChunkSize), uint32(1), uint32(0), uint32(stereoType), channels,
		uint32(audioCorpusDSDRate), uint32(bitsPerSample), uint64(samplesPerChannel),
		uint32(audioCorpusDSFBlockSize), uint32(0))
	file.WriteString("data")
	write(uint64(chunkHeaderSize + len(data)))
	file.Write(data)

	return file.Bytes()
}

package ffmpeg_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The expected arguments are written out literally on purpose: they are the contract with
// FFmpeg and must not follow the implementation's constants.

const (
	goldenForcedKeyframes = "expr:gte(t,n_forced*6)"
	goldenDevice          = "/dev/dri/renderD128"
)

func hardwareEncoderOf(api playback.HardwareAPI, decode bool) playback.HardwareEncoder {
	encoder := playback.HardwareEncoder{
		API: api, Name: "h264_" + string(api), Codec: "h264", Usable: true, Decode: decode,
	}

	if api == playback.HardwareAPIVAAPI || api == playback.HardwareAPIQSV {
		encoder.Device = goldenDevice
	}

	return encoder
}

type hardwareCase struct {
	name       string
	api        playback.HardwareAPI
	decode     bool
	scale      bool
	maxBitrate int64
	// input are the options expected before -i, video the ones after the maps.
	input []string
	video []string
}

func hardwareCases() []hardwareCase {
	return []hardwareCase{
		{
			name: "nvenc uncapped", api: playback.HardwareAPINVENC,
			video: []string{
				"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-cq", "23", "-b:v", "0", "-profile:v", "high",
				"-pix_fmt", "yuv420p", "-force_key_frames", goldenForcedKeyframes, "-forced-idr", "1", "-no-scenecut", "1",
			},
		},
		{
			name: "nvenc capped, scaled, hardware decode", api: playback.HardwareAPINVENC, decode: true, scale: true,
			maxBitrate: 4_000_000, input: []string{"-hwaccel", "cuda"},
			video: []string{
				"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-cq", "23", "-b:v", "3808k", "-maxrate", "3808k",
				"-bufsize", "7616k", "-profile:v", "high", "-pix_fmt", "yuv420p", "-force_key_frames", goldenForcedKeyframes,
				"-forced-idr", "1", "-no-scenecut", "1", "-vf", "scale=1280:720",
			},
		},
		{
			name: "qsv uncapped", api: playback.HardwareAPIQSV,
			input: []string{
				"-init_hw_device", "vaapi=va:" + goldenDevice, "-init_hw_device", "qsv=hw@va", "-filter_hw_device", "hw",
			},
			video: []string{
				"-c:v", "h264_qsv", "-global_quality", "23", "-profile:v", "high", "-force_key_frames", goldenForcedKeyframes,
				"-forced_idr", "1", "-vf", "format=nv12,hwupload=extra_hw_frames=64",
			},
		},
		{
			name: "qsv capped, scaled, hardware decode", api: playback.HardwareAPIQSV, decode: true, scale: true,
			maxBitrate: 2_000_000,
			input: []string{
				"-init_hw_device", "vaapi=va:" + goldenDevice, "-init_hw_device", "qsv=hw@va", "-filter_hw_device", "hw",
				"-hwaccel", "qsv", "-hwaccel_device", "hw",
			},
			video: []string{
				"-c:v", "h264_qsv", "-global_quality", "23", "-b:v", "1808k", "-maxrate", "1808k", "-bufsize", "3616k",
				"-profile:v", "high", "-force_key_frames", goldenForcedKeyframes, "-forced_idr", "1",
				"-vf", "scale=1280:720,format=nv12,hwupload=extra_hw_frames=64",
			},
		},
		{
			name: "vaapi uncapped", api: playback.HardwareAPIVAAPI,
			input: []string{"-vaapi_device", goldenDevice},
			video: []string{
				"-c:v", "h264_vaapi", "-qp", "23", "-profile:v", "high", "-force_key_frames", goldenForcedKeyframes,
				"-vf", "format=nv12,hwupload",
			},
		},
		{
			name: "vaapi capped, scaled, hardware decode", api: playback.HardwareAPIVAAPI, decode: true, scale: true,
			maxBitrate: 1_000_000,
			input:      []string{"-vaapi_device", goldenDevice, "-hwaccel", "vaapi", "-hwaccel_device", goldenDevice},
			video: []string{
				"-c:v", "h264_vaapi", "-rc_mode", "VBR", "-b:v", "808k", "-maxrate", "808k", "-bufsize", "1616k",
				"-profile:v", "high", "-force_key_frames", goldenForcedKeyframes,
				"-vf", "scale=1280:720,format=nv12,hwupload",
			},
		},
		{
			name: "amf uncapped", api: playback.HardwareAPIAMF,
			video: []string{
				"-c:v", "h264_amf", "-quality", "speed", "-rc", "cqp", "-qp_i", "23", "-qp_p", "23", "-qp_b", "23",
				"-profile:v", "high", "-pix_fmt", "yuv420p", "-force_key_frames", goldenForcedKeyframes, "-forced_idr", "1",
			},
		},
		{
			name: "amf capped and scaled never decodes in hardware", api: playback.HardwareAPIAMF, decode: true, scale: true,
			maxBitrate: 8_000_000,
			video: []string{
				"-c:v", "h264_amf", "-quality", "speed", "-rc", "vbr_peak", "-b:v", "7808k", "-maxrate", "7808k",
				"-bufsize", "15616k", "-profile:v", "high", "-pix_fmt", "yuv420p", "-force_key_frames", goldenForcedKeyframes,
				"-forced_idr", "1", "-vf", "scale=1280:720",
			},
		},
	}
}

func hardwareJob(t *testing.T, test hardwareCase) ffmpeg.HLSJob {
	t.Helper()

	encoder := hardwareEncoderOf(test.api, test.decode)

	video := hlsTranscodeVideo(0, 1280, 720, test.scale)
	video.Encoder, video.Hardware, video.MaxBitrate = encoder.Name, true, test.maxBitrate

	return ffmpeg.HLSJob{
		Plan:             hlsPlan(playback.ModeTranscode, video, hlsTranscodeAudio(1)),
		InputPath:        hlsAbsolutePath(t, "media", "film.mkv"),
		OutputDirectory:  hlsAbsolutePath(t, "sessions", "one"),
		HardwareEncoders: []playback.HardwareEncoder{encoder},
	}
}

func TestHLSArgumentsHardwareGolden(t *testing.T) {
	for _, test := range hardwareCases() {
		t.Run(test.name, func(t *testing.T) {
			arguments, err := ffmpeg.HLSArguments(hardwareJob(t, test))
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			// Options before the input.
			inputFlag := slices.Index(arguments, "-i")
			want := slices.Concat([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, test.input)

			if head := arguments[:inputFlag]; !slices.Equal(head[:min(len(head), len(want))], want) {
				t.Errorf("options before -i = %v, want prefix %v", head, want)
			}

			// The video options sit between "-map_chapters -1" and the audio options.
			start := slices.Index(arguments, "-map_chapters") + 2
			end := slices.Index(arguments, "-c:a")

			if got := arguments[start:end]; !slices.Equal(got, test.video) {
				t.Errorf("video options =\n%v\nwant\n%v", got, test.video)
			}
		})
	}
}

func TestHLSArgumentsHardwareKeepsTheSoftwareOutputUntouched(t *testing.T) {
	job := hardwareJob(t, hardwareCases()[0])
	video := hlsTranscodeVideo(0, 1280, 720, false)
	job.Plan = hlsPlan(playback.ModeTranscode, video, hlsTranscodeAudio(1))

	arguments, err := ffmpeg.HLSArguments(job)
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	if slices.Contains(arguments, "-hwaccel") || slices.Contains(arguments, "h264_nvenc") {
		t.Fatalf("software plan carries hardware options: %v", arguments)
	}
}

func TestHLSArgumentsHardwareRefusals(t *testing.T) {
	tests := []struct {
		name   string
		change func(*ffmpeg.HLSJob)
		want   error
	}{
		{"no tested encoders", func(job *ffmpeg.HLSJob) { job.HardwareEncoders = nil }, domain.ErrUnsupported},
		{"untested encoder", func(job *ffmpeg.HLSJob) { job.HardwareEncoders[0].Usable = false }, domain.ErrUnsupported},
		{"wrong codec", func(job *ffmpeg.HLSJob) { job.HardwareEncoders[0].Codec = "hevc" }, domain.ErrUnsupported},
		{"device with a comma", func(job *ffmpeg.HLSJob) { job.HardwareEncoders[0].Device = "/dev/dri/a,b" }, domain.ErrInvalid},
		{"device with a colon", func(job *ffmpeg.HLSJob) { job.HardwareEncoders[0].Device = "/dev/dri/a:b" }, domain.ErrInvalid},
		{"negative cap", func(job *ffmpeg.HLSJob) { job.Plan.Streams[0].MaxBitrate = -1 }, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := hardwareJob(t, hardwareCases()[4])
			test.change(&job)

			if _, err := ffmpeg.HLSArguments(job); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

package ffmpeg_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestHLSArgumentsHardwareBurnInEndsWithTheUploadTail(t *testing.T) {
	image := ffmpeg.BurnIn{Kind: ffmpeg.BurnInImage, StreamIndex: 2}
	text := ffmpeg.BurnIn{Kind: ffmpeg.BurnInText, File: hlsAbsolutePath(t, "media", "film.srt")}

	const (
		imagePrefix  = "[0:0][0:2]overlay,scale=1280:720"
		textPrefix   = "[0:0]scale=1280:720,subtitles=filename="
		textSuffix   = ":si=0"
		vaapiUpload  = ",format=nv12,hwupload[burnedin]"
		qsvUpload    = ",format=nv12,hwupload=extra_hw_frames=64[burnedin]"
		noUploadTail = "[burnedin]"
	)

	tests := []struct {
		name   string
		api    playback.HardwareAPI
		burnIn ffmpeg.BurnIn
		prefix string
		suffix string
	}{
		{"nvenc image", playback.HardwareAPINVENC, image, imagePrefix, noUploadTail},
		{"qsv image", playback.HardwareAPIQSV, image, imagePrefix, qsvUpload},
		{"vaapi image", playback.HardwareAPIVAAPI, image, imagePrefix, vaapiUpload},
		{"amf image", playback.HardwareAPIAMF, image, imagePrefix, noUploadTail},
		{"nvenc text", playback.HardwareAPINVENC, text, textPrefix, textSuffix + noUploadTail},
		{"qsv text", playback.HardwareAPIQSV, text, textPrefix, textSuffix + qsvUpload},
		{"vaapi text", playback.HardwareAPIVAAPI, text, textPrefix, textSuffix + vaapiUpload},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := hardwareJob(t, hardwareCase{api: test.api, scale: true})
			job.BurnIn = &test.burnIn

			graph := filterGraph(t, job)
			if !strings.HasPrefix(graph, test.prefix) || !strings.HasSuffix(graph, test.suffix) {
				t.Errorf("graph = %q, want prefix %q and suffix %q", graph, test.prefix, test.suffix)
			}

			arguments, err := ffmpeg.HLSArguments(job)
			if err != nil {
				t.Fatalf("HLSArguments: %v", err)
			}

			if slices.Contains(arguments, "-vf") {
				t.Errorf("a second -vf next to -filter_complex: %v", arguments)
			}

			// The software fallback of the same job has the graph without any upload.
			job.Plan.Streams[0].Encoder, job.Plan.Streams[0].Hardware = playback.EncoderLibx264, false

			if software := filterGraph(t, job); strings.Contains(software, "hwupload") {
				t.Errorf("software graph %q uploads to a device", software)
			}
		})
	}
}

// filterGraph returns the -filter_complex value of the arguments of job.
func filterGraph(t *testing.T, job ffmpeg.HLSJob) string {
	t.Helper()

	arguments, err := ffmpeg.HLSArguments(job)
	if err != nil {
		t.Fatalf("HLSArguments: %v", err)
	}

	position := slices.Index(arguments, "-filter_complex")
	if position < 0 {
		t.Fatalf("no -filter_complex in %v", arguments)
	}

	return arguments[position+1]
}

func TestHLSArgumentsHardwareRejectsAMissingDevice(t *testing.T) {
	for _, api := range []playback.HardwareAPI{playback.HardwareAPIVAAPI, playback.HardwareAPIQSV} {
		job := hardwareJob(t, hardwareCase{api: api})
		job.HardwareEncoders[0].Device = ""

		if _, err := ffmpeg.HLSArguments(job); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", api, err)
		}
	}
}

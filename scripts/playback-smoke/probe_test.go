package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestParseProbe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		want    outputSummary
		wantErr bool
	}{
		{
			name: "cover picture skipped, default audio preferred",
			json: `{"streams":[
				{"codec_type":"video","codec_name":"mjpeg","disposition":{"attached_pic":1}},
				{"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p"},
				{"codec_type":"audio","codec_name":"mp3","disposition":{"default":0}},
				{"codec_type":"audio","codec_name":"aac","disposition":{"default":1}}],
				"format":{"duration":"2.021000"}}`,
			want: outputSummary{videoCodec: "h264", videoProfile: "High", pixelFormat: "yuv420p", audioCodec: "aac", duration: 2021 * time.Millisecond},
		},
		{
			name: "first audio without a default",
			json: `{"streams":[{"codec_type":"audio","codec_name":"opus"},{"codec_type":"audio","codec_name":"aac"}],"format":{"duration":"1.5"}}`,
			want: outputSummary{audioCodec: "opus", duration: 1500 * time.Millisecond},
		},
		{name: "no duration", json: `{"streams":[],"format":{}}`, wantErr: true},
		{name: "not JSON", json: `ffprobe: error`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseProbe([]byte(test.json))
			if (err != nil) != test.wantErr || got != test.want {
				t.Errorf("parseProbe = %+v, %v; want %+v, error %t", got, err, test.want, test.wantErr)
			}
		})
	}
}

// TestFFprobeProbesACorpusFile runs the real ffprobe against a direct-play file of the
// generated corpus.
func TestFFprobeProbesACorpusFile(t *testing.T) {
	t.Parallel()

	executable, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("skipping the ffprobe integration test: ffprobe is not on PATH (install FFmpeg, see docs/development.md)")
	}

	corpusFile := filepath.Join("..", "..", "testdata", "media", "video", "h264_aac_faststart.mp4")
	if _, err := os.Stat(corpusFile); err != nil {
		t.Skip("skipping the ffprobe integration test: the corpus is missing (generate it with `go run ./scripts/gen-testmedia`)")
	}

	summary, err := ffprobe{executable: executable}.Probe(t.Context(), corpusFile)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}

	if summary.videoCodec != "h264" || summary.audioCodec != "aac" || summary.duration <= 0 {
		t.Errorf("Probe = %+v, want h264 and aac with a duration", summary)
	}
}

func TestFFprobeReportsAFileItCannotRead(t *testing.T) {
	t.Parallel()

	executable, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("skipping the ffprobe integration test: ffprobe is not on PATH (install FFmpeg, see docs/development.md)")
	}

	broken := filepath.Join(t.TempDir(), "broken.mp4")
	if err := os.WriteFile(broken, []byte("not media"), 0o600); err != nil {
		t.Fatalf("write the broken file: %v", err)
	}

	if _, err := (ffprobe{executable: executable}).Probe(t.Context(), broken); err == nil {
		t.Error("Probe of a broken file = nil error, want an error")
	}

	var exitErr *exec.ExitError

	_, err = (ffprobe{executable: filepath.Join(t.TempDir(), "missing-ffprobe")}).Probe(t.Context(), broken)
	if err == nil || errors.As(err, &exitErr) {
		t.Errorf("Probe with a missing executable = %v, want a start error", err)
	}
}

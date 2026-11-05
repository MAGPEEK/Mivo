package ffmpeg

import (
	"bytes"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestCoverStreamIndex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   int
	}{
		{
			name:   "the only attached picture",
			output: `{"streams":[{"index":1,"codec_name":"mjpeg","codec_type":"video","disposition":{"attached_pic":1}}]}`,
			want:   1,
		},
		{
			name: "the front cover wins over an earlier picture",
			output: `{"streams":[
				{"index":1,"codec_name":"png","codec_type":"video","disposition":{"attached_pic":1},"tags":{"comment":"Cover (back)"}},
				{"index":2,"codec_name":"mjpeg","codec_type":"video","disposition":{"attached_pic":1},"tags":{"COMMENT":"cover (front)"}}]}`,
			want: 2,
		},
		{
			name: "without a front cover the first picture wins",
			output: `{"streams":[
				{"index":3,"codec_name":"png","codec_type":"video","disposition":{"attached_pic":1},"tags":{"comment":"Other"}},
				{"index":4,"codec_name":"mjpeg","codec_type":"video","disposition":{"attached_pic":1}}]}`,
			want: 3,
		},
		{
			name: "a real video stream and an unsupported picture codec are skipped",
			output: `{"streams":[
				{"index":0,"codec_name":"h264","codec_type":"video","disposition":{"attached_pic":0}},
				{"index":1,"codec_name":"bmp","codec_type":"video","disposition":{"attached_pic":1}},
				{"index":2,"codec_name":"png","codec_type":"video","disposition":{"attached_pic":1}}]}`,
			want: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := coverStreamIndex([]byte(test.output))
			if err != nil {
				t.Fatalf("coverStreamIndex: %v", err)
			}

			if got != test.want {
				t.Errorf("index = %d, want %d", got, test.want)
			}
		})
	}
}

func TestCoverStreamIndexWithoutCover(t *testing.T) {
	t.Parallel()

	for name, output := range map[string]string{
		"no streams":     `{"streams":[]}`,
		"video only":     `{"streams":[{"index":0,"codec_name":"mjpeg","codec_type":"video","disposition":{"attached_pic":0}}]}`,
		"no disposition": `{"streams":[{"index":0,"codec_name":"png","codec_type":"video"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := coverStreamIndex([]byte(output))
			if !errors.Is(err, ErrNoCover) || !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("error = %v, want ErrNoCover wrapping domain.ErrNotFound", err)
			}
		})
	}
}

func TestCoverStreamIndexRejectsBrokenOutput(t *testing.T) {
	t.Parallel()

	if _, err := coverStreamIndex([]byte(`{`)); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("error = %v, want domain.ErrUnsupported", err)
	}
}

func TestCheckPicture(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		data  []byte
		valid bool
	}{
		{name: "JPEG", data: []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00}, valid: true},
		{name: "PNG", data: []byte("\x89PNG\r\n\x1a\n\x00\x00"), valid: true},
		{name: "empty", data: nil},
		{name: "truncated JPEG marker", data: []byte{0xFF, 0xD8}},
		{name: "BMP", data: []byte("BM\x00\x00\x00\x00")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := checkPicture(test.data)
			if test.valid && err != nil {
				t.Errorf("checkPicture: %v", err)
			}

			if !test.valid && !errors.Is(err, domain.ErrUnsupported) {
				t.Errorf("error = %v, want domain.ErrUnsupported", err)
			}
		})
	}
}

func TestNewCoverExtractorRequiresEveryOption(t *testing.T) {
	t.Parallel()

	valid := CoverExtractorOptions{FFmpegPath: "ffmpeg", FFprobePath: "ffprobe", Logger: slog.New(slog.DiscardHandler)}

	for name, options := range map[string]CoverExtractorOptions{
		"no ffmpeg":  {FFprobePath: valid.FFprobePath, Logger: valid.Logger},
		"no ffprobe": {FFmpegPath: valid.FFmpegPath, Logger: valid.Logger},
		"no logger":  {FFmpegPath: valid.FFmpegPath, FFprobePath: valid.FFprobePath},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := NewCoverExtractor(options); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want domain.ErrInvalid", err)
			}
		})
	}

	if _, err := NewCoverExtractor(valid); err != nil {
		t.Errorf("NewCoverExtractor: %v", err)
	}
}

// fakeCoverLimit is far below what the flooding fake writes.
const fakeCoverLimit = 1 << 10

// The fake tools are configured through the environment, so the next tests run serially.
func TestCopyCoverRefusesOversizedOutput(t *testing.T) {
	extractor := &CoverExtractor{
		ffmpeg: fakeTool(t, fakeModeFloodStdout), logger: slog.New(slog.DiscardHandler),
		timeout: coverTimeout, maxBytes: fakeCoverLimit,
	}

	_, err := extractor.copyCover(t.Context(), absoluteTestPath(t), 1)
	if !errors.Is(err, domain.ErrUnsupported) || !errors.Is(err, errOutputTooLarge) {
		t.Errorf("error = %v, want domain.ErrUnsupported wrapping errOutputTooLarge", err)
	}
}

func TestFindCoverReportsAFailingToolAsUnsupported(t *testing.T) {
	var logs bytes.Buffer

	extractor := &CoverExtractor{
		ffprobe: fakeTool(t, fakeModeFail),
		logger:  slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
		timeout: coverTimeout, maxBytes: maxCoverBytes,
	}

	path := absoluteTestPath(t)

	input, err := inputArguments(path)
	if err != nil {
		t.Fatalf("inputArguments: %v", err)
	}

	if _, err := extractor.findCover(t.Context(), path, input); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("error = %v, want domain.ErrUnsupported", err)
	}

	if !strings.Contains(logs.String(), findCoverOperation) {
		t.Errorf("debug log %q does not name the operation %q", logs.String(), findCoverOperation)
	}
}

// absoluteTestPath is an absolute file path the fakes never open.
func absoluteTestPath(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "song.flac")
}

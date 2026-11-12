package ffmpeg

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

func TestEmbeddedSubtitleArguments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "movie.mkv")

	arguments, err := embeddedSubtitleArguments(path, 2, "subrip")
	if err != nil {
		t.Fatalf("embeddedSubtitleArguments: %v", err)
	}

	for _, want := range [][]string{
		{"-map", "0:s:2"},
		{"-c:s", "webvtt"},
		{"-f", "webvtt", "pipe:1"},
		{"-i", "file:" + path},
		{"-protocol_whitelist", "file"},
	} {
		if !containsRun(arguments, want) {
			t.Errorf("arguments %q lack %q", arguments, want)
		}
	}

	if slices.Contains(arguments, charsetOption) {
		t.Errorf("an embedded stream needs no %s", charsetOption)
	}
}

func TestEmbeddedSubtitleArgumentErrors(t *testing.T) {
	absolute := filepath.Join(t.TempDir(), "movie.mkv")

	tests := []struct {
		name    string
		path    string
		ordinal int
		codec   string
	}{
		{"negative ordinal", absolute, -1, "subrip"},
		{"relative path", "movie.mkv", 0, "subrip"},
		{"pgs", absolute, 0, "hdmv_pgs_subtitle"},
		{"vobsub", absolute, 0, "dvd_subtitle"},
		{"dvb", absolute, 0, "dvb_subtitle"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := embeddedSubtitleArguments(test.path, test.ordinal, test.codec); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestExternalSubtitleArguments(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		charset     string
		wantCharset bool
	}{
		{"utf-8 srt", "srt", "", false},
		{"cp1252 srt", "srt", "CP1252", true},
		{"ass", "ass", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			arguments := externalSubtitleArguments(test.format, test.charset)

			if !containsRun(arguments, []string{"-f", test.format, "-i", "pipe:0"}) {
				t.Errorf("arguments %q do not read %s from standard input", arguments, test.format)
			}

			if got := containsRun(arguments, []string{"-sub_charenc", test.charset}); got != test.wantCharset {
				t.Errorf("charset option present = %t, want %t in %q", got, test.wantCharset, arguments)
			}
		})
	}
}

func TestExternalSubtitleFormat(t *testing.T) {
	tests := []struct {
		path    string
		want    string
		wantErr error
	}{
		{"/media/movie.srt", "srt", nil},
		{"/media/MOVIE.SRT", "srt", nil},
		{"/media/movie.ass", "ass", nil},
		{"/media/movie.ssa", "ass", nil},
		{"/media/movie.vtt", "webvtt", nil},
		{"/media/movie.sup", "", domain.ErrInvalid},
		{"/media/movie.sub", "", domain.ErrInvalid},
		{"/media/movie.idx", "", domain.ErrInvalid},
		{"/media/movie.txt", "", domain.ErrUnsupported},
		{"/media/movie", "", domain.ErrUnsupported},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			got, err := externalSubtitleFormat(test.path)
			if got != test.want || !errors.Is(err, test.wantErr) {
				t.Errorf("externalSubtitleFormat = %q, %v; want %q, %v", got, err, test.want, test.wantErr)
			}
		})
	}
}

func TestSubtitleCharset(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{"empty", nil, ""},
		{"ascii", []byte("hello"), ""},
		{"utf-8", []byte("Grüße"), ""},
		{"utf-8 BOM", []byte("\xEF\xBB\xBFhi"), ""},
		{"utf-16 LE BOM", []byte{0xFF, 0xFE, 'h', 0}, ""},
		{"utf-16 BE BOM", []byte{0xFE, 0xFF, 0, 'h'}, ""},
		{"utf-32 LE BOM", []byte{0xFF, 0xFE, 0, 0, 'h', 0, 0, 0}, ""},
		{"utf-32 BE BOM", []byte{0, 0, 0xFE, 0xFF, 0, 0, 0, 'h'}, ""},
		{"cp1252 umlaut", []byte("Gr\xFC\xDFe"), windows1252},
		{"truncated utf-8", []byte("caf\xC3"), windows1252},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := subtitleCharset(test.data); got != test.want {
				t.Errorf("subtitleCharset = %q, want %q", got, test.want)
			}
		})
	}
}

func containsRun(arguments, run []string) bool {
	for start := 0; start+len(run) <= len(arguments); start++ {
		if slices.Equal(arguments[start:start+len(run)], run) {
			return true
		}
	}

	return false
}

func newFakeExtractor(t *testing.T, mode string, timeout time.Duration) *SubtitleExtractor {
	t.Helper()

	return &SubtitleExtractor{
		ffmpeg: fakeTool(t, mode), logger: slog.New(slog.DiscardHandler), timeout: timeout,
	}
}

func writeTemporaryFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}

	return path
}

func TestNewSubtitleExtractorValidatesOptions(t *testing.T) {
	logger := slog.New(slog.DiscardHandler)

	tests := []struct {
		name    string
		options SubtitleExtractorOptions
	}{
		{"no path", SubtitleExtractorOptions{Logger: logger}},
		{"no logger", SubtitleExtractorOptions{FFmpegPath: "ffmpeg"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewSubtitleExtractor(test.options); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
		})
	}

	if _, err := NewSubtitleExtractor(SubtitleExtractorOptions{FFmpegPath: "ffmpeg", Logger: logger}); err != nil {
		t.Errorf("valid options: %v", err)
	}
}

// A hanging ffmpeg is killed when the timeout ends; the run neither leaks the process nor
// leaves a result file.
func TestExtractKillsAHangingFFmpegAtTheTimeout(t *testing.T) {
	extractor := newFakeExtractor(t, fakeModeHang, 300*time.Millisecond)
	source := writeTemporaryFile(t, "movie.mkv", "x")
	destination := filepath.Join(t.TempDir(), "out.vtt")

	started := time.Now()
	err := extractor.ExtractWebVTT(t.Context(), source, SubtitleTrack{Codec: "subrip"}, destination)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want DeadlineExceeded", err)
	}

	if elapsed := time.Since(started); elapsed > processWaitDelay+time.Second {
		t.Errorf("the kill took %v", elapsed)
	}

	if _, err := os.Stat(destination); err == nil {
		t.Error("a killed extraction left a result file")
	}
}

func TestExtractKillsFFmpegOnCancellation(t *testing.T) {
	extractor := newFakeExtractor(t, fakeModeHang, time.Minute)
	source := writeTemporaryFile(t, "movie.srt", "1\n")
	ctx, cancel := context.WithCancel(t.Context())

	time.AfterFunc(200*time.Millisecond, cancel)

	err := extractor.ExtractWebVTT(ctx, source, SubtitleTrack{External: true}, filepath.Join(t.TempDir(), "out.vtt"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want Canceled", err)
	}
}

func TestExtractClassifiesFailures(t *testing.T) {
	source := writeTemporaryFile(t, "movie.mkv", "x")
	destination := filepath.Join(t.TempDir(), "out.vtt")

	tests := []struct {
		name string
		mode string
		want error
	}{
		{"exit code", fakeModeFail, domain.ErrUnsupported},
		// The fake prints nothing, which is no WebVTT.
		{"empty output", fakeModeRespond, domain.ErrUnsupported},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			extractor := newFakeExtractor(t, test.mode, time.Minute)

			err := extractor.ExtractWebVTT(t.Context(), source, SubtitleTrack{Codec: "subrip"}, destination)
			if !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}

			if err != nil && strings.Contains(err.Error(), fakeFailStderr[:10]) {
				t.Errorf("error %q leaks ffmpeg's error output", err)
			}
		})
	}
}

func TestExtractWritesTheResultAtomically(t *testing.T) {
	directory := t.TempDir()
	destination := filepath.Join(directory, "nested", "out.vtt")

	if err := writeFileAtomically(destination, []byte("WEBVTT\n")); err != nil {
		t.Fatalf("writeFileAtomically: %v", err)
	}

	if err := writeFileAtomically(destination, []byte("WEBVTT\n\nnew")); err != nil {
		t.Fatalf("overwrite: %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(destination))
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != "out.vtt" {
		t.Errorf("directory holds %v, want only out.vtt", entries)
	}

	if data, _ := os.ReadFile(destination); string(data) != "WEBVTT\n\nnew" { //nolint:gosec // temporary directory
		t.Errorf("content = %q", data)
	}
}

func TestExtractRejectsBadRequests(t *testing.T) {
	extractor := newFakeExtractor(t, fakeModeFail, time.Minute)
	absolute := filepath.Join(t.TempDir(), "movie.srt")
	destination := filepath.Join(t.TempDir(), "out.vtt")

	tests := []struct {
		name        string
		source      string
		track       SubtitleTrack
		destination string
		want        error
	}{
		{"relative destination", absolute, SubtitleTrack{External: true}, "out.vtt", domain.ErrInvalid},
		{"relative external source", "movie.srt", SubtitleTrack{External: true}, destination, domain.ErrInvalid},
		{"missing external file", absolute, SubtitleTrack{External: true}, destination, domain.ErrNotFound},
		{"missing directory", filepath.Join(absolute, "x", "movie.srt"), SubtitleTrack{External: true}, destination, domain.ErrNotFound},
		{"unknown format", writeTemporaryFile(t, "movie.txt", "x"), SubtitleTrack{External: true}, destination, domain.ErrUnsupported},
		{"missing media file", absolute, SubtitleTrack{Codec: "subrip"}, destination, domain.ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := extractor.ExtractWebVTT(t.Context(), test.source, test.track, test.destination)
			if !errors.Is(err, test.want) {
				t.Errorf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReadBoundedRefusesAnOversizedFile(t *testing.T) {
	path := writeTemporaryFile(t, "big.srt", strings.Repeat("a", 11))

	if _, err := readBounded(path, 10); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("error = %v, want ErrUnsupported", err)
	}

	if data, err := readBounded(path, 11); err != nil || len(data) != 11 {
		t.Errorf("a file of exactly the limit: %d bytes, %v", len(data), err)
	}
}

package ffmpeg

import (
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func trickplayAbsolute(t *testing.T, name string) string {
	t.Helper()

	path, err := filepath.Abs(name)
	if err != nil {
		t.Fatalf("absolute path: %v", err)
	}

	return path
}

func TestTrickplayArgumentsGolden(t *testing.T) {
	t.Parallel()

	source := trickplayAbsolute(t, "movie.mkv")
	directory := trickplayAbsolute(t, "cache")

	arguments, err := trickplayArguments(trickplayJob{SourcePath: source, VideoStream: 2, OutputDirectory: directory})
	if err != nil {
		t.Fatalf("trickplayArguments: %v", err)
	}

	input, err := ffmpegInputArguments(source)
	if err != nil {
		t.Fatalf("input arguments: %v", err)
	}

	want := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y", "-skip_frame", "nokey"}
	want = append(want, input...)
	want = append(want,
		"-map", "0:2", "-an", "-sn", "-dn", "-map_metadata", "-1",
		"-vf", "fps=1/10:eof_action=pass,scale=160:-2,tile=10x10,format=yuvj420p",
		"-frames:v", "200", "-c:v", "mjpeg", "-q:v", "5", "-start_number", "1", "-f", "image2",
		"file:"+filepath.ToSlash(directory)+"/sheet-%03d.jpg",
	)

	if !slices.Equal(arguments, want) {
		t.Errorf("arguments =\n%q\nwant\n%q", arguments, want)
	}
}

func TestTrickplayArgumentsRefuseBadJobs(t *testing.T) {
	t.Parallel()

	source := trickplayAbsolute(t, "movie.mkv")
	directory := trickplayAbsolute(t, "cache")

	tests := []struct {
		name string
		job  trickplayJob
	}{
		{"negative stream", trickplayJob{SourcePath: source, VideoStream: -1, OutputDirectory: directory}},
		{"relative source", trickplayJob{SourcePath: "movie.mkv", OutputDirectory: directory}},
		{"relative directory", trickplayJob{SourcePath: source, OutputDirectory: "cache"}},
		{"template marker", trickplayJob{SourcePath: source, OutputDirectory: directory + "%03d"}},
		{"device namespace", trickplayJob{SourcePath: source, OutputDirectory: `\\.\pipe\x`}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := trickplayArguments(test.job)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestTrickplayOutputKeepsARootDirectoryWithoutDoubleSeparator(t *testing.T) {
	t.Parallel()

	root := trickplayAbsolute(t, string(filepath.Separator))

	output, err := trickplayOutput(root)
	if err != nil {
		t.Fatalf("trickplayOutput: %v", err)
	}

	if strings.Contains(strings.TrimPrefix(output, fileProtocol), "//") {
		t.Errorf("output %q has a double separator", output)
	}
}

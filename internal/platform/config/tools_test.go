package config_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// A bare program name is kept for the PATH lookup at startup; anything with a path
// separator follows the path rules of ADR 0018.
func TestLoadToolValues(t *testing.T) {
	t.Parallel()

	absolute := filepath.Join(t.TempDir(), "ffmpeg", "bin", "ffmpeg")

	tests := []struct {
		name  string
		value string
		want  func(configDir string) string
	}{
		{name: "bare name", value: "ffmpeg-9", want: func(string) string { return "ffmpeg-9" }},
		{name: "bare name with extension", value: "ffmpeg.exe", want: func(string) string { return "ffmpeg.exe" }},
		{name: "absolute path", value: absolute, want: func(string) string { return absolute }},
		{name: "relative path", value: "./ffmpeg", want: func(configDir string) string { return filepath.Join(configDir, "ffmpeg") }},
		{
			name: "relative path with host separator", value: filepath.Join("tools", "ffmpeg"),
			want: func(configDir string) string { return filepath.Join(configDir, "tools", "ffmpeg") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			f := newFixture(t, "", map[string]string{env("TOOLS_FFMPEG"): tt.value})

			got, err := f.load()
			if err != nil {
				t.Fatalf("Load: %v", err)
			}

			if want := tt.want(f.configDir); got.Tools.FFmpeg != want {
				t.Errorf("Tools.FFmpeg = %q, want %q", got.Tools.FFmpeg, want)
			}
		})
	}
}

// The limit accepts 1 up to the fixed cap of 8 FFmpeg sessions (ADR 0024); 0 and 9 are
// rejected in TestLoadInvalidValues.
func TestLoadTranscodeMaxSessionsBoundaries(t *testing.T) {
	t.Parallel()

	for _, want := range []int{1, 8} {
		f := newFixture(t, "", map[string]string{env("TRANSCODE_MAX_SESSIONS"): strconv.Itoa(want)})

		got, err := f.load()
		if err != nil {
			t.Fatalf("Load(max_sessions %d): %v", want, err)
		}

		if got.Transcode.MaxSessions != want {
			t.Errorf("Transcode.MaxSessions = %d, want %d", got.Transcode.MaxSessions, want)
		}
	}
}

func TestLoadRejectsDriveRelativeToolOnWindows(t *testing.T) {
	t.Parallel()

	if runtime.GOOS != "windows" {
		t.Skip("drive-relative paths exist only on Windows")
	}

	f := newFixture(t, "tools:\n  ffprobe: C:ffprobe.exe\n", nil)

	if _, err := f.load(); !errors.Is(err, paths.ErrDriveRelative) {
		t.Errorf("Load error = %v, want ErrDriveRelative", err)
	}
}

package ffmpeg

import (
	"context"
	"testing"
)

func TestBuildConfigurationHasSoxr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{"listed", "configuration:\n    --enable-gpl\n    --enable-libsoxr\n    --enable-libx264\n", true},
		{"one line", "--enable-gpl --enable-libsoxr --enable-version3", true},
		{"absent", "configuration:\n    --enable-gpl\n    --enable-libx264\n", false},
		{"only a prefix", "--enable-libsoxrx --disable-libsoxr", false},
		{"empty", "", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := buildConfigurationHasSoxr([]byte(test.output)); got != test.want {
				t.Errorf("buildConfigurationHasSoxr = %v, want %v", got, test.want)
			}
		})
	}
}

func TestHasSoxResamplerReportsAFailingTool(t *testing.T) {
	t.Parallel()

	if _, err := HasSoxResampler(context.Background(), "definitely-not-an-ffmpeg-executable"); err == nil {
		t.Fatal("HasSoxResampler succeeded with a missing executable")
	}
}

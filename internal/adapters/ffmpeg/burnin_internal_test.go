package ffmpeg

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestEscapeFilterValueEscapesBothLevels(t *testing.T) {
	t.Parallel()

	// Every character that quotes or structures a filtergraph is escaped twice over: once as
	// part of the option value, once as part of the graph.
	tests := []struct{ name, value, want string }{
		{"plain", "/media/film.srt", "/media/film.srt"},
		{"colon", "C:/media/film.srt", `C\\:/media/film.srt`},
		{"backslash", `/media/a\b.srt`, `/media/a\\\\b.srt`},
		{"quote", "/media/it's.srt", `/media/it\\\'s.srt`},
		{"comma and semicolon", "/media/a,b;c.srt", `/media/a\,b\;c.srt`},
		{"brackets and equals", "/media/[a]=b.srt", `/media/\[a\]\=b.srt`},
		{"injection attempt", "x',drawtext=text=pwned:a='y", `x\\\'\,drawtext\=text\=pwned\\:a\=\\\'y`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := escapeFilterValue(test.value); got != test.want {
				t.Errorf("escapeFilterValue(%q) = %q, want %q", test.value, got, test.want)
			}
		})
	}
}

func TestBurnInForSelectsOnlyEmbeddedImageTracks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		track   *playback.SubtitleTrack
		want    *BurnIn
		wantErr error
	}{
		{name: "no track"},
		{
			name:  "PGS stream 5",
			track: &playback.SubtitleTrack{ID: "s:5", Format: "hdmv_pgs_subtitle"},
			want:  &BurnIn{Kind: BurnInImage, StreamIndex: 5},
		},
		{
			name:    "embedded text needs the app layer",
			track:   &playback.SubtitleTrack{ID: "s:2", Format: "ass", TextBased: true},
			wantErr: domain.ErrUnsupported,
		},
		{
			name:    "external file needs the app layer",
			track:   &playback.SubtitleTrack{ID: "x:abc", Format: "subrip", External: true, TextBased: true},
			wantErr: domain.ErrUnsupported,
		},
		{
			name:    "id that is no stream number",
			track:   &playback.SubtitleTrack{ID: "s:0:scale", Format: "hdmv_pgs_subtitle"},
			wantErr: domain.ErrUnsupported,
		},
		{
			name:    "negative stream number",
			track:   &playback.SubtitleTrack{ID: "s:-1", Format: "hdmv_pgs_subtitle"},
			wantErr: domain.ErrUnsupported,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := burnInFor(test.track)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			if (got == nil) != (test.want == nil) || (got != nil && *got != *test.want) {
				t.Errorf("burnInFor = %+v, want %+v", got, test.want)
			}
		})
	}
}

func TestParseFilterNamesReadsTheFilterColumn(t *testing.T) {
	t.Parallel()

	listing := "Filters:\r\n  T.. = Timeline support\r\n  V = Video input/output\r\n  ------\r\n" +
		" TS overlay           VV->V      Overlay a video source on top of the input.\r\n" +
		" .. subtitles         V->V       Render text subtitles onto input video using the libass library.\r\n" +
		"not an entry\r\n"

	got := parseFilterNames([]byte(listing))
	if want := []string{"overlay", "subtitles"}; !slices.Equal(got, want) {
		t.Errorf("names = %q, want %q", got, want)
	}
}

func TestDetectBurnInTextNeedsTheSubtitlesFilter(t *testing.T) {
	t.Parallel()

	withLibass := "Filters:\n  V = Video\n  ------\n .. subtitles         V->V       libass\n"
	withoutLibass := "Filters:\n  V = Video\n  ------\n TS overlay           VV->V      Overlay\n .. subtitlesx        V->V   x\n"

	tests := []struct {
		name   string
		answer string
		err    error
		want   bool
	}{
		{"libass build", withLibass, nil, true},
		{"build without libass", withoutLibass, nil, false},
		{"query fails", "", errors.New("exit 1"), false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			run := func(_ context.Context, arguments []string) ([]byte, error) {
				if !slices.Equal(arguments, []string{"-hide_banner", "-filters"}) {
					t.Errorf("arguments = %q, want the -filters query", arguments)
				}

				return []byte(test.answer), test.err
			}

			if got := detectBurnInText(t.Context(), run); got != test.want {
				t.Errorf("detectBurnInText = %v, want %v", got, test.want)
			}
		})
	}
}

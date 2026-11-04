package artwork_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestParseKind(t *testing.T) {
	t.Parallel()

	for _, want := range artwork.Kinds() {
		got, err := artwork.ParseKind(want.String())
		if err != nil || got != want {
			t.Errorf("ParseKind(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "Poster", "fanart", "square"} {
		got, err := artwork.ParseKind(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseKind(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

func TestParseSourceKind(t *testing.T) {
	t.Parallel()

	sources := []artwork.SourceKind{
		artwork.SourceLocal, artwork.SourceEmbedded, artwork.SourceProvider, artwork.SourceUpload,
	}

	for _, want := range sources {
		got, err := artwork.ParseSourceKind(want.String())
		if err != nil || got != want {
			t.Errorf("ParseSourceKind(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "Local", "tmdb", "user"} {
		got, err := artwork.ParseSourceKind(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseSourceKind(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

func TestStoredRef(t *testing.T) {
	t.Parallel()

	stored := artwork.Stored{ID: "0b6d", Tag: "0123456789abcdef", Kind: artwork.KindPoster}

	if got, want := stored.Ref(), (artwork.Ref{ID: "0b6d", Tag: "0123456789abcdef"}); got != want {
		t.Errorf("Ref() = %+v, want %+v", got, want)
	}
}

func TestLocationConstructors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		got  artwork.Location
		want artwork.Location
	}{
		{
			name: "local",
			got:  artwork.LocalLocation("Movie/poster.jpg"),
			want: artwork.Location{RelativePath: "Movie/poster.jpg", StreamIndex: artwork.WholeFile},
		},
		{
			name: "embedded",
			got:  artwork.EmbeddedLocation("Movie/Movie.mkv", 0),
			want: artwork.Location{RelativePath: "Movie/Movie.mkv", StreamIndex: 0},
		},
		{
			name: "data directory",
			got:  artwork.DataLocation("artwork/ab/abcd.jpg"),
			want: artwork.Location{DataPath: "artwork/ab/abcd.jpg", StreamIndex: artwork.WholeFile},
		},
	}

	for _, test := range tests {
		if test.got != test.want {
			t.Errorf("%s location = %+v, want %+v", test.name, test.got, test.want)
		}
	}
}

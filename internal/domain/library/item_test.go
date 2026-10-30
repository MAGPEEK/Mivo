package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestItemKindForLibrary(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind library.Kind
		want library.ItemKind
	}{
		{kind: library.KindMovies, want: library.ItemKindMovie},
		{kind: library.KindShows, want: library.ItemKindEpisode},
	}

	for _, test := range tests {
		t.Run(test.kind.String(), func(t *testing.T) {
			t.Parallel()

			got, err := library.ItemKindForLibrary(test.kind)
			if err != nil {
				t.Fatalf("ItemKindForLibrary(%q) error = %v, want nil", test.kind, err)
			}

			if got != test.want {
				t.Errorf("ItemKindForLibrary(%q) = %q, want %q", test.kind, got, test.want)
			}
		})
	}
}

func TestItemKindForLibraryRejectsUnknownKinds(t *testing.T) {
	t.Parallel()

	got, err := library.ItemKindForLibrary("music")
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("ItemKindForLibrary(music) error = %v, want ErrInvalid", err)
	}

	if got != "" {
		t.Errorf("ItemKindForLibrary(music) = %q on error, want empty", got)
	}
}

func TestParseItemKind(t *testing.T) {
	t.Parallel()

	for _, want := range []library.ItemKind{library.ItemKindMovie, library.ItemKindEpisode} {
		got, err := library.ParseItemKind(want.String())
		if err != nil || got != want {
			t.Errorf("ParseItemKind(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "Movie", "movies", "show", "season"} {
		got, err := library.ParseItemKind(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseItemKind(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

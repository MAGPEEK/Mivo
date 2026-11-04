package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// allItemKinds lists every video item kind of ADR 0027 §2.
func allItemKinds() []library.ItemKind {
	return []library.ItemKind{
		library.ItemKindMovie, library.ItemKindShow, library.ItemKindSeason,
		library.ItemKindEpisode, library.ItemKindExtra,
	}
}

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

func TestParseItemKindAcceptsEveryVideoKind(t *testing.T) {
	t.Parallel()

	texts := map[library.ItemKind]string{
		library.ItemKindMovie: "movie", library.ItemKindShow: "show", library.ItemKindSeason: "season",
		library.ItemKindEpisode: "episode", library.ItemKindExtra: "extra",
	}

	for _, want := range allItemKinds() {
		got, err := library.ParseItemKind(texts[want])
		if err != nil || got != want {
			t.Errorf("ParseItemKind(%q) = %q, %v, want %q, nil", texts[want], got, err, want)
		}

		if !got.IsValid() || got.String() != texts[want] {
			t.Errorf("%q: IsValid() = %v, String() = %q", want, got.IsValid(), got.String())
		}
	}
}

func TestParseItemKindRejectsUnknownKinds(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "Movie", "movies", "shows", "SEASON", " extra", "track", "album"} {
		got, err := library.ParseItemKind(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseItemKind(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}

		if library.ItemKind(text).IsValid() {
			t.Errorf("ItemKind(%q).IsValid() = true, want false", text)
		}
	}
}

func TestItemKindOwnsFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind library.ItemKind
		want bool
	}{
		{kind: library.ItemKindMovie, want: true},
		{kind: library.ItemKindEpisode, want: true},
		{kind: library.ItemKindExtra, want: true},
		{kind: library.ItemKindShow, want: false},
		{kind: library.ItemKindSeason, want: false},
		{kind: "", want: false},
		{kind: "track", want: false},
	}

	for _, test := range tests {
		if got := test.kind.OwnsFiles(); got != test.want {
			t.Errorf("ItemKind(%q).OwnsFiles() = %v, want %v", test.kind, got, test.want)
		}
	}
}

func TestValidParentImplementsTheHierarchyTable(t *testing.T) {
	t.Parallel()

	// The allowed pairs of ADR 0027 §2; the empty parent means "top level". Every other pair
	// of known kinds (and the top level) must be rejected.
	allowed := map[[2]library.ItemKind]bool{
		{library.ItemKindMovie, ""}:                       true,
		{library.ItemKindShow, ""}:                        true,
		{library.ItemKindSeason, library.ItemKindShow}:    true,
		{library.ItemKindEpisode, library.ItemKindSeason}: true,
		{library.ItemKindExtra, library.ItemKindMovie}:    true,
		{library.ItemKindExtra, library.ItemKindShow}:     true,
	}

	parents := append([]library.ItemKind{""}, allItemKinds()...)

	for _, child := range allItemKinds() {
		for _, parent := range parents {
			want := allowed[[2]library.ItemKind{child, parent}]
			if got := library.ValidParent(child, parent); got != want {
				t.Errorf("ValidParent(%q, %q) = %v, want %v", child, parent, got, want)
			}
		}
	}
}

func TestValidParentRejectsUnknownKinds(t *testing.T) {
	t.Parallel()

	tests := []struct{ child, parent library.ItemKind }{
		{child: "", parent: ""},
		{child: "track", parent: "album"},
		{child: "movie ", parent: ""},
		{child: library.ItemKindSeason, parent: "shows"},
		{child: library.ItemKindExtra, parent: "unknown"},
	}

	for _, test := range tests {
		if library.ValidParent(test.child, test.parent) {
			t.Errorf("ValidParent(%q, %q) = true, want false", test.child, test.parent)
		}
	}
}

func TestItemKeepsTheMilestoneTwoShape(t *testing.T) {
	t.Parallel()

	// The M2 writer builds items with the original fields only; the v2 fields stay zero.
	item := library.Item{ID: "i", LibraryID: "l", Kind: library.ItemKindMovie, Title: "T", SortTitle: "t"}

	if item.ParentID != "" || item.MatchKey != "" || item.ExtraType != "" {
		t.Errorf("new text fields = %q, %q, %q, want empty", item.ParentID, item.MatchKey, item.ExtraType)
	}

	for name, number := range map[string]library.OptionalNumber{
		"Year": item.Year, "Number": item.Number, "NumberEnd": item.NumberEnd, "AbsoluteNumber": item.AbsoluteNumber,
	} {
		if number.IsSet() {
			t.Errorf("%s is set on a zero item, want absent", name)
		}
	}
}

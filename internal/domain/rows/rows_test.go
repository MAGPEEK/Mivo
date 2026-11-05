package rows_test

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// rowIDPattern is the API pattern of Row.id.
var rowIDPattern = regexp.MustCompile(`^[A-Za-z0-9:._-]+$`)

const libraryID = "3f6c1d2a-9b8e-4f70-8a1c-2d3e4f5a6b7c"

func TestRowIDs(t *testing.T) {
	t.Parallel()

	if got, want := rows.RowID(rows.KindRecentlyAdded, libraryID), "recentlyAdded:"+libraryID; got != want {
		t.Errorf("RowID = %q, want %q", got, want)
	}

	comedy := rows.GenreRowID(libraryID, "Science Fiction & Fantasy")
	if !rowIDPattern.MatchString(comedy) || !strings.HasPrefix(comedy, "moreGenre:"+libraryID+":") {
		t.Errorf("GenreRowID = %q, want moreGenre:<library>:<hash> matching the API pattern", comedy)
	}

	if rows.GenreRowID(libraryID, "COMEDY") != rows.GenreRowID(libraryID, "comedy") {
		t.Error("GenreRowID differs by case")
	}

	if rows.GenreRowID(libraryID, "Drama") == rows.GenreRowID(libraryID, "Comedy") {
		t.Error("GenreRowID is the same for different genres")
	}
}

func TestOverviewPlan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind       library.Kind
		wantKinds  []rows.Kind
		wantListed library.ItemKind
		wantCard   rows.CardFormat
		wantGroups bool
	}{
		{
			kind: library.KindMovies, wantKinds: []rows.Kind{rows.KindRecentlyAdded, rows.KindNewReleases, rows.KindMoreGenre},
			wantListed: library.ItemKindMovie, wantCard: rows.CardPoster,
		},
		{
			kind:       library.KindShows,
			wantKinds:  []rows.Kind{rows.KindRecentlyAdded, rows.KindNewReleases, rows.KindMoreGenre, rows.KindRecentEpisodes},
			wantListed: library.ItemKindShow, wantCard: rows.CardPoster, wantGroups: true,
		},
		{
			kind: library.KindAudiobooks, wantKinds: []rows.Kind{rows.KindRecentlyAdded, rows.KindMoreGenre},
			wantListed: library.ItemKindAudiobook, wantCard: rows.CardPoster,
		},
		{kind: library.KindMusic, wantKinds: []rows.Kind{rows.KindRecentlyAdded}, wantListed: library.ItemKindAlbum, wantCard: rows.CardSquare},
		{kind: library.KindPhotos, wantKinds: []rows.Kind{rows.KindRecentlyAdded}, wantListed: library.ItemKindPhotoAlbum, wantCard: rows.CardSquare},
	}

	for _, test := range tests {
		t.Run(string(test.kind), func(t *testing.T) {
			t.Parallel()

			if got := rows.OverviewKinds(test.kind); !slices.Equal(got, test.wantKinds) {
				t.Errorf("OverviewKinds = %v, want %v", got, test.wantKinds)
			}

			if got := rows.ListedItemKind(test.kind); got != test.wantListed {
				t.Errorf("ListedItemKind = %q, want %q", got, test.wantListed)
			}

			if got := rows.GroupsEpisodes(test.kind); got != test.wantGroups {
				t.Errorf("GroupsEpisodes = %v, want %v", got, test.wantGroups)
			}

			assertCardFormats(t, test.kind, test.wantCard)
		})
	}
}

func assertCardFormats(t *testing.T, kind library.Kind, wantCard rows.CardFormat) {
	t.Helper()

	if got := rows.CardFormatOf(rows.KindRecentlyAdded, kind); got != wantCard {
		t.Errorf("CardFormatOf(recentlyAdded) = %q, want %q", got, wantCard)
	}

	if got := rows.CardFormatOf(rows.KindRecentEpisodes, kind); got != rows.CardEpisode {
		t.Errorf("CardFormatOf(recentEpisodes) = %q, want episode", got)
	}
}

func TestLibraryHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state rows.LibraryState
		want  rows.Hint
	}{
		{name: "empty", state: rows.LibraryState{}, want: rows.HintNoItems},
		{name: "empty while scanning", state: rows.LibraryState{Scanning: true}, want: rows.HintScanning},
		{name: "items", state: rows.LibraryState{HasItems: true, Scanning: true}, want: rows.HintNone},
		{name: "missing metadata", state: rows.LibraryState{HasItems: true, MissingMetadata: true}, want: rows.HintNotEnoughData},
		{name: "empty beats missing metadata", state: rows.LibraryState{MissingMetadata: true}, want: rows.HintNoItems},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := rows.LibraryHint(test.state); got != test.want {
				t.Errorf("LibraryHint = %q, want %q", got, test.want)
			}
		})
	}
}

func TestHomeHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		states []rows.LibraryState
		want   rows.Hint
	}{
		{name: "no libraries", states: nil, want: rows.HintNoLibraries},
		{name: "all empty", states: []rows.LibraryState{{}, {}}, want: rows.HintNoItems},
		{name: "one scanning", states: []rows.LibraryState{{}, {Scanning: true}}, want: rows.HintScanning},
		{name: "one with items", states: []rows.LibraryState{{Scanning: true}, {HasItems: true, MissingMetadata: true}}, want: rows.HintNone},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := rows.HomeHint(test.states); got != test.want {
				t.Errorf("HomeHint = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDateSeed(t *testing.T) {
	t.Parallel()

	berlin := time.FixedZone("CET", 3600)
	tests := []struct {
		name string
		at   time.Time
		want int64
	}{
		{name: "plain date", at: time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC), want: 20261105},
		{name: "local date, not UTC", at: time.Date(2026, time.January, 1, 0, 30, 0, 0, berlin), want: 20260101},
		{name: "end of year", at: time.Date(1999, time.December, 31, 23, 59, 59, 0, time.UTC), want: 19991231},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := rows.DateSeed(test.at); got != test.want {
				t.Errorf("DateSeed = %d, want %d", got, test.want)
			}
		})
	}
}

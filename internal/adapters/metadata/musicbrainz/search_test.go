package musicbrainz_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestSearchAlbumFound(t *testing.T) {
	fake := newFake(t, route{path: "/release-group", fixture: "release_group_search_found.json"})
	query := enrich.MusicQuery{Kind: match.RefKindRelease, Title: " OK  Computer ", Artist: "Radiohead", Year: 1997}

	found, err := newClient(fake, newFakeClock()).Search(t.Context(), query)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(found) != 2 {
		t.Fatalf("candidates = %d, want 2 (entry without a valid MBID dropped)", len(found))
	}

	want := match.Candidate{Ref: mbRef(match.RefKindRelease, okComputerGroup), Title: "OK Computer", Year: 1997, Popularity: 100}
	if found[0] != want {
		t.Errorf("first = %+v, want %+v", found[0], want)
	}

	requests := fake.recorded()
	if len(requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(requests))
	}

	lucene := requests[0].query.Get("query")
	for _, part := range []string{`releasegroup:"OK Computer"`, `artist:"Radiohead"`, "firstreleasedate:1997"} {
		if !strings.Contains(lucene, part) {
			t.Errorf("query %q lacks %q", lucene, part)
		}
	}

	userAgent := requests[0].header.Get("User-Agent")
	if !strings.Contains(userAgent, brand.DisplayName+"/") || !strings.Contains(userAgent, brand.RepositoryURL) {
		t.Errorf("User-Agent = %q, want brand name and contact URL", userAgent)
	}

	fake.assertPrivacy()

	best, auto := match.Decide(match.Rank(match.Query{Title: "OK Computer", Year: 1997}, found))
	if best == nil || !auto || best.Candidate.Ref.ID != okComputerGroup {
		t.Errorf("Decide = %+v, %v; want automatic match of %s", best, auto, okComputerGroup)
	}
}

func TestSearchAlbumAmbiguous(t *testing.T) {
	fake := newFake(t, route{path: "/release-group", fixture: "release_group_search_ambiguous.json"})

	found, err := newClient(fake, newFakeClock()).Search(t.Context(), enrich.MusicQuery{Kind: match.RefKindRelease, Title: "Live", Artist: "Example Band"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(found) != 2 || found[0].Year != 1994 || found[1].Year != 2003 {
		t.Fatalf("candidates = %+v, want both live albums with their years", found)
	}

	if strings.Contains(fake.recorded()[0].query.Get("query"), "firstreleasedate") {
		t.Error("query without year must not filter by year")
	}

	if _, auto := match.Decide(match.Rank(match.Query{Title: "Live"}, found)); auto {
		t.Error("two equal candidates must not match automatically")
	}
}

func TestSearchAlbumNone(t *testing.T) {
	tests := []struct {
		name     string
		year     int
		requests int
	}{
		{"without year", 0, 1},
		{"with year retries without it", 2001, 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFake(t, route{path: "/release-group", fixture: "release_group_search_none.json"})

			found, err := newClient(fake, newFakeClock()).Search(t.Context(),
				enrich.MusicQuery{Kind: match.RefKindRelease, Title: "Unknown", Artist: "Nobody", Year: test.year})
			if err != nil || len(found) != 0 {
				t.Fatalf("Search = %v, %v; want no candidates and no error", found, err)
			}

			requests := fake.recorded()
			if len(requests) != test.requests {
				t.Fatalf("requests = %d, want %d", len(requests), test.requests)
			}

			if strings.Contains(requests[len(requests)-1].query.Get("query"), "firstreleasedate") {
				t.Error("last search must not filter by year")
			}
		})
	}
}

func TestSearchArtist(t *testing.T) {
	fake := newFake(t, route{path: "/artist", fixture: "artist_search.json"})

	found, err := newClient(fake, newFakeClock()).Search(t.Context(),
		enrich.MusicQuery{Kind: match.RefKindArtist, Title: "Radiohead", Artist: "ignored", Year: 1991})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := match.Candidate{Ref: mbRef(match.RefKindArtist, radioheadArtist), Title: "Radiohead", Popularity: 100}
	if len(found) != 2 || found[0] != want {
		t.Fatalf("candidates = %+v, want first %+v", found, want)
	}

	if got := fake.recorded()[0].query.Get("query"); got != `artist:"Radiohead"` {
		t.Errorf("query = %q, want only the artist name", got)
	}

	fake.assertPrivacy()
}

func TestSearchEscapesQuery(t *testing.T) {
	fake := newFake(t, route{path: "/release-group", fixture: "release_group_search_none.json"})

	_, err := newClient(fake, newFakeClock()).Search(t.Context(),
		enrich.MusicQuery{Kind: match.RefKindRelease, Title: `Say "Hi" \o/`, Artist: "AC/DC"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := `releasegroup:"Say \"Hi\" \\o/" AND artist:"AC/DC"`
	if got := fake.recorded()[0].query.Get("query"); got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

func TestSearchRejectsInvalidQueries(t *testing.T) {
	tests := []struct {
		name  string
		query enrich.MusicQuery
	}{
		{"empty title", enrich.MusicQuery{Kind: match.RefKindRelease, Title: "  "}},
		{"movie kind", enrich.MusicQuery{Kind: match.RefKindMovie, Title: "x"}},
		{"unknown kind", enrich.MusicQuery{Title: "x"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFake(t)

			_, err := newClient(fake, newFakeClock()).Search(t.Context(), test.query)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}

			if len(fake.recorded()) != 0 {
				t.Error("an invalid query must not reach MusicBrainz")
			}
		})
	}
}

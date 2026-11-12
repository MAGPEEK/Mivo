package search_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/app/search"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

const testLibraryID = "library-1"

var errBroken = errors.New("broken")

// fakeLibraries is the LibrarySource: the given library IDs are visible.
type fakeLibraries struct {
	ids []string
	err error
}

func (source fakeLibraries) VisibleLibraries(context.Context) ([]library.ManagedLibrary, error) {
	libraries := make([]library.ManagedLibrary, 0, len(source.ids))
	for _, id := range source.ids {
		libraries = append(libraries, library.ManagedLibrary{ID: id})
	}

	return libraries, source.err
}

// indexCall is one recorded Search of the fake index.
type indexCall struct {
	query  string
	filter search.Filter
	limit  int
}

// fakeIndex holds total ranked hits ("item-0", "item-1", ...) and records its calls.
type fakeIndex struct {
	total int
	err   error
	calls []indexCall
}

func (index *fakeIndex) Search(_ context.Context, query string, filter search.Filter, limit int) ([]search.Hit, error) {
	index.calls = append(index.calls, indexCall{query: query, filter: filter, limit: limit})

	if index.err != nil {
		return nil, index.err
	}

	hits := make([]search.Hit, 0, min(index.total, limit))
	for rank := range min(index.total, limit) {
		hits = append(hits, search.Hit{ItemID: fmt.Sprintf("item-%d", rank), LibraryID: testLibraryID, Kind: library.ItemKindMovie})
	}

	return hits, nil
}

// fakeSummaries answers a summary per requested ID except the hidden ones and records the
// restriction it was asked with.
type fakeSummaries struct {
	hidden       []string
	err          error
	restrictions []access.Restriction
}

func (reader *fakeSummaries) ItemSummaries(_ context.Context, itemIDs []string, restriction access.Restriction) ([]applibrary.ItemSummary, error) {
	reader.restrictions = append(reader.restrictions, restriction)

	if reader.err != nil {
		return nil, reader.err
	}

	var summaries []applibrary.ItemSummary

	for _, id := range itemIDs {
		if !slices.Contains(reader.hidden, id) {
			summaries = append(summaries, applibrary.ItemSummary{Item: library.Item{ID: id, LibraryID: testLibraryID}})
		}
	}

	return summaries, nil
}

type fixture struct {
	service   *search.Service
	index     *fakeIndex
	summaries *fakeSummaries
}

func newFixture(t *testing.T, libraries fakeLibraries, total int) fixture {
	t.Helper()

	index := &fakeIndex{total: total}
	summaries := &fakeSummaries{}

	service, err := search.NewService(search.Dependencies{Libraries: libraries, Index: index, Summaries: summaries})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	return fixture{service: service, index: index, summaries: summaries}
}

func oneLibrary() fakeLibraries {
	return fakeLibraries{ids: []string{testLibraryID}}
}

func itemIDs(page search.Page) []string {
	ids := make([]string, 0, len(page.Items))
	for _, summary := range page.Items {
		ids = append(ids, summary.Item.ID)
	}

	return ids
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	complete := search.Dependencies{Libraries: oneLibrary(), Index: &fakeIndex{}, Summaries: &fakeSummaries{}}

	tests := map[string]func(*search.Dependencies){
		"libraries": func(dependencies *search.Dependencies) { dependencies.Libraries = nil },
		"index":     func(dependencies *search.Dependencies) { dependencies.Index = nil },
		"summaries": func(dependencies *search.Dependencies) { dependencies.Summaries = nil },
	}

	for name, remove := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			dependencies := complete
			remove(&dependencies)

			if _, err := search.NewService(dependencies); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("NewService without %s: err = %v, want ErrInvalid", name, err)
			}
		})
	}
}

func TestSearchRoutesTypesToKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		searchType search.Type
		wantKinds  []library.ItemKind
	}{
		{searchType: search.TypeTop, wantKinds: nil},
		{searchType: search.TypeMovies, wantKinds: []library.ItemKind{library.ItemKindMovie}},
		{searchType: search.TypeShows, wantKinds: []library.ItemKind{library.ItemKindShow}},
		{searchType: search.TypeEpisodes, wantKinds: []library.ItemKind{library.ItemKindEpisode}},
		{searchType: search.TypeMusic, wantKinds: []library.ItemKind{library.ItemKindArtist, library.ItemKindAlbum, library.ItemKindTrack}},
		{searchType: search.TypePhotos, wantKinds: []library.ItemKind{library.ItemKindPhotoAlbum, library.ItemKindPhoto}},
		{searchType: search.TypeAudiobooks, wantKinds: []library.ItemKind{library.ItemKindAudiobook}},
	}

	for _, test := range tests {
		t.Run(string(test.searchType), func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, fakeLibraries{ids: []string{testLibraryID, "library-2"}}, 3)

			page, err := fixture.service.Search(context.Background(), search.Request{
				Query: "  matr  ", Type: test.searchType, Restriction: access.Unrestricted(),
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			if got, want := itemIDs(page), []string{"item-0", "item-1", "item-2"}; !slices.Equal(got, want) {
				t.Errorf("items = %v, want %v", got, want)
			}

			assertIndexCall(t, fixture.index.calls[0], test.wantKinds)
		})
	}
}

// assertIndexCall checks that the index got the trimmed query, the kinds of the type and
// both visible libraries.
func assertIndexCall(t *testing.T, call indexCall, wantKinds []library.ItemKind) {
	t.Helper()

	if call.query != "matr" {
		t.Errorf("query = %q, want the trimmed text", call.query)
	}

	if !slices.Equal(call.filter.Kinds, wantKinds) {
		t.Errorf("kinds = %v, want %v", call.filter.Kinds, wantKinds)
	}

	if want := []string{testLibraryID, "library-2"}; !slices.Equal(call.filter.LibraryIDs, want) {
		t.Errorf("libraries = %v, want %v", call.filter.LibraryIDs, want)
	}
}

// followCursors pages through the movies pivot until a page carries no cursor, or one page
// past maxPages, and returns the item IDs in the order received and the number of pages.
func followCursors(t *testing.T, service *search.Service, limit, maxPages int) ([]string, int) {
	t.Helper()

	var (
		seen   []string
		cursor string
		pages  int
	)

	for {
		page, err := service.Search(context.Background(), search.Request{
			Query: "movie", Type: search.TypeMovies, Limit: limit, Cursor: cursor, Restriction: access.Unrestricted(),
		})
		if err != nil {
			t.Fatalf("Search page %d: %v", pages, err)
		}

		seen = append(seen, itemIDs(page)...)
		pages++

		if cursor = page.NextCursor; cursor == "" || pages > maxPages {
			return seen, pages
		}
	}
}

func TestSearchClampsLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		searchType search.Type
		limit      int
		wantItems  int
		wantNext   bool
	}{
		{name: "top default", searchType: search.TypeTop, limit: 0, wantItems: search.TopLimit},
		{name: "top smaller", searchType: search.TypeTop, limit: 3, wantItems: 3},
		{name: "top clamped", searchType: search.TypeTop, limit: 50, wantItems: search.TopLimit},
		{name: "pivot default", searchType: search.TypeMovies, limit: 0, wantItems: search.DefaultPageLimit, wantNext: true},
		{name: "pivot smaller", searchType: search.TypeMovies, limit: 5, wantItems: 5, wantNext: true},
		{name: "pivot clamped", searchType: search.TypeMovies, limit: 5000, wantItems: search.MaxPageLimit, wantNext: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, oneLibrary(), search.MaxLimit)

			page, err := fixture.service.Search(context.Background(), search.Request{
				Query: "movie", Type: test.searchType, Limit: test.limit, Restriction: access.Unrestricted(),
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			if len(page.Items) != test.wantItems {
				t.Errorf("items = %d, want %d", len(page.Items), test.wantItems)
			}

			if got := page.NextCursor != ""; got != test.wantNext {
				t.Errorf("has next cursor = %t, want %t", got, test.wantNext)
			}

			if got, want := fixture.index.calls[0].limit, test.wantItems+1; got != want {
				t.Errorf("index limit = %d, want %d", got, want)
			}
		})
	}
}

// TestSearchPagesThroughRankedHits follows the cursors of a pivot to its end: every hit comes
// once, in rank order, and the last page carries no cursor.
func TestSearchPagesThroughRankedHits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		total     int
		limit     int
		wantPages int
		wantItems int
	}{
		{name: "partial last page", total: 7, limit: 3, wantPages: 3, wantItems: 7},
		{name: "exact pages", total: 6, limit: 3, wantPages: 2, wantItems: 6},
		{name: "one page", total: 2, limit: 3, wantPages: 1, wantItems: 2},
		{name: "ends at the ranked hits of the index", total: 5000, limit: 30, wantPages: 4, wantItems: search.MaxLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, oneLibrary(), test.total)

			seen, pages := followCursors(t, fixture.service, test.limit, test.wantPages)
			if pages != test.wantPages || len(seen) != test.wantItems {
				t.Fatalf("pages = %d with %d items, want %d with %d", pages, len(seen), test.wantPages, test.wantItems)
			}

			for rank, id := range seen {
				if want := fmt.Sprintf("item-%d", rank); id != want {
					t.Fatalf("hit %d = %q, want %q", rank, id, want)
				}
			}
		})
	}
}

func TestSearchRejectsBadRequests(t *testing.T) {
	t.Parallel()

	moviesCursor := nextCursor(t, search.TypeMovies)

	tests := []struct {
		name    string
		request search.Request
		wantErr error
	}{
		{name: "empty query", request: search.Request{Query: "", Type: search.TypeTop}, wantErr: search.ErrQueryTooShort},
		{name: "one character", request: search.Request{Query: " m ", Type: search.TypeTop}, wantErr: search.ErrQueryTooShort},
		{name: "one multi-byte character", request: search.Request{Query: "é", Type: search.TypeTop}, wantErr: search.ErrQueryTooShort},
		{name: "unknown type", request: search.Request{Query: "matr", Type: "people"}, wantErr: search.ErrUnknownType},
		{name: "empty type", request: search.Request{Query: "matr"}, wantErr: search.ErrUnknownType},
		{name: "negative limit", request: search.Request{Query: "matr", Type: search.TypeMovies, Limit: -1}, wantErr: domain.ErrInvalid},
		{name: "cursor on top", request: search.Request{Query: "matr", Type: search.TypeTop, Cursor: moviesCursor}, wantErr: search.ErrInvalidCursor},
		{name: "cursor of another pivot", request: search.Request{Query: "matr", Type: search.TypeShows, Cursor: moviesCursor}, wantErr: search.ErrInvalidCursor},
		{name: "cursor not base64", request: search.Request{Query: "matr", Type: search.TypeMovies, Cursor: "%%%"}, wantErr: search.ErrInvalidCursor},
		{name: "cursor without fields", request: search.Request{Query: "matr", Type: search.TypeMovies, Cursor: "c2VhcmNo"}, wantErr: search.ErrInvalidCursor},
		// base64url of "search\x00movies\x000", "…\x00100" and "…\x00+5".
		{name: "cursor at zero", request: search.Request{Query: "matr", Type: search.TypeMovies, Cursor: "c2VhcmNoAG1vdmllcwAw"}, wantErr: search.ErrInvalidCursor},
		{name: "cursor past the ranked hits", request: search.Request{Query: "matr", Type: search.TypeMovies, Cursor: "c2VhcmNoAG1vdmllcwAxMDA"}, wantErr: search.ErrInvalidCursor},
		{name: "cursor with a sign", request: search.Request{Query: "matr", Type: search.TypeMovies, Cursor: "c2VhcmNoAG1vdmllcwArNQ"}, wantErr: search.ErrInvalidCursor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, oneLibrary(), 10)
			test.request.Restriction = access.Unrestricted()

			_, err := fixture.service.Search(context.Background(), test.request)
			if !errors.Is(err, test.wantErr) || !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("err = %v, want %v wrapping ErrInvalid", err, test.wantErr)
			}

			if len(fixture.index.calls) != 0 {
				t.Errorf("index was searched %d times for a rejected request", len(fixture.index.calls))
			}
		})
	}
}

// nextCursor returns a cursor the service issued for the type.
func nextCursor(t *testing.T, searchType search.Type) string {
	t.Helper()

	fixture := newFixture(t, oneLibrary(), 10)

	page, err := fixture.service.Search(context.Background(), search.Request{
		Query: "matr", Type: searchType, Limit: 2, Restriction: access.Unrestricted(),
	})
	if err != nil || page.NextCursor == "" {
		t.Fatalf("Search: cursor %q, err %v", page.NextCursor, err)
	}

	return page.NextCursor
}

func TestSearchWithoutVisibleLibrariesFindsNothing(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, fakeLibraries{}, 10)

	page, err := fixture.service.Search(context.Background(), search.Request{
		Query: "matr", Type: search.TypeTop, Restriction: access.Unrestricted(),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if page.Items == nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Errorf("page = %+v, want an empty, non-nil list", page)
	}

	if len(fixture.index.calls) != 0 {
		t.Errorf("index was searched without a visible library")
	}
}

func TestSearchWithoutHitsAnswersEmptyList(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, oneLibrary(), 0)

	page, err := fixture.service.Search(context.Background(), search.Request{
		Query: "zzz", Type: search.TypeMovies, Restriction: access.Unrestricted(),
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if page.Items == nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Errorf("page = %+v, want an empty, non-nil list", page)
	}
}

// TestSearchPassesRestriction checks that the viewer's restriction reaches the summaries and
// that the hits it hides are left out, also when it hides the whole page.
func TestSearchPassesRestriction(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		hidden []string
		want   []string
	}{
		{name: "one hidden", hidden: []string{"item-1"}, want: []string{"item-0", "item-2"}},
		{name: "all hidden", hidden: []string{"item-0", "item-1", "item-2"}, want: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, oneLibrary(), 3)
			fixture.summaries.hidden = test.hidden

			restriction := access.Unrestricted()

			page, err := fixture.service.Search(context.Background(), search.Request{
				Query: "matr", Type: search.TypeTop, Restriction: restriction,
			})
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			if got := itemIDs(page); page.Items == nil || !slices.Equal(got, test.want) {
				t.Errorf("items = %v, want %v", got, test.want)
			}

			if len(fixture.summaries.restrictions) != 1 || !fixture.summaries.restrictions[0].AllLibraries() {
				t.Fatalf("summaries were read %d times, want once with the request's restriction", len(fixture.summaries.restrictions))
			}
		})
	}
}

func TestSearchReportsPortFailures(t *testing.T) {
	t.Parallel()

	tooMany := make([]string, 501)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("library-%d", index)
	}

	tests := []struct {
		name      string
		libraries fakeLibraries
		indexErr  error
		readErr   error
		wantText  string
	}{
		{name: "libraries", libraries: fakeLibraries{err: errBroken}, wantText: "list visible libraries"},
		{name: "too many libraries", libraries: fakeLibraries{ids: tooMany}, wantText: "exceed"},
		{name: "index", libraries: oneLibrary(), indexErr: errBroken, wantText: "broken"},
		{name: "index rejects", libraries: oneLibrary(), indexErr: fmt.Errorf("limit: %w", domain.ErrInvalid), wantText: "index rejected"},
		{name: "summaries", libraries: oneLibrary(), readErr: errBroken, wantText: "read hits"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, test.libraries, 3)
			fixture.index.err = test.indexErr
			fixture.summaries.err = test.readErr

			_, err := fixture.service.Search(context.Background(), search.Request{
				Query: "matr", Type: search.TypeTop, Restriction: access.Unrestricted(),
			})
			if err == nil || !strings.Contains(err.Error(), test.wantText) {
				t.Fatalf("err = %v, want one naming %q", err, test.wantText)
			}

			// A port failure is a server fault, never the caller's mistake.
			if errors.Is(err, domain.ErrInvalid) {
				t.Errorf("err = %v wraps ErrInvalid", err)
			}
		})
	}
}

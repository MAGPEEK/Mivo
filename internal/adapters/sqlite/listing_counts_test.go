package sqlite_test

import (
	"encoding/base64"
	"fmt"
	"slices"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

func rawListingCursor(text string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

// facetText renders facet values as "value:count".
func facetText(values []applibrary.FacetValue) []string {
	texts := make([]string, 0, len(values))
	for _, value := range values {
		texts = append(texts, fmt.Sprintf("%s:%d", value.Value, value.Count))
	}

	return texts
}

func TestCountFacetsOfEveryField(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	want := map[library.FacetField][]string{
		library.FacetGenre:            {"Action:1", "Comedy:1", "Drama:2", "War:1"},
		library.FacetYear:             {"1999:1", "2001:1", "2008:1", "2019:1"},
		library.FacetDecade:           {"1990:1", "2000:2", "2010:1"},
		library.FacetContentRating:    {"PG:2", "R:2"},
		library.FacetResolution:       {"sd:1", "720p:1", "1080p:1", "4k:1"},
		library.FacetVideoCodec:       {"h264:1", "hevc:2", "mpeg4:1"},
		library.FacetAudioCodec:       {"aac:2", "eac3:1", "mp3:1", "truehd:1"},
		library.FacetContainer:        {"avi:1", "mkv:3", "mp4:1"},
		library.FacetAudioLanguage:    {"eng:3", "ger:1"},
		library.FacetSubtitleLanguage: {"ger:1"},
	}

	facets, err := fixture.reader.CountFacets(t.Context(), moviesLibraryID, applibrary.FacetRequest{
		Restriction: access.Unrestricted(),
		Fields:      library.FacetFields(), Filter: library.ListingFilter{Kind: library.ItemKindMovie},
	})
	if err != nil {
		t.Fatalf("CountFacets error = %v", err)
	}

	if len(facets) != len(want) {
		t.Fatalf("CountFacets returned %d facets, want %d", len(facets), len(want))
	}

	for index, facet := range facets {
		if facet.Field != library.FacetFields()[index] || facet.Truncated {
			t.Errorf("facet %d = %s (truncated %v), want %s", index, facet.Field, facet.Truncated, library.FacetFields()[index])
		}

		if got := facetText(facet.Values); !slices.Equal(got, want[facet.Field]) {
			t.Errorf("facet %s = %v, want %v", facet.Field, got, want[facet.Field])
		}
	}
}

func TestCountFacetsLeavesOutOnlyTheirOwnFilter(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	facets, err := fixture.reader.CountFacets(t.Context(), moviesLibraryID, applibrary.FacetRequest{
		Restriction: access.Unrestricted(),
		Fields:      []library.FacetField{library.FacetGenre, library.FacetYear, library.FacetContainer},
		Filter:      library.ListingFilter{Kind: library.ItemKindMovie, Genres: []string{"Drama"}, Containers: []string{"mkv"}},
	})
	if err != nil {
		t.Fatalf("CountFacets error = %v", err)
	}

	want := [][]string{
		{"Drama:1", "War:1"}, // the mkv movies: 1917 (War), Alpha (Drama), charlie (none)
		{"2008:1"},           // Drama and mkv: Alpha
		{"avi:1", "mkv:1"},   // the Drama movies: Alpha (mkv), Ärger (avi)
	}

	for index, facet := range facets {
		if got := facetText(facet.Values); !slices.Equal(got, want[index]) {
			t.Errorf("facet %s = %v, want %v", facet.Field, got, want[index])
		}
	}
}

func TestCountFacetsOfShowsCountsEachShowOnce(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	facets, err := fixture.reader.CountFacets(t.Context(), showsLibraryID, applibrary.FacetRequest{
		Restriction: access.Unrestricted(),
		Fields:      []library.FacetField{library.FacetResolution, library.FacetVideoCodec, library.FacetContainer},
		Filter:      library.ListingFilter{Kind: library.ItemKindShow},
	})
	if err != nil {
		t.Fatalf("CountFacets error = %v", err)
	}

	want := [][]string{{"720p:1", "1080p:1", "4k:1"}, {"h264:2", "hevc:1"}, {"mkv:2", "mp4:1"}}
	for index, facet := range facets {
		if got := facetText(facet.Values); !slices.Equal(got, want[index]) {
			t.Errorf("facet %s = %v, want %v", facet.Field, got, want[index])
		}
	}
}

func TestCountFacetsCutsAtTheLimit(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	genres := make([]string, 0, applibrary.MaxFacetValues+1)
	for index := range applibrary.MaxFacetValues + 1 {
		genres = append(genres, fmt.Sprintf("Genre %04d", index))
	}

	insertListingItem(t, fixture.inspection, listingItem{
		id: "m0000099-0000-4000-8000-000000000000", libraryID: moviesLibraryID, kind: "movie",
		title: "Many", sortTitle: "many", genres: genres,
	})

	facets, err := fixture.reader.CountFacets(t.Context(), moviesLibraryID, applibrary.FacetRequest{
		Restriction: access.Unrestricted(),
		Fields:      []library.FacetField{library.FacetGenre}, Filter: library.ListingFilter{Kind: library.ItemKindMovie},
	})
	if err != nil {
		t.Fatalf("CountFacets error = %v", err)
	}

	if len(facets[0].Values) != applibrary.MaxFacetValues || !facets[0].Truncated {
		t.Errorf("genre facet has %d values (truncated %v), want %d and truncated", len(facets[0].Values), facets[0].Truncated, applibrary.MaxFacetValues)
	}
}

func TestCountFirstCharactersFollowsTheTitleOrder(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	tests := []struct {
		name    string
		request applibrary.LetterRequest
		want    []string
	}{
		{name: "ascending", request: applibrary.LetterRequest{Restriction: access.Unrestricted()}, want: []string{"1:1", "a:1", "b:1", "c:1", "ä:1"}},
		{name: "descending", request: applibrary.LetterRequest{Restriction: access.Unrestricted(), Descending: true}, want: []string{"ä:1", "c:1", "b:1", "a:1", "1:1"}},
		{name: "filtered", request: applibrary.LetterRequest{Restriction: access.Unrestricted(), Filter: library.ListingFilter{Genres: []string{"Drama"}}}, want: []string{"a:1", "ä:1"}},
	}
	for _, test := range tests {
		test.request.Filter.Kind = library.ItemKindMovie

		counts, err := fixture.reader.CountFirstCharacters(t.Context(), moviesLibraryID, test.request)
		if err != nil {
			t.Fatalf("%s: CountFirstCharacters error = %v", test.name, err)
		}

		got := make([]string, 0, len(counts))
		for _, count := range counts {
			got = append(got, fmt.Sprintf("%s:%d", count.Character, count.Count))
		}

		if !slices.Equal(got, test.want) {
			t.Errorf("%s: counts = %v, want %v", test.name, got, test.want)
		}
	}
}

func TestListFolderItemsFindsItemsDirectlyInADirectory(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	tests := []struct {
		rootID, directory string
		want              []string
	}{
		{rootID: moviesLibraryID, directory: "", want: []string{movieCharlie + " charlie.mkv", movieAerger + " Ärger.avi"}},
		{rootID: moviesLibraryID, directory: "Alpha (2008)", want: []string{movieAlpha + " Alpha.mkv"}},
		{rootID: moviesLibraryID, directory: "Bravo", want: []string{movieBravo + " Bravo.mp4"}},
		{rootID: moviesLibraryID, directory: "Alph"},
		{rootID: listingSecondRoot, directory: "", want: []string{movie1917 + " 1917.mkv"}},
		{rootID: showsLibraryID, directory: "Show A"},
		{rootID: showsLibraryID, directory: "Show A/Season 1", want: []string{"e0000001-0000-4000-8000-000000000000 e1.mkv", "e0000001-1000-4000-8000-000000000000 e2.mkv"}},
	}
	for _, test := range tests {
		items, err := fixture.reader.ListFolderItems(t.Context(), test.rootID, test.directory, access.Unrestricted())
		if err != nil {
			t.Fatalf("ListFolderItems(%q) error = %v", test.directory, err)
		}

		got := make([]string, 0, len(items))
		for _, item := range items {
			got = append(got, item.ItemID+" "+item.Name)
		}

		slices.Sort(got)

		if !slices.Equal(got, test.want) && (len(got) != 0 || len(test.want) != 0) {
			t.Errorf("ListFolderItems(%q, %q) = %v, want %v", test.rootID, test.directory, got, test.want)
		}
	}
}

func TestItemSummariesKeepTheOrderAndSkipUnknownIDs(t *testing.T) {
	t.Parallel()

	fixture := newListingFixture(t)

	summaries, err := fixture.reader.ItemSummaries(t.Context(), []string{movieBravo, unknownID, movieAlpha}, access.Unrestricted())
	if err != nil {
		t.Fatalf("ItemSummaries error = %v", err)
	}

	if len(summaries) != 2 || summaries[0].Item.ID != movieBravo || summaries[1].Item.ID != movieAlpha {
		t.Fatalf("ItemSummaries = %+v, want Bravo then Alpha", summaries)
	}

	if summaries[0].PrimaryFile.RelativePath != "Bravo/Bravo.mp4" {
		t.Errorf("Bravo primary file = %q, want the first part", summaries[0].PrimaryFile.RelativePath)
	}
}

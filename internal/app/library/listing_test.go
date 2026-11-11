package library_test

import (
	"reflect"
	"slices"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestListItemsCompletesTheFilterKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		libraryID string
		kind      library.ItemKind
		wantKind  library.ItemKind
		wantErr   error
	}{
		{name: "omitted kind is the top-level kind", libraryID: moviesID, wantKind: library.ItemKindMovie},
		{name: "explicit top-level kind", libraryID: showsID, kind: library.ItemKindShow, wantKind: library.ItemKindShow},
		{name: "kind of another library", libraryID: moviesID, kind: library.ItemKindShow, wantErr: applibrary.ErrItemKindMismatch},
		{name: "kind below the top", libraryID: showsID, kind: library.ItemKindEpisode, wantErr: domain.ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			query := library.ListingQuery{Filter: library.ListingFilter{Kind: test.kind}}

			_, err := environment.service(t).ListItems(t.Context(), test.libraryID, library.PageRequest{}, query)
			if test.wantErr != nil {
				assertWrappedError(t, err, test.wantErr, "list items of library")

				if got := environment.items.recordedQueries(); len(got) != 0 {
					t.Errorf("reader queries = %+v, want none", got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ListItems error = %v", err)
			}

			if got := environment.items.recordedQueries(); len(got) != 1 || got[0].Filter.Kind != test.wantKind {
				t.Errorf("reader queries = %+v, want kind %q", got, test.wantKind)
			}
		})
	}
}

func TestListItemsPicksARandomSeedOnlyForAFirstPageWithoutSeed(t *testing.T) {
	t.Parallel()

	random := library.ListingOrder{Sort: library.SortByRandom}
	seeded := library.ListingOrder{Sort: library.SortByRandom, Seed: 7, HasSeed: true}

	tests := []struct {
		name   string
		order  library.ListingOrder
		cursor string
		want   library.ListingOrder
	}{
		{name: "first page without seed", order: random, want: library.ListingOrder{Sort: library.SortByRandom, Seed: 4711, HasSeed: true}},
		{name: "later page keeps the cursor seed", order: random, cursor: "1", want: random},
		{name: "client seed", order: seeded, want: seeded},
		{name: "other sort", order: library.ListingOrder{Sort: library.SortByYear}, want: library.ListingOrder{Sort: library.SortByYear}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			environment.putItems(2)

			_, err := environment.service(t).ListItems(t.Context(), moviesID, library.PageRequest{Cursor: test.cursor},
				library.ListingQuery{Order: test.order})
			if err != nil {
				t.Fatalf("ListItems error = %v", err)
			}

			if got := environment.items.recordedQueries(); len(got) != 1 || got[0].Order != test.want {
				t.Errorf("reader queries = %+v, want order %+v", got, test.want)
			}
		})
	}
}

func TestListItemsMarksOfflineFoldersForTheUnavailableFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		quick library.QuickFilter
		roots applibrary.RootChecker
		want  []string
	}{
		{name: "offline folder", quick: library.QuickUnavailable, roots: availableRoots(), want: []string{"movies-folder"}},
		{name: "online folder", quick: library.QuickUnavailable, roots: availableRoots(moviesRoot)},
		{name: "no root checker", quick: library.QuickUnavailable},
		{name: "other quick filter", quick: library.QuickHDR, roots: availableRoots()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			environment.roots = test.roots

			query := library.ListingQuery{Filter: library.ListingFilter{Quick: test.quick}}
			if _, err := environment.service(t).ListItems(t.Context(), moviesID, library.PageRequest{}, query); err != nil {
				t.Fatalf("ListItems error = %v", err)
			}

			got := environment.items.recordedQueries()
			if len(got) != 1 || !slices.Equal(got[0].Filter.OfflineRootIDs, test.want) {
				t.Errorf("reader queries = %+v, want offline folders %v", got, test.want)
			}
		})
	}
}

func TestListFacetsAsksForTheRequestedFieldsInContractOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		requested []library.FacetField
		want      []library.FacetField
	}{
		{name: "none means all", want: library.FacetFields()},
		{
			name:      "ordered and deduplicated",
			requested: []library.FacetField{library.FacetContainer, library.FacetGenre, library.FacetContainer},
			want:      []library.FacetField{library.FacetGenre, library.FacetContainer},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			environment.listings.facets = []applibrary.Facet{{Field: library.FacetGenre}}

			facets, err := environment.service(t).ListFacets(t.Context(), moviesID, applibrary.FacetRequest{
				Fields: test.requested, Filter: library.ListingFilter{Genres: []string{"Drama"}},
			})
			if err != nil {
				t.Fatalf("ListFacets error = %v", err)
			}

			if !reflect.DeepEqual(facets, environment.listings.facets) {
				t.Errorf("ListFacets = %+v, want the reader's", facets)
			}

			want := applibrary.FacetRequest{
				Fields: test.want, Filter: library.ListingFilter{Kind: library.ItemKindMovie, Genres: []string{"Drama"}},
				Restriction: access.Unrestricted(),
			}
			if got := environment.listings.facetRequests; len(got) != 1 || !reflect.DeepEqual(got[0], want) {
				t.Errorf("reader requests = %+v, want %+v", got, want)
			}
		})
	}
}

func TestListFacetsAndLettersReportErrors(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	service := environment.service(t)

	_, err := service.ListFacets(t.Context(), unknownID, applibrary.FacetRequest{})
	assertWrappedError(t, err, domain.ErrNotFound, "list facets of library")

	_, err = service.ListLetters(t.Context(), unknownID, applibrary.LetterRequest{})
	assertWrappedError(t, err, domain.ErrNotFound, "list letters of library")

	environment.listings.err = errBoom

	_, err = service.ListFacets(t.Context(), moviesID, applibrary.FacetRequest{})
	assertWrappedError(t, err, errBoom, "list facets of library")

	_, err = service.ListLetters(t.Context(), moviesID, applibrary.LetterRequest{})
	assertWrappedError(t, err, errBoom, "list letters of library")

	bare := newTestEnvironment()
	bare.storedMoviesAndShows()

	_, err = bare.service(t).ListFacets(t.Context(), moviesID, applibrary.FacetRequest{})
	assertWrappedError(t, err, domain.ErrUnavailable, "no listing reader")

	_, err = bare.service(t).ListLetters(t.Context(), moviesID, applibrary.LetterRequest{})
	assertWrappedError(t, err, domain.ErrUnavailable, "no listing reader")

	_, err = service.ListLetters(t.Context(), moviesID, applibrary.LetterRequest{Filter: library.ListingFilter{Kind: library.ItemKindShow}})
	assertWrappedError(t, err, applibrary.ErrItemKindMismatch, "list letters of library")
}

func TestListLettersFoldsFirstCharactersAtTheirFirstPosition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		characters []applibrary.CharacterCount
		descending bool
		want       applibrary.LetterIndex
	}{
		{name: "empty", want: applibrary.LetterIndex{}},
		{
			name:       "other script after z still leads the bar",
			characters: []applibrary.CharacterCount{{"a", 1}, {"東", 1}},
			want:       applibrary.LetterIndex{Letters: []applibrary.LetterOffset{{"#", 1}, {"A", 0}}, TotalCount: 2},
		},
		{
			name: "ascending with digits, folding and other scripts",
			characters: []applibrary.CharacterCount{
				{"1", 2}, {"!", 1}, {"a", 3}, {"b", 1}, {"z", 2}, {"ä", 2}, {"東", 1},
			},
			want: applibrary.LetterIndex{
				Letters:    []applibrary.LetterOffset{{"#", 0}, {"A", 3}, {"B", 6}, {"Z", 7}},
				TotalCount: 12,
			},
		},
		{
			name:       "descending",
			descending: true,
			characters: []applibrary.CharacterCount{{"ä", 1}, {"z", 2}, {"a", 1}, {"1", 1}},
			want: applibrary.LetterIndex{
				Letters:    []applibrary.LetterOffset{{"Z", 1}, {"A", 0}, {"#", 4}},
				TotalCount: 5,
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			environment.listings.characters = test.characters

			request := applibrary.LetterRequest{Descending: test.descending}

			index, err := environment.service(t).ListLetters(t.Context(), showsID, request)
			if err != nil {
				t.Fatalf("ListLetters error = %v", err)
			}

			if !reflect.DeepEqual(index, test.want) {
				t.Errorf("ListLetters = %+v, want %+v", index, test.want)
			}

			want := applibrary.LetterRequest{Descending: test.descending, Filter: library.ListingFilter{Kind: library.ItemKindShow}, Restriction: access.Unrestricted()}
			if got := environment.listings.letterRequests; len(got) != 1 || !reflect.DeepEqual(got[0], want) {
				t.Errorf("reader requests = %+v, want %+v", got, want)
			}
		})
	}
}

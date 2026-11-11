package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// IDs of the restriction fixture: six movies (FSK 0, 6, 12, 16, 18 and unrated) and three
// shows with their first episodes.
const (
	ratedZero     = "m0000100-0000-4000-8000-000000000000"
	ratedSix      = "m0000106-0000-4000-8000-000000000000"
	ratedTwelve   = "m0000112-0000-4000-8000-000000000000"
	ratedSixteen  = "m0000116-0000-4000-8000-000000000000"
	ratedEighteen = "m0000118-0000-4000-8000-000000000000"
	unratedMovie  = "m0000199-0000-4000-8000-000000000000"
	kidsShow      = "s0000100-0000-4000-8000-000000000000"
	matureShow    = "s0000116-0000-4000-8000-000000000000"
	unratedShow   = "s0000199-0000-4000-8000-000000000000"
)

// restrictedEpisode is an episode of the restriction fixture: its own rating (empty to
// inherit the show's) and the video of its file.
type restrictedEpisode struct {
	rating, container, codec string
	width, height            int
}

// restrictedEpisodeID returns the ID of the episode with the given number of a fixture show.
func restrictedEpisodeID(showID string, number int) string {
	return fmt.Sprintf("e%s-%d", showID[1:8], number)
}

// newRestrictionFixture seeds movies of every age bucket and an unrated one, spread over
// directories, and three shows: Kids Show (TV-Y) with an inheriting and a TV-MA episode,
// Mature Show (TV-MA) and Unrated Show with a TV-G and an unrated episode. The age buckets
// are computed by the adapter's backfill, as on a real database.
func newRestrictionFixture(t *testing.T) catalogFixture {
	t.Helper()

	fixture := newCatalogFixture(t)

	movies := []struct {
		id, title, rating, path, genre string
	}{
		{ratedZero, "Zero", "FSK 0", "Kids/Zero.mkv", "Family"},
		{ratedSix, "Six", "FSK 6", "Kids/Six.mkv", "Family"},
		{ratedTwelve, "Twelve", "FSK 12", "Teens/Twelve.mkv", "Drama"},
		{ratedSixteen, "Sixteen", "FSK 16", "Adult/Sixteen.mkv", "Action"},
		{ratedEighteen, "Eighteen", "FSK 18", "Adult/Deep/Eighteen.mkv", "Horror"},
		{unratedMovie, "Unrated", "", "Unrated.mkv", "Drama"},
	}
	for index, movie := range movies {
		insertListingItem(t, fixture.inspection, listingItem{
			id: movie.id, libraryID: moviesLibraryID, kind: "movie", title: movie.title, sortTitle: movie.title,
			year: 2001 + index, contentRating: movie.rating, createdAt: int64(index), genres: []string{movie.genre},
		})
		insertListingFile(t, fixture.inspection, listingFile{
			id: "f" + movie.id[1:], itemID: movie.id, libraryID: moviesLibraryID, relativePath: movie.path,
			container: "mkv", sizeBytes: int64(index + 1), streams: []listingStream{video("h264", 1920, 1080, ""), audio("aac", "eng")},
		})
	}

	seedRestrictedShows(t, fixture)

	if err := sqlite.NewLibraryCatalog(fixture.db, time.Now).BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}

	return fixture
}

func seedRestrictedShows(t *testing.T, fixture catalogFixture) {
	t.Helper()

	shows := []struct {
		id, title, rating string
		episodes          []restrictedEpisode
	}{
		{kidsShow, "Kids Show", "TV-Y", []restrictedEpisode{
			{container: "mkv", codec: "h264", width: 1280, height: 720},
			{rating: "TV-MA", container: "mp4", codec: "hevc", width: 3840, height: 2160},
		}},
		{matureShow, "Mature Show", "TV-MA", []restrictedEpisode{{container: "mkv", codec: "h264", width: 1920, height: 1080}}},
		{unratedShow, "Unrated Show", "", []restrictedEpisode{
			{rating: "TV-G", container: "mkv", codec: "h264", width: 1920, height: 1080},
			{container: "mkv", codec: "h264", width: 1920, height: 1080},
		}},
	}

	for index, show := range shows {
		insertListingItem(t, fixture.inspection, listingItem{
			id: show.id, libraryID: showsLibraryID, kind: "show", title: show.title, sortTitle: show.title,
			year: 2010 + index, contentRating: show.rating,
		})

		seasonID := "t" + show.id[1:]
		insertListingItem(t, fixture.inspection, listingItem{
			id: seasonID, libraryID: showsLibraryID, parentID: show.id, kind: "season", title: "Season 1", sortTitle: "season 1", number: 1,
		})

		for episodeIndex, episode := range show.episodes {
			episodeID := restrictedEpisodeID(show.id, episodeIndex+1)
			insertListingItem(t, fixture.inspection, listingItem{
				id: episodeID, libraryID: showsLibraryID, parentID: seasonID, kind: "episode", title: "Episode",
				sortTitle: "episode", number: episodeIndex + 1, contentRating: episode.rating,
			})
			insertListingFile(t, fixture.inspection, listingFile{
				id: "g" + episodeID[1:], itemID: episodeID, libraryID: showsLibraryID, container: episode.container,
				relativePath: fmt.Sprintf("%s/Season 1/e%d.mkv", show.title, episodeIndex+1), sizeBytes: 1,
				streams: []listingStream{video(episode.codec, episode.width, episode.height, "")},
			})
		}
	}
}

// restrictionOf builds the restriction of a viewer with the given access and limit.
func restrictionOf(libraries access.LibraryAccess, limit access.ContentLimit) access.Restriction {
	return access.Viewer{Libraries: libraries, Limit: limit}.Restriction()
}

func maxAgeLimit(t *testing.T, maxAge access.AgeBucket, hideUnrated bool) access.ContentLimit {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d) error = %v", maxAge, err)
	}

	return limit
}

// childrenRestriction is the "Children" preset (up to 6, unrated hidden) on every library.
func childrenRestriction(t *testing.T) access.Restriction {
	t.Helper()

	limit, err := access.LimitForPreset(access.PresetChildren, access.AgeAll)
	if err != nil {
		t.Fatalf("LimitForPreset error = %v", err)
	}

	return restrictionOf(access.AllLibraries(), limit)
}

// folderItemIDs returns the sorted IDs of the items directly in the given directories.
func (fixture catalogFixture) folderItemIDs(t *testing.T, rootID string, restriction access.Restriction, directories ...string) []string {
	t.Helper()

	var ids []string

	for _, directory := range directories {
		items, err := fixture.reader.ListFolderItems(t.Context(), rootID, directory, restriction)
		if err != nil {
			t.Fatalf("ListFolderItems(%q) error = %v", directory, err)
		}

		for _, item := range items {
			ids = append(ids, item.ItemID)
		}
	}

	slices.Sort(ids)

	return ids
}

// facetCount returns the summed item counts of one facet field of the movies library.
func (fixture catalogFixture) facetCount(t *testing.T, field library.FacetField, restriction access.Restriction) int64 {
	t.Helper()

	facets, err := fixture.reader.CountFacets(t.Context(), moviesLibraryID, applibrary.FacetRequest{
		Fields: []library.FacetField{field}, Filter: library.ListingFilter{Kind: library.ItemKindMovie}, Restriction: restriction,
	})
	if err != nil {
		t.Fatalf("CountFacets(%s) error = %v", field, err)
	}

	var count int64
	for _, value := range facets[0].Values {
		count += value.Count
	}

	return count
}

// letterText renders the first characters of the movies library as "character:count".
func (fixture catalogFixture) letterText(t *testing.T, restriction access.Restriction) []string {
	t.Helper()

	counts, err := fixture.reader.CountFirstCharacters(t.Context(), moviesLibraryID, applibrary.LetterRequest{
		Filter: library.ListingFilter{Kind: library.ItemKindMovie}, Restriction: restriction,
	})
	if err != nil {
		t.Fatalf("CountFirstCharacters error = %v", err)
	}

	texts := make([]string, 0, len(counts))
	for _, count := range counts {
		texts = append(texts, fmt.Sprintf("%s:%d", count.Character, count.Count))
	}

	return texts
}

// The movies of the restriction fixture in title order: Eighteen, Six, Sixteen, Twelve,
// Unrated, Zero. Every shape of the listing must show the same movies for a restriction:
// pages in several orders, a page whose filter reads the files (counted in the page query),
// the total count, facets, letters and the folder view.
func TestListingRestrictionShowsTheSameMoviesInEveryShape(t *testing.T) {
	t.Parallel()

	fixture := newRestrictionFixture(t)
	rated := []string{ratedEighteen, ratedSix, ratedSixteen, ratedTwelve, ratedZero}

	tests := []struct {
		name        string
		restriction access.Restriction
		want        []string
		wantLetters []string
	}{
		{
			name: "no restriction", restriction: access.Unrestricted(),
			want:        []string{ratedEighteen, ratedSix, ratedSixteen, ratedTwelve, unratedMovie, ratedZero},
			wantLetters: []string{"E:1", "S:2", "T:1", "U:1", "Z:1"},
		},
		{name: "children", restriction: childrenRestriction(t), want: []string{ratedSix, ratedZero}, wantLetters: []string{"S:1", "Z:1"}},
		{
			name: "young children", restriction: restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.AgeAll, true)),
			want: []string{ratedZero}, wantLetters: []string{"Z:1"},
		},
		{
			name: "up to 12 includes 12 and excludes 16", restriction: restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age12, true)),
			want: []string{ratedSix, ratedTwelve, ratedZero}, wantLetters: []string{"S:1", "T:1", "Z:1"},
		},
		{
			name: "up to 12 with unrated shown", restriction: restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age12, false)),
			want: []string{ratedSix, ratedTwelve, unratedMovie, ratedZero}, wantLetters: []string{"S:1", "T:1", "U:1", "Z:1"},
		},
		{
			name: "up to 18 hides only unrated", restriction: restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age18, true)),
			want: rated, wantLetters: []string{"E:1", "S:2", "T:1", "Z:1"},
		},
		{
			name: "no maximum with unrated hidden", restriction: restrictionOf(access.AllLibraries(), access.NoLimit().HidingUnrated(true)),
			want: rated, wantLetters: []string{"E:1", "S:2", "T:1", "Z:1"},
		},
		{
			name: "only the movies library", restriction: restrictionOf(access.OnlyLibraries(moviesLibraryID), access.NoLimit()),
			want:        []string{ratedEighteen, ratedSix, ratedSixteen, ratedTwelve, unratedMovie, ratedZero},
			wantLetters: []string{"E:1", "S:2", "T:1", "U:1", "Z:1"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture.assertMoviesInEveryShape(t, test.restriction, test.want)

			if got := fixture.letterText(t, test.restriction); !slices.Equal(got, test.wantLetters) {
				t.Errorf("letters = %v, want %v", got, test.wantLetters)
			}
		})
	}
}

// assertMoviesInEveryShape checks pages, counts, facets and folders against want, the
// visible movies in title order.
func (fixture catalogFixture) assertMoviesInEveryShape(t *testing.T, restriction access.Restriction, want []string) {
	t.Helper()

	sorted := slices.Sorted(slices.Values(want))
	total := int64(len(want))
	movies := library.ListingFilter{Kind: library.ItemKindMovie}

	queries := map[string]library.ListingQuery{
		"title":       {Filter: movies},
		"year":        {Filter: movies, Order: library.ListingOrder{Sort: library.SortByYear, Descending: true}},
		"file size":   {Filter: movies, Order: library.ListingOrder{Sort: library.SortByFileSize}},
		"file filter": {Filter: library.ListingFilter{Kind: library.ItemKindMovie, Containers: []string{"mkv"}}},
	}
	for name, query := range queries {
		query.Restriction = restriction

		got, gotTotal := fixture.listAllIDs(t, moviesLibraryID, query, 1)
		if name == "title" && !slices.Equal(got, want) {
			t.Errorf("title listing = %v, want %v", got, want)
		}

		slices.Sort(got)

		if !slices.Equal(got, sorted) || gotTotal != total {
			t.Errorf("%s listing = %v (total %d), want %v (total %d)", name, got, gotTotal, sorted, total)
		}
	}

	// Every fixture movie has one genre and one container, so these facets count each
	// visible movie once.
	for _, field := range []library.FacetField{library.FacetGenre, library.FacetContainer, library.FacetYear} {
		if got := fixture.facetCount(t, field, restriction); got != total {
			t.Errorf("facet %s counts %d movies, want %d", field, got, total)
		}
	}

	folders := fixture.folderItemIDs(t, moviesLibraryID, restriction, "", "Kids", "Teens", "Adult", "Adult/Deep")
	if !slices.Equal(folders, sorted) {
		t.Errorf("folder items = %v, want %v", folders, sorted)
	}
}

func TestListingRestrictionContentRatingFacetListsOnlyVisibleRatings(t *testing.T) {
	t.Parallel()

	fixture := newRestrictionFixture(t)

	facets, err := fixture.reader.CountFacets(t.Context(), moviesLibraryID, applibrary.FacetRequest{
		Fields: []library.FacetField{library.FacetContentRating, library.FacetGenre}, Restriction: childrenRestriction(t),
		Filter: library.ListingFilter{Kind: library.ItemKindMovie, Genres: []string{"Family"}},
	})
	if err != nil {
		t.Fatalf("CountFacets error = %v", err)
	}

	if got, want := facetText(facets[0].Values), []string{"FSK 0:1", "FSK 6:1"}; !slices.Equal(got, want) {
		t.Errorf("content rating facet = %v, want %v", got, want)
	}

	// The genre facet leaves out its own filter, but never the restriction.
	if got, want := facetText(facets[1].Values), []string{"Family:2"}; !slices.Equal(got, want) {
		t.Errorf("genre facet = %v, want %v", got, want)
	}
}

// A show is listed by its own bucket; its episodes above the limit do not decide its
// technical filters and facets, and the folder view lists only the episodes whose whole
// chain passes the limit.
func TestListingRestrictionOfShowsAndTheirEpisodes(t *testing.T) {
	t.Parallel()

	fixture := newRestrictionFixture(t)
	children := childrenRestriction(t)
	shows := library.ListingFilter{Kind: library.ItemKindShow}

	if got, total := fixture.listAllIDs(t, showsLibraryID, library.ListingQuery{Filter: shows, Restriction: children}, 1); !slices.Equal(got, []string{kidsShow}) || total != 1 {
		t.Errorf("children's shows = %v (total %d), want only the kids show", got, total)
	}

	tests := []struct {
		name        string
		restriction access.Restriction
		wantCodecs  []string
		wantHEVC    []string
	}{
		{name: "children", restriction: children, wantCodecs: []string{"h264:1"}},
		{name: "no restriction", restriction: access.Unrestricted(), wantCodecs: []string{"h264:3", "hevc:1"}, wantHEVC: []string{kidsShow}},
	}
	for _, test := range tests {
		facets, err := fixture.reader.CountFacets(t.Context(), showsLibraryID, applibrary.FacetRequest{
			Fields: []library.FacetField{library.FacetVideoCodec}, Filter: shows, Restriction: test.restriction,
		})
		if err != nil {
			t.Fatalf("%s: CountFacets error = %v", test.name, err)
		}

		if got := facetText(facets[0].Values); !slices.Equal(got, test.wantCodecs) {
			t.Errorf("%s: video codec facet = %v, want %v", test.name, got, test.wantCodecs)
		}

		hevc := library.ListingQuery{Filter: library.ListingFilter{Kind: library.ItemKindShow, VideoCodecs: []string{"hevc"}}, Restriction: test.restriction}
		if got, _ := fixture.listAllIDs(t, showsLibraryID, hevc, 5); !slices.Equal(got, test.wantHEVC) {
			t.Errorf("%s: shows with an HEVC episode = %v, want %v", test.name, got, test.wantHEVC)
		}
	}

	// The second kids episode has its own TV-MA rating; the TV-G episode of the unrated show
	// is hidden with its show: a child's lower rating opens no path through a hidden parent
	// (ADR 0032 section 7, ancestor visibility).
	wantEpisodes := []string{restrictedEpisodeID(kidsShow, 1)}
	if got := fixture.folderItemIDs(t, showsLibraryID, children, "Kids Show/Season 1", "Mature Show/Season 1", "Unrated Show/Season 1"); !slices.Equal(got, wantEpisodes) {
		t.Errorf("children's episodes in folders = %v, want %v", got, wantEpisodes)
	}
}

func TestListingRestrictionHidesFoldersWithOnlyHiddenItems(t *testing.T) {
	t.Parallel()

	fixture := newRestrictionFixture(t)
	teens := restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age12, true))
	adults := restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age18, true))

	tests := []struct {
		name              string
		rootID, directory string
		restriction       access.Restriction
		want              []string
	}{
		{name: "everything", rootID: moviesLibraryID, restriction: access.Unrestricted(), want: []string{"Adult", "Kids", "Teens"}},
		{name: "children", rootID: moviesLibraryID, restriction: childrenRestriction(t), want: []string{"Kids"}},
		{name: "teens", rootID: moviesLibraryID, restriction: teens, want: []string{"Kids", "Teens"}},
		{name: "nested directory", rootID: moviesLibraryID, directory: "Adult", restriction: adults, want: []string{"Deep"}},
		{name: "nested directory hidden", rootID: moviesLibraryID, directory: "Adult", restriction: teens},
		{name: "shows", rootID: showsLibraryID, restriction: childrenRestriction(t), want: []string{"Kids Show"}},
		{name: "season", rootID: showsLibraryID, directory: "Mature Show", restriction: adults, want: []string{"Season 1"}},
		{name: "other library", rootID: moviesLibraryID, restriction: restrictionOf(access.OnlyLibraries(showsLibraryID), access.NoLimit())},
		{name: "zero restriction", rootID: moviesLibraryID},
	}
	for _, test := range tests {
		got, err := fixture.reader.ListFolderDirectories(t.Context(), test.rootID, test.directory, test.restriction)
		if err != nil {
			t.Fatalf("%s: ListFolderDirectories error = %v", test.name, err)
		}

		if !slices.Equal(got, test.want) {
			t.Errorf("%s: directories = %v, want %v", test.name, got, test.want)
		}
	}
}

// A library outside the viewer's access set answers like an unknown library, and the zero
// restriction (a caller that forgot to set one) sees nothing at all.
func TestListingRestrictionDeniesLibraries(t *testing.T) {
	t.Parallel()

	fixture := newRestrictionFixture(t)

	tests := []struct {
		name        string
		restriction access.Restriction
		denied      []string
	}{
		{name: "zero restriction", denied: []string{moviesLibraryID, showsLibraryID}},
		{name: "no library", restriction: restrictionOf(access.OnlyLibraries(), access.NoLimit()), denied: []string{moviesLibraryID, showsLibraryID}},
		{name: "only shows", restriction: restrictionOf(access.OnlyLibraries(showsLibraryID), access.NoLimit()), denied: []string{moviesLibraryID}},
		{name: "an unknown library only", restriction: restrictionOf(access.OnlyLibraries(unknownID), access.NoLimit()), denied: []string{moviesLibraryID, showsLibraryID, unknownID}},
	}
	for _, test := range tests {
		for _, libraryID := range test.denied {
			_, listErr := fixture.reader.ListItems(t.Context(), libraryID, library.PageRequest{}, library.ListingQuery{Restriction: test.restriction})
			_, facetErr := fixture.reader.CountFacets(t.Context(), libraryID, applibrary.FacetRequest{
				Fields: []library.FacetField{library.FacetGenre}, Restriction: test.restriction,
			})
			_, letterErr := fixture.reader.CountFirstCharacters(t.Context(), libraryID, applibrary.LetterRequest{Restriction: test.restriction})

			for shape, err := range map[string]error{"ListItems": listErr, "CountFacets": facetErr, "CountFirstCharacters": letterErr} {
				if !errors.Is(err, domain.ErrNotFound) {
					t.Errorf("%s: %s(%s) error = %v, want domain.ErrNotFound", test.name, shape, libraryID, err)
				}
			}

			// The fixture folders share their library's ID.
			if got := fixture.folderItemIDs(t, libraryID, test.restriction, "", "Kids", "Kids Show/Season 1"); len(got) != 0 {
				t.Errorf("%s: folder items of %s = %v, want none", test.name, libraryID, got)
			}
		}
	}

	onlyShows := restrictionOf(access.OnlyLibraries(showsLibraryID), access.NoLimit())
	want := []string{kidsShow, matureShow, unratedShow}

	if got, total := fixture.listAllIDs(t, showsLibraryID, library.ListingQuery{Restriction: onlyShows}, 2); !slices.Equal(got, want) || total != 3 {
		t.Errorf("allowed library = %v (total %d), want %v", got, total, want)
	}
}

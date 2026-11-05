package sqlite_test

import (
	"errors"
	"slices"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
)

// DT-24: a season of 131 episodes pages in 50s; the order is by number, an unnumbered episode
// comes last, and descending reverses the whole order.
func TestListChildrenPagesThroughALongSeasonInBothDirections(t *testing.T) {
	t.Parallel()

	const episodes = 130

	fixture := newShowFixture(t, episodes)
	unnumbered := testID('e', 5_000)
	fixture.insertEpisode(t, fixture.season1ID, unnumbered, noNumber)

	ascending := childIDs(t, fixture, childQuery{itemID: fixture.season1ID, descending: false, limit: 50}, []int{50, 50, 31})
	if len(ascending) != episodes+1 || ascending[0] != testID('e', 1) || ascending[episodes-1] != testID('e', episodes) ||
		ascending[episodes] != unnumbered {
		t.Fatalf("ascending order = first %q, 130th %q, last %q, want e1, e130, the unnumbered episode",
			ascending[0], ascending[episodes-1], ascending[episodes])
	}

	descending := childIDs(t, fixture, childQuery{itemID: fixture.season1ID, descending: true, limit: 50}, []int{50, 50, 31})
	slices.Reverse(descending)
	assertDeepEqual(t, "descending order reversed", descending, ascending)
}

func TestListChildrenOrdersSeasonsWithTheSpecialsLast(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 1)
	want := []string{fixture.season1ID, fixture.season2ID, fixture.specialsID}

	assertDeepEqual(t, "ascending", childIDs(t, fixture, childQuery{itemID: fixture.showID, descending: false, limit: 2}, []int{2, 1}), want)
	assertDeepEqual(t, "descending", childIDs(t, fixture, childQuery{itemID: fixture.showID, descending: true, limit: 2}, []int{2, 1}),
		[]string{fixture.specialsID, fixture.season2ID, fixture.season1ID})
}

// Episodes without a number order by sort title, then ID; extras are never children.
func TestListChildrenSortsUnnumberedEpisodesByTitleAndSkipsExtras(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 1)
	for index, title := range []string{"b", "a", "a"} {
		spec := newItemSpec(testID('u', index+1), showsLibraryID, "episode", title)
		spec.parentID = fixture.season1ID
		insertItem(t, fixture.inspection, spec)
	}

	extra := newItemSpec(testID('x', 1), showsLibraryID, "extra", "Trailer")
	extra.parentID, extra.extraType = fixture.showID, "trailer"
	insertItem(t, fixture.inspection, extra)

	want := []string{testID('e', 1), testID('u', 2), testID('u', 3), testID('u', 1)}
	assertDeepEqual(t, "episodes", childIDs(t, fixture, childQuery{itemID: fixture.season1ID, descending: false, limit: 100}, []int{4}), want)

	for _, id := range childIDs(t, fixture, childQuery{itemID: fixture.showID, descending: false, limit: 100}, []int{3}) {
		if id == extra.id {
			t.Errorf("children of the show contain the extra")
		}
	}
}

func TestListChildrenRejectsForeignAndMalformedCursors(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 5)
	ctx := t.Context()

	ascending, err := fixture.reader.ListChildren(ctx, fixture.season1ID,
		applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 2}})
	if err != nil || ascending.NextCursor == "" {
		t.Fatalf("first page = %+v, %v, want a next cursor", ascending, err)
	}

	tests := []struct {
		name    string
		itemID  string
		request applibrary.ChildPageRequest
	}{
		{"other direction", fixture.season1ID, request(ascending.NextCursor, true)},
		{"other item", fixture.season2ID, request(ascending.NextCursor, false)},
		{"not base64", fixture.season1ID, request("%%%", false)},
		{"trailing newline", fixture.season1ID, request(ascending.NextCursor+"\n", false)},
		{"trailing carriage return", fixture.season1ID, request(ascending.NextCursor+"\r", false)},
		{"top-level cursor", fixture.season1ID, request("c29tZQ", false)},
	}

	for _, test := range tests {
		if _, err := fixture.reader.ListChildren(ctx, test.itemID, test.request); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want domain.ErrInvalid", test.name, err)
		}
	}

	// The top-level reader rejects the cursor of a child page as well.
	_, err = fixture.reader.ListItems(ctx, showsLibraryID, library.PageRequest{Limit: 1, Cursor: ascending.NextCursor}, library.ListingQuery{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ListItems with a child cursor error = %v, want domain.ErrInvalid", err)
	}
}

func request(cursor string, descending bool) applibrary.ChildPageRequest {
	return applibrary.ChildPageRequest{Page: library.PageRequest{Limit: 2, Cursor: cursor}, Descending: descending}
}

func TestListChildrenOfUnknownItemsAndLeaves(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 1)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "Movie")
	insertItem(t, fixture.inspection, movie)

	_, err := fixture.reader.ListChildren(t.Context(), unknownID, request("", false))
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown item error = %v, want domain.ErrNotFound", err)
	}

	page, err := fixture.reader.ListChildren(t.Context(), movie.id, request("", false))
	if err != nil || len(page.Items) != 0 || page.NextCursor != "" {
		t.Errorf("children of a movie = %+v, %v, want an empty last page", page, err)
	}

	tooMany := applibrary.ChildPageRequest{Page: library.PageRequest{Limit: library.MaxPageLimit + 1}}
	if _, err = fixture.reader.ListChildren(t.Context(), fixture.season1ID, tooMany); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("limit above the maximum error = %v, want domain.ErrInvalid", err)
	}
}

// The top level lists movies and shows only, with year, poster, counts and, for movies, the
// primary media; seasons, episodes and extras never appear.
func TestListItemsServesShowsWithCountsAndPoster(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 3)
	execute(t, fixture.inspection, `UPDATE items SET year = 2019 WHERE id = ?`, fixture.showID)
	insertLocalArtwork(t, fixture.inspection, artworkSpec{
		id: testID('a', 1), itemID: fixture.showID, kind: "poster", libraryID: showsLibraryID,
		relativePath: "The Show/poster.jpg", tag: "tagshow", selected: true,
	})
	insertLocalArtwork(t, fixture.inspection, artworkSpec{
		id: testID('a', 2), itemID: fixture.showID, kind: "poster", libraryID: showsLibraryID,
		relativePath: "The Show/poster2.jpg", tag: "tagother",
	})

	page, err := fixture.reader.ListItems(t.Context(), showsLibraryID, library.PageRequest{Limit: 10}, library.ListingQuery{})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("ListItems = %+v, %v, want the show only", page, err)
	}

	got := page.Items[0]
	if got.Item.ID != fixture.showID || got.Item.Kind != library.ItemKindShow || got.Item.Year != library.NumberOf(2019) {
		t.Errorf("item = %+v, want the show of 2019", got.Item)
	}

	if got.HasPrimaryFile() || got.ChildCount != 3 || got.LeafCount != 4 {
		t.Errorf("summary = %+v, want no file, 3 seasons and 4 episodes", got)
	}

	if got.Poster == nil || *got.Poster != (artwork.Ref{ID: testID('a', 1), Tag: "tagshow"}) || got.Thumb != nil {
		t.Errorf("poster = %+v, thumb = %+v, want the selected poster and no thumb", got.Poster, got.Thumb)
	}
}

func TestListItemsServesAMovieWithItsPrimaryFile(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "Movie")
	insertItem(t, fixture.inspection, movie)
	insertFile(t, fixture.inspection, fileSpec{
		id: testID('f', 2), itemID: movie.id, libraryID: moviesLibraryID,
		relativePath: "b.mkv", partNumber: 2,
	})
	insertFile(t, fixture.inspection, fileSpec{
		id: testID('f', 1), itemID: movie.id, libraryID: moviesLibraryID,
		relativePath: "z.mkv", partNumber: 1,
	})

	page, err := fixture.reader.ListItems(t.Context(), moviesLibraryID, library.PageRequest{Limit: 10}, library.ListingQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].PrimaryFile.ID != testID('f', 1) {
		t.Fatalf("ListItems = %+v, %v, want the movie with the part 1 file", page, err)
	}

	if page.Items[0].ChildCount != 0 || page.Items[0].LeafCount != 0 {
		t.Errorf("counts = %d, %d, want 0, 0", page.Items[0].ChildCount, page.Items[0].LeafCount)
	}
}

// A season without a poster of its own shows the poster of its show in listings and details.
func TestSeasonPosterFallsBackToTheShow(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 2)
	insertLocalArtwork(t, fixture.inspection, artworkSpec{
		id: testID('a', 1), itemID: fixture.showID, kind: "poster", libraryID: showsLibraryID,
		relativePath: "show.jpg", tag: "tagshow", selected: true,
	})
	insertLocalArtwork(t, fixture.inspection, artworkSpec{
		id: testID('a', 2), itemID: fixture.season2ID, kind: "poster", libraryID: showsLibraryID,
		relativePath: "season2.jpg", tag: "tagseason", selected: true,
	})

	page, err := fixture.reader.ListChildren(t.Context(), fixture.showID, request("", false))
	if err != nil {
		t.Fatalf("ListChildren error = %v", err)
	}

	showPoster := artwork.Ref{ID: testID('a', 1), Tag: "tagshow"}
	seasonPoster := artwork.Ref{ID: testID('a', 2), Tag: "tagseason"}

	posters := []*artwork.Ref{page.Items[0].Poster, page.Items[1].Poster}
	if posters[0] == nil || *posters[0] != showPoster || posters[1] == nil || *posters[1] != seasonPoster {
		t.Errorf("season posters = %+v, %+v, want the show's and the season's own", posters[0], posters[1])
	}

	if page.Items[0].ChildCount != 2 || page.Items[1].ChildCount != 0 {
		t.Errorf("episode counts = %d, %d, want 2, 0", page.Items[0].ChildCount, page.Items[1].ChildCount)
	}

	detail := mustGetV2Item(t, fixture.reader, fixture.season1ID)
	if got := detail.Artwork[artwork.KindPoster]; got != showPoster || len(detail.Artwork) != 1 {
		t.Errorf("season 1 artwork = %+v, want only the show poster", detail.Artwork)
	}

	detail = mustGetV2Item(t, fixture.reader, fixture.season2ID)
	if got := detail.Artwork[artwork.KindPoster]; got != seasonPoster {
		t.Errorf("season 2 poster = %+v, want its own", got)
	}
}

func TestGetItemOfAShowAndItsSeasons(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 3)

	show := mustGetV2Item(t, fixture.reader, fixture.showID)
	if show.PlayItemID != testID('e', 1) || show.ChildCount != 3 || show.LeafCount != 4 ||
		len(show.Ancestors) != 0 || show.PreviousID != "" || show.NextID != "" || len(show.ItemFiles) != 0 {
		t.Errorf("show detail = %+v, want play target e1, 3 seasons, 4 episodes, no ancestors or siblings", show)
	}

	tests := []struct {
		name                     string
		id, previous, next, play string
	}{
		{"first season", fixture.season1ID, "", fixture.season2ID, testID('e', 1)},
		{"empty middle season", fixture.season2ID, fixture.season1ID, fixture.specialsID, ""},
		{"specials last", fixture.specialsID, fixture.season2ID, "", testID('e', 9_000)},
	}

	for _, test := range tests {
		detail := mustGetV2Item(t, fixture.reader, test.id)
		if detail.PreviousID != test.previous || detail.NextID != test.next || detail.PlayItemID != test.play {
			t.Errorf("%s: previous %q, next %q, play %q, want %q, %q, %q", test.name,
				detail.PreviousID, detail.NextID, detail.PlayItemID, test.previous, test.next, test.play)
		}

		if len(detail.Ancestors) != 1 || detail.Ancestors[0].ID != fixture.showID {
			t.Errorf("%s: ancestors = %+v, want the show", test.name, detail.Ancestors)
		}
	}
}

func TestGetItemOfAnEpisodeHasAncestorsSiblingsAndItself(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 3)

	middle := mustGetV2Item(t, fixture.reader, testID('e', 2))
	if middle.PreviousID != testID('e', 1) || middle.NextID != testID('e', 3) || middle.PlayItemID != middle.Item.ID {
		t.Errorf("middle episode = previous %q, next %q, play %q", middle.PreviousID, middle.NextID, middle.PlayItemID)
	}

	wantAncestors := []applibrary.ItemRef{
		{ID: fixture.showID, Kind: library.ItemKindShow, Title: "The Show"},
		{ID: fixture.season1ID, Kind: library.ItemKindSeason, Title: "Season 1", Number: library.NumberOf(1)},
	}
	assertDeepEqual(t, "ancestors", middle.Ancestors, wantAncestors)

	if len(middle.Files) != 1 || len(middle.ItemFiles) != 1 {
		t.Errorf("files = %d, item files = %d, want 1 and 1", len(middle.Files), len(middle.ItemFiles))
	}

	first := mustGetV2Item(t, fixture.reader, testID('e', 1))
	last := mustGetV2Item(t, fixture.reader, testID('e', 3))

	if first.PreviousID != "" || first.NextID != testID('e', 2) || last.PreviousID != testID('e', 2) || last.NextID != "" {
		t.Errorf("episode ends = (%q, %q) and (%q, %q)", first.PreviousID, first.NextID, last.PreviousID, last.NextID)
	}
}

func TestGetItemFillsTheMetadataRecordInStoredOrder(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "The Movie")
	movie.sortTitle, movie.year = "movie", 2020
	insertItem(t, fixture.inspection, movie)
	execute(t, fixture.inspection, `UPDATE items SET original_title = 'Der Film', release_date = '2020-05-17',
		summary = 'About.', tagline = 'Tag.', content_rating = 'PG-13' WHERE id = ?`, movie.id)
	execute(t, fixture.inspection, `INSERT INTO tags (id, kind, name) VALUES
		('t1', 'genre', 'Drama'), ('t2', 'genre', 'Action'), ('t3', 'studio', 'Acme'),
		('t4', 'country', 'DE'), ('t5', 'collection', 'Saga')`)
	execute(t, fixture.inspection, `INSERT INTO item_tags (item_id, tag_id, position) VALUES
		(?, 't1', 1), (?, 't2', 0), (?, 't3', 0), (?, 't4', 0), (?, 't5', 0)`,
		movie.id, movie.id, movie.id, movie.id, movie.id)
	execute(t, fixture.inspection, `INSERT INTO people (id, name, sort_name) VALUES
		('p1', 'Ann Actor', 'actor, ann'), ('p2', 'Dan Director', 'director, dan')`)
	execute(t, fixture.inspection, `INSERT INTO item_people (item_id, person_id, role, character, position) VALUES
		(?, 'p2', 'director', '', 1), (?, 'p1', 'actor', 'Hero', 0)`, movie.id, movie.id)
	execute(t, fixture.inspection, `INSERT INTO item_provider_ids (item_id, provider, external_id) VALUES
		(?, 'tmdb', '603'), (?, 'imdb', 'tt0133093')`, movie.id, movie.id)

	record := mustGetV2Item(t, fixture.reader, movie.id).Metadata

	if record.Title != "The Movie" || record.SortTitle != "movie" || record.OriginalTitle != "Der Film" ||
		record.Year != 2020 || record.ReleaseDate.String() != "2020-05-17" || record.Summary != "About." ||
		record.Tagline != "Tag." || record.ContentRating != "PG-13" {
		t.Errorf("record = %+v", record)
	}

	assertDeepEqual(t, "genres", record.Genres, []string{"Action", "Drama"})
	assertDeepEqual(t, "studios", record.Studios, []string{"Acme"})
	assertDeepEqual(t, "countries", record.Countries, []string{"DE"})
	assertDeepEqual(t, "collections", record.Collections, []string{"Saga"})

	if len(record.People) != 2 || record.People[0].Name != "Ann Actor" || record.People[0].Character != "Hero" ||
		record.People[0].Role != "actor" || record.People[1].Name != "Dan Director" || record.People[1].Order != 1 {
		t.Errorf("people = %+v, want the actor first", record.People)
	}

	if id, ok := record.ProviderID("tmdb"); !ok || id != "603" || len(record.ProviderIDs) != 2 {
		t.Errorf("provider IDs = %+v", record.ProviderIDs)
	}
}

func TestGetItemOfAMovieHasExtrasInTypeThenTitleOrderAndFileVersions(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "Movie")
	insertItem(t, fixture.inspection, movie)
	insertFile(t, fixture.inspection, fileSpec{
		id: testID('f', 1), itemID: movie.id, libraryID: moviesLibraryID,
		relativePath: "Movie/Movie - cd2.mkv", versionLabel: "Director's Cut", partNumber: 2,
	})
	insertFile(t, fixture.inspection, fileSpec{
		id: testID('f', 2), itemID: movie.id, libraryID: moviesLibraryID,
		relativePath: "Movie/Movie - cd1.mkv", versionLabel: "Director's Cut", partNumber: 1,
	})
	insertFile(t, fixture.inspection, fileSpec{
		id: testID('f', 3), itemID: movie.id, libraryID: moviesLibraryID,
		relativePath: "Movie/Movie.mkv",
	})

	for index, extra := range []struct{ title, extraType string }{
		{"b-roll", "other"}, {"Trailer 2", "trailer"}, {"Trailer 1", "trailer"}, {"Making of", "featurette"},
	} {
		spec := newItemSpec(testID('x', index+1), moviesLibraryID, "extra", extra.title)
		spec.parentID, spec.extraType = movie.id, extra.extraType
		insertItem(t, fixture.inspection, spec)
		insertFile(t, fixture.inspection, fileSpec{
			id: testID('g', index+1), itemID: spec.id,
			libraryID: moviesLibraryID, relativePath: "Movie/extras/" + extra.title + ".mkv",
		})
	}

	detail := mustGetV2Item(t, fixture.reader, movie.id)

	var titles []string
	for _, extra := range detail.Extras {
		titles = append(titles, extra.Item.Title)

		if !extra.HasPrimaryFile() || extra.Item.ExtraType == "" || extra.Item.ParentID != movie.id {
			t.Errorf("extra %+v lacks its file, type or parent", extra.Item)
		}
	}

	assertDeepEqual(t, "extras", titles, []string{"Trailer 1", "Trailer 2", "Making of", "b-roll"})

	if detail.PlayItemID != movie.id || len(detail.Files) != 3 || len(detail.ItemFiles) != 3 {
		t.Fatalf("detail = play %q, %d files, %d item files", detail.PlayItemID, len(detail.Files), len(detail.ItemFiles))
	}

	first, last := detail.ItemFiles[0], detail.ItemFiles[2]
	if first.File.ID != testID('f', 3) || first.VersionLabel != "" || first.PartNumber.IsSet() {
		t.Errorf("first file = %+v, want the file without a part", first)
	}

	if last.File.ID != testID('f', 1) || last.VersionLabel != "Director's Cut" || last.PartNumber != library.NumberOf(2) {
		t.Errorf("last file = %+v", last)
	}
}

func TestGetItemCapsTheExtrasAtOneHundred(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	movie := newItemSpec(testID('m', 1), moviesLibraryID, "movie", "Movie")
	insertItem(t, fixture.inspection, movie)

	for index := range 105 {
		spec := newItemSpec(testID('x', index+1), moviesLibraryID, "extra", "Extra")
		spec.parentID, spec.extraType = movie.id, "other"
		insertItem(t, fixture.inspection, spec)
		insertFile(t, fixture.inspection, fileSpec{
			id: testID('g', index+1), itemID: spec.id,
			libraryID: moviesLibraryID, relativePath: spec.id + ".mkv",
		})
	}

	if got := len(mustGetV2Item(t, fixture.reader, movie.id).Extras); got != 100 {
		t.Errorf("extras = %d, want 100", got)
	}
}

func TestReadingACorruptV2RowIsAServerError(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 1)
	storeUnchecked(t, fixture.inspection, `UPDATE items SET kind = 'hologram' WHERE id = ?`, testID('e', 1))

	_, err := fixture.reader.ListChildren(t.Context(), fixture.season1ID, request("", false))
	assertUndecodableRowError(t, "ListChildren", err)

	storeUnchecked(t, fixture.inspection, `UPDATE items SET release_date = 'soon' WHERE id = ?`, fixture.showID)

	_, err = fixture.reader.GetItem(t.Context(), fixture.showID)
	assertUndecodableRowError(t, "GetItem", err)
}

// childQuery names a children listing of the tests.
type childQuery struct {
	itemID     string
	descending bool
	limit      int
}

// childIDs pages through the children of query.itemID and checks the page sizes.
func childIDs(t *testing.T, fixture showFixture, query childQuery, wantSizes []int) []string {
	t.Helper()

	var (
		ids   []string
		sizes []int
	)

	cursor := ""

	for range 20 {
		page, err := fixture.reader.ListChildren(t.Context(), query.itemID, applibrary.ChildPageRequest{
			Page: library.PageRequest{Limit: query.limit, Cursor: cursor}, Descending: query.descending,
		})
		if err != nil {
			t.Fatalf("ListChildren error = %v", err)
		}

		sizes = append(sizes, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.Item.ID)
		}

		if page.NextCursor == "" {
			assertDeepEqual(t, "page sizes", sizes, wantSizes)

			return ids
		}

		cursor = page.NextCursor
	}

	t.Fatalf("paging did not end")

	return nil
}

func mustGetV2Item(t *testing.T, reader applibrary.ItemReader, itemID string) applibrary.ItemDetail {
	t.Helper()

	detail, err := reader.GetItem(t.Context(), itemID)
	if err != nil {
		t.Fatalf("GetItem(%q) error = %v", itemID, err)
	}

	return detail
}

// Equal numbers tie by sort title, then by ID; unnumbered children sort the same way.
func TestListChildrenBreaksTiesBySortTitleThenID(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 0)

	for index, title := range []string{"b", "a", "a"} {
		spec := newItemSpec(testID('t', index+1), showsLibraryID, "episode", title)
		spec.parentID, spec.number = fixture.season2ID, 5
		insertItem(t, fixture.inspection, spec)
	}

	want := []string{testID('t', 2), testID('t', 3), testID('t', 1)}
	query := childQuery{itemID: fixture.season2ID, limit: 1}

	assertDeepEqual(t, "ascending", childIDs(t, fixture, query, []int{1, 1, 1}), want)

	query.descending = true
	slices.Reverse(want)
	assertDeepEqual(t, "descending", childIDs(t, fixture, query, []int{1, 1, 1}), want)
}

// Pages of one child cross the boundary between the numbered and the trailing group, and a
// season without a number joins the specials there (ordered by sort title).
func TestListChildrenPagesOneByOneAcrossTheGroupBoundary(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 2)
	unnumbered := newItemSpec(testID('s', 9), showsLibraryID, "season", "Odd")
	unnumbered.parentID = fixture.showID
	insertItem(t, fixture.inspection, unnumbered)

	want := []string{fixture.season1ID, fixture.season2ID, unnumbered.id, fixture.specialsID}
	query := childQuery{itemID: fixture.showID, limit: 1}

	assertDeepEqual(t, "seasons ascending", childIDs(t, fixture, query, []int{1, 1, 1, 1}), want)

	query.descending = true
	slices.Reverse(want)
	assertDeepEqual(t, "seasons descending", childIDs(t, fixture, query, []int{1, 1, 1, 1}), want)

	fixture.insertEpisode(t, fixture.season1ID, testID('e', 3_000), noNumber)

	episodes := childQuery{itemID: fixture.season1ID, limit: 1}
	wantEpisodes := []string{testID('e', 1), testID('e', 2), testID('e', 3_000)}

	assertDeepEqual(t, "episodes ascending", childIDs(t, fixture, episodes, []int{1, 1, 1}), wantEpisodes)

	episodes.descending = true
	slices.Reverse(wantEpisodes)
	assertDeepEqual(t, "episodes descending", childIDs(t, fixture, episodes, []int{1, 1, 1}), wantEpisodes)
}

// The cursor is checked before the item is read, so an invalid cursor wins over an unknown item.
func TestListChildrenReportsABadCursorBeforeAnUnknownItem(t *testing.T) {
	t.Parallel()

	fixture := newShowFixture(t, 1)

	if _, err := fixture.reader.ListChildren(t.Context(), unknownID, request("%%%", false)); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("bad cursor on an unknown item error = %v, want domain.ErrInvalid", err)
	}
}

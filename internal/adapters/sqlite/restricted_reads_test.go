package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	appsearch "github.com/magpeek/mivo/internal/app/search"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Extras the detail fixture adds to the ancestor fixture: a trailer that inherits the FSK 0
// of its film, one rated FSK 16 below the same film and one below the FSK 16 film.
const (
	openTrailer    = "x0000100-0000-4000-8000-000000000000"
	lockedTrailer  = "x0000116-0000-4000-8000-000000000000"
	sixteenTrailer = "x0000216-0000-4000-8000-000000000000"
)

// newDetailFixture is the ancestor fixture with extras, for the children, detail and search
// reads.
func newDetailFixture(t *testing.T) catalogFixture {
	t.Helper()

	fixture := newAncestorFixture(t)

	extras := []listingItem{
		{id: openTrailer, parentID: ratedZero, title: "Open Trailer"},
		{id: lockedTrailer, parentID: ratedZero, title: "Locked Trailer", contentRating: "FSK 16"},
		{id: sixteenTrailer, parentID: ratedSixteen, title: "Sixteen Trailer"},
	}
	for _, extra := range extras {
		extra.libraryID, extra.kind, extra.sortTitle = moviesLibraryID, "extra", extra.title
		insertListingItem(t, fixture.inspection, extra)
	}

	fixture.backfill(t)

	return fixture
}

func (fixture catalogFixture) backfill(t *testing.T) {
	t.Helper()

	if err := sqlite.NewLibraryCatalog(fixture.db, nil).BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}
}

func (fixture catalogFixture) searchIDs(t *testing.T, query string, filter appsearch.Filter, limit int) []string {
	t.Helper()

	filter.LibraryIDs = []string{moviesLibraryID, showsLibraryID}

	hits, err := sqlite.NewSearchIndex(fixture.db).Search(t.Context(), query, filter, limit)
	if err != nil {
		t.Fatalf("Search(%q) error = %v", query, err)
	}

	ids := make([]string, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ItemID)
	}

	return ids
}

// detailReads is every children, detail and search read of the detail fixture that returned
// something, for the leak check.
type detailReads struct {
	children map[string][]applibrary.ItemSummary
	details  map[string]applibrary.ItemDetail
	hits     map[string][]string
}

// readDetails reads the children and the detail of every item of the fixture and searches
// for every title word; hidden items are left out of the maps.
func (fixture catalogFixture) readDetails(t *testing.T, restriction access.Restriction) detailReads {
	t.Helper()

	reads := detailReads{
		children: map[string][]applibrary.ItemSummary{}, details: map[string]applibrary.ItemDetail{}, hits: map[string][]string{},
	}
	itemIDs := append(ancestorFixtureItemIDs(),
		ratedZero, ratedSix, ratedTwelve, ratedSixteen, ratedEighteen, unratedMovie, openTrailer, lockedTrailer, sixteenTrailer)

	for _, id := range itemIDs {
		page, childErr := fixture.reader.ListChildren(t.Context(), id, applibrary.ChildPageRequest{}, restriction)
		detail, detailErr := fixture.reader.GetItem(t.Context(), id, restriction)

		if errors.Is(childErr, domain.ErrNotFound) != errors.Is(detailErr, domain.ErrNotFound) {
			t.Errorf("item %s: children error = %v, detail error = %v, want both or neither not found", id, childErr, detailErr)
		}

		if childErr == nil && detailErr == nil {
			reads.children[id], reads.details[id] = page.Items, detail
		} else if !errors.Is(childErr, domain.ErrNotFound) || !errors.Is(detailErr, domain.ErrNotFound) {
			t.Errorf("item %s: children error = %v, detail error = %v", id, childErr, detailErr)
		}
	}

	for _, word := range []string{"show", "chain", "episode", "sixteen", "unrated", "mature", "zero", "kids"} {
		reads.hits[word] = fixture.searchIDs(t, word, appsearch.Filter{Restriction: restriction}, appsearch.MaxLimit)
	}

	return reads
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	slices.Sort(keys)

	return keys
}

func sorted(values ...string) []string {
	slices.Sort(values)

	return values
}

// ratedTitle is the title saveRating gives an item along with its rating.
func ratedTitle(itemID string) string {
	return "Title " + itemID[:8]
}

// The "Children" preset (up to 6, unrated hidden): children, detail and search show an item
// only when it and every ancestor pass, hidden items and their descendants are not found by
// ID, counts and play targets cover visible members only, and no read names a hidden item.
func TestChildrenDetailAndSearchForChildrenPreset(t *testing.T) {
	t.Parallel()

	fixture := newDetailFixture(t)
	restriction := childrenRestriction(t)
	reads := fixture.readDetails(t, restriction)
	kidsEpisode := restrictedEpisodeID(kidsShow, 1)

	visible := sorted(kidsShow, kidsSeason, kidsEpisode, chainShow, chainOpenSeason, chainOpen, ratedZero, ratedSix, openTrailer)
	assertTexts(t, "items found by ID", sortedKeys(reads.details), visible)

	assertTexts(t, "children of Chain Show", countText(reads.children[chainShow]), []string{chainOpenSeason + ":1/0"})
	assertTexts(t, "children of Kids Show", countText(reads.children[kidsShow]), []string{kidsSeason + ":1/0"})
	assertTexts(t, "children of the Kids season", countText(reads.children[kidsSeason]), []string{kidsEpisode + ":0/0"})

	chain, kids := reads.details[chainShow], reads.details[kidsShow]
	if chain.ChildCount != 1 || chain.LeafCount != 1 || chain.PlayItemID != chainOpen {
		t.Errorf("Chain Show = %d seasons, %d episodes, play %s, want 1, 1 and the episode of season 2",
			chain.ChildCount, chain.LeafCount, chain.PlayItemID)
	}

	if kids.ChildCount != 1 || kids.LeafCount != 1 || kids.PlayItemID != kidsEpisode {
		t.Errorf("Kids Show = %d seasons, %d episodes, play %s, want 1, 1 and episode 1", kids.ChildCount, kids.LeafCount, kids.PlayItemID)
	}

	if season := reads.details[kidsSeason]; season.ChildCount != 1 || season.PlayItemID != kidsEpisode {
		t.Errorf("Kids season = %d episodes, play %s, want 1 and episode 1", season.ChildCount, season.PlayItemID)
	}

	if episode := reads.details[kidsEpisode]; episode.NextID != "" || episode.PreviousID != "" {
		t.Errorf("Kids episode 1 has siblings %q and %q, want none: episode 2 is hidden", episode.PreviousID, episode.NextID)
	}

	if season := reads.details[chainOpenSeason]; season.NextID != "" || season.PreviousID != "" {
		t.Errorf("Chain season 2 has siblings %q and %q, want none: season 1 is hidden", season.PreviousID, season.NextID)
	}

	assertTexts(t, "extras of Zero", countText(reads.details[ratedZero].Extras), []string{openTrailer + ":0/0"})

	assertTexts(t, "search show", sorted(reads.hits["show"]...), sorted(kidsShow, chainShow))
	assertTexts(t, "search chain", sorted(reads.hits["chain"]...), sorted(chainShow, chainOpen))
	assertTexts(t, "search episode", reads.hits["episode"], []string{kidsEpisode})

	for _, word := range []string{"sixteen", "unrated", "mature"} {
		if len(reads.hits[word]) != 0 {
			t.Errorf("search %q = %v, want none", word, reads.hits[word])
		}
	}

	// Nothing of a hidden item may leak: not its ID, title or directory.
	assertNothingOf(t, fmt.Sprintf("%+v", reads),
		matureShow, unratedShow, sixteenShow, sixteenSeason, sixteenChild, chainHiddenSeason, chainHidden,
		restrictedEpisodeID(kidsShow, 2), restrictedEpisodeID(unratedShow, 1), ratedTwelve, ratedSixteen, ratedEighteen,
		unratedMovie, lockedTrailer, sixteenTrailer,
		"Mature Show", "Unrated", "Sixteen", "Chain Locked Season", "Chain Hidden", "Chain Show/Season 1", "Locked Trailer",
		"Twelve", "Eighteen", "Adult/")
}

// assertNothingOf fails for every text of a hidden item that the serialised reads contain.
func assertNothingOf(t *testing.T, serialised string, hidden ...string) {
	t.Helper()

	for _, text := range hidden {
		if strings.Contains(serialised, text) {
			t.Errorf("the reads of a restricted viewer contain %q of a hidden item", text)
		}
	}
}

// The content limit is part of the search query: a hidden hit that ranks first does not use
// up the limit.
func TestSearchAppliesTheRestrictionBeforeTheLimit(t *testing.T) {
	t.Parallel()

	fixture := newDetailFixture(t)
	episodes := appsearch.Filter{Kinds: []library.ItemKind{library.ItemKindEpisode}, Restriction: access.Unrestricted()}

	if got := fixture.searchIDs(t, "chain", episodes, 1); !slices.Equal(got, []string{chainHidden}) {
		t.Fatalf("unrestricted first hit = %v, want the hidden episode %s (the fixture must rank it first)", got, chainHidden)
	}

	episodes.Restriction = childrenRestriction(t)
	assertTexts(t, "restricted first hit", fixture.searchIDs(t, "chain", episodes, 1), []string{chainOpen})

	// The library set of the restriction narrows the libraries of the filter.
	episodes.Restriction = restrictionOf(access.OnlyLibraries(moviesLibraryID), access.NoLimit())
	assertTexts(t, "hits outside the allowed libraries", fixture.searchIDs(t, "chain", episodes, 10), []string{})

	episodes.Restriction = access.Restriction{}
	assertTexts(t, "hits of the zero restriction", fixture.searchIDs(t, "chain", episodes, 10), []string{})
}

// With unrated titles shown, the unrated show, its season and both episodes are found, with
// their counts; the FSK 16 chains stay hidden.
func TestChildrenAndDetailWithUnratedShown(t *testing.T) {
	t.Parallel()

	fixture := newDetailFixture(t)
	reads := fixture.readDetails(t, restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age6, false)))
	first, second := restrictedEpisodeID(unratedShow, 1), restrictedEpisodeID(unratedShow, 2)

	assertTexts(t, "children of Unrated Show", countText(reads.children[unratedShow]), []string{unratedSeason + ":2/0"})
	assertTexts(t, "children of its season", countText(reads.children[unratedSeason]), []string{first + ":0/0", second + ":0/0"})

	if show := reads.details[unratedShow]; show.ChildCount != 1 || show.LeafCount != 2 || show.PlayItemID != first {
		t.Errorf("Unrated Show = %d seasons, %d episodes, play %s, want 1, 2 and episode 1", show.ChildCount, show.LeafCount, show.PlayItemID)
	}

	if _, found := reads.details[unratedMovie]; !found {
		t.Error("the unrated movie is not found, want it visible")
	}

	for _, id := range []string{matureShow, sixteenShow, sixteenChild, chainHiddenSeason, chainHidden, ratedTwelve, lockedTrailer} {
		if _, found := reads.details[id]; found {
			t.Errorf("item %s is found, want it hidden", id)
		}
	}
}

// An unrestricted viewer finds every item, with the counts, siblings and play targets of
// before the restriction.
func TestChildrenAndDetailUnrestricted(t *testing.T) {
	t.Parallel()

	fixture := newDetailFixture(t)
	reads := fixture.readDetails(t, access.Unrestricted())

	if got, want := len(reads.details), len(ancestorFixtureItemIDs())+9; got != want {
		t.Errorf("items found = %d, want all %d", got, want)
	}

	assertTexts(t, "children of Chain Show", countText(reads.children[chainShow]),
		[]string{chainHiddenSeason + ":1/0", chainOpenSeason + ":1/0"})

	chain := reads.details[chainShow]
	if chain.ChildCount != 2 || chain.LeafCount != 2 || chain.PlayItemID != chainHidden {
		t.Errorf("Chain Show = %d seasons, %d episodes, play %s, want 2, 2 and the episode of season 1",
			chain.ChildCount, chain.LeafCount, chain.PlayItemID)
	}

	if episode := reads.details[restrictedEpisodeID(kidsShow, 1)]; episode.NextID != restrictedEpisodeID(kidsShow, 2) {
		t.Errorf("next of Kids episode 1 = %q, want episode 2", episode.NextID)
	}

	assertTexts(t, "extras of Zero", countText(reads.details[ratedZero].Extras), []string{lockedTrailer + ":0/0", openTrailer + ":0/0"})
	assertTexts(t, "search chain", sorted(reads.hits["chain"]...), sorted(chainShow, chainHidden, chainOpen))
}

// The zero restriction (a caller that forgot to set one) finds nothing, and a library set
// hides the items of the other libraries.
func TestChildrenAndDetailDenyLibraries(t *testing.T) {
	t.Parallel()

	fixture := newDetailFixture(t)

	if reads := fixture.readDetails(t, access.Restriction{}); len(reads.details) != 0 || len(reads.children) != 0 {
		t.Errorf("the zero restriction found %v, want nothing", sortedKeys(reads.details))
	}

	reads := fixture.readDetails(t, restrictionOf(access.OnlyLibraries(moviesLibraryID), access.NoLimit()))
	movies := sorted(ratedZero, ratedSix, ratedTwelve, ratedSixteen, ratedEighteen, unratedMovie, openTrailer, lockedTrailer, sixteenTrailer)
	assertTexts(t, "items of the movies library", sortedKeys(reads.details), movies)

	if hits := reads.hits["show"]; len(hits) != 0 {
		t.Errorf("search show = %v, want none outside the allowed library", hits)
	}
}

// "Hide unrated" applies to video only: music and photos carry no ratings, so they stay
// visible with unchanged counts, while a maximum age still hides a rated album, track, photo
// or photo album together with what lies below it.
func TestMusicReadsUnderARestriction(t *testing.T) {
	t.Parallel()

	fixture := newMusicFixture(t)
	open := fixture.addTrack(t, musicTrack{path: "Band/Open/01 One.ogg"})
	rated := fixture.addTrack(t, musicTrack{path: "Band/Open/02 Two.ogg"})
	locked := fixture.addTrack(t, musicTrack{path: "Band/Locked/01 Three.ogg"})
	artist, openAlbum, lockedAlbum := open[0], open[1], locked[1]

	saveRating(t, fixture.placementFixture, rated[2], "FSK 16")
	saveRating(t, fixture.placementFixture, lockedAlbum, "FSK 18")

	read := func(restriction access.Restriction) (albums, libraryAlbums, tracks []string, lockedErr error) {
		page := library.PageRequest{}
		artistPage, artistErr := fixture.reader.ListArtistAlbums(t.Context(), artist, page, restriction)
		libraryPage, libraryErr := fixture.reader.ListLibraryAlbums(t.Context(), musicLibraryID, page, restriction)
		trackPage, trackErr := fixture.reader.ListAlbumTracks(t.Context(), openAlbum, page, restriction)
		children, childErr := fixture.reader.ListChildren(t.Context(), openAlbum, applibrary.ChildPageRequest{}, restriction)

		if err := errors.Join(artistErr, libraryErr, trackErr, childErr); err != nil {
			t.Fatalf("read music: %v", err)
		}

		assertTexts(t, "children of the album", countText(children.Items), countText(trackPage.Items))

		_, lockedErr = fixture.reader.ListAlbumTracks(t.Context(), lockedAlbum, page, restriction)

		return countText(artistPage.Items), countText(libraryPage.Items), countText(trackPage.Items), lockedErr
	}

	// Unrated music is not hidden by "hide unrated" alone.
	albums, libraryAlbums, tracks, lockedErr := read(restrictionOf(access.AllLibraries(), access.NoLimit().HidingUnrated(true)))
	assertTexts(t, "albums with unrated hidden", albums, []string{openAlbum + ":2/0", lockedAlbum + ":1/0"})
	assertTexts(t, "library albums with unrated hidden", libraryAlbums, albums)
	assertTexts(t, "tracks with unrated hidden", tracks, []string{open[2] + ":0/0", rated[2] + ":0/0"})

	if lockedErr != nil {
		t.Errorf("tracks of the FSK 18 album without a maximum age: %v", lockedErr)
	}

	albums, libraryAlbums, tracks, lockedErr = read(childrenRestriction(t))
	assertTexts(t, "albums for children", albums, []string{openAlbum + ":1/0"})
	assertTexts(t, "library albums for children", libraryAlbums, albums)
	assertTexts(t, "tracks for children", tracks, []string{open[2] + ":0/0"})

	if !errors.Is(lockedErr, domain.ErrNotFound) {
		t.Errorf("tracks of the FSK 18 album for children: error = %v, want domain.ErrNotFound", lockedErr)
	}

	for _, id := range []string{lockedAlbum, locked[2], rated[2]} {
		if _, err := fixture.reader.GetItem(t.Context(), id, childrenRestriction(t)); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("GetItem(%s) for children: error = %v, want domain.ErrNotFound", id, err)
		}
	}

	other := restrictionOf(access.OnlyLibraries(moviesLibraryID), access.NoLimit())
	if _, err := fixture.reader.ListLibraryAlbums(t.Context(), musicLibraryID, library.PageRequest{}, other); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ListLibraryAlbums of a library that is not allowed: error = %v, want domain.ErrNotFound", err)
	}

	if _, err := fixture.reader.ListArtistAlbums(t.Context(), artist, library.PageRequest{}, other); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ListArtistAlbums in a library that is not allowed: error = %v, want domain.ErrNotFound", err)
	}
}

// photoAlbumReads is what the photo reads return for one album: its children, the library's
// timeline, the cover of the first child and the album's photo count from its detail.
type photoAlbumReads struct {
	children, timeline []string
	cover              string
	leafCount          int
}

// readAlbum reads an album through the photo reader and the generic item reads, which must
// agree on the children and their photo counts.
func (fixture *photoFixture) readAlbum(t *testing.T, album string, restriction access.Restriction) photoAlbumReads {
	t.Helper()

	page, childErr := fixture.reader.ListPhotoChildren(t.Context(), album, library.PageRequest{}, restriction)
	generic, genericErr := fixture.reader.ListChildren(t.Context(), album, applibrary.ChildPageRequest{}, restriction)
	photos, photoErr := fixture.reader.ListLibraryPhotos(t.Context(), photosLibraryID, library.PageRequest{}, restriction)
	detail, detailErr := fixture.reader.GetItem(t.Context(), album, restriction)

	if err := errors.Join(childErr, genericErr, photoErr, detailErr); err != nil {
		t.Fatalf("read photos: %v", err)
	}

	if len(generic.Items) != len(page.Children) || len(page.Children) == 0 {
		t.Fatalf("ListChildren = %d items, ListPhotoChildren = %d, want the same and some", len(generic.Items), len(page.Children))
	}

	for index, child := range page.Children {
		if child.Photo == nil && generic.Items[index].LeafCount != child.LeafCount {
			t.Errorf("album %s: generic leaf count %d, photo leaf count %d", child.Item.Title, generic.Items[index].LeafCount, child.LeafCount)
		}
	}

	return photoAlbumReads{
		children: childNames(page), timeline: childNames(photos), cover: page.Children[0].CoverPhotoID, leafCount: detail.LeafCount,
	}
}

// "Hide unrated" hides no photo; a maximum age hides a rated photo and a rated album with
// its photos, and the counts and the cover of an album follow.
func TestPhotoReadsUnderARestriction(t *testing.T) {
	t.Parallel()

	fixture := newPhotoFixture(t)
	album := fixture.addPhoto(t, "Trip/early.jpg", 1)[0]
	lockedPhoto := fixture.addPhoto(t, "Trip/locked.jpg", 40)[1]
	beach := fixture.addPhoto(t, "Trip/Beach/x.jpg", 2)
	lockedInBeach := fixture.addPhoto(t, "Trip/Beach/newest.jpg", 50)[2]
	secret := fixture.addPhoto(t, "Trip/Secret/s.jpg", 55)

	saveRating(t, fixture.placementFixture, lockedPhoto, "FSK 16")
	saveRating(t, fixture.placementFixture, lockedInBeach, "FSK 16")
	saveRating(t, fixture.placementFixture, secret[1], "FSK 18")

	// "Hide unrated" alone hides no photo.
	reads := fixture.readAlbum(t, album, restrictionOf(access.AllLibraries(), access.NoLimit().HidingUnrated(true)))
	assertTexts(t, "children with unrated hidden", reads.children,
		[]string{"album:Beach(2/2)", "album:" + ratedTitle(secret[1]) + "(1/1)", "photo:early", "photo:" + ratedTitle(lockedPhoto)})
	assertTexts(t, "timeline with unrated hidden", reads.timeline,
		[]string{"photo:early", "photo:x", "photo:" + ratedTitle(lockedPhoto), "photo:" + ratedTitle(lockedInBeach), "photo:s"})

	if reads.cover != lockedInBeach || reads.leafCount != 5 {
		t.Errorf("cover of Beach = %s and photos of Trip = %d, want the newest photo %s and 5", reads.cover, reads.leafCount, lockedInBeach)
	}

	restriction := childrenRestriction(t)
	reads = fixture.readAlbum(t, album, restriction)
	assertTexts(t, "children for children", reads.children, []string{"album:Beach(1/1)", "photo:early"})
	assertTexts(t, "timeline for children", reads.timeline, []string{"photo:early", "photo:x"})

	if reads.cover != beach[2] || reads.leafCount != 2 {
		t.Errorf("cover of Beach = %s and photos of Trip = %d, want the visible photo %s and 2", reads.cover, reads.leafCount, beach[2])
	}

	for _, id := range []string{lockedPhoto, lockedInBeach, secret[2]} {
		if _, err := fixture.reader.PhotoDetail(t.Context(), id, restriction); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("PhotoDetail(%s) for children: error = %v, want domain.ErrNotFound", id, err)
		}
	}

	if _, err := fixture.reader.ListPhotoChildren(t.Context(), secret[1], library.PageRequest{}, restriction); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("children of the FSK 18 album: error = %v, want domain.ErrNotFound", err)
	}

	if _, err := fixture.reader.PhotoDetail(t.Context(), beach[2], restriction); err != nil {
		t.Errorf("PhotoDetail of a visible photo: %v", err)
	}

	other := restrictionOf(access.OnlyLibraries(moviesLibraryID), access.NoLimit())
	if page, err := fixture.reader.ListLibraryPhotos(t.Context(), photosLibraryID, library.PageRequest{}, other); err != nil || len(page.Children) != 0 {
		t.Errorf("timeline of a library that is not allowed = %v, %v, want none", childNames(page), err)
	}
}

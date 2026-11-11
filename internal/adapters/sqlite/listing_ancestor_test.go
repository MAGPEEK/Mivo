package sqlite_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	approws "github.com/magpeek/mivo/internal/app/rows"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/rows"
)

// Items the ancestor fixture adds to the restriction fixture: Sixteen Show (FSK 16) with an
// FSK 0 episode, and Chain Show (FSK 0) whose season 1 is FSK 16 with an FSK 0 episode and
// whose unrated season 2 has an FSK 6 episode.
const (
	sixteenShow       = "s0000216-0000-4000-8000-000000000000"
	sixteenSeason     = "t0000216-0000-4000-8000-000000000000"
	sixteenChild      = "e0000216-0000-4000-8000-000000000000"
	chainShow         = "s0000300-0000-4000-8000-000000000000"
	chainHiddenSeason = "t0000300-0000-4000-8000-000000000000"
	chainHidden       = "e0000300-0000-4000-8000-000000000000"
	chainOpenSeason   = "u0000300-0000-4000-8000-000000000000"
	chainOpen         = "f0000300-0000-4000-8000-000000000000"
)

// Seasons of the restriction fixture's shows (seedRestrictedShows).
const (
	kidsSeason    = "t0000100-0000-4000-8000-000000000000"
	matureSeason  = "t0000116-0000-4000-8000-000000000000"
	unratedSeason = "t0000199-0000-4000-8000-000000000000"
)

// ancestorItem is an item of the ancestor fixture; an episode has a file of sizeBytes at path.
type ancestorItem struct {
	id, parentID, kind, title, rating string
	number                            int
	path, releaseDate                 string
	sizeBytes                         int64
}

// newAncestorFixture extends the restriction fixture by the chains of the ancestor rule (ADR
// 0032 section 7). Chain Show's hidden episode is larger than everything else, so the size
// order of the shows differs between a viewer who sees it and one who does not.
func newAncestorFixture(t *testing.T) catalogFixture {
	t.Helper()

	fixture := newRestrictionFixture(t)

	items := []ancestorItem{
		{id: sixteenShow, kind: "show", title: "Sixteen Show", rating: "FSK 16"},
		{id: sixteenSeason, parentID: sixteenShow, kind: "season", title: "Sixteen Season", number: 1},
		{
			id: sixteenChild, parentID: sixteenSeason, kind: "episode", title: "Sixteen Child", rating: "FSK 0", number: 1,
			path: "Sixteen Show/Season 1/e1.mkv", releaseDate: "2020-01-03", sizeBytes: 1,
		},
		{id: chainShow, kind: "show", title: "Chain Show", rating: "FSK 0"},
		{id: chainHiddenSeason, parentID: chainShow, kind: "season", title: "Chain Locked Season", rating: "FSK 16", number: 1},
		{
			id: chainHidden, parentID: chainHiddenSeason, kind: "episode", title: "Chain Hidden", rating: "FSK 0", number: 1,
			path: "Chain Show/Season 1/e1.mkv", releaseDate: "2020-01-02", sizeBytes: 5,
		},
		{id: chainOpenSeason, parentID: chainShow, kind: "season", title: "Chain Open Season", number: 2},
		{
			id: chainOpen, parentID: chainOpenSeason, kind: "episode", title: "Chain Open", rating: "FSK 6", number: 1,
			path: "Chain Show/Season 2/e1.mkv", releaseDate: "2020-01-01", sizeBytes: 1,
		},
	}
	for _, item := range items {
		insertListingItem(t, fixture.inspection, listingItem{
			id: item.id, libraryID: showsLibraryID, parentID: item.parentID, kind: item.kind, title: item.title,
			sortTitle: item.title, number: item.number, contentRating: item.rating, releaseDate: item.releaseDate,
		})

		if item.path != "" {
			insertListingFile(t, fixture.inspection, listingFile{
				id: "h" + item.id, itemID: item.id, libraryID: showsLibraryID, container: "mkv", relativePath: item.path,
				sizeBytes: item.sizeBytes, streams: []listingStream{video("h264", 1920, 1080, "")},
			})
		}
	}

	if err := sqlite.NewLibraryCatalog(fixture.db, time.Now).BackfillAgeRatings(t.Context()); err != nil {
		t.Fatalf("BackfillAgeRatings error = %v", err)
	}

	return fixture
}

// ancestorFixtureItemIDs lists every show, season and episode of the ancestor fixture.
func ancestorFixtureItemIDs() []string {
	return []string{
		kidsShow, kidsSeason, restrictedEpisodeID(kidsShow, 1), restrictedEpisodeID(kidsShow, 2),
		matureShow, matureSeason, restrictedEpisodeID(matureShow, 1),
		unratedShow, unratedSeason, restrictedEpisodeID(unratedShow, 1), restrictedEpisodeID(unratedShow, 2),
		sixteenShow, sixteenSeason, sixteenChild,
		chainShow, chainHiddenSeason, chainHidden, chainOpenSeason, chainOpen,
	}
}

// showSeasonDirectories are the directories that hold the episodes of the ancestor fixture.
func showSeasonDirectories() []string {
	return []string{
		"Kids Show/Season 1", "Mature Show/Season 1", "Unrated Show/Season 1", "Sixteen Show/Season 1",
		"Chain Show/Season 1", "Chain Show/Season 2",
	}
}

// countText renders summaries as "id:childCount/leafCount".
func countText(summaries []applibrary.ItemSummary) []string {
	texts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		texts = append(texts, fmt.Sprintf("%s:%d/%d", summary.Item.ID, summary.ChildCount, summary.LeafCount))
	}

	return texts
}

// ancestorReads is every read of the fixture's shows library that takes a restriction.
type ancestorReads struct {
	listing, sizeOrder, row, summaries []applibrary.ItemSummary
	groups                             []rows.EpisodeGroup
	recent                             []approws.EpisodeRef
	folderItems                        []string
	directories                        []string
}

func (fixture catalogFixture) readShows(t *testing.T, restriction access.Restriction) ancestorReads {
	t.Helper()

	var (
		reads ancestorReads
		ctx   = t.Context()
		shows = library.ListingQuery{Filter: library.ListingFilter{Kind: library.ItemKindShow}, Restriction: restriction}
		sized = shows
	)

	sized.Order = library.ListingOrder{Sort: library.SortByFileSize}
	episodes := approws.EpisodeRequest{LibraryID: showsLibraryID, Limit: 10, Window: rows.RecentEpisodeWindow, Restriction: restriction}

	page, listErr := fixture.reader.ListItems(ctx, showsLibraryID, library.PageRequest{}, shows)
	sizePage, sizeErr := fixture.reader.ListItems(ctx, showsLibraryID, library.PageRequest{}, sized)
	reads.listing, reads.sizeOrder = page.Items, sizePage.Items

	var rowErr, groupErr, recentErr, summaryErr error

	reads.row, rowErr = fixture.reader.ListRowItems(ctx, approws.ItemRequest{LibraryID: showsLibraryID, Query: shows, Limit: 10})
	reads.groups, groupErr = fixture.reader.ListEpisodeGroups(ctx, episodes)
	reads.recent, recentErr = fixture.reader.ListRecentEpisodes(ctx, episodes)
	reads.summaries, summaryErr = fixture.reader.ItemSummaries(ctx, ancestorFixtureItemIDs(), restriction)

	if err := errors.Join(listErr, sizeErr, rowErr, groupErr, recentErr, summaryErr); err != nil {
		t.Fatalf("read shows: %v", err)
	}

	slices.SortFunc(reads.groups, func(left, right rows.EpisodeGroup) int { return strings.Compare(left.SeasonID, right.SeasonID) })

	reads.folderItems = fixture.folderItemIDs(t, showsLibraryID, restriction, showSeasonDirectories()...)

	for _, directory := range []string{"", "Chain Show", "Unrated Show"} {
		names, err := fixture.reader.ListFolderDirectories(ctx, showsLibraryID, directory, restriction)
		if err != nil {
			t.Fatalf("ListFolderDirectories(%q) error = %v", directory, err)
		}

		for _, name := range names {
			reads.directories = append(reads.directories, directory+"/"+name)
		}
	}

	return reads
}

// groupText renders episode groups as "show/season:count".
func groupText(groups []rows.EpisodeGroup) []string {
	texts := make([]string, 0, len(groups))
	for _, group := range groups {
		texts = append(texts, fmt.Sprintf("%s/%s:%d", group.ShowID, group.SeasonID, group.EpisodeCount))
	}

	return texts
}

func recentIDs(refs []approws.EpisodeRef) []string {
	ids := make([]string, 0, len(refs))
	for _, ref := range refs {
		ids = append(ids, ref.EpisodeID)
	}

	return ids
}

func assertTexts(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

// The "Children" preset (up to 6, unrated hidden) sees an item only when it and every
// ancestor pass: no episode of the FSK 16 show or season, none of the unrated show, and no
// count, row, size order or directory tells that they exist.
func TestListingAncestorRuleForChildren(t *testing.T) {
	t.Parallel()

	fixture := newAncestorFixture(t)
	reads := fixture.readShows(t, childrenRestriction(t))
	kidsEpisode := restrictedEpisodeID(kidsShow, 1)

	wantShows := []string{chainShow + ":1/1", kidsShow + ":1/1"}
	assertTexts(t, "listing", countText(reads.listing), wantShows)
	assertTexts(t, "row", countText(reads.row), wantShows)
	// Both shows have one visible episode of one byte, so the title breaks the tie; the five
	// hidden bytes of Chain Show would put it last.
	assertTexts(t, "size order", countText(reads.sizeOrder), wantShows)
	assertTexts(t, "summaries", countText(reads.summaries), []string{
		kidsShow + ":1/1", kidsSeason + ":1/0", kidsEpisode + ":0/0",
		chainShow + ":1/1", chainOpenSeason + ":1/0", chainOpen + ":0/0",
	})
	assertTexts(t, "episode groups", groupText(reads.groups), []string{
		kidsShow + "/" + kidsSeason + ":1", chainShow + "/" + chainOpenSeason + ":1",
	})
	assertTexts(t, "recent episodes", recentIDs(reads.recent), []string{chainOpen})
	assertTexts(t, "folder items", reads.folderItems, []string{kidsEpisode, chainOpen})
	assertTexts(t, "directories", reads.directories, []string{"/Chain Show", "/Kids Show", "Chain Show/Season 2"})

	// Nothing of a hidden item may leak: not its ID, title or directory.
	serialised := fmt.Sprintf("%+v", reads)
	hidden := []string{
		matureShow, unratedShow, sixteenShow, sixteenSeason, sixteenChild, chainHiddenSeason, chainHidden,
		restrictedEpisodeID(kidsShow, 2), restrictedEpisodeID(unratedShow, 1),
		"Mature Show", "Unrated Show", "Sixteen", "Chain Locked Season", "Chain Hidden", "Chain Show/Season 1",
	}
	for _, text := range hidden {
		if strings.Contains(serialised, text) {
			t.Errorf("the reads of a restricted viewer contain %q of a hidden item", text)
		}
	}
}

// With unrated titles shown, the unrated show and its episodes pass (its TV-G episode and
// the unrated one); the FSK 16 chains stay hidden.
func TestListingAncestorRuleWithUnratedShown(t *testing.T) {
	t.Parallel()

	fixture := newAncestorFixture(t)
	reads := fixture.readShows(t, restrictionOf(access.AllLibraries(), maxAgeLimit(t, access.Age6, false)))

	assertTexts(t, "listing", countText(reads.listing), []string{chainShow + ":1/1", kidsShow + ":1/1", unratedShow + ":1/2"})
	assertTexts(t, "row", countText(reads.row), []string{chainShow + ":1/1", kidsShow + ":1/1", unratedShow + ":1/2"})
	assertTexts(t, "folder items", reads.folderItems, []string{
		restrictedEpisodeID(kidsShow, 1), restrictedEpisodeID(unratedShow, 1), restrictedEpisodeID(unratedShow, 2), chainOpen,
	})
	assertTexts(t, "directories", reads.directories,
		[]string{"/Chain Show", "/Kids Show", "/Unrated Show", "Chain Show/Season 2", "Unrated Show/Season 1"})

	// Kids Show (show, season, first episode), Unrated Show (show, season, both episodes) and
	// Chain Show (show, season 2, its episode).
	if want := 10; len(reads.summaries) != want {
		t.Errorf("summaries = %v, want the %d items of the three visible shows", countText(reads.summaries), want)
	}
}

// Only unrated hidden (no maximum age): a rated episode of an unrated show is hidden with it.
func TestListingAncestorRuleHidesTheSubtreeOfAnUnratedShow(t *testing.T) {
	t.Parallel()

	fixture := newAncestorFixture(t)
	reads := fixture.readShows(t, restrictionOf(access.AllLibraries(), access.NoLimit().HidingUnrated(true)))

	assertTexts(t, "listing", countText(reads.listing),
		[]string{chainShow + ":2/2", kidsShow + ":1/2", matureShow + ":1/1", sixteenShow + ":1/1"})

	if serialised := fmt.Sprintf("%+v", reads); strings.Contains(serialised, unratedShow[1:8]) || strings.Contains(serialised, "Unrated Show") {
		t.Errorf("the reads contain the unrated show or one of its episodes: %s", serialised)
	}
}

// An unrestricted viewer sees everything with the counts of before the ancestor rule.
func TestListingAncestorRuleLeavesUnrestrictedReadsUnchanged(t *testing.T) {
	t.Parallel()

	fixture := newAncestorFixture(t)
	reads := fixture.readShows(t, access.Unrestricted())

	wantShows := []string{chainShow + ":2/2", kidsShow + ":1/2", matureShow + ":1/1", sixteenShow + ":1/1", unratedShow + ":1/2"}
	assertTexts(t, "listing", countText(reads.listing), wantShows)
	assertTexts(t, "row", countText(reads.row), wantShows)
	assertTexts(t, "size order", countText(reads.sizeOrder),
		[]string{matureShow + ":1/1", sixteenShow + ":1/1", kidsShow + ":1/2", unratedShow + ":1/2", chainShow + ":2/2"})
	assertTexts(t, "recent episodes", recentIDs(reads.recent), []string{sixteenChild, chainHidden, chainOpen})

	if got, want := len(reads.summaries), len(ancestorFixtureItemIDs()); got != want {
		t.Errorf("summaries = %d, want all %d items", got, want)
	}

	if len(reads.groups) != 6 || len(reads.folderItems) != 8 {
		t.Errorf("episode groups = %d and folder items = %d, want 6 seasons and 8 episodes", len(reads.groups), len(reads.folderItems))
	}
}

// The zero restriction (a caller that forgot to set one) reads nothing.
func TestRowsAndSummariesOfTheZeroRestrictionAreEmpty(t *testing.T) {
	t.Parallel()

	fixture := newAncestorFixture(t)
	shows := library.ListingQuery{Filter: library.ListingFilter{Kind: library.ItemKindShow}}

	_, rowErr := fixture.reader.ListRowItems(t.Context(), approws.ItemRequest{LibraryID: showsLibraryID, Query: shows, Limit: 10})
	_, groupErr := fixture.reader.ListEpisodeGroups(t.Context(), approws.EpisodeRequest{LibraryID: showsLibraryID, Limit: 10})
	_, recentErr := fixture.reader.ListRecentEpisodes(t.Context(), approws.EpisodeRequest{LibraryID: showsLibraryID, Limit: 10})

	for name, err := range map[string]error{"ListRowItems": rowErr, "ListEpisodeGroups": groupErr, "ListRecentEpisodes": recentErr} {
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s error = %v, want domain.ErrNotFound", name, err)
		}
	}

	summaries, err := fixture.reader.ItemSummaries(t.Context(), ancestorFixtureItemIDs(), access.Restriction{})
	if err != nil || len(summaries) != 0 {
		t.Errorf("ItemSummaries = %v, %v, want none", countText(summaries), err)
	}

	otherLibrary := restrictionOf(access.OnlyLibraries(moviesLibraryID), access.NoLimit())

	summaries, err = fixture.reader.ItemSummaries(t.Context(), []string{kidsShow, ratedZero}, otherLibrary)
	if err != nil || !slices.Equal(countText(summaries), []string{ratedZero + ":0/0"}) {
		t.Errorf("ItemSummaries of another library = %v, %v, want only the movie", countText(summaries), err)
	}
}

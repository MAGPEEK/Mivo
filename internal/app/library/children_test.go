package library_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// IDs of the show tree the children tests use.
const (
	showID       = "show"
	seasonOneID  = "season-1"
	seasonTwoID  = "season-2"
	specialsID   = "season-0"
	showExtraID  = "show-extra"
	episodeOneID = "episode-1"
	movieID      = "movie"
)

func container(id, parentID string, kind library.ItemKind, title string) applibrary.ItemSummary {
	return applibrary.ItemSummary{Item: library.Item{
		ID: id, LibraryID: showsID, ParentID: parentID, Kind: kind, Title: title, SortTitle: title,
	}}
}

func numbered(summary applibrary.ItemSummary, number int) applibrary.ItemSummary {
	summary.Item.Number = library.NumberOf(number)

	return summary
}

func leaf(id, parentID string, kind library.ItemKind, title string) applibrary.ItemSummary {
	summary := container(id, parentID, kind, title)
	summary.PrimaryFile = media.File{ID: "file-" + id, ItemID: id, LibraryID: showsID, RelativePath: title + ".mkv"}

	return summary
}

// putShow stores a show with two regular seasons, the specials and an extra, all out of
// order, and the episodes of season one with numbered and unnumbered episodes.
func (environment *testEnvironment) putShow() {
	extra := leaf(showExtraID, showID, library.ItemKindExtra, "aaa trailer")
	extra.Item.ExtraType = library.ExtraTypeTrailer

	environment.children.put(
		container(showID, "", library.ItemKindShow, "show"),
		numbered(container(seasonTwoID, showID, library.ItemKindSeason, "season 2"), 2),
		numbered(container(specialsID, showID, library.ItemKindSeason, "specials"), 0),
		extra,
		numbered(container(seasonOneID, showID, library.ItemKindSeason, "season 1"), 1),
		leaf("episode-unnumbered-b", seasonOneID, library.ItemKindEpisode, "b unnumbered"),
		numbered(leaf("episode-3", seasonOneID, library.ItemKindEpisode, "three"), 3),
		leaf("episode-unnumbered-a", seasonOneID, library.ItemKindEpisode, "a unnumbered"),
		numbered(leaf(episodeOneID, seasonOneID, library.ItemKindEpisode, "one"), 1),
		numbered(leaf("episode-2", seasonOneID, library.ItemKindEpisode, "two"), 2),
		leaf(movieID, "", library.ItemKindMovie, "movie"),
	)
}

func childIDs(page applibrary.ItemPage) []string {
	ids := make([]string, 0, len(page.Items))
	for _, summary := range page.Items {
		ids = append(ids, summary.Item.ID)
	}

	return ids
}

func TestListChildrenOrdersByTheContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		itemID     string
		descending bool
		want       []string
	}{
		{
			name:   "seasons ascend with the specials last and without extras",
			itemID: showID,
			want:   []string{seasonOneID, seasonTwoID, specialsID},
		},
		{
			name:       "descending reverses the seasons",
			itemID:     showID,
			descending: true,
			want:       []string{specialsID, seasonTwoID, seasonOneID},
		},
		{
			name:   "episodes ascend with the unnumbered last by sort title",
			itemID: seasonOneID,
			want: []string{
				episodeOneID, "episode-2", "episode-3", "episode-unnumbered-a", "episode-unnumbered-b",
			},
		},
		{
			name:       "descending reverses the episodes",
			itemID:     seasonOneID,
			descending: true,
			want: []string{
				"episode-unnumbered-b", "episode-unnumbered-a", "episode-3", "episode-2", episodeOneID,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.putShow()

			page, err := environment.service(t).ListChildren(t.Context(), test.itemID,
				applibrary.ChildPageRequest{Descending: test.descending})
			if err != nil {
				t.Fatalf("ListChildren error = %v", err)
			}

			if got := childIDs(page); !slices.Equal(got, test.want) || page.NextCursor != "" {
				t.Errorf("ListChildren = %v (cursor %q), want %v on one page", got, page.NextCursor, test.want)
			}
		})
	}
}

func TestListChildrenFollowsTheCursorAndNormalisesTheLimit(t *testing.T) {
	t.Parallel()

	const pageLimit = 2

	environment := newTestEnvironment()
	environment.putShow()
	service := environment.service(t)

	request := applibrary.ChildPageRequest{Descending: true, Page: library.PageRequest{Limit: pageLimit}}

	var listed []string

	for pages := 1; ; pages++ {
		page, err := service.ListChildren(t.Context(), seasonOneID, request)
		if err != nil {
			t.Fatalf("ListChildren page %d error = %v", pages, err)
		}

		listed = append(listed, childIDs(page)...)

		if page.NextCursor == "" {
			break
		}

		if pages > library.MaxPageLimit {
			t.Fatal("ListChildren never reached the last page")
		}

		request.Page.Cursor = page.NextCursor
	}

	want := []string{"episode-unnumbered-b", "episode-unnumbered-a", "episode-3", "episode-2", episodeOneID}
	if !slices.Equal(listed, want) {
		t.Errorf("listed %v over all pages, want %v", listed, want)
	}

	_, err := service.ListChildren(t.Context(), seasonOneID, applibrary.ChildPageRequest{})
	if err != nil {
		t.Fatalf("ListChildren error = %v", err)
	}

	requests := environment.children.recordedRequests()
	if got := requests[len(requests)-1].request.Page.Limit; got != library.DefaultPageLimit {
		t.Errorf("reader limit for an unset limit = %d, want %d", got, library.DefaultPageLimit)
	}
}

func TestListChildrenOfALeafOrAnEmptyContainerIsEmpty(t *testing.T) {
	t.Parallel()

	for _, itemID := range []string{movieID, episodeOneID, showExtraID, specialsID} {
		t.Run(itemID, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.putShow()

			page, err := environment.service(t).ListChildren(t.Context(), itemID, applibrary.ChildPageRequest{})
			if err != nil {
				t.Fatalf("ListChildren error = %v", err)
			}

			if len(page.Items) != 0 || page.NextCursor != "" {
				t.Errorf("ListChildren = %+v, want an empty last page", page)
			}
		})
	}
}

func TestListChildrenFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testEnvironment)
		itemID    string
		request   applibrary.ChildPageRequest
		wantErr   error
	}{
		{name: "unknown item", itemID: unknownID, wantErr: domain.ErrNotFound},
		{
			name: "tampered cursor", itemID: showID, wantErr: domain.ErrInvalid,
			request: applibrary.ChildPageRequest{Page: library.PageRequest{Cursor: "not-a-cursor"}},
		},
		{
			name: "cursor past the end", itemID: showID, wantErr: domain.ErrInvalid,
			request: applibrary.ChildPageRequest{Page: library.PageRequest{Cursor: ascendingCursorPrefix + "4"}},
		},
		{
			name: "cursor of the other direction", itemID: showID, wantErr: domain.ErrInvalid,
			request: applibrary.ChildPageRequest{
				Descending: true, Page: library.PageRequest{Cursor: ascendingCursorPrefix + "1"},
			},
		},
		{
			name:      "reader fails",
			configure: func(environment *testEnvironment) { environment.children.listErr = errBoom },
			itemID:    showID, wantErr: errBoom,
		},
		{
			name:      "no child reader wired",
			configure: func(environment *testEnvironment) { environment.children = nil },
			itemID:    showID, wantErr: domain.ErrUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.putShow()

			if test.configure != nil {
				test.configure(environment)
			}

			page, err := environment.service(t).ListChildren(t.Context(), test.itemID, test.request)
			assertWrappedError(t, err, test.wantErr, "list children of item")

			if page.Items != nil || page.NextCursor != "" {
				t.Errorf("ListChildren = %+v on error, want zero value", page)
			}
		})
	}
}

// A stored row the reader cannot read is a server fault: the service must not turn it into
// ErrInvalid (that would answer 400).
func TestListChildrenPassesAStoredRowFaultOnAsAPlainError(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.putShow()
	environment.children.listErr = fmt.Errorf("read season row: %w", errBoom)

	_, err := environment.service(t).ListChildren(t.Context(), showID, applibrary.ChildPageRequest{})
	if !errors.Is(err, errBoom) || errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ListChildren error = %v, want the reader fault and not ErrInvalid", err)
	}
}

func TestListChildrenRejectsALimitOutOfRangeWithoutReading(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{-1, library.MaxPageLimit + 1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.putShow()

			_, err := environment.service(t).ListChildren(t.Context(), showID,
				applibrary.ChildPageRequest{Page: library.PageRequest{Limit: limit}})
			assertWrappedError(t, err, domain.ErrInvalid, "list children of item")

			if requests := environment.children.recordedRequests(); len(requests) != 0 {
				t.Errorf("reader requests = %+v, want none", requests)
			}
		})
	}
}

func TestGetItemPassesTheV2DetailOn(t *testing.T) {
	t.Parallel()

	poster := artwork.Ref{ID: "poster-1", Tag: "tag-1"}
	episode := leaf(episodeOneID, seasonOneID, library.ItemKindEpisode, "one")
	want := applibrary.ItemDetail{
		Item:     episode.Item,
		Files:    []media.File{episode.PrimaryFile},
		Metadata: metadata.Record{Title: "Pilot", Year: 2008, Genres: []string{"Drama"}},
		Ancestors: []applibrary.ItemRef{
			{ID: showID, Kind: library.ItemKindShow, Title: "Show"},
			{ID: seasonOneID, Kind: library.ItemKindSeason, Title: "Season 1", Number: library.NumberOf(1)},
		},
		NextID:     "episode-2",
		PlayItemID: episodeOneID,
		Artwork:    map[artwork.Kind]artwork.Ref{artwork.KindThumb: poster},
		ItemFiles: []applibrary.ItemFile{
			{File: episode.PrimaryFile, VersionLabel: "1080p", PartNumber: library.NumberOf(1)},
		},
	}

	environment := newTestEnvironment()
	environment.items.details[episodeOneID] = want

	detail, err := environment.service(t).GetItem(t.Context(), episodeOneID)
	if err != nil {
		t.Fatalf("GetItem error = %v", err)
	}

	if detail.Metadata.Title != "Pilot" || !slices.Equal(detail.Ancestors, want.Ancestors) ||
		detail.NextID != want.NextID || detail.PlayItemID != want.PlayItemID ||
		detail.Artwork[artwork.KindThumb] != poster || len(detail.ItemFiles) != 1 ||
		detail.ItemFiles[0].VersionLabel != "1080p" {
		t.Errorf("GetItem = %+v, want %+v", detail, want)
	}
}

func TestHasPrimaryFile(t *testing.T) {
	t.Parallel()

	if !leaf(movieID, "", library.ItemKindMovie, "movie").HasPrimaryFile() {
		t.Error("a movie summary has no primary file, want one")
	}

	if container(showID, "", library.ItemKindShow, "show").HasPrimaryFile() {
		t.Error("a show summary has a primary file, want none")
	}
}

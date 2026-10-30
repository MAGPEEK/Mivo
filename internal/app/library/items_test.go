package library_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const itemID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"

// putItems stores count items with one file each in the movies library.
func (environment *testEnvironment) putItems(count int) []applibrary.ItemSummary {
	summaries := make([]applibrary.ItemSummary, 0, count)

	for index := range count {
		item := library.Item{
			ID:        fmt.Sprintf("item-%03d", index),
			LibraryID: moviesID,
			Kind:      library.ItemKindMovie,
			Title:     fmt.Sprintf("Title %03d", index),
			SortTitle: fmt.Sprintf("title %03d", index),
		}
		summaries = append(summaries, applibrary.ItemSummary{Item: item, PrimaryFile: media.File{
			ID: "file-" + item.ID, ItemID: item.ID, LibraryID: moviesID,
			RelativePath: item.Title + ".mkv", Size: int64(index),
		}})
	}

	environment.items.items[moviesID] = summaries

	return summaries
}

func TestListItemsNormalisesTheLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{name: "unset limit means the default", limit: 0, wantLimit: library.DefaultPageLimit},
		{name: "smallest limit", limit: 1, wantLimit: 1},
		{name: "largest limit", limit: library.MaxPageLimit, wantLimit: library.MaxPageLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.storedMoviesAndShows()
			all := environment.putItems(library.MaxPageLimit + 1)

			page, err := environment.service(t).ListItems(t.Context(), moviesID, library.PageRequest{Limit: test.limit})
			if err != nil {
				t.Fatalf("ListItems error = %v", err)
			}

			if !slices.EqualFunc(page.Items, all[:test.wantLimit], itemSummariesEqual) {
				t.Errorf("ListItems returned %d items, want the first %d", len(page.Items), test.wantLimit)
			}

			want := []listRequest{{libraryID: moviesID, page: library.PageRequest{Limit: test.wantLimit}}}
			if got := environment.items.recordedRequests(); !slices.Equal(got, want) {
				t.Errorf("reader requests = %+v, want %+v", got, want)
			}
		})
	}
}

func itemSummariesEqual(first, second applibrary.ItemSummary) bool {
	return first.Item == second.Item && first.PrimaryFile.ID == second.PrimaryFile.ID
}

func TestListItemsRejectsALimitOutOfRangeWithoutReading(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{-1, library.MaxPageLimit + 1} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.storedMoviesAndShows()

			page, err := environment.service(t).ListItems(t.Context(), moviesID, library.PageRequest{Limit: limit})
			assertWrappedError(t, err, domain.ErrInvalid, "list items of library")

			if page.Items != nil || page.NextCursor != "" {
				t.Errorf("ListItems = %+v on error, want zero value", page)
			}

			if requests := environment.items.recordedRequests(); len(requests) != 0 {
				t.Errorf("reader requests = %+v, want none", requests)
			}
		})
	}
}

// The reader alone cannot tell an unknown library from an empty one, so the service asks the
// library store first.
func TestListItemsOfAnUnknownLibraryReturnsNotFound(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	environment.storedMoviesAndShows()

	_, err := environment.service(t).ListItems(t.Context(), unknownID, library.PageRequest{})
	assertWrappedError(t, err, domain.ErrNotFound, "list items of library")

	if requests := environment.items.recordedRequests(); len(requests) != 0 {
		t.Errorf("reader requests = %+v, want none", requests)
	}
}

func TestListItemsFollowsTheCursorToTheLastPage(t *testing.T) {
	t.Parallel()

	const (
		itemCount = 5
		pageLimit = 2
	)

	environment := newTestEnvironment()
	environment.storedMoviesAndShows()
	all := environment.putItems(itemCount)
	service := environment.service(t)

	var listed []applibrary.ItemSummary

	request := library.PageRequest{Limit: pageLimit}

	for pages := 1; ; pages++ {
		page, err := service.ListItems(t.Context(), moviesID, request)
		if err != nil {
			t.Fatalf("ListItems page %d error = %v", pages, err)
		}

		listed = append(listed, page.Items...)

		if page.NextCursor == "" {
			break
		}

		if pages > itemCount {
			t.Fatal("ListItems never reached the last page")
		}

		request.Cursor = page.NextCursor
	}

	if !slices.EqualFunc(listed, all, itemSummariesEqual) {
		t.Errorf("listed %d items over all pages, want all %d in order", len(listed), len(all))
	}
}

func TestListItemsWrapsReaderAndStoreErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testEnvironment)
		cursor    string
		wantErr   error
	}{
		{
			name:      "reader fails",
			configure: func(environment *testEnvironment) { environment.items.listErr = errBoom },
			wantErr:   errBoom,
		},
		{
			name:      "store fails",
			configure: func(environment *testEnvironment) { environment.store.getErr = errBoom },
			wantErr:   errBoom,
		},
		{
			name:      "tampered cursor",
			configure: func(*testEnvironment) {},
			cursor:    "not-a-cursor",
			wantErr:   domain.ErrInvalid,
		},
		{
			name:      "cursor past the end",
			configure: func(environment *testEnvironment) { environment.putItems(2) },
			cursor:    "3",
			wantErr:   domain.ErrInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			environment.storedMoviesAndShows()
			test.configure(environment)

			page, err := environment.service(t).ListItems(t.Context(), moviesID,
				library.PageRequest{Cursor: test.cursor})
			assertWrappedError(t, err, test.wantErr, "list items of library")

			if page.Items != nil || page.NextCursor != "" {
				t.Errorf("ListItems = %+v on error, want zero value", page)
			}
		})
	}
}

func TestGetItemReturnsTheItemWithItsFiles(t *testing.T) {
	t.Parallel()

	environment := newTestEnvironment()
	want := applibrary.ItemDetail{
		Item: library.Item{
			ID: itemID, LibraryID: moviesID, Kind: library.ItemKindMovie,
			Title: "Big Buck Bunny", SortTitle: "big buck bunny", CreatedAt: time.Unix(1, 0).UTC(),
		},
		Files: []media.File{{ID: "file-1", ItemID: itemID, LibraryID: moviesID, RelativePath: "Big.Buck.Bunny.mkv"}},
	}
	environment.items.details[itemID] = want

	detail, err := environment.service(t).GetItem(t.Context(), itemID)
	if err != nil {
		t.Fatalf("GetItem error = %v", err)
	}

	if detail.Item != want.Item || len(detail.Files) != 1 || detail.Files[0].ID != want.Files[0].ID {
		t.Errorf("GetItem = %+v, want %+v", detail, want)
	}
}

func TestGetItemFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configure func(*testEnvironment)
		wantErr   error
	}{
		{name: "unknown item", configure: func(*testEnvironment) {}, wantErr: domain.ErrNotFound},
		{
			name:      "reader fails",
			configure: func(environment *testEnvironment) { environment.items.getErr = errBoom },
			wantErr:   errBoom,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newTestEnvironment()
			test.configure(environment)

			detail, err := environment.service(t).GetItem(t.Context(), itemID)
			assertWrappedError(t, err, test.wantErr, "get item")

			if detail.Item != (library.Item{}) || detail.Files != nil {
				t.Errorf("GetItem = %+v on error, want zero value", detail)
			}
		})
	}
}

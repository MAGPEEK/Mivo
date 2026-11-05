package sqlite_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

// An audiobook sorts by the sum of its parts' durations, not by its first part (API 1.8.0).
func TestListItemsSortsAudiobooksByTheirTotalDuration(t *testing.T) {
	t.Parallel()

	const (
		booksLibraryID = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		bookLong       = "k0000001-0000-4000-8000-000000000000"
		bookShort      = "k0000002-0000-4000-8000-000000000000"
	)

	fixture := newCatalogFixture(t)
	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES (?1, 'Books', 'audiobooks', 0, 0)`, booksLibraryID)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, '/media/books', '/media/books', 0, 0)`, booksLibraryID)

	// The long book has a short first part and two more; the short book one long part.
	books := []struct {
		id, title string
		parts     []int64
	}{{bookLong, "Long", []int64{1000, 4000, 4000}}, {bookShort, "Short", []int64{5000}}}

	for bookIndex, book := range books {
		insertListingItem(t, fixture.inspection, listingItem{id: book.id, libraryID: booksLibraryID, kind: "audiobook", title: book.title, sortTitle: book.title})

		for part, millis := range book.parts {
			insertListingFile(t, fixture.inspection, listingFile{
				id: fmt.Sprintf("p%07d-%d000-4000-8000-000000000000", bookIndex+1, part), itemID: book.id, libraryID: booksLibraryID,
				relativePath: fmt.Sprintf("%s/%d.m4b", book.title, part+1), container: "mp4", durationMillis: millis, partNumber: part + 1,
			})
		}
	}

	query := library.ListingQuery{
		Filter: library.ListingFilter{Kind: library.ItemKindAudiobook},
		Order:  library.ListingOrder{Sort: library.SortByDuration},
	}

	got, _ := fixture.listAllIDs(t, booksLibraryID, query, 1)
	if want := []string{bookShort, bookLong}; !slices.Equal(got, want) {
		t.Errorf("ids = %v, want %v (5 s before 9 s)", got, want)
	}
}

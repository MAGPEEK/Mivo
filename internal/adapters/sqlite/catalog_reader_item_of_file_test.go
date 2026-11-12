package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/app/activity"
	"github.com/magpeek/mivo/internal/domain"
)

var _ activity.ItemTitles = (*sqlite.CatalogReader)(nil)

func TestItemOfFileNamesTheCatalogItemAndReportsUnknownFiles(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "Big Buck Bunny (2008).mkv")
	mustSave(t, fixture.writer, record)

	item, err := fixture.reader.ItemOfFile(t.Context(), record.File.ID)
	if err != nil {
		t.Fatalf("ItemOfFile error = %v", err)
	}

	if item.ID != record.Item.ID || item.Title != record.Item.Title {
		t.Errorf("ItemOfFile = %+v, want item %s titled %q", item, record.Item.ID, record.Item.Title)
	}

	if _, err := fixture.reader.ItemOfFile(t.Context(), testID('f', 99)); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ItemOfFile of an unknown file error = %v, want ErrNotFound", err)
	}
}

package sqlite_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// placementFixture is a catalog fixture with the v2 catalog on it and an ID generator that
// numbers items in creation order.
type placementFixture struct {
	catalogFixture
	catalog *sqlite.LibraryCatalog
	counter int
}

func newPlacementFixture(t *testing.T) *placementFixture {
	t.Helper()

	base := newCatalogFixture(t)

	return &placementFixture{
		catalogFixture: base,
		catalog:        sqlite.NewLibraryCatalog(base.db, func() time.Time { return time.UnixMilli(scannedMilli).UTC() }),
	}
}

func (fixture *placementFixture) newID() (string, error) {
	fixture.counter++

	return testID('i', fixture.counter), nil
}

func (fixture *placementFixture) save(t *testing.T, record scan.PlacedFile) []string {
	t.Helper()

	chain, err := fixture.catalog.SavePlacedFile(t.Context(), record, fixture.newID)
	if err != nil {
		t.Fatalf("SavePlacedFile(%s) error = %v", record.File.RelativePath, err)
	}

	return chain
}

// placedFile wraps a probed file of the movies library (or shows, by libraryID) for a placement.
func placedFile(libraryID, fileID, relativePath string, placement library.Placement) scan.PlacedFile {
	record := probedRecord(libraryID, "", fileID, relativePath)

	return scan.PlacedFile{RootID: libraryID, Placement: placement, File: record.File, ProbedAt: record.ProbedAt}
}

func moviePlacement(key, title string, year int, version string) library.Placement {
	return library.Placement{
		Levels:       []library.Level{{Kind: library.ItemKindMovie, MatchKey: key, Title: title, Year: library.NumberOf(year)}},
		VersionLabel: version,
	}
}

func episodePlacement(show string, season, episode int) library.Placement {
	return library.Placement{Levels: []library.Level{
		{Kind: library.ItemKindShow, MatchKey: show, Title: show},
		{Kind: library.ItemKindSeason, MatchKey: fmt.Sprintf("s%d", season), Title: fmt.Sprintf("Season %d", season), Number: library.NumberOf(season)},
		{
			Kind: library.ItemKindEpisode, MatchKey: fmt.Sprintf("e%d", episode), Title: fmt.Sprintf("Episode %d", episode),
			Number: library.NumberOf(episode), AbsoluteNumber: library.NumberOf(episode),
		},
	}}
}

func itemCount(t *testing.T, fixture *placementFixture, kind string) int {
	t.Helper()

	var count int

	err := fixture.inspection.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM items WHERE kind = ?", kind).Scan(&count)
	if err != nil {
		t.Fatalf("count %s items: %v", kind, err)
	}

	return count
}

func streamsOf(file media.File) int { return len(file.Description.Streams) }

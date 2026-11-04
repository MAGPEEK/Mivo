package sqlite_test

import (
	"database/sql"
	"fmt"
	"testing"
)

// noNumber marks an item without a number in itemSpec.
const noNumber = -1

// itemSpec is one items row of the plain-SQL fixtures of the reader tests; they insert rows
// directly so they do not depend on the catalog writers.
type itemSpec struct {
	id        string
	libraryID string
	parentID  string
	kind      string
	title     string
	sortTitle string
	number    int
	year      int
	extraType string
}

// newItemSpec returns a spec with the title as sort title, no number and no parent.
func newItemSpec(id, libraryID, kind, title string) itemSpec {
	return itemSpec{id: id, libraryID: libraryID, kind: kind, title: title, sortTitle: title, number: noNumber}
}

func nullable[T comparable](value, none T) any {
	if value == none {
		return nil
	}

	return value
}

func insertItem(t *testing.T, inspection *sql.DB, spec itemSpec) {
	t.Helper()

	execute(t, inspection, `INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title,
		year, number, extra_type, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		spec.id, spec.libraryID, nullable(spec.parentID, ""), spec.kind, spec.id, spec.title, spec.sortTitle,
		spec.year, nullable(spec.number, noNumber), nullable(spec.extraType, ""), scannedMilli, scannedMilli)
}

// fileSpec is one media_files row with a valid probe result.
type fileSpec struct {
	id           string
	itemID       string
	libraryID    string
	relativePath string
	versionLabel string
	partNumber   int
}

func insertFile(t *testing.T, inspection *sql.DB, spec fileSpec) {
	t.Helper()

	execute(t, inspection, `INSERT INTO media_files (id, item_id, library_id, root_id, relative_path,
		size_bytes, modified_at, container, format_name, duration_ms, bit_rate, probed_at, version_label,
		part_number) VALUES (?, ?, ?3, ?3, ?, 1000, ?, 'mkv', 'matroska,webm', 60000, 1000, ?, ?, ?)`,
		spec.id, spec.itemID, spec.libraryID, spec.relativePath, fileModifiedMilli, scannedMilli,
		spec.versionLabel, spec.partNumber)
}

// artworkSpec is one local artwork row.
type artworkSpec struct {
	id           string
	itemID       string
	kind         string
	libraryID    string
	relativePath string
	tag          string
	selected     bool
}

func insertLocalArtwork(t *testing.T, inspection *sql.DB, spec artworkSpec) {
	t.Helper()

	execute(t, inspection, `INSERT INTO artwork (id, item_id, kind, source, root_id, relative_path,
		size_bytes, modified_at, tag, selected, created_at) VALUES (?, ?, ?, 'local', ?, ?, 2048, ?, ?, ?, ?)`,
		spec.id, spec.itemID, spec.kind, spec.libraryID, spec.relativePath, fileModifiedMilli, spec.tag,
		spec.selected, scannedMilli)
}

// showFixture is a show with seasons 1, 2 and the specials; season 1 has the given number
// of episodes with files, season 2 is empty, the specials hold one episode.
type showFixture struct {
	catalogFixture
	showID, season1ID, season2ID, specialsID string
}

func newShowFixture(t *testing.T, season1Episodes int) showFixture {
	t.Helper()

	fixture := showFixture{
		catalogFixture: newCatalogFixture(t),
		showID:         testID('s', 1), season1ID: testID('s', 2), season2ID: testID('s', 3), specialsID: testID('s', 4),
	}

	show := newItemSpec(fixture.showID, showsLibraryID, "show", "The Show")
	show.sortTitle = "show"
	insertItem(t, fixture.inspection, show)

	// Inserted out of order: the specials first.
	for _, season := range []struct {
		id     string
		number int
	}{{fixture.specialsID, 0}, {fixture.season2ID, 2}, {fixture.season1ID, 1}} {
		spec := newItemSpec(season.id, showsLibraryID, "season", fmt.Sprintf("Season %d", season.number))
		spec.parentID, spec.number = fixture.showID, season.number
		insertItem(t, fixture.inspection, spec)
	}

	for number := 1; number <= season1Episodes; number++ {
		fixture.insertEpisode(t, fixture.season1ID, testID('e', number), number)
	}

	fixture.insertEpisode(t, fixture.specialsID, testID('e', 9_000), 1)

	return fixture
}

// insertEpisode stores an episode with one file; number noNumber leaves it unnumbered.
func (fixture showFixture) insertEpisode(t *testing.T, seasonID, episodeID string, number int) {
	t.Helper()

	spec := newItemSpec(episodeID, showsLibraryID, "episode", "Episode "+episodeID[:8])
	spec.parentID, spec.number = seasonID, number
	insertItem(t, fixture.inspection, spec)
	insertFile(t, fixture.inspection, fileSpec{
		id: "f" + episodeID[1:], itemID: episodeID, libraryID: showsLibraryID, relativePath: episodeID + ".mkv",
	})
}

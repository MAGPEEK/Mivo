package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Libraries every catalog fixture starts with. The catalog tests insert them directly, so
// they do not depend on the library sync.
const (
	moviesLibraryID = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	showsLibraryID  = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	unknownID       = "99999999-9999-4999-8999-999999999999"
)

// scannedMilli is when the test records were probed and their items created (2026-10-30
// 12:00 UTC), in Unix milliseconds.
const scannedMilli int64 = 1_793_361_600_000

// fileModifiedMilli is the modification time of the test files (2026-10-01 00:00 UTC), in
// Unix milliseconds.
const fileModifiedMilli int64 = 1_790_812_800_000

// writeTimeout bounds the writes of the recovery tests: a writer connection leaked by a
// failed write would otherwise block the next write forever (the writer pool has one).
const writeTimeout = 10 * time.Second

// catalogFixture is a migrated database with the movies and shows libraries, the catalog
// writer and reader on it and an independent inspection connection with foreign keys on.
type catalogFixture struct {
	writer     *sqlite.CatalogWriter
	reader     *sqlite.CatalogReader
	db         *sqlite.DB
	inspection *sql.DB
}

func newCatalogFixture(t *testing.T) catalogFixture {
	t.Helper()

	db, path := openMigratedTestDB(t)
	fixture := catalogFixture{
		writer:     sqlite.NewCatalogWriter(db),
		reader:     sqlite.NewCatalogReader(db),
		db:         db,
		inspection: openForeignKeyInspectionDB(t, path),
	}

	execute(t, fixture.inspection, `INSERT INTO libraries (id, name, kind, created_at, updated_at)
		VALUES (?, 'Movies', 'movies', 0, 0), (?, 'Shows', 'shows', 0, 0)`, moviesLibraryID, showsLibraryID)
	// The folder of a fixture library shares the library's id, as migration 00004 does.
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?1, ?1, ?2, ?2, 0, 0), (?3, ?3, ?4, ?4, 0, 0)`,
		moviesLibraryID, moviesRoot, showsLibraryID, showsRoot)

	return fixture
}

func execute(t *testing.T, inspection *sql.DB, query string, args ...any) {
	t.Helper()

	if _, err := inspection.ExecContext(t.Context(), query, args...); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
}

// testItemKind returns the item kind a library of the fixture requires.
func testItemKind(libraryID string) library.ItemKind {
	if libraryID == showsLibraryID {
		return library.ItemKindEpisode
	}

	return library.ItemKindMovie
}

// probedRecord returns a successfully probed file at relativePath with a new item titled
// after the file, a video stream that sets every column and an audio stream, as a scan at
// scannedMilli produces it.
func probedRecord(libraryID, itemID, fileID, relativePath string) sqlite.ProbedFile {
	title := library.TitleFromFileName(relativePath)
	scanned := time.UnixMilli(scannedMilli).UTC()

	return sqlite.ProbedFile{
		Item: library.Item{
			ID: itemID, LibraryID: libraryID, Kind: testItemKind(libraryID), MatchKey: relativePath,
			Title: title, SortTitle: library.SortTitle(title), CreatedAt: scanned,
		},
		File: media.File{
			ID: fileID, ItemID: itemID, LibraryID: libraryID, RelativePath: relativePath,
			Size: 734_003_200, ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
			Description: media.Description{
				Container: media.ContainerMKV, FormatName: "matroska,webm",
				Duration: 5_423_042 * time.Millisecond, BitRate: 8_000_000,
				Streams: []media.Stream{videoStream(), audioStream()},
			},
		},
		ProbedAt: scanned,
	}
}

func videoStream() media.Stream {
	return media.Stream{
		Index: 0, Kind: media.StreamKindVideo, Codec: "hevc", CodecTag: "hvc1", Profile: "Main 10",
		Level: 150, PixelFormat: "yuv420p10le", BitDepth: 10, Width: 3840, Height: 2160,
		FrameRate: media.NewFrameRate(24000, 1001), ColorPrimaries: "bt2020", ColorTransfer: "smpte2084",
		ColorSpace: "bt2020nc", HDR: media.HDRFormatHDR10, Language: "und", Title: "Main",
		Default: true, BitRate: 7_000_000,
	}
}

func audioStream() media.Stream {
	return media.Stream{
		Index: 1, Kind: media.StreamKindAudio, Codec: "eac3", Channels: 6,
		ChannelLayout: "5.1(side)", SampleRate: 48_000, Language: "deu", Title: "Deutsch",
		Default: true, Forced: true, BitRate: 640_000,
	}
}

// failedRecord is probedRecord after a failed probe: no description, a probe error.
func failedRecord(libraryID, itemID, fileID, relativePath string) sqlite.ProbedFile {
	record := probedRecord(libraryID, itemID, fileID, relativePath)
	record.File.Description = media.Description{}
	record.File.ProbeError = media.ProbeErrorTimedOut

	return record
}

func mustSave(t *testing.T, writer *sqlite.CatalogWriter, record sqlite.ProbedFile) {
	t.Helper()

	if err := writer.SaveProbedFile(t.Context(), record); err != nil {
		t.Fatalf("SaveProbedFile(%s) error = %v", record.File.RelativePath, err)
	}
}

func assertDeepEqual(t *testing.T, what string, got, want any) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s =\n%+v\nwant\n%+v", what, got, want)
	}
}

// catalogRowCounts returns the number of rows of every catalog table.
func catalogRowCounts(t *testing.T, inspection *sql.DB) map[string]int {
	t.Helper()

	counts := map[string]int{}
	for _, table := range catalogTables() {
		counts[table] = rowCount(t, inspection, table)
	}

	return counts
}

func streamRowCount(t *testing.T, inspection *sql.DB, fileID string) int {
	t.Helper()

	var count int

	err := inspection.QueryRowContext(t.Context(),
		"SELECT COUNT(*) FROM media_streams WHERE media_file_id = ?", fileID).Scan(&count)
	if err != nil {
		t.Fatalf("count streams of %s: %v", fileID, err)
	}

	return count
}

// storeUnchecked runs an update that a CHECK constraint would refuse, to simulate a row the
// domain cannot read (an edited database or a newer version after a downgrade).
func storeUnchecked(t *testing.T, inspection *sql.DB, query string, args ...any) {
	t.Helper()

	connection, err := inspection.Conn(t.Context())
	if err != nil {
		t.Fatalf("check out inspection connection: %v", err)
	}

	defer func() { _ = connection.Close() }()

	statements := []struct {
		query string
		args  []any
	}{
		{query: "PRAGMA ignore_check_constraints = ON"},
		{query: query, args: args},
		{query: "PRAGMA ignore_check_constraints = OFF"},
	}

	for _, statement := range statements {
		if _, err := connection.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
			t.Fatalf("%s: %v", statement.query, err)
		}
	}
}

// withWriteTimeout returns a context that ends after writeTimeout.
func withWriteTimeout(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), writeTimeout)
	t.Cleanup(cancel)

	return ctx
}

// testID returns a UUID-shaped ID that sorts by kind and number, e.g. testID('f', 3).
func testID(kind byte, number int) string {
	return fmt.Sprintf("%c%07d-0000-4000-8000-000000000000", kind, number)
}

// assertUndecodableRowError checks the error of reading a corrupt row: reported as a fault
// of the stored data, not as invalid client input.
func assertUndecodableRowError(t *testing.T, operation string, err error) {
	t.Helper()

	if err == nil || errors.Is(err, domain.ErrInvalid) || !strings.Contains(err.Error(), "stored row cannot be decoded") {
		t.Errorf("%s error = %v, want an undecodable-row error that does not wrap domain.ErrInvalid", operation, err)
	}
}

// manyItemStreams is the number of streams insertManyItems stores per file: video, audio
// and subtitle, the shape of a typical film.
const manyItemStreams = 3

// insertManyItems stores count items with one probed file and manyItemStreams streams each
// in one transaction, much faster than count saves, and returns the file IDs in sort order.
func insertManyItems(t *testing.T, inspection *sql.DB, libraryID string, count int) []string {
	t.Helper()

	transaction, err := inspection.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}

	defer func() { _ = transaction.Rollback() }()

	fileIDs := make([]string, 0, count)

	for number := range count {
		itemID, fileID, title := testID('i', number), testID('f', number), testID('t', number)
		statements := []struct {
			query string
			args  []any
		}{
			{query: `INSERT INTO items (id, library_id, kind, match_key, title, sort_title, created_at, updated_at)
				VALUES (?, ?, 'movie', ?, ?, ?, 0, 0)`, args: []any{itemID, libraryID, fileID + ".mkv", title, title}},
			{query: insertMediaFileQuery, args: []any{fileID, itemID, libraryID, fileID + ".mkv"}},
			{query: insertMediaStreamQuery, args: []any{fileID, 0, "video"}},
			{query: insertMediaStreamQuery, args: []any{fileID, 1, "audio"}},
			{query: insertMediaStreamQuery, args: []any{fileID, 2, "subtitle"}},
		}

		for _, statement := range statements {
			if _, err := transaction.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
				t.Fatalf("insert item %d: %v", number, err)
			}
		}

		fileIDs = append(fileIDs, fileID)
	}

	if err := transaction.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	return fileIDs
}

func closeTestDB(t *testing.T, db *sqlite.DB) {
	t.Helper()

	if err := db.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
	}
}

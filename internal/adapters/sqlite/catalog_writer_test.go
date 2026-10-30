package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestSaveProbedFileStoresANewFileWithItsItemAndStreams(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "Big.Buck.Bunny.mkv")

	mustSave(t, fixture.writer, record)

	detail := mustGetItem(t, fixture.reader, record.Item.ID)
	assertDeepEqual(t, "GetItem", detail, applibrary.ItemDetail{
		Item:  record.Item,
		Files: []media.File{record.File},
	})

	if got := streamRowCount(t, fixture.inspection, record.File.ID); got != len(record.File.Description.Streams) {
		t.Errorf("stored streams = %d, want %d", got, len(record.File.Description.Streams))
	}

	assertFileTimes(t, fixture.inspection, record.File.ID, scannedMilli)
}

// A changed file keeps its ID and item: the item is neither re-created nor re-titled, while
// size, time, probe result and the full set of streams are replaced.
func TestSaveProbedFileOfAStoredFileKeepsItsItemAndReplacesItsStreams(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	first := probedRecord(showsLibraryID, testID('i', 1), testID('f', 1), "Show/S01E01.mkv")
	mustSave(t, fixture.writer, first)

	changed := first
	changed.ProbedAt = first.ProbedAt.Add(time.Hour)
	changed.Item.Title, changed.Item.SortTitle = "Ignored", "ignored"
	changed.Item.CreatedAt = changed.ProbedAt
	changed.File.Size = 1024
	changed.File.ModifiedAt = time.UnixMilli(fileModifiedMilli + time.Hour.Milliseconds()).UTC()
	changed.File.Description = media.Description{
		Container: media.ContainerMP4, FormatName: "mov,mp4,m4a,3gp,3g2,mj2", Duration: time.Second,
		Streams: []media.Stream{{Index: 3, Kind: media.StreamKindVideo, Codec: "h264", FrameRate: media.NewFrameRate(25, 1)}},
	}

	mustSave(t, fixture.writer, changed)

	detail := mustGetItem(t, fixture.reader, first.Item.ID)
	assertDeepEqual(t, "GetItem after the re-save", detail, applibrary.ItemDetail{
		Item:  first.Item,
		Files: []media.File{changed.File},
	})

	assertFileTimes(t, fixture.inspection, first.File.ID, scannedMilli+time.Hour.Milliseconds())

	if got := catalogRowCounts(t, fixture.inspection); got["items"] != 1 || got["media_files"] != 1 || got["media_streams"] != 1 {
		t.Errorf("row counts = %v, want one item, one file and one stream", got)
	}
}

// A failed probe stores the file with its code, the container "unknown" and no streams,
// also when the file had streams from an earlier, successful probe.
func TestSaveProbedFileStoresAProbeFailureWithoutStreams(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	probed := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "broken.avi")
	mustSave(t, fixture.writer, probed)

	failed := failedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "broken.avi")
	mustSave(t, fixture.writer, failed)

	detail := mustGetItem(t, fixture.reader, failed.Item.ID)
	assertDeepEqual(t, "files", detail.Files, []media.File{failed.File})

	var container, probeError string

	err := fixture.inspection.QueryRowContext(t.Context(),
		"SELECT container, probe_error FROM media_files WHERE id = ?", failed.File.ID).Scan(&container, &probeError)
	if err != nil {
		t.Fatalf("read stored file: %v", err)
	}

	if container != "unknown" || probeError != string(media.ProbeErrorTimedOut) {
		t.Errorf("stored container, probe_error = %q, %q, want unknown, %q", container, probeError, media.ProbeErrorTimedOut)
	}

	if got := streamRowCount(t, fixture.inspection, failed.File.ID); got != 0 {
		t.Errorf("stored streams = %d, want 0", got)
	}
}

// An item may own several files (milestone 3 versions); a new file naming a stored item of
// the same library is added to it.
func TestSaveProbedFileAddsANewFileToAStoredItemOfTheSameLibrary(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	first := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "Movie/b.mkv")
	second := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 2), "Movie/a.mkv")

	mustSave(t, fixture.writer, first)
	mustSave(t, fixture.writer, second)

	detail := mustGetItem(t, fixture.reader, first.Item.ID)
	assertDeepEqual(t, "files, primary (first by path) first", detail.Files, []media.File{second.File, first.File})
}

func TestSaveProbedFileRejectsInvalidRecordsAndChangesNothing(t *testing.T) {
	t.Parallel()

	valid := func() scan.ProbedFile {
		return probedRecord(moviesLibraryID, testID('i', 9), testID('f', 9), "new.mkv")
	}

	tests := []struct {
		name    string
		record  func() scan.ProbedFile
		wantErr error
	}{
		{name: "empty file id", record: changeRecord(valid, func(record *scan.ProbedFile) { record.File.ID = "" })},
		{name: "empty item id", record: changeRecord(valid, func(record *scan.ProbedFile) {
			record.Item.ID, record.File.ItemID = "", ""
		})},
		{name: "file names another item", record: changeRecord(valid, func(record *scan.ProbedFile) { record.File.ItemID = testID('i', 1) })},
		{name: "file and item in different libraries", record: changeRecord(valid, func(record *scan.ProbedFile) {
			record.File.LibraryID = showsLibraryID
		})},
		{name: "absolute path", record: changeRecord(valid, func(record *scan.ProbedFile) { record.File.RelativePath = "/media/new.mkv" })},
		{name: "path with backslash", record: changeRecord(valid, func(record *scan.ProbedFile) { record.File.RelativePath = `dir\new.mkv` })},
		{name: "negative size", record: changeRecord(valid, func(record *scan.ProbedFile) { record.File.Size = -1 })},
		{name: "unknown container", record: changeRecord(valid, func(record *scan.ProbedFile) { record.File.Description.Container = "zip" })},
		{name: "probe error with streams", record: changeRecord(valid, func(record *scan.ProbedFile) {
			record.File.ProbeError = media.ProbeErrorFailed
		})},
		{name: "blank title of a new item", record: changeRecord(valid, func(record *scan.ProbedFile) { record.Item.Title = " " })},
		{name: "episode in a movies library", record: changeRecord(valid, func(record *scan.ProbedFile) {
			record.Item.Kind = library.ItemKindEpisode
		})},
		{name: "unknown item kind", record: changeRecord(valid, func(record *scan.ProbedFile) { record.Item.Kind = "show" })},
		{name: "zero probe time", record: changeRecord(valid, func(record *scan.ProbedFile) { record.ProbedAt = time.Time{} })},
		{name: "zero creation time of a new item", record: changeRecord(valid, func(record *scan.ProbedFile) {
			record.Item.CreatedAt = time.Time{}
		})},
		{name: "stored file moved to another item", record: func() scan.ProbedFile {
			return probedRecord(moviesLibraryID, testID('i', 9), testID('f', 1), "a.mkv")
		}},
		{name: "stored file moved to another library", record: func() scan.ProbedFile {
			return probedRecord(showsLibraryID, testID('i', 1), testID('f', 1), "a.mkv")
		}},
		{name: "new file of a stored item in another library", record: func() scan.ProbedFile {
			return probedRecord(showsLibraryID, testID('i', 1), testID('f', 9), "a.mkv")
		}},
		{name: "unknown library", wantErr: domain.ErrNotFound, record: func() scan.ProbedFile {
			return probedRecord(unknownID, testID('i', 9), testID('f', 9), "a.mkv")
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCatalogFixture(t)
			stored := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv")
			mustSave(t, fixture.writer, stored)

			before := catalogRowCounts(t, fixture.inspection)

			wantErr := test.wantErr
			if wantErr == nil {
				wantErr = domain.ErrInvalid
			}

			if err := fixture.writer.SaveProbedFile(t.Context(), test.record()); !errors.Is(err, wantErr) {
				t.Fatalf("SaveProbedFile error = %v, want %v", err, wantErr)
			}

			assertDeepEqual(t, "row counts", catalogRowCounts(t, fixture.inspection), before)
			assertDeepEqual(t, "stored item", mustGetItem(t, fixture.reader, stored.Item.ID), applibrary.ItemDetail{
				Item: stored.Item, Files: []media.File{stored.File},
			})
		})
	}
}

func changeRecord(base func() scan.ProbedFile, change func(record *scan.ProbedFile)) func() scan.ProbedFile {
	return func() scan.ProbedFile {
		record := base()
		change(&record)

		return record
	}
}

// Each case makes one statement of a save fail through a trigger. Whichever step fails, the
// steps before it are rolled back, and the next write succeeds: a failed write must return
// the single writer connection.
func TestSaveProbedFileRollsBackAFailedStatementAndTheNextWriteSucceeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		trigger string
		stored  bool
	}{
		{name: "item insert", trigger: "BEFORE INSERT ON items"},
		{name: "file insert", trigger: "BEFORE INSERT ON media_files"},
		{name: "stream insert of a new file", trigger: "BEFORE INSERT ON media_streams"},
		{name: "file update", trigger: "BEFORE UPDATE ON media_files", stored: true},
		{name: "stream delete", trigger: "BEFORE DELETE ON media_streams", stored: true},
		{name: "stream insert of a stored file", trigger: "BEFORE INSERT ON media_streams", stored: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCatalogFixture(t)
			record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv")

			if test.stored {
				mustSave(t, fixture.writer, record)
				record.File.Size++
			}

			before := catalogRowCounts(t, fixture.inspection)

			failStatement(t, fixture.inspection, test.trigger)

			if err := fixture.writer.SaveProbedFile(withWriteTimeout(t), record); err == nil {
				t.Fatal("SaveProbedFile error = nil, want the trigger's error")
			}

			assertDeepEqual(t, "row counts after the failed save", catalogRowCounts(t, fixture.inspection), before)

			execute(t, fixture.inspection, "DROP TRIGGER fail_statement")

			if err := fixture.writer.SaveProbedFile(withWriteTimeout(t), record); err != nil {
				t.Fatalf("SaveProbedFile after a failed save error = %v", err)
			}

			assertDeepEqual(t, "files", mustGetItem(t, fixture.reader, record.Item.ID).Files, []media.File{record.File})
		})
	}
}

func TestFileStatesReturnsTheFilesOfOneLibraryOrderedByPath(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	second := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "b/z.mkv")
	first := failedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "a.mkv")
	other := probedRecord(showsLibraryID, testID('i', 3), testID('f', 3), "a.mkv")
	// Sub-millisecond parts are not stored (ADR 0004); library.Changed ignores them too.
	second.File.ModifiedAt = second.File.ModifiedAt.Add(999 * time.Microsecond)

	for _, record := range []scan.ProbedFile{second, first, other} {
		mustSave(t, fixture.writer, record)
	}

	states, err := fixture.writer.FileStates(t.Context(), moviesLibraryID)
	if err != nil {
		t.Fatalf("FileStates error = %v", err)
	}

	assertDeepEqual(t, "FileStates", states, []scan.CatalogFile{
		{ID: first.File.ID, ItemID: first.Item.ID, State: library.FileState{
			RelativePath: "a.mkv", Size: first.File.Size, ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
		}},
		{ID: second.File.ID, ItemID: second.Item.ID, State: library.FileState{
			RelativePath: "b/z.mkv", Size: second.File.Size, ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
		}},
	})
}

func TestFileStatesOfALibraryWithoutFilesIsAnEmptyList(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)

	for _, libraryID := range []string{moviesLibraryID, unknownID} {
		states, err := fixture.writer.FileStates(t.Context(), libraryID)
		if err != nil || states == nil || len(states) != 0 {
			t.Errorf("FileStates(%s) = %#v, %v, want an empty, non-nil list", libraryID, states, err)
		}
	}
}

// Removing a file deletes its streams by cascade and its item once the item has no file
// left. IDs of another library or unknown IDs are ignored.
func TestRemoveFilesDeletesFilesAndItemsLeftWithoutFiles(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	single := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "single.mkv")
	versionA := probedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "Movie/a.mkv")
	versionB := probedRecord(moviesLibraryID, testID('i', 2), testID('f', 3), "Movie/b.mkv")
	other := probedRecord(showsLibraryID, testID('i', 4), testID('f', 4), "single.mkv")

	for _, record := range []scan.ProbedFile{single, versionA, versionB, other} {
		mustSave(t, fixture.writer, record)
	}

	err := fixture.writer.RemoveFiles(t.Context(), moviesLibraryID,
		[]string{single.File.ID, versionA.File.ID, other.File.ID, unknownID})
	if err != nil {
		t.Fatalf("RemoveFiles error = %v", err)
	}

	if _, err := fixture.reader.GetItem(t.Context(), single.Item.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("GetItem of the item without files error = %v, want domain.ErrNotFound", err)
	}

	assertDeepEqual(t, "files of the item with a file left", mustGetItem(t, fixture.reader, versionB.Item.ID).Files,
		[]media.File{versionB.File})
	assertDeepEqual(t, "files of the other library's item", mustGetItem(t, fixture.reader, other.Item.ID).Files,
		[]media.File{other.File})

	wantRows := map[string]int{"libraries": 2, "items": 2, "media_files": 2, "media_streams": 4}
	assertDeepEqual(t, "row counts", catalogRowCounts(t, fixture.inspection), wantRows)
}

func TestRemoveFilesWithoutIDsChangesNothing(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv"))

	before := catalogRowCounts(t, fixture.inspection)

	if err := fixture.writer.RemoveFiles(t.Context(), moviesLibraryID, nil); err != nil {
		t.Fatalf("RemoveFiles(nil) error = %v", err)
	}

	assertDeepEqual(t, "row counts", catalogRowCounts(t, fixture.inspection), before)
}

// A vanished folder can hold more files than SQLite binds in one statement; the removal is
// split into batches of 500 IDs inside the one transaction. 1 234 IDs make three batches,
// the last one partial.
func TestRemoveFilesCrossesTheBatchBoundaryOf500IDs(t *testing.T) {
	t.Parallel()

	const fileCount = 1_234

	fixture := newCatalogFixture(t)
	fileIDs := insertManyItems(t, fixture.inspection, moviesLibraryID, fileCount)

	if got := rowCount(t, fixture.inspection, "media_streams"); got != fileCount*manyItemStreams {
		t.Fatalf("streams before the removal = %d, want %d", got, fileCount*manyItemStreams)
	}

	if err := fixture.writer.RemoveFiles(t.Context(), moviesLibraryID, fileIDs); err != nil {
		t.Fatalf("RemoveFiles error = %v", err)
	}

	wantRows := map[string]int{"libraries": 2, "items": 0, "media_files": 0, "media_streams": 0}
	assertDeepEqual(t, "row counts", catalogRowCounts(t, fixture.inspection), wantRows)
}

// The removal is one transaction: when deleting the items fails, the files stay as well,
// and the next write succeeds.
func TestRemoveFilesRollsBackAFailedStatementAndTheNextWriteSucceeds(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv")
	mustSave(t, fixture.writer, record)

	before := catalogRowCounts(t, fixture.inspection)

	failStatement(t, fixture.inspection, "BEFORE DELETE ON items")

	if err := fixture.writer.RemoveFiles(withWriteTimeout(t), moviesLibraryID, []string{record.File.ID}); err == nil {
		t.Fatal("RemoveFiles error = nil, want the trigger's error")
	}

	assertDeepEqual(t, "row counts after the failed removal", catalogRowCounts(t, fixture.inspection), before)

	execute(t, fixture.inspection, "DROP TRIGGER fail_statement")

	if err := fixture.writer.RemoveFiles(withWriteTimeout(t), moviesLibraryID, []string{record.File.ID}); err != nil {
		t.Fatalf("RemoveFiles after a failed removal error = %v", err)
	}

	if got := rowCount(t, fixture.inspection, "items"); got != 0 {
		t.Errorf("items after the removal = %d, want 0", got)
	}
}

func TestCatalogWriterReportsDatabaseErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ctx     func(t *testing.T) context.Context
		closeDB bool
		wantErr error
	}{
		{name: "cancelled context", ctx: cancelledContext, wantErr: context.Canceled},
		{name: "closed database", ctx: func(t *testing.T) context.Context { return t.Context() }, closeDB: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCatalogFixture(t)
			record := probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv")

			if test.closeDB {
				closeTestDB(t, fixture.db)
			}

			_, statesErr := fixture.writer.FileStates(test.ctx(t), moviesLibraryID)
			saveErr := fixture.writer.SaveProbedFile(test.ctx(t), record)
			removeErr := fixture.writer.RemoveFiles(test.ctx(t), moviesLibraryID, []string{record.File.ID})

			errs := map[string]error{"FileStates": statesErr, "SaveProbedFile": saveErr, "RemoveFiles": removeErr}
			for operation, err := range errs {
				assertDatabaseError(t, operation, err, test.wantErr)
			}
		})
	}
}

// assertFileTimes checks probed_at of a file and that its item's updated_at stays at
// created_at: re-probing a file does not touch its item.
func assertFileTimes(t *testing.T, inspection *sql.DB, fileID string, wantProbedAt int64) {
	t.Helper()

	var probedAt, itemCreated, itemUpdated int64

	err := inspection.QueryRowContext(t.Context(), `
		SELECT media_files.probed_at, items.created_at, items.updated_at
		FROM media_files JOIN items ON items.id = media_files.item_id
		WHERE media_files.id = ?`, fileID).Scan(&probedAt, &itemCreated, &itemUpdated)
	if err != nil {
		t.Fatalf("read times of file %s: %v", fileID, err)
	}

	if probedAt != wantProbedAt || itemUpdated != itemCreated {
		t.Errorf("probed_at = %d, item created_at, updated_at = %d, %d, want probed_at %d and an untouched item",
			probedAt, itemCreated, itemUpdated, wantProbedAt)
	}
}

func closeTestDB(t *testing.T, db *sqlite.DB) {
	t.Helper()

	if err := db.Close(); err != nil {
		t.Fatalf("Close error = %v", err)
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
			{query: `INSERT INTO items (id, library_id, kind, title, sort_title, created_at, updated_at)
				VALUES (?, ?, 'movie', ?, ?, 0, 0)`, args: []any{itemID, libraryID, title, title}},
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

// A stored library kind the domain does not know is a fault of the stored data: creating an
// item in that library fails without blaming the record as invalid input.
func TestSaveProbedFileInALibraryWithACorruptKindIsAServerError(t *testing.T) {
	t.Parallel()

	fixture := newCatalogFixture(t)
	storeUnchecked(t, fixture.inspection, "UPDATE libraries SET kind = 'music' WHERE id = ?", moviesLibraryID)

	err := fixture.writer.SaveProbedFile(t.Context(), probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv"))
	assertUndecodableRowError(t, "SaveProbedFile", err)

	if got := rowCount(t, fixture.inspection, "items"); got != 0 {
		t.Errorf("items = %d, want 0", got)
	}
}

// A path is stored once per library (UNIQUE(library_id, relative_path)). Another file ID at
// a stored path, as a new file or by moving a stored one, is a conflict: the item created
// for it is rolled back, and the next write succeeds.
func TestSaveProbedFileOfAnotherFileAtAStoredPathIsAConflict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		record scan.ProbedFile
	}{
		{name: "new file", record: probedRecord(moviesLibraryID, testID('i', 3), testID('f', 3), "a.mkv")},
		{name: "stored file moved onto the path", record: probedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "a.mkv")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newCatalogFixture(t)
			mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 1), testID('f', 1), "a.mkv"))
			mustSave(t, fixture.writer, probedRecord(moviesLibraryID, testID('i', 2), testID('f', 2), "b.mkv"))

			before := catalogRowCounts(t, fixture.inspection)

			err := fixture.writer.SaveProbedFile(withWriteTimeout(t), test.record)
			if !errors.Is(err, domain.ErrConflict) {
				t.Fatalf("SaveProbedFile error = %v, want domain.ErrConflict", err)
			}

			assertDeepEqual(t, "row counts after the conflict", catalogRowCounts(t, fixture.inspection), before)

			test.record.File.RelativePath = "c.mkv"
			if err := fixture.writer.SaveProbedFile(withWriteTimeout(t), test.record); err != nil {
				t.Fatalf("SaveProbedFile at a free path after the conflict error = %v", err)
			}
		})
	}
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// LibrarySummary is a library with the number of its items, as the library list and the
// library detail show it.
type LibrarySummary struct {
	Library   library.Library
	ItemCount int64
}

// LibraryRepository stores the configured libraries (ADR 0022). Reads go to the reader pool;
// a sync runs in one transaction on the writer pool.
type LibraryRepository struct {
	writer *sql.DB
	reader *sqlcgen.Queries
	clock  func() time.Time
}

// NewLibraryRepository returns a repository on db. db must be migrated before the first call.
// clock stamps created_at and updated_at (Unix milliseconds, ADR 0004); nil means time.Now.
func NewLibraryRepository(db *DB, clock func() time.Time) *LibraryRepository {
	if clock == nil {
		clock = time.Now
	}

	return &LibraryRepository{writer: db.writer, reader: sqlcgen.New(db.reader), clock: clock}
}

// SyncLibraries makes the stored libraries match definitions in one write transaction. The
// root path is a library's identity: a new root is inserted with an ID from newID, a known
// root gets the configured name and kind, and a stored root that is no longer configured is
// deleted with its items, files and streams. A library whose definition did not change is
// not written, so its updated_at stays. It returns all libraries ordered by name.
//
// Roots are compared exactly as given; the configuration has already cleaned them and
// rejected duplicates that differ only in case on Windows. Invalid definitions, two
// definitions with the same root and a nil newID wrap domain.ErrInvalid; nothing is changed
// then. When newID fails, the whole sync is rolled back.
func (repository *LibraryRepository) SyncLibraries(ctx context.Context, definitions []library.Definition,
	newID func() (string, error),
) ([]library.Library, error) {
	if err := checkSyncInput(definitions, newID); err != nil {
		return nil, fmt.Errorf("sync libraries: %w", err)
	}

	var synced []library.Library

	err := inWriteTransaction(ctx, repository.writer, func(queries *sqlcgen.Queries) error {
		run := librarySync{queries: queries, newID: newID, now: repository.clock().UnixMilli()}

		var err error

		synced, err = run.apply(ctx, definitions)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("sync libraries: %w", err)
	}

	return synced, nil
}

// ListLibraries returns every library with its item count, ordered by name.
func (repository *LibraryRepository) ListLibraries(ctx context.Context) ([]LibrarySummary, error) {
	summaries, err := listSummaries(ctx, repository.reader)
	if err != nil {
		return nil, fmt.Errorf("list libraries: %w", err)
	}

	return summaries, nil
}

// GetLibrary returns the library with the given ID and its item count. An unknown ID wraps
// domain.ErrNotFound.
func (repository *LibraryRepository) GetLibrary(ctx context.Context, id string) (LibrarySummary, error) {
	row, err := repository.reader.GetLibrary(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return LibrarySummary{}, fmt.Errorf("get library %q: %w", id, domain.ErrNotFound)
	}

	if err != nil {
		return LibrarySummary{}, fmt.Errorf("get library %q: %w", id, err)
	}

	summary, err := summaryFromRow(sqlcgen.ListLibrariesRow(row))
	if err != nil {
		return LibrarySummary{}, fmt.Errorf("get library %q: %w", id, err)
	}

	return summary, nil
}

// checkSyncInput rejects what would otherwise surface as a constraint error halfway through
// the transaction.
func checkSyncInput(definitions []library.Definition, newID func() (string, error)) error {
	if newID == nil {
		return fmt.Errorf("no id generator: %w", domain.ErrInvalid)
	}

	roots := make(map[string]bool, len(definitions))

	for index, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return fmt.Errorf("definition %d: %w", index, err)
		}

		if roots[definition.Root] {
			return fmt.Errorf("definition %d: root %q is configured twice: %w", index, definition.Root, domain.ErrInvalid)
		}

		roots[definition.Root] = true
	}

	return nil
}

// librarySync is one SyncLibraries run inside its write transaction.
type librarySync struct {
	queries *sqlcgen.Queries
	newID   func() (string, error)
	// now is the sync's timestamp in Unix milliseconds, shared by every row it writes.
	now int64
}

func (run librarySync) apply(ctx context.Context, definitions []library.Definition) ([]library.Library, error) {
	stored, err := listSummaries(ctx, run.queries)
	if err != nil {
		return nil, err
	}

	storedByRoot := make(map[string]library.Library, len(stored))
	for _, summary := range stored {
		storedByRoot[summary.Library.Root] = summary.Library
	}

	configured := make(map[string]bool, len(definitions))

	for _, definition := range definitions {
		configured[definition.Root] = true

		if err := run.save(ctx, definition, storedByRoot); err != nil {
			return nil, err
		}
	}

	for _, summary := range stored {
		if configured[summary.Library.Root] {
			continue
		}

		if err := run.queries.DeleteLibrary(ctx, summary.Library.ID); err != nil {
			return nil, fmt.Errorf("delete library %q: %w", summary.Library.Name, err)
		}
	}

	return run.libraries(ctx)
}

// save inserts a new root, updates a known root whose name or kind changed and leaves an
// unchanged one alone.
func (run librarySync) save(ctx context.Context, definition library.Definition,
	storedByRoot map[string]library.Library,
) error {
	stored, known := storedByRoot[definition.Root]

	switch {
	case !known:
		return run.insert(ctx, definition)
	case stored.Name == definition.Name && stored.Kind == definition.Kind:
		return nil
	default:
		return run.update(ctx, stored, definition)
	}
}

func (run librarySync) insert(ctx context.Context, definition library.Definition) error {
	id, err := run.newID()
	if err != nil {
		return fmt.Errorf("new id for library %q: %w", definition.Name, err)
	}

	err = run.queries.InsertLibrary(ctx, sqlcgen.InsertLibraryParams{
		ID:        id,
		Name:      definition.Name,
		Kind:      definition.Kind.String(),
		RootPath:  definition.Root,
		CreatedAt: run.now,
		UpdatedAt: run.now,
	})
	if err != nil {
		return fmt.Errorf("insert library %q: %w", definition.Name, err)
	}

	return nil
}

func (run librarySync) update(ctx context.Context, stored library.Library, definition library.Definition) error {
	err := run.queries.UpdateLibrary(ctx, sqlcgen.UpdateLibraryParams{
		Name:      definition.Name,
		Kind:      definition.Kind.String(),
		UpdatedAt: run.now,
		ID:        stored.ID,
	})
	if err != nil {
		return fmt.Errorf("update library %q: %w", definition.Name, err)
	}

	if stored.Kind == definition.Kind {
		return nil
	}

	itemKind, err := library.ItemKindForLibrary(definition.Kind)
	if err != nil {
		return fmt.Errorf("update items of library %q: %w", definition.Name, err)
	}

	err = run.queries.UpdateLibraryItemKinds(ctx, sqlcgen.UpdateLibraryItemKindsParams{
		Kind:      itemKind.String(),
		UpdatedAt: run.now,
		LibraryID: stored.ID,
	})
	if err != nil {
		return fmt.Errorf("update items of library %q: %w", definition.Name, err)
	}

	return nil
}

// libraries reads the synced state back inside the transaction.
func (run librarySync) libraries(ctx context.Context) ([]library.Library, error) {
	summaries, err := listSummaries(ctx, run.queries)
	if err != nil {
		return nil, err
	}

	libraries := make([]library.Library, 0, len(summaries))
	for _, summary := range summaries {
		libraries = append(libraries, summary.Library)
	}

	return libraries, nil
}

func listSummaries(ctx context.Context, queries *sqlcgen.Queries) ([]LibrarySummary, error) {
	rows, err := queries.ListLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("read libraries: %w", err)
	}

	summaries := make([]LibrarySummary, 0, len(rows))

	for _, row := range rows {
		summary, err := summaryFromRow(row)
		if err != nil {
			return nil, err
		}

		summaries = append(summaries, summary)
	}

	return summaries, nil
}

// summaryFromRow rejects a stored kind the domain does not know (an edited database or a
// downgrade) with an error wrapping domain.ErrInvalid instead of handing it to clients.
func summaryFromRow(row sqlcgen.ListLibrariesRow) (LibrarySummary, error) {
	kind, err := library.ParseKind(row.Kind)
	if err != nil {
		return LibrarySummary{}, fmt.Errorf("library %q: stored kind: %w", row.ID, err)
	}

	return LibrarySummary{
		Library:   library.Library{ID: row.ID, Name: row.Name, Root: row.RootPath, Kind: kind},
		ItemCount: row.ItemCount,
	}, nil
}

// inWriteTransaction runs work in one transaction on the writer pool, which starts it with
// BEGIN IMMEDIATE (see dataSourceName). It commits when work succeeds and rolls back
// otherwise.
func inWriteTransaction(ctx context.Context, writer *sql.DB, work func(*sqlcgen.Queries) error) error {
	transaction, err := writer.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write transaction: %w", err)
	}

	if err := work(sqlcgen.New(transaction)); err != nil {
		return errors.Join(err, rollBack(transaction))
	}

	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit write transaction: %w", err)
	}

	return nil
}

// rollBack ignores sql.ErrTxDone: database/sql has already rolled back a transaction whose
// context was cancelled.
func rollBack(transaction *sql.Tx) error {
	err := transaction.Rollback()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return fmt.Errorf("roll back write transaction: %w", err)
	}

	return nil
}

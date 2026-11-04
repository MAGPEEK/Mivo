package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// positionShift moves the folders of a library out of the 0..MaxFolders-1 range while an
// update renumbers them, so the unique (library_id, position) never collides midway.
const positionShift = 1000

// ManagedLibraryStore stores the managed libraries, their folders and the import ledger
// (ADR 0028). It implements app/library.ManagedWriter, ManagedReader and ImportLedger. Reads
// go to the reader pool; every change runs in one transaction on the writer pool.
type ManagedLibraryStore struct {
	writer *sql.DB
	reader *sql.DB
	clock  func() time.Time
	newID  func() (string, error)
	// caseInsensitive selects the Windows folder comparison; fixed per build target.
	caseInsensitive bool
}

// NewManagedLibraryStore returns a store on db, which must be migrated. newID creates the IDs
// of new libraries and folders; clock stamps created_at and updated_at (nil means time.Now).
func NewManagedLibraryStore(db *DB, clock func() time.Time, newID func() (string, error)) *ManagedLibraryStore {
	if clock == nil {
		clock = time.Now
	}

	return &ManagedLibraryStore{
		writer: db.writer, reader: db.reader, clock: clock, newID: newID,
		caseInsensitive: foldPathCase(),
	}
}

// CreateLibrary inserts the library and its folders in one transaction and returns it with
// the generated IDs. A folder another library holds wraps domain.ErrConflict and names the
// folder index.
func (store *ManagedLibraryStore) CreateLibrary(ctx context.Context,
	candidate library.ManagedLibrary,
) (library.ManagedLibrary, error) {
	created := candidate

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		var err error

		created, err = store.insertLibrary(ctx, queries, candidate)

		return err
	})
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("create library %q: %w", candidate.Name, err)
	}

	return created, nil
}

func (store *ManagedLibraryStore) insertLibrary(ctx context.Context, queries *sqlcgen.Queries,
	candidate library.ManagedLibrary,
) (library.ManagedLibrary, error) {
	id, err := store.newID()
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("new library id: %w", err)
	}

	now := store.clock().UnixMilli()

	err = queries.InsertManagedLibrary(ctx, sqlcgen.InsertManagedLibraryParams{
		ID: id, Name: candidate.Name, Kind: candidate.Kind.String(), MetadataLanguage: candidate.MetadataLanguage,
		ScanExtras: boolToInt(candidate.Options.ScanExtras), ReadNfo: boolToInt(candidate.Options.ReadNFO),
		LocalArtwork:       boolToInt(candidate.Options.LocalArtwork),
		ImportedFromConfig: boolToInt(candidate.ImportedFromConfig), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("insert library: %w", err)
	}

	created := candidate
	created.ID = id
	created.Folders = make([]library.Folder, 0, len(candidate.Folders))

	for position, folder := range candidate.Folders {
		stored, err := store.insertFolder(ctx, queries, folderPlacement{libraryID: id, folder: folder, position: position, now: now})
		if err != nil {
			return library.ManagedLibrary{}, err
		}

		created.Folders = append(created.Folders, stored)
	}

	return created, nil
}

// folderPlacement is where and when insertFolder stores a folder.
type folderPlacement struct {
	libraryID string
	folder    library.Folder
	position  int
	now       int64
}

// insertFolder stores one folder; every folder gets a fresh ID from the generator.
func (store *ManagedLibraryStore) insertFolder(ctx context.Context, queries *sqlcgen.Queries,
	place folderPlacement,
) (library.Folder, error) {
	id, err := store.newID()
	if err != nil {
		return library.Folder{}, fmt.Errorf("new id for folder %d: %w", place.position, err)
	}

	err = queries.InsertManagedFolder(ctx, sqlcgen.InsertManagedFolderParams{
		ID: id, LibraryID: place.libraryID, Path: place.folder.Path,
		PathKey:  folderKey(place.folder.Path, store.caseInsensitive),
		Position: int64(place.position), CreatedAt: place.now,
	})
	if isUniqueViolation(err) {
		return library.Folder{}, fmt.Errorf("folder %d is already used: %w", place.position, domain.ErrConflict)
	}

	if err != nil {
		return library.Folder{}, fmt.Errorf("insert folder %d: %w", place.position, err)
	}

	return library.Folder{ID: id, Path: place.folder.Path}, nil
}

// UpdateLibrary replaces the name, language, options and folders in one transaction. Kept
// folders (known ID) keep their ID, path and catalog; stored folders missing from the list
// are deleted with their catalog rows; folders without an ID are inserted. An unknown library
// or a folder ID of another library wraps domain.ErrNotFound.
func (store *ManagedLibraryStore) UpdateLibrary(ctx context.Context, updated library.ManagedLibrary) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		return store.applyUpdate(ctx, queries, updated)
	})
	if err != nil {
		return fmt.Errorf("update library %q: %w", updated.ID, err)
	}

	return nil
}

func (store *ManagedLibraryStore) applyUpdate(ctx context.Context, queries *sqlcgen.Queries,
	updated library.ManagedLibrary,
) error {
	now := store.clock().UnixMilli()

	changed, err := queries.UpdateManagedLibrary(ctx, sqlcgen.UpdateManagedLibraryParams{
		Name: updated.Name, MetadataLanguage: updated.MetadataLanguage,
		ScanExtras: boolToInt(updated.Options.ScanExtras), ReadNfo: boolToInt(updated.Options.ReadNFO),
		LocalArtwork: boolToInt(updated.Options.LocalArtwork), UpdatedAt: now, ID: updated.ID,
	})
	if err != nil {
		return fmt.Errorf("write library: %w", err)
	}

	if changed == 0 {
		return fmt.Errorf("library not stored: %w", domain.ErrNotFound)
	}

	stored, err := queries.ListManagedFoldersOfLibrary(ctx, updated.ID)
	if err != nil {
		return fmt.Errorf("read folders: %w", err)
	}

	if err := store.removeMissingFolders(ctx, queries, updated, stored); err != nil {
		return err
	}

	return store.placeFolders(ctx, queries, updated, now)
}

// removeMissingFolders deletes the stored folders the update no longer lists and rejects a
// listed ID that is not one of the library's folders.
func (store *ManagedLibraryStore) removeMissingFolders(ctx context.Context, queries *sqlcgen.Queries,
	updated library.ManagedLibrary, stored []sqlcgen.ListManagedFoldersOfLibraryRow,
) error {
	storedIDs := make(map[string]bool, len(stored))
	for _, folder := range stored {
		storedIDs[folder.ID] = true
	}

	kept := make(map[string]bool, len(updated.Folders))

	for index, folder := range updated.Folders {
		if folder.ID == "" {
			continue
		}

		if !storedIDs[folder.ID] {
			return fmt.Errorf("folder %d is not a folder of the library: %w", index, domain.ErrNotFound)
		}

		kept[folder.ID] = true
	}

	var emptied []string

	for _, folder := range stored {
		if kept[folder.ID] {
			continue
		}

		itemIDs, err := queries.ListFolderItemIDs(ctx, folder.ID)
		if err != nil {
			return fmt.Errorf("read items of folder %q: %w", folder.ID, err)
		}

		err = queries.DeleteManagedFolder(ctx, sqlcgen.DeleteManagedFolderParams{ID: folder.ID, LibraryID: updated.ID})
		if err != nil {
			return fmt.Errorf("delete folder %q: %w", folder.ID, err)
		}

		emptied = append(emptied, itemIDs...)
	}

	// The cascade removed the files; the items they belonged to would stay as empty entries
	// in the library until someone deletes them.
	return pruneEmptiedItems(ctx, queries, emptied)
}

// pruneEmptiedItems deletes the listed items that own no file and no child, then checks their
// parents the same way, so a show disappears with its last episode. Unlike the scan's prune it
// keeps a movie that still has an extra with a file in a kept folder.
func pruneEmptiedItems(ctx context.Context, queries *sqlcgen.Queries, itemIDs []string) error {
	slices.Sort(itemIDs)

	for _, start := range slices.Compact(itemIDs) {
		for itemID := start; itemID != ""; {
			parent, err := queries.DeleteFolderEmptiedItem(ctx, itemID)
			if errors.Is(err, sql.ErrNoRows) {
				break
			}

			if err != nil {
				return fmt.Errorf("delete emptied item %q: %w", itemID, err)
			}

			itemID = parent.String
		}
	}

	return nil
}

// placeFolders renumbers the kept folders and inserts the new ones in request order.
func (store *ManagedLibraryStore) placeFolders(ctx context.Context, queries *sqlcgen.Queries,
	updated library.ManagedLibrary, now int64,
) error {
	err := queries.ShiftManagedFolderPositions(ctx, sqlcgen.ShiftManagedFolderPositionsParams{
		Shift: positionShift, LibraryID: updated.ID,
	})
	if err != nil {
		return fmt.Errorf("renumber folders: %w", err)
	}

	for position, folder := range updated.Folders {
		if folder.ID == "" {
			_, err = store.insertFolder(ctx, queries, folderPlacement{libraryID: updated.ID, folder: folder, position: position, now: now})
		} else {
			err = queries.SetManagedFolderPosition(ctx, sqlcgen.SetManagedFolderPositionParams{
				Position: int64(position), ID: folder.ID, LibraryID: updated.ID,
			})
		}

		if err != nil {
			return fmt.Errorf("place folder %d: %w", position, err)
		}
	}

	return nil
}

// DeleteLibrary deletes the library; its folders and catalog follow by cascade. An unknown ID
// wraps domain.ErrNotFound.
func (store *ManagedLibraryStore) DeleteLibrary(ctx context.Context, id string) error {
	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		deleted, err := queries.DeleteManagedLibrary(ctx, id)
		if err != nil {
			return err
		}

		if deleted == 0 {
			return domain.ErrNotFound
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("delete library %q: %w", id, err)
	}

	return nil
}

// ListManaged returns every library with its folders, ordered by name (then ID).
func (store *ManagedLibraryStore) ListManaged(ctx context.Context) ([]library.ManagedLibrary, error) {
	var libraries []library.ManagedLibrary

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		rows, err := queries.ListManagedLibraries(ctx)
		if err != nil {
			return fmt.Errorf("read libraries: %w", err)
		}

		folders, err := queries.ListManagedFolders(ctx)
		if err != nil {
			return fmt.Errorf("read folders: %w", err)
		}

		libraries, err = assembleManaged(rows, folders)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list managed libraries: %w", err)
	}

	return libraries, nil
}

// GetManaged returns one library with its folders. An unknown ID wraps domain.ErrNotFound.
func (store *ManagedLibraryStore) GetManaged(ctx context.Context, id string) (library.ManagedLibrary, error) {
	var found library.ManagedLibrary

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		row, err := queries.GetManagedLibrary(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read library: %w", err)
		}

		folders, err := queries.ListManagedFoldersOfLibrary(ctx, id)
		if err != nil {
			return fmt.Errorf("read folders: %w", err)
		}

		found, err = managedFromRow(sqlcgen.ListManagedLibrariesRow(row), foldersOf(folders))

		return err
	})
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("get managed library %q: %w", id, err)
	}

	return found, nil
}

func assembleManaged(rows []sqlcgen.ListManagedLibrariesRow,
	folders []sqlcgen.ListManagedFoldersRow,
) ([]library.ManagedLibrary, error) {
	byLibrary := make(map[string][]library.Folder, len(rows))
	for _, folder := range folders {
		byLibrary[folder.LibraryID] = append(byLibrary[folder.LibraryID], library.Folder{ID: folder.ID, Path: folder.Path})
	}

	libraries := make([]library.ManagedLibrary, 0, len(rows))

	for _, row := range rows {
		managed, err := managedFromRow(row, byLibrary[row.ID])
		if err != nil {
			return nil, err
		}

		libraries = append(libraries, managed)
	}

	return libraries, nil
}

func foldersOf(rows []sqlcgen.ListManagedFoldersOfLibraryRow) []library.Folder {
	folders := make([]library.Folder, 0, len(rows))
	for _, row := range rows {
		folders = append(folders, library.Folder{ID: row.ID, Path: row.Path})
	}

	return folders
}

// managedFromRow reports a stored kind the domain does not know as a plain error, not
// domain.ErrInvalid (ADR 0022 Notes): the caller's request is not at fault.
func managedFromRow(row sqlcgen.ListManagedLibrariesRow, folders []library.Folder) (library.ManagedLibrary, error) {
	kind, err := library.ParseKind(row.Kind)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("library %q: stored kind %q is unknown", row.ID, row.Kind)
	}

	if folders == nil {
		folders = []library.Folder{}
	}

	return library.ManagedLibrary{
		ID: row.ID, Name: row.Name, Kind: kind, Folders: folders, MetadataLanguage: row.MetadataLanguage,
		Options: library.Options{
			ScanExtras: row.ScanExtras != 0, ReadNFO: row.ReadNfo != 0, LocalArtwork: row.LocalArtwork != 0,
		},
		ImportedFromConfig: row.ImportedFromConfig != 0,
	}, nil
}

// NeverImported returns those of roots without a ledger entry, in the given order. Roots are
// compared by folder key.
func (store *ManagedLibraryStore) NeverImported(ctx context.Context, roots []string) ([]string, error) {
	entries, err := sqlcgen.New(store.reader).ListLibraryImports(ctx)
	if err != nil {
		return nil, fmt.Errorf("read import ledger: %w", err)
	}

	recorded := make(map[string]bool, len(entries))
	for _, entry := range entries {
		recorded[entry.PathKey] = true
	}

	pending := []string{}

	for _, root := range roots {
		if !recorded[folderKey(root, store.caseInsensitive)] {
			pending = append(pending, root)
		}
	}

	return pending, nil
}

// RecordImported records roots in the ledger in one transaction; known ones stay as they are.
func (store *ManagedLibraryStore) RecordImported(ctx context.Context, roots []string) error {
	now := store.clock().UnixMilli()

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		for _, root := range roots {
			err := queries.RecordLibraryImport(ctx, sqlcgen.RecordLibraryImportParams{
				PathKey: folderKey(root, store.caseInsensitive), Path: root, ImportedAt: now,
			})
			if err != nil {
				return fmt.Errorf("record %q: %w", root, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("record imported roots: %w", err)
	}

	return nil
}

// TopLevelItemCounts returns, per library ID, the number of top-level items (movies and
// shows, the items listLibraryItems returns). A library without items has no entry.
func (store *ManagedLibraryStore) TopLevelItemCounts(ctx context.Context) (map[string]int64, error) {
	counts := map[string]int64{}

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		rows, err := queries.CountTopLevelItems(ctx)
		if err != nil {
			return fmt.Errorf("count items: %w", err)
		}

		for _, row := range rows {
			counts[row.LibraryID] = row.ItemCount
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("count top-level items: %w", err)
	}

	return counts, nil
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}

	return 0
}

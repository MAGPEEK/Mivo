package sqlite

import (
	"context"
	"fmt"
	"runtime"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/library"
)

// filesystemRootKey is the path_key of a path without elements ("/"), which FolderKey maps to
// the empty string; the column refuses an empty key.
const filesystemRootKey = "/"

// foldPathCase reports whether the build target uses the Windows folder comparison of
// library.FolderKey: case folded, "\" as separator. It follows the build target, not the
// configuration.
func foldPathCase() bool { return runtime.GOOS == "windows" }

// folderKey is library.FolderKey with a non-empty result for the filesystem root.
func folderKey(path string, caseInsensitive bool) string {
	key := library.FolderKey(path, caseInsensitive)
	if key == "" {
		return filesystemRootKey
	}

	return key
}

// RekeyFolders recomputes path_key of every stored folder and import ledger entry with
// library.FolderKey. Migration 00004 keys carried-over rows without case folding, because
// SQL lower() is not Unicode-complete; on Windows this fix-up folds them. Migrate runs it
// after the migrations. It is idempotent and a no-op where the stored keys are current.
func (db *DB) RekeyFolders(ctx context.Context) error {
	return db.rekeyFolders(ctx, foldPathCase())
}

func (db *DB) rekeyFolders(ctx context.Context, caseInsensitive bool) error {
	err := inWriteTransaction(ctx, db.writer, func(queries *sqlcgen.Queries) error {
		err := rekeyFolderRows(ctx, queries, caseInsensitive)
		if err != nil {
			return err
		}

		return rekeyLedger(ctx, queries, caseInsensitive)
	})
	if err != nil {
		return fmt.Errorf("rekey folders: %w", err)
	}

	return nil
}

func rekeyFolderRows(ctx context.Context, queries *sqlcgen.Queries, caseInsensitive bool) error {
	roots, err := queries.ListLibraryRoots(ctx)
	if err != nil {
		return fmt.Errorf("list folders: %w", err)
	}

	for _, root := range roots {
		key := folderKey(root.Path, caseInsensitive)
		if key == root.PathKey {
			continue
		}

		err = queries.UpdateLibraryRootKey(ctx, sqlcgen.UpdateLibraryRootKeyParams{PathKey: key, ID: root.ID})
		if err != nil {
			return fmt.Errorf("rekey folder %q: %w", root.Path, err)
		}
	}

	return nil
}

// rekeyLedger re-keys every ledger entry, also those whose folder is gone. When the new key
// is already taken by another entry, the stale one is dropped: both name the same root.
func rekeyLedger(ctx context.Context, queries *sqlcgen.Queries, caseInsensitive bool) error {
	entries, err := queries.ListLibraryImports(ctx)
	if err != nil {
		return fmt.Errorf("list imports: %w", err)
	}

	taken := make(map[string]bool, len(entries))
	for _, entry := range entries {
		taken[entry.PathKey] = true
	}

	for _, entry := range entries {
		key := folderKey(entry.Path, caseInsensitive)
		if key == entry.PathKey {
			continue
		}

		if taken[key] {
			err = queries.DeleteLibraryImport(ctx, entry.PathKey)
		} else {
			err = queries.UpdateLibraryImportKey(ctx, sqlcgen.UpdateLibraryImportKeyParams{NewKey: key, OldKey: entry.PathKey})
			taken[key] = true
		}

		if err != nil {
			return fmt.Errorf("rekey import of %q: %w", entry.Path, err)
		}
	}

	return nil
}

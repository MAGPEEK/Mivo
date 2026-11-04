package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// SidecarStates returns the stored NFO and image files of the library ordered by relative
// path. An unknown library has none.
func (catalog *LibraryCatalog) SidecarStates(ctx context.Context, libraryID string) ([]library.FileState, error) {
	var rows []sqlcgen.ListSidecarStatesRow

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		var err error

		rows, err = queries.ListSidecarStates(ctx, libraryID)

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("read sidecar states of library %q: %w", libraryID, err)
	}

	states := make([]library.FileState, 0, len(rows))

	for _, row := range rows {
		class, err := library.ParseFileClass(row.Kind)
		if err != nil {
			return nil, undecodable(fmt.Sprintf("sidecar %q: stored kind", row.RelativePath), err)
		}

		states = append(states, library.FileState{
			RelativePath: row.RelativePath,
			Class:        class,
			Size:         row.SizeBytes,
			ModifiedAt:   time.UnixMilli(row.ModifiedAt).UTC(),
		})
	}

	return states, nil
}

// SaveSidecarStates upserts the sidecars and deletes the removed paths in one write
// transaction. Paths that are not stored are ignored, so a repeated call changes nothing.
func (catalog *LibraryCatalog) SaveSidecarStates(
	ctx context.Context, libraryID string, upserts []library.FileState, removals []string,
) error {
	if err := checkSidecars(upserts); err != nil {
		return fmt.Errorf("save sidecar states of library %q: %w", libraryID, err)
	}

	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		found, err := queries.LibraryExists(ctx, libraryID)
		if err != nil {
			return fmt.Errorf("read library: %w", err)
		}

		if !found {
			return fmt.Errorf("library: %w", domain.ErrNotFound)
		}

		return applySidecars(ctx, queries, sidecarChanges{libraryID: libraryID, upserts: upserts, removals: removals})
	})
	if err != nil {
		return fmt.Errorf("save sidecar states of library %q: %w", libraryID, err)
	}

	return nil
}

// sidecarChanges is what one SaveSidecarStates call writes to a library.
type sidecarChanges struct {
	libraryID string
	upserts   []library.FileState
	removals  []string
}

func applySidecars(ctx context.Context, queries *sqlcgen.Queries, changes sidecarChanges) error {
	libraryID := changes.libraryID

	for _, state := range changes.upserts {
		err := queries.UpsertSidecarState(ctx, sqlcgen.UpsertSidecarStateParams{
			LibraryID: libraryID, RelativePath: state.RelativePath, Kind: state.Class.String(),
			SizeBytes: state.Size, ModifiedAt: state.ModifiedAt.UnixMilli(),
		})
		if err != nil {
			return fmt.Errorf("upsert sidecar %q: %w", state.RelativePath, err)
		}
	}

	for _, path := range changes.removals {
		err := queries.DeleteSidecarState(ctx, sqlcgen.DeleteSidecarStateParams{
			LibraryID: libraryID, RelativePath: path,
		})
		if err != nil {
			return fmt.Errorf("delete sidecar %q: %w", path, err)
		}
	}

	return nil
}

// checkSidecars requires a valid relative path and a sidecar class (not video) for each state.
func checkSidecars(states []library.FileState) error {
	for _, state := range states {
		if _, err := library.ParseRelativePath(state.RelativePath); err != nil {
			return err
		}

		if state.Class != library.FileClassNFO && state.Class != library.FileClassImage {
			return fmt.Errorf("sidecar %q has class %q: %w", state.RelativePath, state.Class, domain.ErrInvalid)
		}
	}

	return nil
}

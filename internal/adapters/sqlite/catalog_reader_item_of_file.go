package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/activity"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

// ItemOfFile names the catalog item of a media file, without a visibility restriction: the
// activity view is an administrator's, who sees the whole catalog. An unknown file wraps
// domain.ErrNotFound.
func (catalog *CatalogReader) ItemOfFile(ctx context.Context, fileID string) (activity.Item, error) {
	var found activity.Item

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		owner, err := queries.GetFileOwner(ctx, fileID)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read file owner: %w", err)
		}

		item, err := viewOf(access.Unrestricted()).item(ctx, queries, owner.ItemID)
		if err != nil {
			return err
		}

		found = activity.Item{ID: item.ID, Title: item.Title}

		return nil
	})
	if err != nil {
		return activity.Item{}, fmt.Errorf("item of file %q: %w", fileID, err)
	}

	return found, nil
}

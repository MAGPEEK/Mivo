package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
)

// The reader serves the catalog side of the folder view.
var _ applibrary.FolderItemReader = (*CatalogReader)(nil)

// folderSeparator ends the directory prefix of a relative path; folderSeparatorSuccessor is
// the next character, so every path below "dir/" sorts in ["dir/", "dir0").
const (
	folderSeparator          = "/"
	folderSeparatorSuccessor = "0"
)

// folderItemsSQL selects the items that own a file directly in a directory of a library
// folder: the path starts with the prefix and has no "/" after it. The range on relative_path
// lets SQLite search the unique index (root_id, relative_path); the top of the folder (empty
// prefix) has no range. substr and length count characters, so they agree on any name.
const folderItemsSQL = `SELECT media_files.item_id, MIN(substr(media_files.relative_path, length(:prefix) + 1))
FROM media_files JOIN items ON items.id = media_files.item_id
WHERE media_files.root_id = :root
  AND (:prefix = '' OR (media_files.relative_path >= :prefix AND media_files.relative_path < :upper))
  AND instr(substr(media_files.relative_path, length(:prefix) + 1), '/') = 0
  AND items.kind <> 'extra'
GROUP BY media_files.item_id`

// ListFolderItems returns the items (never extras) with a file directly in the directory of
// the library folder rootID, each named after its first file there by path.
func (catalog *CatalogReader) ListFolderItems(ctx context.Context, rootID, directory string) ([]applibrary.FolderItem, error) {
	prefix, upper := "", ""
	if directory != "" {
		prefix, upper = directory+folderSeparator, directory+folderSeparatorSuccessor
	}

	rows, err := catalog.reader.QueryContext(ctx, folderItemsSQL,
		sql.Named("prefix", prefix), sql.Named("upper", upper), sql.Named("root", rootID))
	if err != nil {
		return nil, fmt.Errorf("list folder items of %q: %w", directory, err)
	}

	defer closeRows(rows)

	var items []applibrary.FolderItem

	for rows.Next() {
		var item applibrary.FolderItem
		if err := rows.Scan(&item.ItemID, &item.Name); err != nil {
			return nil, fmt.Errorf("list folder items of %q: %w", directory, err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list folder items of %q: %w", directory, err)
	}

	return items, nil
}

// ItemSummaries returns the summaries of the given items in the given order, as the listings
// build them; unknown IDs are left out.
func (catalog *CatalogReader) ItemSummaries(ctx context.Context, itemIDs []string) ([]applibrary.ItemSummary, error) {
	keys := make([]listingKey, 0, len(itemIDs))
	for _, id := range itemIDs {
		keys = append(keys, listingKey{id: id})
	}

	var summaries []applibrary.ItemSummary

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		listed, err := listedItemsInOrder(ctx, queries, keys)
		if err != nil {
			return err
		}

		summaries, err = summariesOf(ctx, queries, listed, sqlcgen.Item{})

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("item summaries: %w", err)
	}

	return summaries, nil
}

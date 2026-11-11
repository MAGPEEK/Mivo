package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/access"
)

// The reader serves the catalog side of the folder view.
var _ applibrary.FolderItemReader = (*CatalogReader)(nil)

// folderSeparator ends the directory prefix of a relative path; folderSeparatorSuccessor is
// the next character, so every path below "dir/" sorts in ["dir/", "dir0").
const (
	folderSeparator          = "/"
	folderSeparatorSuccessor = "0"
)

// folderScope is a directory of a library folder as a viewer may see it; directory is
// "/"-separated and empty for the folder itself.
type folderScope struct {
	rootID      string
	directory   string
	restriction access.Restriction
}

// folderFiles is the part the folder queries share: the files below the directory that
// belong to items the viewer may see.
type folderFiles struct {
	// rest is the SQL of a file's path below the directory.
	rest string
	// from is the FROM and WHERE clause of those files.
	from string
}

// filesSQL selects the files of visible items (never extras) below the directory: the path
// starts with the directory prefix. The range on relative_path lets SQLite search the unique
// index (root_id, relative_path); the top of the folder (empty prefix) has no range. substr
// and length count characters, so they agree on any name.
func (scope folderScope) filesSQL(builder *sqlBuilder) folderFiles {
	prefix := ""
	if scope.directory != "" {
		prefix = scope.directory + folderSeparator
	}

	prefixParameter := builder.arg(prefix)
	from := " FROM media_files JOIN items ON items.id = media_files.item_id" +
		" WHERE media_files.root_id = " + builder.arg(scope.rootID)

	if prefix != "" {
		from += " AND media_files.relative_path >= " + prefixParameter +
			" AND media_files.relative_path < " + builder.arg(scope.directory+folderSeparatorSuccessor)
	}

	return folderFiles{
		rest: "substr(media_files.relative_path, length(" + prefixParameter + ") + 1)",
		from: from + " AND items.kind <> 'extra'" + andSQL(restrictionConditionSQL(builder, scope.restriction, "items")),
	}
}

// folderItemsQuery returns the SQL of the visible items that own a file directly in the
// directory (no "/" after the prefix), each with the name of its first file there.
func folderItemsQuery(scope folderScope) (string, []any) {
	builder := &sqlBuilder{}
	files := scope.filesSQL(builder)

	builder.write("SELECT media_files.item_id, MIN(", files.rest, ")", files.from,
		" AND instr(", files.rest, ", '/') = 0 GROUP BY media_files.item_id")

	return builder.text.String(), builder.args
}

// folderDirectoriesQuery returns the SQL of the names of the directories directly in the
// directory that hold a file of a visible item, at any depth below them, in name order.
func folderDirectoriesQuery(scope folderScope) (string, []any) {
	builder := &sqlBuilder{}
	files := scope.filesSQL(builder)

	builder.write("SELECT DISTINCT substr(rest, 1, instr(rest, '/') - 1) AS name FROM (SELECT ", files.rest, " AS rest",
		files.from, ") WHERE instr(rest, '/') > 0 ORDER BY name")

	return builder.text.String(), builder.args
}

// ListFolderItems returns the items (never extras) with a file directly in the directory of
// the library folder rootID, each named after its first file there by path. Items the
// restriction hides are left out; a restriction that allows no library reads nothing.
func (catalog *CatalogReader) ListFolderItems(
	ctx context.Context, rootID, directory string, restriction access.Restriction,
) ([]applibrary.FolderItem, error) {
	if restriction.DeniesAll() {
		return nil, nil
	}

	text, args := folderItemsQuery(folderScope{rootID: rootID, directory: directory, restriction: restriction})

	rows, err := catalog.reader.QueryContext(ctx, text, args...)
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

// ListFolderDirectories returns the names of the directories directly in the directory of the
// library folder rootID that hold at least one file of an item the restriction allows, at any
// depth, in name order. The folder view of a restricted viewer shows only these, so a
// directory whose items are all hidden does not appear (ADR 0032 §7).
func (catalog *CatalogReader) ListFolderDirectories(
	ctx context.Context, rootID, directory string, restriction access.Restriction,
) ([]string, error) {
	if restriction.DeniesAll() {
		return nil, nil
	}

	text, args := folderDirectoriesQuery(folderScope{rootID: rootID, directory: directory, restriction: restriction})

	rows, err := catalog.reader.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, fmt.Errorf("list folder directories of %q: %w", directory, err)
	}

	defer closeRows(rows)

	var names []string

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("list folder directories of %q: %w", directory, err)
		}

		names = append(names, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list folder directories of %q: %w", directory, err)
	}

	return names, nil
}

// ItemSummaries returns the summaries of the given items in the given order, as the listings
// build them. Unknown IDs and items the restriction hides are left out, and the counts of a
// summary cover only the children and episodes the restriction allows.
func (catalog *CatalogReader) ItemSummaries(
	ctx context.Context, itemIDs []string, restriction access.Restriction,
) ([]applibrary.ItemSummary, error) {
	keys := make([]listingKey, 0, len(itemIDs))
	for _, id := range itemIDs {
		keys = append(keys, listingKey{id: id})
	}

	var summaries []applibrary.ItemSummary

	err := inListingTransaction(ctx, catalog.reader, func(transaction *sql.Tx, queries *sqlcgen.Queries) error {
		listed, err := listedItemsInOrder(ctx, listingSession{transaction: transaction, queries: queries}, keys, restriction)
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

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
)

var _ images.PhotoSource = (*CatalogReader)(nil)

// PhotoFile returns the file a photo item's thumbnails are rendered from: the photo's own
// file, not its RAW alternate. An ID that names no photo, and a photo without files, wrap
// domain.ErrNotFound.
func (catalog *CatalogReader) PhotoFile(ctx context.Context, itemID string) (images.PhotoFile, error) {
	row, err := sqlcgen.New(catalog.reader).GetPhotoMainFile(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return images.PhotoFile{}, fmt.Errorf("photo file of %q: %w", itemID, domain.ErrNotFound)
	}

	if err != nil {
		return images.PhotoFile{}, fmt.Errorf("read photo file of %q: %w", itemID, err)
	}

	return images.PhotoFile{
		ItemID: itemID, LibraryRoot: row.RootPath, RelativePath: row.RelativePath,
		Size: row.SizeBytes, ModifiedAt: time.UnixMilli(row.ModifiedAt).UTC(),
	}, nil
}

// PhotoTags returns the thumbnail cache tag (images.PhotoFile.Tag) of every photo with a
// file, keyed by item ID, for the purge of the image cache.
func (catalog *CatalogReader) PhotoTags(ctx context.Context) (map[string]string, error) {
	rows, err := sqlcgen.New(catalog.reader).ListPhotoMainFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("read photo tags: %w", err)
	}

	tags := make(map[string]string, len(rows))

	for _, row := range rows {
		// The rows of an item come main file first; the others are alternates.
		if _, seen := tags[row.ItemID]; seen {
			continue
		}

		file := images.PhotoFile{
			ItemID: row.ItemID, RelativePath: row.RelativePath,
			Size: row.SizeBytes, ModifiedAt: time.UnixMilli(row.ModifiedAt).UTC(),
		}
		tags[row.ItemID] = file.Tag()
	}

	return tags, nil
}

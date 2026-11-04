package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// Artwork returns one stored artwork with the root of the library its bytes live in. An
// unknown ID, and a provider picture that is not downloaded yet, wrap domain.ErrNotFound.
func (catalog *CatalogReader) Artwork(ctx context.Context, id string) (artwork.Stored, error) {
	var stored artwork.Stored

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		row, err := queries.GetArtworkWithRoot(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return domain.ErrNotFound
		}

		if err != nil {
			return fmt.Errorf("read artwork: %w", err)
		}

		stored, err = storedArtworkFromRow(row)

		return err
	})
	if err != nil {
		return artwork.Stored{}, fmt.Errorf("artwork %q: %w", id, err)
	}

	return stored, nil
}

// ArtworkTags returns the current cache tag of every artwork, keyed by artwork ID, for the
// purge of the image cache.
func (catalog *CatalogReader) ArtworkTags(ctx context.Context) (map[string]string, error) {
	var tags map[string]string

	err := inReadTransaction(ctx, catalog.reader, func(queries *sqlcgen.Queries) error {
		rows, err := queries.ListArtworkTags(ctx)
		if err != nil {
			return fmt.Errorf("read artwork tags: %w", err)
		}

		tags = make(map[string]string, len(rows))
		for _, row := range rows {
			tags[row.ID] = row.Tag
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("artwork tags: %w", err)
	}

	return tags, nil
}

func storedArtworkFromRow(row sqlcgen.GetArtworkWithRootRow) (artwork.Stored, error) {
	kind, err := artwork.ParseKind(row.Kind)
	if err != nil {
		return artwork.Stored{}, undecodable(fmt.Sprintf("artwork %q: stored kind", row.ID), err)
	}

	source, err := artwork.ParseSourceKind(row.Source)
	if err != nil {
		return artwork.Stored{}, undecodable(fmt.Sprintf("artwork %q: stored source", row.ID), err)
	}

	location, err := artworkLocation(source, row)
	if err != nil {
		return artwork.Stored{}, err
	}

	return artwork.Stored{
		ID: row.ID, Kind: kind, Source: source, LibraryRoot: row.LibraryRoot, Location: location,
		Size: row.SizeBytes, ModifiedAt: time.UnixMilli(row.ModifiedAt).UTC(), Tag: row.Tag,
	}, nil
}

// artworkLocation picks the location columns the source fills (the CHECKs of the artwork
// table guarantee the others are empty).
func artworkLocation(source artwork.SourceKind, row sqlcgen.GetArtworkWithRootRow) (artwork.Location, error) {
	switch source {
	case artwork.SourceLocal:
		return artwork.LocalLocation(row.RelativePath.String), nil
	case artwork.SourceEmbedded:
		return artwork.EmbeddedLocation(row.MediaRelativePath.String, int(row.StreamIndex.Int64)), nil
	case artwork.SourceProvider, artwork.SourceUpload:
		if !row.DataPath.Valid {
			return artwork.Location{}, fmt.Errorf("artwork %q has no downloaded file: %w", row.ID, domain.ErrNotFound)
		}

		return artwork.DataLocation(row.DataPath.String), nil
	default:
		return artwork.Location{}, undecodable(fmt.Sprintf("artwork %q", row.ID), fmt.Errorf("unknown source %q", source))
	}
}

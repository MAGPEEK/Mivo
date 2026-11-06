package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

var _ scan.LocalRecordStore = (*LibraryCatalog)(nil)

// storedLocalRecord is the JSON form of a local record in item_local_records. metadata.Date
// has no exported fields, so the release date travels as ISO text; the outer field is
// shallower than the embedded one and therefore wins in encoding/json.
type storedLocalRecord struct {
	metadata.Record
	ReleaseDate string `json:"ReleaseDate,omitempty"`
}

func encodeLocalRecord(record metadata.Record) (string, error) {
	stored := storedLocalRecord{Record: record}
	if !record.ReleaseDate.IsZero() {
		stored.ReleaseDate = record.ReleaseDate.String()
	}

	encoded, err := json.Marshal(stored)
	if err != nil {
		return "", fmt.Errorf("encode local record: %w", err)
	}

	return string(encoded), nil
}

func decodeLocalRecord(text string) (metadata.Record, error) {
	var stored storedLocalRecord
	if err := json.Unmarshal([]byte(text), &stored); err != nil {
		return metadata.Record{}, fmt.Errorf("decode local record: %w", err)
	}

	record := stored.Record
	record.ReleaseDate = metadata.Date{}

	if stored.ReleaseDate != "" {
		date, err := metadata.ParseDate(stored.ReleaseDate)
		if err != nil {
			return metadata.Record{}, fmt.Errorf("decode local record: %w", err)
		}

		record.ReleaseDate = date
	}

	return record, nil
}

// SaveLocalRecords stores what the local sources of an item said in a scan (scan.
// LocalRecordStore). With KeepNFO a stored NFO record stays. The error wraps
// domain.ErrNotFound for an unknown item.
func (catalog *LibraryCatalog) SaveLocalRecords(ctx context.Context, itemID string, records scan.LocalRecords) error {
	nfo, err := encodeLocalRecord(records.NFO)
	if err != nil {
		return fmt.Errorf("save local records of item %q: %w", itemID, err)
	}

	filename, err := encodeLocalRecord(records.Filename)
	if err != nil {
		return fmt.Errorf("save local records of item %q: %w", itemID, err)
	}

	queries := sqlcgen.New(catalog.writer)
	updatedAt := catalog.now().UnixMilli()

	if records.KeepNFO {
		err = queries.UpsertItemLocalFilename(ctx, sqlcgen.UpsertItemLocalFilenameParams{
			ItemID: itemID, Nfo: nfo, Filename: filename, UpdatedAt: updatedAt,
		})
	} else {
		err = queries.UpsertItemLocalRecords(ctx, sqlcgen.UpsertItemLocalRecordsParams{
			ItemID: itemID, Nfo: nfo, Filename: filename, UpdatedAt: updatedAt,
		})
	}

	if err != nil {
		return fmt.Errorf("save local records of item %q: %w", itemID, foreignKeyAsNotFound(err))
	}

	return nil
}

// readLocalRecords returns the stored local records; found is false for an item scanned
// before they were kept.
func readLocalRecords(ctx context.Context, queries *sqlcgen.Queries, itemID string) (enrich.LocalRecords, bool, error) {
	row, err := queries.GetItemLocalRecords(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return enrich.LocalRecords{}, false, nil
	}

	if err != nil {
		return enrich.LocalRecords{}, false, fmt.Errorf("read local records: %w", err)
	}

	nfo, err := decodeLocalRecord(row.Nfo)
	if err != nil {
		return enrich.LocalRecords{}, false, undecodable(fmt.Sprintf("item %q: nfo record", itemID), err)
	}

	filename, err := decodeLocalRecord(row.Filename)
	if err != nil {
		return enrich.LocalRecords{}, false, undecodable(fmt.Sprintf("item %q: file name record", itemID), err)
	}

	return enrich.LocalRecords{NFO: nfo, Filename: filename}, true, nil
}

// localRecordsOf returns the local records of an item. An item without them was scanned
// before they were kept and never saved by enrichment since (seedLocalRecords runs first),
// so its stored record holds local data only; it stands in for the file name source, the
// lowest, so provider data still wins over it.
func localRecordsOf(ctx context.Context, queries *sqlcgen.Queries, itemID string, stored metadata.Record) (enrich.LocalRecords, error) {
	records, found, err := readLocalRecords(ctx, queries, itemID)
	if err != nil || found {
		return records, err
	}

	return enrich.LocalRecords{Filename: stored}, nil
}

// seedLocalRecords keeps the stored record as the local file name record of an item that
// has none yet, before enrichment or a match change overwrites it with provider data.
func seedLocalRecords(ctx context.Context, queries *sqlcgen.Queries, itemID string, now time.Time) error {
	if _, found, err := readLocalRecords(ctx, queries, itemID); err != nil || found {
		return err
	}

	stored, err := readStoredRecord(ctx, queries, itemID)
	if err != nil {
		return err
	}

	nfo, err := encodeLocalRecord(metadata.Record{})
	if err != nil {
		return err
	}

	filename, err := encodeLocalRecord(stored)
	if err != nil {
		return err
	}

	err = queries.InsertItemLocalRecordsIfMissing(ctx, sqlcgen.InsertItemLocalRecordsIfMissingParams{
		ItemID: itemID, Nfo: nfo, Filename: filename, UpdatedAt: now.UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("seed local records: %w", err)
	}

	return nil
}

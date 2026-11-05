package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// EnrichmentStore persists what online enrichment needs (ADR 0031): the match state of an
// item, the provider response cache, the enrichment queue, the per-library switch and the
// removal of provider artwork. Reads go to the reader pool; every change runs on the writer
// pool. The ports of M6-06 and M6-11 are implemented on top of these methods.
type EnrichmentStore struct {
	writer *sql.DB
	reader *sql.DB
	clock  func() time.Time
	logger *slog.Logger
}

// NewEnrichmentStore returns a store on db, which must be migrated. clock supplies the
// current time for the queue and for updated_at (nil means time.Now).
func NewEnrichmentStore(db *DB, clock func() time.Time) *EnrichmentStore {
	if clock == nil {
		clock = time.Now
	}

	return &EnrichmentStore{writer: db.writer, reader: db.reader, clock: clock, logger: db.logger}
}

// ItemMatch is the online match of an item. Provider and MatchedAt are set for a match;
// Confidence is the score that led to it (1 for a user identification).
type ItemMatch struct {
	State      match.MatchState
	Provider   string
	Confidence float64
	MatchedAt  time.Time
}

// ItemMatch returns the match of an item; an item that was never matched is unmatched.
// The error wraps domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) ItemMatch(ctx context.Context, itemID string) (ItemMatch, error) {
	row, err := sqlcgen.New(store.reader).GetItemMatch(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return ItemMatch{}, fmt.Errorf("read match of item %q: %w", itemID, domain.ErrNotFound)
	}

	if err != nil {
		return ItemMatch{}, fmt.Errorf("read match of item %q: %w", itemID, err)
	}

	return storedItemMatch(row)
}

func storedItemMatch(row sqlcgen.GetItemMatchRow) (ItemMatch, error) {
	if !row.State.Valid {
		return ItemMatch{State: match.MatchStateUnmatched}, nil
	}

	state, err := match.ParseMatchState(row.State.String)
	if err != nil {
		return ItemMatch{}, fmt.Errorf("read match of item %q: %w", row.ID, err)
	}

	stored := ItemMatch{State: state, Provider: row.Provider.String, Confidence: row.Confidence.Float64}
	if row.MatchedAt.Valid {
		stored.MatchedAt = time.UnixMilli(row.MatchedAt.Int64).UTC()
	}

	return stored, nil
}

// SetItemMatch replaces the match of an item. The error wraps domain.ErrInvalid for an
// unknown state, a confidence outside [0, 1] or a match without provider or time, and
// domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) SetItemMatch(ctx context.Context, itemID string, itemMatch ItemMatch) error {
	if err := validateItemMatch(itemMatch); err != nil {
		return fmt.Errorf("set match of item %q: %w", itemID, err)
	}

	err := sqlcgen.New(store.writer).UpsertItemMatch(ctx, sqlcgen.UpsertItemMatchParams{
		ItemID:     itemID,
		State:      itemMatch.State.String(),
		Provider:   itemMatch.Provider,
		Confidence: itemMatch.Confidence,
		MatchedAt:  optionalMillis(itemMatch.MatchedAt),
	})
	if err != nil {
		return fmt.Errorf("set match of item %q: %w", itemID, foreignKeyAsNotFound(err))
	}

	return nil
}

func validateItemMatch(itemMatch ItemMatch) error {
	switch {
	case !itemMatch.State.IsValid():
		return fmt.Errorf("unknown match state %q: %w", itemMatch.State, domain.ErrInvalid)
	case math.IsNaN(itemMatch.Confidence) || itemMatch.Confidence < 0 || itemMatch.Confidence > 1:
		return fmt.Errorf("confidence %v outside [0, 1]: %w", itemMatch.Confidence, domain.ErrInvalid)
	case itemMatch.State == match.MatchStateMatched && (itemMatch.Provider == "" || itemMatch.MatchedAt.IsZero()):
		return fmt.Errorf("match without provider or time: %w", domain.ErrInvalid)
	default:
		return nil
	}
}

// DeleteProviderArtwork removes the provider artwork rows of an item ("Remove match", ADR
// 0031 §9) and returns the data-directory-relative paths of their downloaded files, which
// the caller deletes. A kind whose selected picture was removed is left without selection
// until the next artwork save. A stored path that would leave the data directory (absolute,
// with "..", or with a backslash) is not returned, so a tampered row cannot make the caller
// delete a file elsewhere; it is logged without the path.
func (store *EnrichmentStore) DeleteProviderArtwork(ctx context.Context, itemID string) ([]string, error) {
	dataPaths, err := sqlcgen.New(store.writer).DeleteProviderArtwork(ctx, itemID)
	if err != nil {
		return nil, fmt.Errorf("delete provider artwork of item %q: %w", itemID, err)
	}

	files := make([]string, 0, len(dataPaths))
	for _, dataPath := range dataPaths {
		if !dataPath.Valid {
			continue
		}

		if !isLocalDataPath(dataPath.String) {
			store.logger.LogAttrs(ctx, slog.LevelWarn, "skipped provider artwork file outside the data directory",
				slog.String("item_id", itemID))

			continue
		}

		files = append(files, dataPath.String)
	}

	return files, nil
}

// isLocalDataPath reports a slash-separated path that stays below the data directory. The
// backslash is rejected on every platform: it is a separator on Windows, and data paths
// never contain one.
func isLocalDataPath(dataPath string) bool {
	return !strings.Contains(dataPath, `\`) && filepath.IsLocal(filepath.FromSlash(dataPath))
}

// ItemOnlineMetadata reports whether the library of an item allows online metadata. The
// error wraps domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) ItemOnlineMetadata(ctx context.Context, itemID string) (bool, error) {
	enabled, err := sqlcgen.New(store.reader).GetItemOnlineMetadata(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("read online metadata switch of item %q: %w", itemID, domain.ErrNotFound)
	}

	if err != nil {
		return false, fmt.Errorf("read online metadata switch of item %q: %w", itemID, err)
	}

	return enabled == storedTrue, nil
}

// SetLibraryOnlineMetadata switches online metadata of a library on or off. The error wraps
// domain.ErrNotFound for an unknown library.
func (store *EnrichmentStore) SetLibraryOnlineMetadata(ctx context.Context, libraryID string, enabled bool) error {
	changed, err := sqlcgen.New(store.writer).SetLibraryOnlineMetadata(ctx, sqlcgen.SetLibraryOnlineMetadataParams{
		OnlineMetadata: storedFlag(enabled),
		UpdatedAt:      store.clock().UnixMilli(),
		ID:             libraryID,
	})
	if err != nil {
		return fmt.Errorf("set online metadata switch of library %q: %w", libraryID, err)
	}

	if changed == 0 {
		return fmt.Errorf("set online metadata switch of library %q: %w", libraryID, domain.ErrNotFound)
	}

	return nil
}

// optionalMillis stores the zero time as NULL.
func optionalMillis(moment time.Time) sql.NullInt64 {
	if moment.IsZero() {
		return sql.NullInt64{}
	}

	return sql.NullInt64{Int64: moment.UnixMilli(), Valid: true}
}

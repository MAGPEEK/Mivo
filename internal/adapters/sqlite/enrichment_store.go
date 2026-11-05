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
// Confidence is the score that led to it (1 for a user identification). Ref is the provider
// entry the item was linked by (enrich.MatchResult.Ref), zero when none; it is the only
// place a MusicBrainz ID is kept. CheckedAt is when the match was last stored (zero when
// never); SetItemMatch takes it from the store's clock and ignores the field.
type ItemMatch struct {
	State      match.MatchState
	Provider   string
	Confidence float64
	MatchedAt  time.Time
	Ref        match.ProviderRef
	CheckedAt  time.Time
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

	return ItemMatch{
		State:      state,
		Provider:   row.Provider.String,
		Confidence: row.Confidence.Float64,
		MatchedAt:  storedMillis(row.MatchedAt),
		Ref: match.ProviderRef{
			Provider: match.RefProvider(row.RefProvider.String),
			Kind:     match.RefKind(row.RefKind.String),
			ID:       row.RefID.String,
		},
		CheckedAt: storedMillis(row.CheckedAt),
	}, nil
}

// SetItemMatch replaces the match of an item. The error wraps domain.ErrInvalid for an
// unknown state, a confidence outside [0, 1], a match without provider or time or a
// malformed reference, and domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) SetItemMatch(ctx context.Context, itemID string, itemMatch ItemMatch) error {
	if err := validateItemMatch(itemMatch); err != nil {
		return fmt.Errorf("set match of item %q: %w", itemID, err)
	}

	err := sqlcgen.New(store.writer).UpsertItemMatch(ctx, sqlcgen.UpsertItemMatchParams{
		ItemID:      itemID,
		State:       itemMatch.State.String(),
		Provider:    itemMatch.Provider,
		Confidence:  itemMatch.Confidence,
		MatchedAt:   optionalMillis(itemMatch.MatchedAt),
		RefProvider: string(itemMatch.Ref.Provider),
		RefKind:     string(itemMatch.Ref.Kind),
		RefID:       itemMatch.Ref.ID,
		CheckedAt:   optionalMillis(store.clock()),
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
	case !validMatchRef(itemMatch.Ref):
		return fmt.Errorf("malformed reference %s/%s/%q: %w",
			itemMatch.Ref.Provider, itemMatch.Ref.Kind, itemMatch.Ref.ID, domain.ErrInvalid)
	default:
		return nil
	}
}

// validMatchRef accepts the zero reference and a reference of a known provider and kind
// with an ID; the item_match CHECK constraints enforce the same lists.
func validMatchRef(ref match.ProviderRef) bool {
	if ref == (match.ProviderRef{}) {
		return true
	}

	switch ref.Provider {
	case match.RefTMDB, match.RefIMDb, match.RefMusicBrainz:
	default:
		return false
	}

	switch ref.Kind {
	case match.RefKindUnknown, match.RefKindMovie, match.RefKindShow, match.RefKindRelease, match.RefKindArtist:
		return ref.ID != ""
	default:
		return false
	}
}

// ItemsDueForRefresh returns up to limit items of libraries with online metadata whose match
// was last stored before cutoff (or never had a time recorded), oldest first, skipping
// disabled matches and items that are already queued, so a job in backoff keeps its due
// time. It implements enrich.RefreshSource.
func (store *EnrichmentStore) ItemsDueForRefresh(ctx context.Context, cutoff time.Time, limit int) ([]string, error) {
	itemIDs, err := sqlcgen.New(store.reader).ListItemsDueForRefresh(ctx, sqlcgen.ListItemsDueForRefreshParams{
		CheckedAt: sql.NullInt64{Int64: cutoff.UnixMilli(), Valid: true},
		Limit:     int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list items due for refresh: %w", err)
	}

	return itemIDs, nil
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

// storedMillis reads a nullable millisecond column; NULL is the zero time.
func storedMillis(millis sql.NullInt64) time.Time {
	if !millis.Valid {
		return time.Time{}
	}

	return time.UnixMilli(millis.Int64).UTC()
}

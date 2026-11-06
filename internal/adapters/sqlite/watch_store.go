package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// watchBatchSize bounds the item IDs bound to one watch state statement: marking a long
// show watched names every episode, and SQLite accepts at most 32 766 parameters.
const watchBatchSize = 500

// WatchStore stores the per-profile watch state (ADR 0032 §8). Rows are keyed by profile and
// item ID without a foreign key to items, so no catalog change deletes them; deleting the
// profile does. Reads go to the reader pool; UpdateStates runs on the writer pool, whose
// transactions start with BEGIN IMMEDIATE, so a read-modify-write never loses a concurrent
// one.
//
// The request and result types mirror the ports of app/watch (M7-18) field for field; this
// package cannot import app/watch, so cmd/mivo needs a thin adapter from the store to
// StateWriter and StateReader (type conversions only).
type WatchStore struct {
	writer *sql.DB
	reader *sql.DB
}

// NewWatchStore returns a store on db, which must be migrated.
func NewWatchStore(db *DB) *WatchStore {
	return &WatchStore{writer: db.writer, reader: db.reader}
}

// WatchStateKey names the stored state of one item of a profile and the attach key (the
// item's provider reference, empty when unmatched) to store with it.
type WatchStateKey struct {
	ItemID    string
	AttachKey string
}

// WatchStateUpdate is one read-modify-write of a profile's states: Change applied to the
// stored state of each key's item (the zero State when there is none), Now stored as
// updated_at.
type WatchStateUpdate struct {
	ProfileID string
	Keys      []WatchStateKey
	Change    func(watch.State) watch.State
	Now       time.Time
}

// UpdateStates applies update.Change to the stored state of every key's item and upserts the
// results with the key's attach key, all in one transaction. A result equal to the stored
// state (with the same attach key) is not written, and neither is a zero result for an item
// without a row, so marking never-started episodes unwatched adds no rows. A key named twice
// is changed once, with its first attach key. An unknown profile wraps domain.ErrNotFound.
func (store *WatchStore) UpdateStates(ctx context.Context, update WatchStateUpdate) error {
	keys := uniqueKeys(update.Keys)
	if len(keys) == 0 {
		return nil
	}

	err := inWriteTransaction(ctx, store.writer, func(queries *sqlcgen.Queries) error {
		for batch := range slices.Chunk(keys, watchBatchSize) {
			if err := updateBatch(ctx, queries, update, batch); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("update watch states of profile %s: %w", update.ProfileID, err)
	}

	return nil
}

func updateBatch(ctx context.Context, queries *sqlcgen.Queries, update WatchStateUpdate, keys []WatchStateKey) error {
	stored, err := readRows(ctx, queries, update.ProfileID, itemIDsOf(keys))
	if err != nil {
		return err
	}

	for _, key := range keys {
		current, exists := stored[key.ItemID]
		next := rowOf(update.Change(current.state()), key.AttachKey)

		if next == current || (!exists && next == watchRow{attachKey: next.attachKey}) {
			continue
		}

		if err := queries.UpsertWatchState(ctx, next.params(update.ProfileID, key.ItemID, update.Now)); err != nil {
			return fmt.Errorf("store watch state of item %s: %w", key.ItemID, foreignKeyAsNotFound(err))
		}
	}

	return nil
}

// readRows reads the stored rows of the items (at most watchBatchSize) by item ID.
func readRows(ctx context.Context, queries *sqlcgen.Queries, profileID string, itemIDs []string) (map[string]watchRow, error) {
	rows, err := queries.ListWatchStates(ctx, sqlcgen.ListWatchStatesParams{ProfileID: profileID, ItemIds: itemIDs})
	if err != nil {
		return nil, fmt.Errorf("read watch states: %w", err)
	}

	stored := make(map[string]watchRow, len(rows))
	for _, row := range rows {
		stored[row.ItemID] = watchRow{
			positionSeconds: row.PositionSeconds, watched: row.Watched == 1, playCount: row.PlayCount,
			lastPlayedMillis: row.LastPlayedAt, hidden: row.HiddenFromContinue == 1, attachKey: row.AttachKey,
		}
	}

	return stored, nil
}

func uniqueKeys(keys []WatchStateKey) []WatchStateKey {
	seen := make(map[string]bool, len(keys))
	unique := make([]WatchStateKey, 0, len(keys))

	for _, key := range keys {
		if !seen[key.ItemID] {
			seen[key.ItemID] = true
			unique = append(unique, key)
		}
	}

	return unique
}

func itemIDsOf(keys []WatchStateKey) []string {
	itemIDs := make([]string, len(keys))
	for index, key := range keys {
		itemIDs[index] = key.ItemID
	}

	return itemIDs
}

// watchRow is a watch_state row as the store compares it: times at the stored millisecond
// precision, so a change that keeps a state is recognised as unchanged.
type watchRow struct {
	positionSeconds  float64
	watched          bool
	playCount        int64
	lastPlayedMillis sql.NullInt64
	hidden           bool
	attachKey        string
}

func rowOf(state watch.State, attachKey string) watchRow {
	return watchRow{
		positionSeconds: max(state.PositionSeconds, 0), watched: state.Watched, playCount: int64(max(state.PlayCount, 0)),
		lastPlayedMillis: optionalMillis(state.LastPlayed), hidden: state.Hidden, attachKey: attachKey,
	}
}

func (row watchRow) state() watch.State {
	return watch.State{
		PositionSeconds: row.positionSeconds, Watched: row.watched, PlayCount: int(row.playCount),
		LastPlayed: storedMillis(row.lastPlayedMillis), Hidden: row.hidden,
	}
}

func (row watchRow) params(profileID, itemID string, now time.Time) sqlcgen.UpsertWatchStateParams {
	return sqlcgen.UpsertWatchStateParams{
		ProfileID: profileID, ItemID: itemID, PositionSeconds: row.positionSeconds,
		Watched: boolToInt(row.watched), PlayCount: row.playCount, LastPlayedAt: row.lastPlayedMillis,
		HiddenFromContinue: boolToInt(row.hidden), AttachKey: row.attachKey, UpdatedAt: now.UnixMilli(),
	}
}

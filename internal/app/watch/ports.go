package watch

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/watch"
)

// Target is a catalog item as the watch use cases need it.
type Target struct {
	ItemID string
	Kind   library.ItemKind
	// DurationSeconds is the catalog duration (the primary file's length, the summed parts of
	// an audiobook); 0 means unknown. The client never supplies it (ADR 0032 §8).
	DurationSeconds float64
	// AttachKey is the item's provider reference (e.g. "tmdb:movie:603"), empty when the item
	// is unmatched; stored with the state for a later re-attach step.
	AttachKey string
}

// ItemLookup reads one item for the viewer of ctx. It is viewer-scoped: an item outside the
// viewer's libraries or content limit is reported exactly like an unknown one, wrapping
// domain.ErrNotFound, so its existence does not leak. Satisfied by the scoped app/library
// service (M7-14) through a small adapter in cmd/mivo.
type ItemLookup interface {
	// LookupWatchTarget returns the item with its kind, catalog duration and attach key.
	LookupWatchTarget(ctx context.Context, itemID string) (Target, error)
}

// EpisodeLister lists the episodes below a show or season the caller already looked up
// through ItemLookup, so they need no scope check of their own.
type EpisodeLister interface {
	// ListEpisodes returns every episode of the show (all seasons, specials included) or the
	// season, in any order; DurationSeconds may be left 0. A leaf yields none.
	ListEpisodes(ctx context.Context, containerID string) ([]Target, error)
}

// StateKey names the stored state of one item of a profile and the attach key to store with
// it.
type StateKey struct {
	ItemID    string
	AttachKey string
}

// StateChange computes an item's new state from its stored one (the zero State when there is
// none).
type StateChange func(watch.State) watch.State

// StateUpdate is one read-modify-write of a profile's states: Change applied to each key's
// item, Now stored as updated_at.
type StateUpdate struct {
	ProfileID string
	Keys      []StateKey
	Change    StateChange
	Now       time.Time
}

// StateWriter writes watch state. Implemented by adapters/sqlite (M7-10).
type StateWriter interface {
	// UpdateStates reads the stored state of every key's item (zero when there is none),
	// applies update.Change and upserts the results with the key's attach key and
	// update.Now, all in one transaction, so two players reporting at once never lose an
	// increment. The store may skip writing a result equal to the stored state. Keys is never
	// empty.
	UpdateStates(ctx context.Context, update StateUpdate) error
}

// EpisodeCount is the number of episodes below a show or season and how many of them the
// profile has not watched.
type EpisodeCount struct {
	Total     int
	Unwatched int
}

// StateReader reads watch state for item lists. Implemented by adapters/sqlite (M7-10).
type StateReader interface {
	// States returns the profile's stored states of the items; an item without a row is
	// absent from the map. itemIDs is never empty and has no duplicates.
	States(ctx context.Context, profileID string, itemIDs []string) (map[string]watch.State, error)
	// EpisodeCounts returns per show or season the episodes below it (as in
	// app/library ItemSummary.LeafCount and ChildCount) and the number of them that are not
	// watched by the profile; a container without episodes may be absent. containerIDs is
	// never empty and has no duplicates.
	EpisodeCounts(ctx context.Context, profileID string, containerIDs []string) (map[string]EpisodeCount, error)
}

// StateStore is the watch state store: writes and list reads.
type StateStore interface {
	StateWriter
	StateReader
}

// Clock returns the current time; injected so the stored times are testable.
type Clock func() time.Time

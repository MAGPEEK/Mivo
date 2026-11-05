// Package identify holds the use cases behind the "Zuordnung korrigieren" dialog (ADR 0031
// §9, §10): search provider candidates for an item, apply the user's choice, remove a match
// and queue a metadata refresh. Authorisation is the HTTP guard's job (ADR 0028); the use
// cases stay user-agnostic until accounts exist.
package identify

import (
	"context"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// ItemReader reads what the use cases need to know about an item. The enrichment store
// implements it for enrich.MetadataStore already. The error wraps domain.ErrNotFound for an
// unknown item.
type ItemReader interface {
	// ItemSnapshot returns the item's current state.
	ItemSnapshot(ctx context.Context, itemID string) (enrich.ItemSnapshot, error)
}

// MatchChange is what Apply and Unmatch store for an item, together: the record without the
// earlier provider data, the item's locks and its new match.
type MatchChange struct {
	Record metadata.Record
	Locks  metadata.LockSet
	Match  enrich.MatchResult
}

// MatchWriter stores a match change. The error wraps domain.ErrNotFound for an unknown item.
// An enrichment job already running for the item may still save after a removal; the
// enrichment store must not overwrite a disabled match (BACKLOG, M6-13/M6-14).
type MatchWriter interface {
	// SaveMatchChange replaces record, locks and match of the item in one transaction.
	SaveMatchChange(ctx context.Context, itemID string, change MatchChange) error
}

// ProviderArtworkRemover removes the provider artwork rows of an item. Implemented by the
// enrichment store of adapters/sqlite.
type ProviderArtworkRemover interface {
	// DeleteProviderArtwork removes the rows and returns the data-directory-relative paths
	// of their downloaded files.
	DeleteProviderArtwork(ctx context.Context, itemID string) ([]string, error)
}

// DataFileRemover deletes a downloaded file inside the data directory. A file that is gone
// already wraps domain.ErrNotFound.
type DataFileRemover interface {
	// RemoveDataFile deletes the file at the data-directory-relative path.
	RemoveDataFile(ctx context.Context, dataPath string) error
}

// ShowChildren lists the seasons and episodes of a show, so identifying or refreshing a
// show also refreshes them.
type ShowChildren interface {
	// SeasonsAndEpisodes returns the item IDs of the show's seasons and episodes.
	SeasonsAndEpisodes(ctx context.Context, showID string) ([]string, error)
}

// LibraryItems reads what a library refresh needs.
type LibraryItems interface {
	// LibraryOnlineEnabled returns the library's online_metadata switch. The error wraps
	// domain.ErrNotFound for an unknown library.
	LibraryOnlineEnabled(ctx context.Context, libraryID string) (bool, error)
	// MatchableItems returns the IDs of the library's items whose match state is not
	// disabled.
	MatchableItems(ctx context.Context, libraryID string) ([]string, error)
}

// TitleLookup searches and looks up movies or shows; the caching lookups of enrich
// (MovieLookup, ShowLookup) implement it. Errors wrap domain.ErrUnavailable,
// domain.ErrRateLimited or, for an unknown reference, domain.ErrNotFound.
type TitleLookup interface {
	// Search returns candidates in the provider's order.
	Search(ctx context.Context, query enrich.SearchQuery) ([]match.Candidate, error)
	// Details returns the entry a TMDB or IMDb reference points to.
	Details(ctx context.Context, request enrich.DetailsRequest) (enrich.Details, error)
}

// MusicLookup searches and looks up albums and artists, with the error contract of
// TitleLookup; enrich.MusicLookup implementations fit.
type MusicLookup interface {
	// Search returns candidates of the queried kind in the provider's order.
	Search(ctx context.Context, query enrich.MusicQuery) ([]match.Candidate, error)
	// Details returns the release or artist a MusicBrainz reference points to.
	Details(ctx context.Context, request enrich.DetailsRequest) (enrich.Details, error)
}

// Enqueuer queues an item for enrichment; *enrich.Queue implements it. The error wraps
// domain.ErrNotFound for an unknown item.
type Enqueuer interface {
	// Enqueue queues the item once (dedupe), due now.
	Enqueue(ctx context.Context, itemID string, reason enrich.Reason) error
}

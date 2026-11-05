// Package enrich enriches one library item with online metadata (ADR 0031): it gathers the
// local sources, asks the providers behind small ports, merges the result with
// metadata.Merge respecting field locks, registers provider artwork and saves. The package
// knows nothing about the network; adapters translate provider answers into domain types.
package enrich

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// The request types below are everything a provider is told (ADR 0031 §5). They carry a
// title, a year, season numbers, a language and provider references only: never a path,
// a file name, a library or an item ID.

// SearchQuery asks a movie or show provider for candidates. Year is 0 when unknown;
// Language is the metadata language tag (for example "de" or "pt-BR").
type SearchQuery struct {
	Title    string
	Year     int
	Language string
}

// MusicQuery asks a music provider for candidates of one kind (match.RefKindRelease for an
// album, match.RefKindArtist for an artist). Artist is the album artist when searching
// releases; Year is 0 when unknown.
type MusicQuery struct {
	Kind   match.RefKind
	Title  string
	Artist string
	Year   int
}

// DetailsRequest asks for the full entry a reference points to.
type DetailsRequest struct {
	Ref      match.ProviderRef
	Language string
}

// SeasonRequest asks for one season of a show; Season is the number from the item's
// placement (0 for specials).
type SeasonRequest struct {
	Show     match.ProviderRef
	Season   int
	Language string
}

// Details is a provider entry in domain form: its record and its pictures, best first.
type Details struct {
	Record  metadata.Record
	Artwork []ArtworkRef
}

// SeasonDetails is a season with its episodes.
type SeasonDetails struct {
	Details
	Episodes []EpisodeDetails
}

// EpisodeDetails is one episode of a season; Number is the provider's episode number, which
// is compared with the number from the placement, never used to move the item.
type EpisodeDetails struct {
	Number int
	Details
}

// ArtworkProvider names the service an ArtworkRef points into.
type ArtworkProvider string

// Artwork providers (ADR 0031 §1).
const (
	ArtworkProviderTMDB     ArtworkProvider = "tmdb"
	ArtworkProviderCoverArt ArtworkProvider = "coverartarchive"
)

// ArtworkRef is a provider picture that has not been downloaded. Path is the provider-side
// address (a TMDB file path or a Cover Art Archive URL); the downloader checks it against
// its allowed hosts.
type ArtworkRef struct {
	Provider ArtworkProvider
	Kind     artwork.Kind
	Path     string
}

// Stored is a downloaded provider picture: DataPath is relative to the data directory.
type Stored struct {
	DataPath   string
	Size       int64
	ModifiedAt time.Time
}

// MovieLookup finds movies. Implemented by adapters/metadata/tmdb (M6-07). Errors wrap
// domain.ErrUnavailable (offline, breaker open), domain.ErrRateLimited or domain.ErrNotFound
// (unknown reference).
type MovieLookup interface {
	// Search returns candidates in the provider's order.
	Search(ctx context.Context, query SearchQuery) ([]match.Candidate, error)
	// Details returns the entry a TMDB or IMDb reference points to.
	Details(ctx context.Context, request DetailsRequest) (Details, error)
}

// ShowLookup finds shows, seasons and episodes, with the error contract of MovieLookup.
type ShowLookup interface {
	// Search returns candidates in the provider's order.
	Search(ctx context.Context, query SearchQuery) ([]match.Candidate, error)
	// Details returns the show a TMDB or IMDb reference points to.
	Details(ctx context.Context, request DetailsRequest) (Details, error)
	// SeasonDetails returns one season of a show with its episodes. The show reference may
	// be an IMDb ID; the adapter resolves it to its own show ID first.
	SeasonDetails(ctx context.Context, request SeasonRequest) (SeasonDetails, error)
}

// MusicLookup finds albums and artists. Implemented by adapters/metadata/musicbrainz (M6-09),
// with the error contract of MovieLookup.
type MusicLookup interface {
	// Search returns candidates of the queried kind in the provider's order.
	Search(ctx context.Context, query MusicQuery) ([]match.Candidate, error)
	// Details returns the release or artist a MusicBrainz reference points to.
	Details(ctx context.Context, request DetailsRequest) (Details, error)
}

// ArtworkDownloader stores a provider picture in the data directory. Implemented by the
// artwork fetcher (M6-08).
type ArtworkDownloader interface {
	// Fetch downloads the picture once and returns where it is stored.
	Fetch(ctx context.Context, ref ArtworkRef) (Stored, error)
}

// OnlineSettings are the global switches (ADR 0031 §2) and the default metadata language.
// MusicBrainz covers the Cover Art Archive too.
type OnlineSettings struct {
	Enabled     bool
	TMDB        bool
	MusicBrainz bool
	Language    string
}

// SettingsSource reads the current switches; they can change at runtime in Settings.
type SettingsSource interface {
	// OnlineSettings returns the current switches.
	OnlineSettings(ctx context.Context) (OnlineSettings, error)
}

// LocalRecords are the records the local sources produced for an item in the last scan.
type LocalRecords struct {
	NFO      metadata.Record
	Filename metadata.Record
}

// ArtworkState is the item's stored artwork: every candidate and the selection per kind.
type ArtworkState struct {
	Candidates []artwork.Candidate
	Current    map[artwork.Kind]artwork.Candidate
}

// ItemSnapshot is what enrichment needs to know about an item.
type ItemSnapshot struct {
	Kind library.ItemKind
	// OnlineEnabled is the library's online_metadata switch.
	OnlineEnabled bool
	// Language is the library or item metadata language; empty means the global default.
	Language   string
	MatchState match.MatchState
	// Record is the stored merged record, Locks its locked fields (artwork kinds included).
	Record metadata.Record
	Locks  metadata.LockSet
	Local  LocalRecords
	// SeasonNumber and EpisodeNumber come from the placement (path) of a season or episode.
	SeasonNumber  int
	EpisodeNumber int
	// ParentProviderIDs are the provider IDs of the show a season or episode belongs to.
	ParentProviderIDs []metadata.ProviderID
	// ArtistName is the album artist of an album, used to search releases.
	ArtistName string
	// LinkedRef is the persisted MatchResult.Ref of the last match, zero when none. It is
	// only a fallback: a TMDB or IMDb ID in Record wins (a trusted NFO ID or an Identify
	// edit may have changed it), so in practice it serves MusicBrainz, which is not a
	// metadata.Provider.
	LinkedRef match.ProviderRef
	Artwork   ArtworkState
}

// MatchResult is the match state to store. Provider, Ref and Confidence are set for a match.
// Ref is the entry the item was linked by (for a season or episode: its show); the store
// persists it and returns it as ItemSnapshot.LinkedRef. Enrichment reads that only when the
// record holds no usable ID, so it is the only place a MusicBrainz ID lives.
type MatchResult struct {
	State      match.MatchState
	Provider   match.RefProvider
	Ref        match.ProviderRef
	Confidence float64
}

// Enrichment is the outcome of a successful match: the merged record, the match, the
// provider pictures that replace the item's earlier provider rows, and the selection per kind.
type Enrichment struct {
	Record          metadata.Record
	Match           MatchResult
	ProviderArtwork []artwork.Candidate
	Selection       map[artwork.Kind]artwork.Candidate
}

// MetadataStore reads and writes an item's metadata. Implemented on top of the catalog and
// the enrichment store of adapters/sqlite (M6-03). Errors wrap domain.ErrNotFound for an
// unknown item (EnrichItem adds ErrItemGone); the catalog writer refuses locked fields as an
// integrity guard.
type MetadataStore interface {
	// ItemSnapshot returns the item's current state.
	ItemSnapshot(ctx context.Context, itemID string) (ItemSnapshot, error)
	// SaveMatchState stores only the match state; record and artwork stay as they are.
	SaveMatchState(ctx context.Context, itemID string, result MatchResult) error
	// SaveEnrichment stores record, match, provider artwork and selection together.
	SaveEnrichment(ctx context.Context, itemID string, enrichment Enrichment) error
}

// CacheEntry is one cached provider answer: provider-neutral payload, or NotFound without
// one. An expired entry is still returned; it is the offline fallback (ADR 0031 §8).
type CacheEntry struct {
	Payload   []byte
	NotFound  bool
	FetchedAt time.Time
	ExpiresAt time.Time
}

// Expired reports whether the entry should be fetched again at now.
func (entry CacheEntry) Expired(now time.Time) bool {
	return !now.Before(entry.ExpiresAt)
}

// ProviderCache keeps provider answers (TTL 7 days, 30 days for "not found"). Used by the
// caching lookups of the enrichment queue (M6-11); implemented by adapters/sqlite.
type ProviderCache interface {
	// ReadProviderCache returns the entry, expired or not; nothing cached wraps
	// domain.ErrNotFound.
	ReadProviderCache(ctx context.Context, provider, key string) (CacheEntry, error)
	// WriteProviderCache stores or replaces the entry.
	WriteProviderCache(ctx context.Context, provider, key string, entry CacheEntry) error
}

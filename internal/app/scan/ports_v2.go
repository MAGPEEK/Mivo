package scan

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// PlacedFileState is a media file as the v2 catalog knows it: its file state, the canonical
// key of the placement it was stored with (library.Placement.Key) and the IDs of its item
// chain, top-level item first, leaf last.
type PlacedFileState struct {
	ID           string
	PlacementKey string
	ItemIDs      []string
	State        library.FileState
}

// PlacedFile is the result of probing one new or changed file together with its placement.
// File.ID is set (new, or the stored ID of a changed file); File.ItemID is left empty and set
// by the catalog from the chain it creates or finds. ProbedAt must not be zero.
type PlacedFile struct {
	// RootID is the library folder File.RelativePath is relative to.
	RootID    string
	Placement library.Placement
	File      media.File
	ProbedAt  time.Time
}

// PlacedFileCatalog persists the item hierarchy and the files in it. Implemented by
// adapters/sqlite (M3-05b).
//
// newID creates the IDs of items the catalog has to create; it is called once per new item.
// A chain is upserted level by level by (library, parent, kind, match key); existing items
// keep their ID and creation time. Errors of every catalog method must not contain absolute
// paths: the scan logs their text.
type PlacedFileCatalog interface {
	// PlacedFiles returns every media file of the library with its stored placement key.
	PlacedFiles(ctx context.Context, libraryID string) ([]PlacedFileState, error)
	// SavePlacedFile creates or replaces one file (and its streams), upserts its item chain and
	// returns the chain's item IDs, top-level item first. It changes nothing and returns an
	// error wrapping
	//   - domain.ErrInvalid for an invalid placement, a zero ProbedAt or an invalid file,
	//   - domain.ErrNotFound for an unknown library,
	//   - domain.ErrConflict when another file ID is stored at the same relative path,
	//   - domain.ErrInvalid when a stored file ID belongs to another library.
	//
	// Items the file leaves are removed bottom-up when they end up without files and children.
	SavePlacedFile(ctx context.Context, record PlacedFile, newID func() (string, error)) ([]string, error)
	// MoveFile gives an unchanged file a new placement without touching its description and
	// returns the new chain's item IDs. Emptied former ancestors are removed bottom-up. It
	// wraps domain.ErrNotFound for an unknown file or library and domain.ErrInvalid for an
	// invalid placement.
	MoveFile(ctx context.Context, request MoveRequest) ([]string, error)
	// RemovePlacedFiles deletes the files and then every item left without files and children,
	// bottom-up, atomically. It is idempotent: unknown IDs and IDs of another library are
	// ignored.
	RemovePlacedFiles(ctx context.Context, libraryID string, fileIDs []string) error
}

// MoveRequest asks the catalog to give a file a new placement; NewID creates the IDs of new
// chain items like newID of SavePlacedFile.
type MoveRequest struct {
	LibraryID string
	FileID    string
	Placement library.Placement
	NewID     func() (string, error)
}

// SidecarCatalog persists the NFO and image files the scan has seen.
type SidecarCatalog interface {
	// SidecarStates returns the stored NFO and image files of the library.
	SidecarStates(ctx context.Context, libraryID string) ([]library.FileState, error)
	// SaveSidecarStates upserts the sidecars by (RootID, path) and deletes the removed ones, which
	// only need RootID and RelativePath, in one transaction. It is idempotent. Errors wrap
	// domain.ErrNotFound for an unknown library and domain.ErrInvalid for a sidecar with an
	// invalid path or class.
	SaveSidecarStates(ctx context.Context, libraryID string, upserts, removals []library.FileState) error
}

// MetadataState is what the refresh needs to know about an item before it merges.
type MetadataState struct {
	// Record is the stored merged record and Locks its locked fields.
	Record metadata.Record
	Locks  metadata.LockSet
	// LockedArtwork lists the artwork kinds the user locked; CurrentArtwork is the stored
	// selection per kind.
	LockedArtwork  map[artwork.Kind]bool
	CurrentArtwork map[artwork.Kind]artwork.Candidate
}

// MetadataCatalog persists merged metadata and artwork. Implemented by adapters/sqlite
// (M3-05d). The writer recomputes the sort title of a saved record from its title when the
// record has none (BACKLOG, m3 wave 1), so stored and new titles sort alike.
type MetadataCatalog interface {
	// ItemMetadataState returns the stored metadata state; an unknown item wraps
	// domain.ErrNotFound.
	ItemMetadataState(ctx context.Context, itemID string) (MetadataState, error)
	// SaveMetadata replaces the merged record of the item. Errors wrap domain.ErrNotFound for
	// an unknown item and domain.ErrInvalid for an invalid record.
	SaveMetadata(ctx context.Context, itemID string, record metadata.Record) error
	// SaveArtwork replaces the candidates of the item and its selection per kind. Errors wrap
	// domain.ErrNotFound for an unknown item and domain.ErrInvalid for a candidate that does
	// not belong to the item's library.
	SaveArtwork(ctx context.Context, itemID string, candidates []artwork.Candidate,
		selection map[artwork.Kind]artwork.Candidate) error
}

// CatalogV2 is the complete catalog port of the v2 scan.
type CatalogV2 interface {
	PlacedFileCatalog
	SidecarCatalog
	MetadataCatalog
}

// SidecarReader reads a small sidecar file confined to the library root. Implemented by
// adapters/fs/libraryfs (ReadSidecar).
type SidecarReader interface {
	// ReadSidecar returns the content of at most maxBytes bytes. Errors wrap
	// domain.ErrForbidden, domain.ErrNotFound or domain.ErrUnavailable.
	ReadSidecar(ctx context.Context, root, relativePath string, maxBytes int64) ([]byte, error)
}

// NFOKind says which element an NFO file is expected to have.
type NFOKind string

// Known NFO kinds, matching the adapter's.
const (
	NFOKindMovie   NFOKind = "movie"
	NFOKindShow    NFOKind = "show"
	NFOKindSeason  NFOKind = "season"
	NFOKindEpisode NFOKind = "episode"
)

// NFOResult is what an NFO parser found: one record per described item plus warnings.
type NFOResult struct {
	Records  []metadata.Record
	Warnings []string
}

// NFOParser parses NFO content. Implemented by an adapter around metadata/nfo; errors wrap
// domain.ErrUnsupported for unusable content.
type NFOParser interface {
	Parse(data []byte, kind NFOKind) (NFOResult, error)
}

// RefreshRequest lists the items whose metadata and artwork must be recomputed after a scan:
// new items, items with new, changed, moved or removed files, and items below a folder whose
// sidecars changed. ItemIDs may name items that are gone; refreshing skips them. Sidecars
// is the full set of sidecar files of the scan. The refresher must pass the full ordered
// source list (nfo, filename) to metadata.Merge (BACKLOG, m3 wave 1).
//
// A scan that ends early (failure, cancel) still refreshes the items it touched, but the
// refresher may fail too; so the refresher should also refresh items that have no stored
// metadata yet (MetadataCatalog.ItemMetadataState) when it is asked about them.
type RefreshRequest struct {
	// Library carries the folders (sidecars are read from the folder their RootID names) and
	// the options: ReadNFO and LocalArtwork switch the NFO and local artwork sources off.
	Library  library.ManagedLibrary
	ItemIDs  []string
	Sidecars []library.FileState
	// Files lists every placed media file of the scan with its placement and item chain; the
	// refresher derives each item's kind, files and folders from it. After an early-ended
	// scan it holds the files placed before the end.
	Files []RefreshFile
	// CountMetadataWarnings, when set, receives the number of NFO problems met (unreadable,
	// malformed, parser warnings); they never fail the refresh.
	CountMetadataWarnings func(count int)
}

// RefreshFile is a placed media file as the refresher needs it. ItemIDs has one ID per level
// of Placement. EmbeddedPictures are the stream indexes of attached pictures; they are only
// known (PicturesKnown) for files probed by this scan.
type RefreshFile struct {
	State            library.FileState
	Placement        library.Placement
	ItemIDs          []string
	EmbeddedPictures []int
	PicturesKnown    bool
}

// ItemRefresher is the extension point M3-10b fills: it reads the NFO and image sidecars,
// merges the sources with metadata.Merge and saves the result with MetadataCatalog.
type ItemRefresher interface {
	// RefreshItems recomputes metadata and artwork of the requested items.
	RefreshItems(ctx context.Context, request RefreshRequest) error
}

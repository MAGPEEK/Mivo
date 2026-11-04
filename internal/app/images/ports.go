// Package images serves rendered artwork: it looks an artwork up, renders it in one of the
// allowed widths at most once per version and keeps the result in a disk cache (ADR 0027 §8).
package images

import (
	"context"
	"io"

	"github.com/magpeek/mivo/internal/domain/artwork"
)

// ArtworkSource reads stored artwork. Implemented by adapters/sqlite.
type ArtworkSource interface {
	// Artwork returns one artwork. An unknown ID wraps domain.ErrNotFound.
	Artwork(ctx context.Context, id string) (artwork.Stored, error)
	// ArtworkTags returns the current tag of every artwork, keyed by artwork ID.
	ArtworkTags(ctx context.Context) (map[string]string, error)
}

// PathResolver turns a root-relative path into an absolute path confined to the root, with
// the same contract as the scan's resolver. Implemented by adapters/fs/libraryfs.
type PathResolver interface {
	// Resolve returns the absolute path. A vanished file wraps domain.ErrNotFound, a path
	// leaving the root domain.ErrForbidden, an unusable root domain.ErrUnavailable.
	Resolve(root, relativePath string) (string, error)
}

// Renderer decodes, scales and encodes one picture. Implemented by adapters/ffmpeg.
type Renderer interface {
	// Render returns the encoded picture. Errors wrap domain.ErrUnsupported for a source
	// that cannot be decoded and domain.ErrUnavailable when the tool cannot run.
	Render(ctx context.Context, request artwork.RenderRequest) (artwork.Rendered, error)
}

// Key identifies one cached rendering.
type Key struct {
	ID     string
	Tag    string
	Width  artwork.Width
	Format artwork.Format
}

// Cached is an open cache entry. The caller closes Content.
type Cached struct {
	Content io.ReadSeekCloser
	Size    int64
}

// Cache keeps rendered pictures. Implemented by adapters/fs/imagecache.
type Cache interface {
	// Open returns the entry for key. A missing entry wraps domain.ErrNotFound.
	Open(ctx context.Context, key Key) (Cached, error)
	// Store saves data under key atomically, so Open never sees a partial file.
	Store(ctx context.Context, key Key, data []byte) error
	// Purge removes every entry whose (id, tag) keep rejects.
	Purge(ctx context.Context, keep func(id, tag string) bool) error
}

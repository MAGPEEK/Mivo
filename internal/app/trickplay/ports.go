// Package trickplay generates the seek-bar preview sheets of a video in the background and
// answers lookups for them (ADR 0030 section 8). One job runs at a time and only while no
// viewer's re-encode needs the CPU; failures are remembered for the process lifetime so a
// broken file never causes a retry loop.
//
// The package declares the ports it needs; adapters implement them (sqlite: FileSource,
// fs/libraryfs: PathResolver, ffmpeg: Generator, a cache-directory adapter: Store and Cache,
// app/playback: SessionLister) and cmd/mivo wires them (ADR 0003).
package trickplay

import (
	"context"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/access"
)

// Manifest describes the sprite sheets of one video, see ffmpeg.Manifest.
type Manifest struct {
	// TileWidth and TileHeight are the pixel size of one thumbnail.
	TileWidth, TileHeight int
	// Columns and Rows are the thumbnails per sheet.
	Columns, Rows int
	// Interval is the nominal time between two thumbnails.
	Interval time.Duration
	// SheetCount is the number of sheets.
	SheetCount int
}

// Stored is a manifest as persisted next to its sheets, with the key of the source file it
// was generated from.
type Stored struct {
	Manifest Manifest
	// Key is size and modification time of the source; a changed file has another key.
	Key string
}

// Entry is one cached file's trickplay directory, for eviction.
type Entry struct {
	FileID   string
	Bytes    int64
	LastUsed time.Time
}

// FileSource loads media files. Implemented by adapters/sqlite (same contract as
// playback.FileSource: a file the restriction hides is domain.ErrNotFound).
type FileSource interface {
	PlayableFile(ctx context.Context, fileID string, restriction access.Restriction) (appplayback.PlayableFile, error)
}

// PathResolver turns a library-relative path into an absolute path below the root.
type PathResolver interface {
	Resolve(root, relativePath string) (string, error)
}

// Generator writes sheet-001.jpg, sheet-002.jpg, … (numbered from 1) into destinationDir.
// Implemented by a thin adapter over ffmpeg.TrickplayGenerator at low priority.
type Generator interface {
	GenerateTrickplay(ctx context.Context, source, destinationDir string) (Manifest, error)
}

// Store keeps the manifests next to the sheets under <cache>/trickplay/<fileID>.
type Store interface {
	// Directory returns the absolute directory of a file ID. The ID is always one the catalog
	// returned, never request input.
	Directory(fileID string) string
	// Load returns the stored manifest or an error wrapping domain.ErrNotFound.
	Load(ctx context.Context, fileID string) (Stored, error)
	// Save writes the manifest after the sheets are in place.
	Save(ctx context.Context, fileID string, stored Stored) error
	// Touch marks the entry as used now, for least-recently-used eviction.
	Touch(ctx context.Context, fileID string) error
}

// Cache enumerates and cleans the trickplay cache directory.
type Cache interface {
	// Entries lists the cached directories.
	Entries(ctx context.Context) ([]Entry, error)
	// Remove deletes the directory of a file ID.
	Remove(ctx context.Context, fileID string) error
	// SweepStaging deletes .trickplay-* staging directories a hard kill left behind.
	SweepStaging(ctx context.Context) error
}

// SessionLister lists all playback sessions regardless of owner; the wiring gives it a
// context of an administrator, since the worker has no viewer.
type SessionLister interface {
	List(ctx context.Context) ([]appplayback.SessionInfo, error)
}

// Clock lets tests control time.
type Clock interface {
	Now() time.Time
	// After returns a channel that receives once d has elapsed.
	After(d time.Duration) <-chan time.Time
}

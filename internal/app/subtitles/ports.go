// Package subtitles serves WebVTT for the text subtitle tracks of a media file, embedded or
// external. A track is converted once per version of its source and kept in a size-capped
// disk cache; conversions run at most two at a time (ADR 0030 §3).
//
// The package declares the ports it needs; adapters implement them (sqlite: the file source
// and external subtitle lists, fs/libraryfs: the path resolver and stat, ffmpeg: the
// converter, an fs cache adapter) and cmd/mivo wires them (ADR 0003).
package subtitles

import (
	"context"
	"io"
	"time"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
)

// PlayableFile is a catalogued media file with the root of its library.
type PlayableFile struct {
	File media.File
	// LibraryRoot is the absolute root folder of the file's library.
	LibraryRoot string
}

// FileSource loads media files. Implemented by adapters/sqlite.
type FileSource interface {
	// PlayableFile returns the file with its streams and library root. An unknown file ID and
	// a file the restriction hides (itself, an ancestor or its library) wrap domain.ErrNotFound;
	// the zero restriction hides all.
	PlayableFile(ctx context.Context, fileID string, restriction access.Restriction) (PlayableFile, error)
	// FileKnown reports whether a file with the ID exists, for purging the cache.
	FileKnown(ctx context.Context, fileID string) (bool, error)
}

// ExternalSubtitles lists the subtitle files next to a media file. Implemented by
// adapters/sqlite.
type ExternalSubtitles interface {
	// ExternalSubtitles returns the stored subtitles of the file, RelativePath relative to the
	// library root; empty when there are none.
	ExternalSubtitles(ctx context.Context, fileID string) ([]media.ExternalSubtitle, error)
}

// PathResolver turns a library-relative path into an absolute path confined to the root.
// Implemented by adapters/fs/libraryfs.
type PathResolver interface {
	// Resolve returns the absolute path. A vanished file wraps domain.ErrNotFound, a path
	// leaving the root domain.ErrForbidden, an unusable root domain.ErrUnavailable.
	Resolve(root, relativePath string) (string, error)
}

// FileStat describes a file for the cache stamp.
type FileStat struct {
	Size       int64
	ModifiedAt time.Time
}

// Statter reads the size and modification time of a resolved file. Implemented by
// adapters/fs/libraryfs.
type Statter interface {
	// Stat returns the stat of the file at an absolute path; a missing file wraps
	// domain.ErrNotFound.
	Stat(ctx context.Context, path string) (FileStat, error)
}

// Track names the subtitle to convert; it mirrors what the FFmpeg extractor needs.
type Track struct {
	// External marks a subtitle file next to the video; the converter's source is that file.
	External bool
	// Ordinal counts the subtitle streams of the media file only, from 0.
	Ordinal int
	// Codec is the stream's codec name from the probe.
	Codec string
}

// Converter converts one text subtitle to UTF-8 WebVTT. Implemented by adapters/ffmpeg.
type Converter interface {
	// ConvertWebVTT reads source and writes the WebVTT to destination atomically. Both are
	// absolute; the service chose destination under the cache directory and confined source to
	// its library root.
	ConvertWebVTT(ctx context.Context, source string, track Track, destination string) error
}

// CacheFile is an open cache entry. The caller closes Content.
type CacheFile struct {
	Content io.ReadSeekCloser
	Size    int64
	ModTime time.Time
}

// CacheEntry is one file below the cache directory.
type CacheEntry struct {
	Path string
	// Directory is the base name of the entry's parent folder, the file ID it belongs to.
	Directory string
	// Name is the base name of the file.
	Name    string
	Size    int64
	ModTime time.Time
}

// Cache reads and trims the cache directory. Implemented by an adapter in adapters/fs.
type Cache interface {
	// Open returns the file at path; a missing file wraps domain.ErrNotFound.
	Open(ctx context.Context, path string) (CacheFile, error)
	// List returns every regular file below root, an absent root yielding none.
	List(ctx context.Context, root string) ([]CacheEntry, error)
	// Remove deletes the file at path; an absent file is no error.
	Remove(ctx context.Context, path string) error
}

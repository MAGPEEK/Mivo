package scan

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/media"
)

// ChapterReader reads the chapter list of a media file. Implemented by adapters/ffmpeg
// ((*Prober).ReadChapters, M3-14c).
type ChapterReader interface {
	// ReadChapters returns the chapters of the file at absolutePath, offsets relative to the
	// start of the file; a file without chapters returns an empty list and no error. Errors
	// wrap context.Canceled or context.DeadlineExceeded when ctx ends, domain.ErrUnavailable
	// when the tool cannot run and domain.ErrUnsupported for a file that cannot be read.
	ReadChapters(ctx context.Context, absolutePath string) ([]media.Chapter, error)
}

// AudiobookCatalog stores the chapters of the part files of books. Implemented by
// adapters/sqlite (M3-14d).
type AudiobookCatalog interface {
	// ReplaceFileChapters replaces the chapters of one media file atomically; an empty list
	// removes them. Offsets are relative to the start of that file, in play order; the reader
	// adds the durations of the preceding parts (by part number) for book offsets. Errors
	// wrap domain.ErrNotFound for an unknown file and domain.ErrInvalid for chapters that
	// fail media.ValidateChapters. Errors must not contain absolute paths.
	ReplaceFileChapters(ctx context.Context, fileID string, chapters []media.Chapter) error
}

// AudiobookPorts are the ports only the audiobook pipeline needs. Tags reuses the reader of
// the music pipeline; embedded covers need no port (attached pictures, ADR 0027 §8), and
// author and narrator reach the catalog as people of the book's embedded metadata record.
type AudiobookPorts struct {
	Tags     AudioTagReader
	Chapters ChapterReader
	Catalog  AudiobookCatalog
}

func (ports AudiobookPorts) complete() bool {
	return ports.Tags != nil && ports.Chapters != nil && ports.Catalog != nil
}

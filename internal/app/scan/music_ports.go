package scan

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
)

// AudioTagReader reads the tags of an audio file. Implemented by adapters/ffmpeg
// ((*Prober).ReadAudioTags).
type AudioTagReader interface {
	// ReadAudioTags returns the tags and the duration of the file at absolutePath. Errors wrap
	// context.Canceled or context.DeadlineExceeded when ctx ends, domain.ErrUnavailable when
	// the tool cannot run and domain.ErrUnsupported for a file whose tags cannot be read.
	ReadAudioTags(ctx context.Context, absolutePath string) (media.AudioTags, time.Duration, error)
}

// MusicCatalog stores what a track's placement cannot carry. Implemented by adapters/sqlite
// (LibraryCatalog).
type MusicCatalog interface {
	// SaveTrackDetails stores the disc number and the MusicBrainz IDs of a track saved by
	// PlacedFileCatalog.SavePlacedFile. Errors wrap domain.ErrNotFound for an unknown item and
	// domain.ErrInvalid for an item that is no track, a disc below 1 or a malformed ID.
	SaveTrackDetails(ctx context.Context, trackID string, disc int, tags media.AudioTags) error
}

// MusicPorts are the ports only the music pipeline needs. Embedded covers need no port of
// their own: the prober reports attached pictures, which become embedded artwork candidates
// that the image service extracts on demand (ADR 0027 §8).
type MusicPorts struct {
	Tags    AudioTagReader
	Catalog MusicCatalog
}

func (ports MusicPorts) complete() bool {
	return ports.Tags != nil && ports.Catalog != nil
}

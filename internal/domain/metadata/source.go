package metadata

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Source is where a Record came from. The documented merge order, highest priority first, is
// SourceUser (edit dialog and identify; locks always win), SourceNFO, the providers
// SourceTMDB and SourceMusicBrainz, then SourceFilename (the name parser). SourceEmbedded
// holds container tags (not used for videos, ADR 0027 §6; music in M3-13). Callers pass the
// records to Merge in that order; Merge does not sort them (ADR 0031 §8).
type Source string

// Known metadata sources. The text values are stored and used in the API.
const (
	SourceFilename    Source = "filename"
	SourceNFO         Source = "nfo"
	SourceEmbedded    Source = "embedded"
	SourceTMDB        Source = "tmdb"
	SourceMusicBrainz Source = "musicbrainz"
	SourceUser        Source = "user"
)

// ParseSource accepts the exact text of a known or reserved source. The error wraps
// domain.ErrInvalid.
func ParseSource(text string) (Source, error) {
	source := Source(text)
	if !source.IsValid() {
		return "", fmt.Errorf("parse metadata source %q: unknown source: %w", text, domain.ErrInvalid)
	}

	return source, nil
}

// IsValid reports whether the source is a known or reserved source.
func (source Source) IsValid() bool {
	switch source {
	case SourceFilename, SourceNFO, SourceEmbedded, SourceTMDB, SourceMusicBrainz, SourceUser:
		return true
	default:
		return false
	}
}

// String returns the text value of the source.
func (source Source) String() string {
	return string(source)
}

// SourcedRecord is the record one source produced for an item.
type SourcedRecord struct {
	Source Source
	Record Record
}

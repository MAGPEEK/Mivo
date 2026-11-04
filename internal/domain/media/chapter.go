package media

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Chapter is a titled time span of a playable work. It serves audiobooks (one chapter per
// part file or per embedded marker) and is designed for video chapters too (M4, PL-30).
// Start and End are offsets from the beginning of the whole work. FileRef optionally names
// the part file (a library-relative path) the chapter was taken from.
type Chapter struct {
	Title   string
	Start   time.Duration
	End     time.Duration
	FileRef string
}

// Duration returns the length of the chapter.
func (chapter Chapter) Duration() time.Duration {
	return chapter.End - chapter.Start
}

// Validate checks that the chapter starts at or after zero and ends after it starts. The
// error wraps domain.ErrInvalid.
func (chapter Chapter) Validate() error {
	if chapter.Start < 0 {
		return fmt.Errorf("validate chapter %q: negative start: %w", chapter.Title, domain.ErrInvalid)
	}

	if chapter.End <= chapter.Start {
		return fmt.Errorf("validate chapter %q: end must be after start: %w", chapter.Title, domain.ErrInvalid)
	}

	return nil
}

// ValidateChapters checks every chapter and that the list is ordered and non-overlapping.
// Gaps between chapters are allowed. The error wraps domain.ErrInvalid.
func ValidateChapters(chapters []Chapter) error {
	var previousEnd time.Duration

	for index, chapter := range chapters {
		if err := chapter.Validate(); err != nil {
			return fmt.Errorf("chapter %d: %w", index, err)
		}

		if chapter.Start < previousEnd {
			return fmt.Errorf("chapter %d %q: starts before the previous one ends: %w",
				index, chapter.Title, domain.ErrInvalid)
		}

		previousEnd = chapter.End
	}

	return nil
}

// Part is one audio file of a multi-file work with its title and measured length.
type Part struct {
	Title    string
	FileRef  string
	Duration time.Duration
}

// ChaptersFromParts builds one chapter per part in the given order with cumulative offsets.
// A part without a positive duration cannot be placed on the timeline, so the call fails
// with an error wrapping domain.ErrInvalid.
func ChaptersFromParts(parts []Part) ([]Chapter, error) {
	chapters := make([]Chapter, 0, len(parts))

	var offset time.Duration

	for index, part := range parts {
		if part.Duration <= 0 {
			return nil, fmt.Errorf("chapters from part %d %q: duration must be positive: %w",
				index, part.FileRef, domain.ErrInvalid)
		}

		chapters = append(chapters, Chapter{
			Title: part.Title, Start: offset, End: offset + part.Duration, FileRef: part.FileRef,
		})
		offset += part.Duration
	}

	return chapters, nil
}

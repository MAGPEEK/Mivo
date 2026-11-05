package photo

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// TakenAtSource says where the capture time of a photo comes from (API takenAtSource).
type TakenAtSource string

// The capture time sources.
const (
	// TakenAtExif means the EXIF block of the file carries the capture time.
	TakenAtExif TakenAtSource = "exif"
	// TakenAtFile means the file has no EXIF time and its modification time stands in.
	TakenAtFile TakenAtSource = "file"
)

// String returns the stored and transferred text of the source.
func (source TakenAtSource) String() string {
	return string(source)
}

// SourceOf returns the source CaptureTime uses for info: EXIF when it has a capture time.
func SourceOf(info Info) TakenAtSource {
	if info.TakenAt.IsZero() {
		return TakenAtFile
	}

	return TakenAtExif
}

// ParseTakenAtSource returns the source named by text; anything else wraps domain.ErrInvalid.
func ParseTakenAtSource(text string) (TakenAtSource, error) {
	switch source := TakenAtSource(text); source {
	case TakenAtExif, TakenAtFile:
		return source, nil
	default:
		return "", fmt.Errorf("parse capture time source %q: %w", text, domain.ErrInvalid)
	}
}

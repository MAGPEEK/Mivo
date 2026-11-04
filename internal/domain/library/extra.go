package library

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// ExtraType says what kind of bonus material an extra is (ADR 0027 §4, UX DT-09). The text
// values are stored in the database and used in the API.
type ExtraType string

// Known extra types.
const (
	ExtraTypeTrailer         ExtraType = "trailer"
	ExtraTypeFeaturette      ExtraType = "featurette"
	ExtraTypeBehindTheScenes ExtraType = "behind_the_scenes"
	ExtraTypeDeletedScene    ExtraType = "deleted_scene"
	ExtraTypeInterview       ExtraType = "interview"
	ExtraTypeScene           ExtraType = "scene"
	ExtraTypeShort           ExtraType = "short"
	ExtraTypeOther           ExtraType = "other"
)

// ParseExtraType accepts the exact text of a known extra type. The error wraps
// domain.ErrInvalid.
func ParseExtraType(text string) (ExtraType, error) {
	extraType := ExtraType(text)
	if !extraType.IsValid() {
		return "", fmt.Errorf("parse extra type %q: unknown type: %w", text, domain.ErrInvalid)
	}

	return extraType, nil
}

// IsValid reports whether the extra type is one of the known types.
func (extraType ExtraType) IsValid() bool {
	switch extraType {
	case ExtraTypeTrailer, ExtraTypeFeaturette, ExtraTypeBehindTheScenes, ExtraTypeDeletedScene,
		ExtraTypeInterview, ExtraTypeScene, ExtraTypeShort, ExtraTypeOther:
		return true
	default:
		return false
	}
}

// String returns the text value of the extra type.
func (extraType ExtraType) String() string {
	return string(extraType)
}

package access

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Preset names a content limit choice of US-11; the values are the API strings.
type Preset string

// The content limit presets of ADR 0032 §7. Every preset with a maximum age hides unrated
// titles, the default when a limit is set.
const (
	PresetYoungChildren Preset = "youngChildren"
	PresetChildren      Preset = "children"
	PresetTeens         Preset = "teens"
	PresetAdults        Preset = "adults"
	PresetCustom        Preset = "custom"
)

// LimitForPreset returns the content limit of a preset. customMaxAge is only read for
// PresetCustom, which hides unrated titles like every limited preset; callers that want them
// shown use ContentLimit.HidingUnrated. An unknown preset or an invalid custom age reports
// domain.ErrInvalid.
func LimitForPreset(preset Preset, customMaxAge AgeBucket) (ContentLimit, error) {
	switch preset {
	case PresetYoungChildren:
		return MaxAgeLimit(AgeAll, true)
	case PresetChildren:
		return MaxAgeLimit(Age6, true)
	case PresetTeens:
		return MaxAgeLimit(Age12, true)
	case PresetAdults:
		return NoLimit(), nil
	case PresetCustom:
		return MaxAgeLimit(customMaxAge, true)
	default:
		return ContentLimit{}, fmt.Errorf("content limit preset %q: %w", preset, domain.ErrInvalid)
	}
}

// Preset returns the fixed preset l equals, or PresetCustom when it matches none (for example
// a 16 limit or a limit that shows unrated titles).
func (l ContentLimit) Preset() Preset {
	for _, preset := range []Preset{PresetYoungChildren, PresetChildren, PresetTeens, PresetAdults} {
		if limit, err := LimitForPreset(preset, AgeAll); err == nil && limit == l {
			return preset
		}
	}

	return PresetCustom
}

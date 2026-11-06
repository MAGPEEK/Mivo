package access

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// ContentLimit is the content limit of a profile (ADR 0032 §7): an optional maximum age bucket
// and whether titles without a known rating are hidden. The zero value limits nothing, like
// the "adults" preset.
type ContentLimit struct {
	maxAge      AgeBucket
	hasMaxAge   bool
	hideUnrated bool
}

// NoLimit returns the limit that allows every title, rated or not.
func NoLimit() ContentLimit {
	return ContentLimit{}
}

// MaxAgeLimit returns a limit that allows titles up to maxAge. An invalid bucket reports
// domain.ErrInvalid.
func MaxAgeLimit(maxAge AgeBucket, hideUnrated bool) (ContentLimit, error) {
	if !maxAge.Valid() {
		return ContentLimit{}, fmt.Errorf("content limit max age %d: %w", maxAge, domain.ErrInvalid)
	}

	return ContentLimit{maxAge: maxAge, hasMaxAge: true, hideUnrated: hideUnrated}, nil
}

// HidingUnrated returns l with titles of unknown rating hidden or shown.
func (l ContentLimit) HidingUnrated(hide bool) ContentLimit {
	l.hideUnrated = hide

	return l
}

// MaxAge returns the highest allowed age bucket; false means no age limit.
func (l ContentLimit) MaxAge() (AgeBucket, bool) {
	return l.maxAge, l.hasMaxAge
}

// HideUnrated reports whether titles without a known rating are hidden.
func (l ContentLimit) HideUnrated() bool {
	return l.hideUnrated
}

// Allows reports whether a title with the given bucket may be shown; known is false when the
// title has no recognised rating, which only the hide-unrated rule decides.
func (l ContentLimit) Allows(bucket AgeBucket, known bool) bool {
	if !known {
		return !l.hideUnrated
	}

	return !l.hasMaxAge || bucket <= l.maxAge
}

// Stricter combines two limits so that a title passes only if both allow it: the lower
// maximum age wins and unrated titles are hidden if either limit hides them. A managed
// profile's effective limit is Stricter(own, primary profile's).
func Stricter(first, second ContentLimit) ContentLimit {
	combined := ContentLimit{hideUnrated: first.hideUnrated || second.hideUnrated}

	switch {
	case first.hasMaxAge && second.hasMaxAge:
		combined.maxAge, combined.hasMaxAge = min(first.maxAge, second.maxAge), true
	case first.hasMaxAge:
		combined.maxAge, combined.hasMaxAge = first.maxAge, true
	case second.hasMaxAge:
		combined.maxAge, combined.hasMaxAge = second.maxAge, true
	}

	return combined
}

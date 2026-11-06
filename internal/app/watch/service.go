// Package watch contains the watch state use cases of ADR 0032 §8 (task M7-18): recording
// the signed-in profile's playback progress, manual watched and unwatched marks, hiding an
// item from Continue watching and decorating item lists with the profile's state. The rules
// live in domain/watch; this package resolves the viewer, checks the item and moves the data.
// It declares the ports it needs; adapters implement them and cmd/mivo wires them (ADR 0003).
package watch

import (
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// ErrNoWatchState reports an item whose kind carries no watch state, for example a photo, a
// music track or an extra (music listen state is deferred, ADR 0032 §8).
var ErrNoWatchState = fmt.Errorf("item kind has no watch state: %w", domain.ErrInvalid)

// Dependencies are the ports of the Service; all are required.
type Dependencies struct {
	Items    ItemLookup
	Episodes EpisodeLister
	Store    StateStore
	Clock    Clock
}

// Service implements the watch state use cases. It is safe for concurrent use when its ports
// are.
type Service struct {
	deps Dependencies
}

// NewService returns a Service or an error wrapping domain.ErrInvalid when a dependency is
// missing.
func NewService(deps Dependencies) (*Service, error) {
	checks := []struct {
		name    string
		missing bool
	}{
		{"Items", deps.Items == nil},
		{"Episodes", deps.Episodes == nil},
		{"Store", deps.Store == nil},
		{"Clock", deps.Clock == nil},
	}

	var errs []error

	for _, check := range checks {
		if check.missing {
			errs = append(errs, fmt.Errorf("watch service: %s is required: %w", check.name, domain.ErrInvalid))
		}
	}

	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &Service{deps: deps}, nil
}

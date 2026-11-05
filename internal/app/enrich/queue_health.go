package enrich

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// ProviderName names an online provider in the status and in the provider cache.
type ProviderName string

// The providers of ADR 0031 §1, in the order the status lists them.
const (
	ProviderTMDB        ProviderName = "tmdb"
	ProviderMusicBrainz ProviderName = "musicbrainz"
	ProviderCoverArt    ProviderName = "coverartarchive"
)

// knownProviders returns the providers in the order of Status.Providers.
func knownProviders() []ProviderName {
	return []ProviderName{ProviderTMDB, ProviderMusicBrainz, ProviderCoverArt}
}

// ProviderState is the health of a provider as the API reports it.
type ProviderState string

// Provider states (API schema ProviderState).
const (
	ProviderStateOK          ProviderState = "ok"
	ProviderStateRateLimited ProviderState = "rate_limited"
	ProviderStateOffline     ProviderState = "offline"
	ProviderStateDisabled    ProviderState = "disabled"
)

// HealthOptions configure a Health. IsInvalidKey recognises the error of a refused API key
// (the TMDB adapter's ErrInvalidKey); the app cannot import adapters, so the wiring passes
// the check in. Nil means no error counts as an invalid key.
type HealthOptions struct {
	IsInvalidKey func(error) bool
}

// Health tracks the state of every provider from the outcome of its calls, and the time of
// the last successful enrichment. It is safe for concurrent use.
type Health struct {
	isInvalidKey func(error) bool

	mutex       sync.Mutex
	states      map[ProviderName]ProviderState
	lastSuccess time.Time
}

// NewHealth returns a Health in which every provider is ok.
func NewHealth(options HealthOptions) *Health {
	isInvalidKey := options.IsInvalidKey
	if isInvalidKey == nil {
		isInvalidKey = func(error) bool { return false }
	}

	return &Health{isInvalidKey: isInvalidKey, states: map[ProviderName]ProviderState{}}
}

// Observe records the outcome of one call to provider. A refused key disables the provider
// until SettingsChanged; a cancelled call says nothing about the provider and is ignored.
func (health *Health) Observe(provider ProviderName, err error) {
	state, known := health.stateAfter(err)
	if !known {
		return
	}

	health.mutex.Lock()
	defer health.mutex.Unlock()

	if health.states[provider] == ProviderStateDisabled && state != ProviderStateDisabled {
		return // only a settings change lifts a refused key
	}

	health.states[provider] = state
}

// stateAfter maps a call outcome to a state; known is false when it tells nothing.
func (health *Health) stateAfter(err error) (state ProviderState, known bool) {
	switch {
	case err == nil, errors.Is(err, domain.ErrNotFound):
		return ProviderStateOK, true // the provider answered
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "", false
	case health.isInvalidKey(err):
		return ProviderStateDisabled, true
	case errors.Is(err, domain.ErrRateLimited):
		return ProviderStateRateLimited, true
	case errors.Is(err, domain.ErrUnavailable):
		return ProviderStateOffline, true
	default:
		return "", false
	}
}

// Usable reports whether provider may be called; a disabled provider is not, so a refused
// key causes no retry storm.
func (health *Health) Usable(provider ProviderName) bool {
	return health.State(provider) != ProviderStateDisabled
}

// State returns the current state of provider; a provider never called is ok.
func (health *Health) State(provider ProviderName) ProviderState {
	health.mutex.Lock()
	defer health.mutex.Unlock()

	if state, ok := health.states[provider]; ok {
		return state
	}

	return ProviderStateOK
}

// SettingsChanged forgets every state, so a provider disabled by a refused key is tried
// again with the new settings. The settings use case calls it after saving.
func (health *Health) SettingsChanged() {
	health.mutex.Lock()
	defer health.mutex.Unlock()

	clear(health.states)
}

// recordSuccess remembers when an item was last enriched without error.
func (health *Health) recordSuccess(at time.Time) {
	health.mutex.Lock()
	defer health.mutex.Unlock()

	health.lastSuccess = at
}

// LastSuccess returns when an item was last enriched without error, zero when never.
func (health *Health) LastSuccess() time.Time {
	health.mutex.Lock()
	defer health.mutex.Unlock()

	return health.lastSuccess
}

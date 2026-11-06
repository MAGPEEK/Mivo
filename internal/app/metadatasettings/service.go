// Package metadatasettings holds the online metadata settings that can change at runtime in
// Settings › Metadata (ADR 0031 §2, §3): the global switch, the provider switches, the
// metadata language and the user's own TMDB key. The service is the one source the
// enrichment queue, the identify use cases and the TMDB adapter read, so a change counts
// from the next job on. The own key is write-only: no read path returns it except APIKey,
// which only the TMDB adapter calls.
package metadatasettings

import (
	"context"
	"fmt"
	"sync"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// MaxKeyLength is the longest own TMDB key accepted (api/openapi.yaml, tmdbApiKey).
const MaxKeyLength = 128

// Printable ASCII bounds a key must stay in: keys travel in a query parameter or header.
const (
	firstPrintable = '!'
	lastPrintable  = '~'
)

// KeyStatus names which TMDB key applies, never the key itself.
type KeyStatus string

// Key statuses (api/openapi.yaml, ProviderKeyStatus).
const (
	KeyNone    KeyStatus = "none"
	KeyBundled KeyStatus = "bundled"
	KeyOwn     KeyStatus = "own"
)

// State is the complete settings state the Store persists. Language is empty for the
// server default; OwnTMDBKey is empty when the user set none.
type State struct {
	Online      bool
	TMDB        bool
	MusicBrainz bool
	Language    string
	OwnTMDBKey  string
}

// Settings are the settings as clients see them: the key only as its status, the language
// with the default filled in.
type Settings struct {
	Online      bool
	TMDB        bool
	MusicBrainz bool
	Language    string
	TMDBKey     KeyStatus
}

// Change lists the settings to change; nil fields stay. An empty Language resets it to the
// server default, an empty TMDBKey removes the own key so the bundled one applies again.
type Change struct {
	Online      *bool
	TMDB        *bool
	MusicBrainz *bool
	Language    *string
	TMDBKey     *string
}

// Store persists the state, so a restart keeps the user's changes. The adapter must keep
// the key out of logs.
type Store interface {
	// SaveMetadataSettings replaces the stored state.
	SaveMetadataSettings(ctx context.Context, state State) error
}

// ChangeListener hears about every effective change; *enrich.Health implements it, so a
// provider disabled by a refused key is tried again after the key changed.
type ChangeListener interface {
	// SettingsChanged is called after a change was stored.
	SettingsChanged()
}

// ForcedOff lists the switches the configuration (file or environment) turns off. They stay
// off whatever the user's settings say, so an operator can always switch online lookups off
// (ADR 0018, notes M6-14); the zero value forces nothing.
type ForcedOff struct {
	Online      bool
	TMDB        bool
	MusicBrainz bool
}

// Options configure the Service. Initial is the user's state at start (StartState);
// ForcedOff is applied on top of it but never stored; DefaultLanguage is the language an
// empty Language stands for; BundledTMDBKey is the key shipped with the build, empty in
// development builds. Store and Listener are optional.
type Options struct {
	Initial         State
	ForcedOff       ForcedOff
	DefaultLanguage string
	BundledTMDBKey  string
	Store           Store
	Listener        ChangeListener
}

// StartState is the user's state a server starts with: the stored one when the settings
// were changed before (found), else every switch on with the configured own key. The
// configured switches apply as ForcedOff, so a configured "off" is never stored as the
// user's choice.
func StartState(configuredKey string, stored State, found bool) State {
	if found {
		return stored
	}

	return State{Online: true, TMDB: true, MusicBrainz: true, OwnTMDBKey: configuredKey}
}

// Service holds the current settings. It is safe for concurrent use.
type Service struct {
	mutex sync.RWMutex
	// state is the user's state, which Update changes and the Store persists; the forced
	// switches are applied on every read (effective).
	state   State
	options Options
}

// Compile-time check: the enrichment queue and the identify use cases read the switches here.
var _ enrich.SettingsSource = (*Service)(nil)

// NewService returns a Service starting with options.Initial.
func NewService(options Options) *Service {
	return &Service{state: options.Initial, options: options}
}

// Settings returns the current settings without any key.
func (service *Service) Settings(_ context.Context) (Settings, error) {
	return service.view(service.current()), nil
}

// OnlineSettings returns the switches and the effective language; it implements
// enrich.SettingsSource.
func (service *Service) OnlineSettings(_ context.Context) (enrich.OnlineSettings, error) {
	state := service.current()

	return enrich.OnlineSettings{
		Enabled: state.Online, TMDB: state.TMDB, MusicBrainz: state.MusicBrainz,
		Language: service.language(state),
	}, nil
}

// APIKey returns the TMDB key to use: the own key, else the bundled one, else "". It is
// meant for the TMDB adapter only (tmdb.KeyProvider); never log it.
func (service *Service) APIKey(_ context.Context) (string, error) {
	return service.key(service.current()), nil
}

// Update applies the change to the user's state, stores it and tells the listener. A
// malformed language or key wraps domain.ErrInvalid (FieldError names the field); a failed
// store leaves the settings unchanged. A switch forced off by the configuration is stored as
// the user chose it but stays off.
func (service *Service) Update(ctx context.Context, change Change) (Settings, error) {
	next, changed, err := service.apply(ctx, change)
	if err != nil {
		return Settings{}, fmt.Errorf("update metadata settings: %w", err)
	}

	// Outside the lock: the listener may read the settings again (the queue does).
	if changed && service.options.Listener != nil {
		service.options.Listener.SettingsChanged()
	}

	return service.view(service.effective(next)), nil
}

// apply changes and stores the user's state under the lock and reports whether it changed.
func (service *Service) apply(ctx context.Context, change Change) (State, bool, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	next, err := applyChange(service.state, change)
	if err != nil {
		return State{}, false, err
	}

	if next == service.state {
		return next, false, nil
	}

	if service.options.Store != nil {
		if err := service.options.Store.SaveMetadataSettings(ctx, next); err != nil {
			return State{}, false, err
		}
	}

	service.state = next

	return next, true, nil
}

// current returns the effective state: the user's with the forced switches off.
func (service *Service) current() State {
	service.mutex.RLock()
	defer service.mutex.RUnlock()

	return service.effective(service.state)
}

func (service *Service) effective(state State) State {
	forced := service.options.ForcedOff
	state.Online = state.Online && !forced.Online
	state.TMDB = state.TMDB && !forced.TMDB
	state.MusicBrainz = state.MusicBrainz && !forced.MusicBrainz

	return state
}

func (service *Service) view(state State) Settings {
	return Settings{
		Online: state.Online, TMDB: state.TMDB, MusicBrainz: state.MusicBrainz,
		Language: service.language(state), TMDBKey: service.keyStatus(state),
	}
}

func (service *Service) language(state State) string {
	if state.Language == "" {
		return service.options.DefaultLanguage
	}

	return state.Language
}

func (service *Service) key(state State) string {
	if state.OwnTMDBKey != "" {
		return state.OwnTMDBKey
	}

	return service.options.BundledTMDBKey
}

func (service *Service) keyStatus(state State) KeyStatus {
	switch {
	case state.OwnTMDBKey != "":
		return KeyOwn
	case service.options.BundledTMDBKey != "":
		return KeyBundled
	default:
		return KeyNone
	}
}

// Field names a setting in a FieldError.
type Field string

// Fields Update can reject.
const (
	FieldLanguage Field = "language"
	FieldTMDBKey  Field = "tmdbApiKey"
)

// FieldError reports a malformed setting. It wraps domain.ErrInvalid; the reason never
// quotes a key.
type FieldError struct {
	Field  Field
	Reason string
}

// Error implements error.
func (err *FieldError) Error() string { return fmt.Sprintf("setting %s: %s", err.Field, err.Reason) }

// Unwrap lets errors.Is match domain.ErrInvalid.
func (err *FieldError) Unwrap() error { return domain.ErrInvalid }

// applyChange returns state with the change applied, or a FieldError.
func applyChange(state State, change Change) (State, error) {
	if change.Language != nil {
		language, err := library.ParseMetadataLanguage(*change.Language)
		if err != nil {
			return State{}, &FieldError{Field: FieldLanguage, Reason: "not a language tag"}
		}

		state.Language = language
	}

	if change.TMDBKey != nil {
		if !validKey(*change.TMDBKey) {
			return State{}, &FieldError{Field: FieldTMDBKey, Reason: "too long or not printable ASCII"}
		}

		state.OwnTMDBKey = *change.TMDBKey
	}

	setIfPresent(&state.Online, change.Online)
	setIfPresent(&state.TMDB, change.TMDB)
	setIfPresent(&state.MusicBrainz, change.MusicBrainz)

	return state, nil
}

func setIfPresent(target, value *bool) {
	if value != nil {
		*target = *value
	}
}

// validKey accepts the empty key (removal) and printable ASCII up to MaxKeyLength.
func validKey(key string) bool {
	if len(key) > MaxKeyLength {
		return false
	}

	for index := range len(key) {
		if key[index] < firstPrintable || key[index] > lastPrintable {
			return false
		}
	}

	return true
}

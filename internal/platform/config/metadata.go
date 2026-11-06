package config

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
)

// BundledTMDBKey is the project's TMDB API key. It is empty in development builds and set
// at release time with
//
//	-ldflags "-X github.com/magpeek/mivo/internal/platform/config.BundledTMDBKey=<key>"
//
// It has to be a package variable because the linker can only set variables (ADR 0031 §3).
// Nothing but TMDBKey and Config.BundledTMDBKey read it.
var BundledTMDBKey string //nolint:gochecknoglobals // set by the linker (-X); only TMDBKey reads it

// Defaults of the online metadata settings (ADR 0031 §2). Online lookup is on so that the
// first scan yields posters; one switch turns all of it off.
const (
	DefaultMetadataOnline      = true
	DefaultMetadataTMDB        = true
	DefaultMetadataMusicBrainz = true
	// DefaultMetadataLanguage is the language of titles and overviews requested from providers.
	DefaultMetadataLanguage = "en"
)

const (
	keyMetadataOnline      = "metadata.online.enabled"
	keyMetadataTMDB        = "metadata.tmdb.enabled"
	keyMetadataTMDBKey     = "metadata.tmdb.api_key"
	keyMetadataMusicBrainz = "metadata.musicbrainz.enabled"
	keyMetadataLanguage    = "metadata.language"
)

// redactedSecret replaces a set secret in every textual form.
const redactedSecret = "[redacted]"

// languagePattern accepts a two or three letter language with an optional two letter
// region, e.g. en, de, pt-BR.
var languagePattern = regexp.MustCompile(`^([A-Za-z]{2,3})(?:-([A-Za-z]{2}))?$`)

// KeyOrigin tells where the TMDB key in use comes from.
type KeyOrigin string

// Origins of the TMDB key. The API reports these words, never the key itself.
const (
	KeyOriginNone    KeyOrigin = "none"
	KeyOriginBundled KeyOrigin = "bundled"
	KeyOriginOwn     KeyOrigin = "own"
)

// Metadata holds the online metadata settings (ADR 0031).
type Metadata struct {
	// Online is the global switch; off means no outbound connection at all.
	Online      bool
	TMDB        TMDB
	MusicBrainz MusicBrainz
	// Language is a language tag such as en or pt-BR.
	Language string
}

// TMDB holds the TMDB provider settings.
type TMDB struct {
	Enabled bool
	// APIKey is the owner's own key; empty when none is set. It wins over BundledTMDBKey.
	APIKey Secret
}

// MusicBrainz holds the MusicBrainz and Cover Art Archive settings.
type MusicBrainz struct {
	Enabled bool
}

// Secret is a value that must never reach a log or a dump. Every formatting path prints
// a marker instead; read the value with a plain string conversion where it is needed.
type Secret string

// String returns "" for an unset and a marker for a set secret, so a dump still shows
// whether a key is configured.
func (secret Secret) String() string {
	if secret == "" {
		return ""
	}

	return redactedSecret
}

// GoString redacts the secret in %#v output.
func (secret Secret) GoString() string { return fmt.Sprintf("%q", secret.String()) }

// MarshalText redacts the secret in encoding/json, YAML and other text encoders.
func (secret Secret) MarshalText() ([]byte, error) { return []byte(secret.String()), nil }

// MarshalJSON redacts the secret in JSON.
func (secret Secret) MarshalJSON() ([]byte, error) {
	encoded, err := json.Marshal(secret.String())
	if err != nil {
		return nil, fmt.Errorf("encode redacted secret: %w", err)
	}

	return encoded, nil
}

// LogValue redacts the secret in structured logs.
func (secret Secret) LogValue() slog.Value { return slog.StringValue(secret.String()) }

// TMDBKey returns the key to use against TMDB and where it came from. An own key wins over
// the bundled project key. The key is meant for the TMDB adapter only; do not log it.
func (cfg Config) TMDBKey() (string, KeyOrigin) {
	return selectTMDBKey(string(cfg.Metadata.TMDB.APIKey), BundledTMDBKey)
}

// BundledTMDBKey returns the project key shipped with the build, "" in development builds.
// The settings service needs it apart from the own key, which can change at runtime in
// Settings › Metadata; do not log it.
func (Config) BundledTMDBKey() string {
	return BundledTMDBKey
}

func selectTMDBKey(own, bundled string) (string, KeyOrigin) {
	switch {
	case own != "":
		return own, KeyOriginOwn
	case bundled != "":
		return bundled, KeyOriginBundled
	default:
		return "", KeyOriginNone
	}
}

func defaultMetadata() Metadata {
	return Metadata{
		Online:      DefaultMetadataOnline,
		TMDB:        TMDB{Enabled: DefaultMetadataTMDB},
		MusicBrainz: MusicBrainz{Enabled: DefaultMetadataMusicBrainz},
		Language:    DefaultMetadataLanguage,
	}
}

// metadataSettings lists the keys of the metadata section.
func metadataSettings() []setting {
	return []setting{
		{key: keyMetadataOnline, apply: applyBool(func(cfg *Config) *bool { return &cfg.Metadata.Online })},
		{key: keyMetadataTMDB, apply: applyBool(func(cfg *Config) *bool { return &cfg.Metadata.TMDB.Enabled })},
		{key: keyMetadataTMDBKey, apply: applyTMDBKey},
		{
			key:   keyMetadataMusicBrainz,
			apply: applyBool(func(cfg *Config) *bool { return &cfg.Metadata.MusicBrainz.Enabled }),
		},
		{key: keyMetadataLanguage, apply: applyLanguage},
	}
}

// applyBool accepts only true and false (parseStrictBool).
func applyBool(field func(cfg *Config) *bool) applyFunc {
	return func(cfg *Config, raw, _ string) error {
		value, err := parseStrictBool(raw)
		if err != nil {
			return err
		}

		*field(cfg) = value

		return nil
	}
}

// applyTMDBKey stores the trimmed key. An empty value clears it. The error never contains
// the value, because it may be a key with a typo.
func applyTMDBKey(cfg *Config, raw, _ string) error {
	key := strings.TrimSpace(raw)
	if strings.ContainsFunc(key, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return fmt.Errorf("%w: the TMDB API key must not contain spaces or control characters", ErrInvalid)
	}

	cfg.Metadata.TMDB.APIKey = Secret(key)

	return nil
}

// applyLanguage stores the language in canonical case: "pt-br" becomes "pt-BR".
func applyLanguage(cfg *Config, raw, _ string) error {
	match := languagePattern.FindStringSubmatch(strings.TrimSpace(raw))
	if match == nil {
		return fmt.Errorf("%w: language %q is not a tag such as en, de or pt-BR", ErrInvalid, raw)
	}

	language := strings.ToLower(match[1])
	if match[2] != "" {
		language += "-" + strings.ToUpper(match[2])
	}

	cfg.Metadata.Language = language

	return nil
}

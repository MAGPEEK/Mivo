package library

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
)

// Kind is the kind of content a library holds. Milestone 2 lists every library flat
// (OD-2); milestone 3 uses the kind to choose the name parser and the item hierarchy.
type Kind string

// Known library kinds. The text values are part of the configuration and the API contract.
const (
	KindMovies Kind = "movies"
	KindShows  Kind = "shows"
)

// MaxNameLength is the longest library name in runes. It keeps names readable in the UI
// sidebar and bounds what a configuration file can push into the database.
const MaxNameLength = 128

// ParseKind accepts the exact lowercase text of a known kind. Defaults (an omitted kind
// means movies) are applied by the configuration, not here. The error wraps
// domain.ErrInvalid.
func ParseKind(text string) (Kind, error) {
	kind := Kind(text)
	if !kind.IsValid() {
		return "", fmt.Errorf("parse library kind %q: unknown kind: %w", text, domain.ErrInvalid)
	}

	return kind, nil
}

// IsValid reports whether the kind is one of the known kinds.
func (kind Kind) IsValid() bool {
	switch kind {
	case KindMovies, KindShows, KindMusic, KindPhotos:
		return true
	case KindAudiobooks:
		return true
	default:
		return false
	}
}

// String returns the text value of the kind.
func (kind Kind) String() string {
	return string(kind)
}

// TopLevelItemKinds returns the item kinds a library of this kind can list (API type
// parameter), the default first: movie in a movies library, show in a shows library, artist
// or album in a music library, audiobook in an audiobooks library, photo_album or photo in a
// photos library ("photo" lists every photo of the library: the timeline, ADR 0029).
func (kind Kind) TopLevelItemKinds() []ItemKind {
	switch kind {
	case KindMovies:
		return []ItemKind{ItemKindMovie}
	case KindShows:
		return []ItemKind{ItemKindShow}
	case KindMusic:
		return []ItemKind{ItemKindArtist, ItemKindAlbum}
	case KindAudiobooks:
		return []ItemKind{ItemKindAudiobook}
	case KindPhotos:
		return []ItemKind{ItemKindPhotoAlbum, ItemKindPhoto}
	default:
		return nil
	}
}

// IsTopLevelItemKind reports whether some library kind lists items of this kind at its top.
func IsTopLevelItemKind(itemKind ItemKind) bool {
	for _, kind := range []Kind{KindMovies, KindShows, KindMusic, KindAudiobooks, KindPhotos} {
		if slices.Contains(kind.TopLevelItemKinds(), itemKind) {
			return true
		}
	}

	return false
}

// Definition is a library as the operator configures it. Its identity is the root path:
// syncing definitions into the catalog creates, renames or removes libraries by root.
type Definition struct {
	Name string
	Root string
	Kind Kind
}

// Validate checks the rules that do not depend on the operating system: a non-blank UTF-8
// name of at most MaxNameLength runes, a non-blank root without NUL bytes and a known kind.
// Whether the root is absolute, unique and outside the server's own directories is checked
// by the configuration, which knows the host OS. The error wraps domain.ErrInvalid.
func (definition Definition) Validate() error {
	if err := validateName(definition.Name); err != nil {
		return fmt.Errorf("validate library definition: %w", err)
	}

	if err := validateRoot(definition.Root); err != nil {
		return fmt.Errorf("validate library definition %q: %w", definition.Name, err)
	}

	if !definition.Kind.IsValid() {
		return fmt.Errorf("validate library definition %q: unknown kind %q: %w",
			definition.Name, definition.Kind, domain.ErrInvalid)
	}

	return nil
}

func validateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("name is empty: %w", domain.ErrInvalid)
	}

	if !utf8.ValidString(name) {
		return fmt.Errorf("name is not valid UTF-8: %w", domain.ErrInvalid)
	}

	if utf8.RuneCountInString(name) > MaxNameLength {
		return fmt.Errorf("name is longer than %d characters: %w", MaxNameLength, domain.ErrInvalid)
	}

	return nil
}

func validateRoot(root string) error {
	// A whitespace-only root can never be the absolute path the configuration requires, and
	// rejecting it here gives a clearer message than the later "not absolute" check.
	if strings.TrimSpace(root) == "" {
		return fmt.Errorf("root is empty: %w", domain.ErrInvalid)
	}

	if strings.ContainsRune(root, nulCharacter) {
		return fmt.Errorf("root contains a NUL byte: %w", domain.ErrInvalid)
	}

	return nil
}

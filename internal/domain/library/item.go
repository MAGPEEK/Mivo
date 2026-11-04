package library

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Library is a configured library as stored in the catalog. ID is a UUID assigned when the
// root first appeared; Root is the cleaned absolute path produced by the configuration.
type Library struct {
	ID   string
	Name string
	Root string
	Kind Kind
}

// ItemKind is the kind of a catalog item. Movies, episodes and extras own media files; shows
// and seasons are containers (ADR 0027 §2). The second-wave kinds (music, audiobooks, photos)
// are added by the tasks that implement their scans.
type ItemKind string

// Known item kinds. The text values are stored in the database and used in the API.
const (
	ItemKindMovie   ItemKind = "movie"
	ItemKindShow    ItemKind = "show"
	ItemKindSeason  ItemKind = "season"
	ItemKindEpisode ItemKind = "episode"
	ItemKindExtra   ItemKind = "extra"
)

// noParent stands for "top level" in ValidParent. It is untyped on purpose: as an ItemKind
// constant the exhaustive linter would demand it as a case of every kind switch.
const noParent = ""

// ParseItemKind accepts the exact text of a known item kind, for example when an adapter
// reads a stored row back. The error wraps domain.ErrInvalid.
func ParseItemKind(text string) (ItemKind, error) {
	kind := ItemKind(text)
	if !kind.IsValid() {
		return "", fmt.Errorf("parse item kind %q: unknown kind: %w", text, domain.ErrInvalid)
	}

	return kind, nil
}

// IsValid reports whether the item kind is one of the known kinds.
func (kind ItemKind) IsValid() bool {
	switch kind {
	case ItemKindMovie, ItemKindShow, ItemKindSeason, ItemKindEpisode, ItemKindExtra:
		return true
	default:
		return false
	}
}

// OwnsFiles reports whether items of this kind hold media files (movie, episode, extra).
// Shows and seasons only group other items.
func (kind ItemKind) OwnsFiles() bool {
	switch kind {
	case ItemKindMovie, ItemKindEpisode, ItemKindExtra:
		return true
	case ItemKindShow, ItemKindSeason:
		return false
	default:
		return false
	}
}

// ValidParent reports whether an item of kind child may sit below an item of kind parent;
// an empty parent asks whether child may be a top-level item. It implements the hierarchy
// table of ADR 0027 §2: movies and shows are top level, a season belongs to a show, an
// episode to a season and an extra to a movie or a show.
func ValidParent(child, parent ItemKind) bool {
	switch child {
	case ItemKindMovie, ItemKindShow:
		return parent == noParent
	case ItemKindSeason:
		return parent == ItemKindShow
	case ItemKindEpisode:
		return parent == ItemKindSeason
	case ItemKindExtra:
		return parent == ItemKindMovie || parent == ItemKindShow
	default:
		return false
	}
}

// ItemKindForLibrary returns the kind of the item created for one video file in a library of
// the given kind: a movie in a movies library, an episode in a shows library. The error wraps
// domain.ErrInvalid for an unknown library kind.
func ItemKindForLibrary(kind Kind) (ItemKind, error) {
	switch kind {
	case KindMovies:
		return ItemKindMovie, nil
	case KindShows:
		return ItemKindEpisode, nil
	default:
		return "", fmt.Errorf("item kind for library kind %q: unknown kind: %w", kind, domain.ErrInvalid)
	}
}

// String returns the text value of the item kind.
func (kind ItemKind) String() string {
	return string(kind)
}

// Item is one entry of a library's catalog. In milestone 2 every video file becomes one item
// whose title comes from TitleFromFileName and whose SortTitle is SortTitle(Title).
//
// The hierarchy fields of milestone 3 (ADR 0027 §2, §3) are zero for M2 items: ParentID is
// empty for top-level items, MatchKey identifies the item among its siblings of the same
// kind across rescans, Number is the season or episode number (season 0 = specials),
// NumberEnd the last episode of a multi-episode file, AbsoluteNumber the absolute episode
// number and ExtraType is set for extras only.
type Item struct {
	ID             string
	LibraryID      string
	ParentID       string
	Kind           ItemKind
	MatchKey       string
	Title          string
	SortTitle      string
	Year           OptionalNumber
	Number         OptionalNumber
	NumberEnd      OptionalNumber
	AbsoluteNumber OptionalNumber
	ExtraType      ExtraType
	CreatedAt      time.Time
}

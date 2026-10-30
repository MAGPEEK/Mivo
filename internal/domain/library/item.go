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

// ItemKind is the kind of a catalog item. Milestone 2 creates one leaf item per video file;
// milestone 3 adds the container kinds (show, season) with the name parser.
type ItemKind string

// Known item kinds. The text values are stored in the database and used in the API.
const (
	ItemKindMovie   ItemKind = "movie"
	ItemKindEpisode ItemKind = "episode"
)

// ParseItemKind accepts the exact text of a known item kind, for example when an adapter
// reads a stored row back. The error wraps domain.ErrInvalid.
func ParseItemKind(text string) (ItemKind, error) {
	switch kind := ItemKind(text); kind {
	case ItemKindMovie, ItemKindEpisode:
		return kind, nil
	default:
		return "", fmt.Errorf("parse item kind %q: unknown kind: %w", text, domain.ErrInvalid)
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
type Item struct {
	ID        string
	LibraryID string
	Kind      ItemKind
	Title     string
	SortTitle string
	CreatedAt time.Time
}

// Package search contains the search use cases (SR-01 to SR-03): the port of the full-text
// index (task M5-14) and the Service that answers the search dropdown and the pivots of the
// results page (task M5-15). cmd/mivo wires the adapters (ADR 0003).
package search

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// MaxLimit is the largest number of hits one Search call returns.
const MaxLimit = 100

// Filter narrows a search.
type Filter struct {
	// LibraryIDs are the libraries searched. Empty finds nothing: a caller that may see no
	// library must not get the hits of every library.
	LibraryIDs []string
	// Kinds are the item kinds returned; empty returns every indexed kind. Seasons and
	// extras are not indexed, so asking for them finds nothing.
	Kinds []library.ItemKind
	// Restriction is what the viewer may see (ADR 0032 section 7): a hit lies in a library
	// of LibraryIDs that the restriction allows too, and it and every ancestor pass the
	// content limit. The index applies it before the limit, so hidden items never shorten a
	// result. The zero value finds nothing.
	Restriction access.Restriction
}

// Hit is one search result.
type Hit struct {
	ItemID    string
	LibraryID string
	Kind      library.ItemKind
}

// Index searches the catalog by text. Implemented by adapters/sqlite (SearchIndex).
type Index interface {
	// Search returns up to limit hits for query, best match first. Every word of query
	// matches as a prefix, case- and accent-insensitively, in a title, original title, sort
	// title, person or genre of the item; all words must match. Any query text is accepted:
	// one without a letter or digit finds nothing. A hit by a person's name is a hit of the
	// item, so it follows the item's visibility. A limit outside 1..MaxLimit or an unknown
	// kind wraps domain.ErrInvalid.
	Search(ctx context.Context, query string, filter Filter, limit int) ([]Hit, error)
}

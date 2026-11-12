package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Type names what a search returns: the mixed top hits of the dropdown (SR-01) or one pivot
// of the results page (SR-02).
type Type string

// The search types of the contract (components.schemas.SearchType).
const (
	TypeTop        Type = "top"
	TypeMovies     Type = "movies"
	TypeShows      Type = "shows"
	TypeEpisodes   Type = "episodes"
	TypeMusic      Type = "music"
	TypePhotos     Type = "photos"
	TypeAudiobooks Type = "audiobooks"
)

const (
	// MinQueryLength is the shortest query, in characters without surrounding spaces. The
	// index has prefix indexes from two characters on; a one-character prefix would scan
	// every term (M5-14).
	MinQueryLength = 2
	// TopLimit is the largest and the default number of hits of TypeTop.
	TopLimit = 8
	// DefaultPageLimit is the page size of a pivot when the caller names none.
	DefaultPageLimit = 24
	// MaxPageLimit is the largest page of a pivot.
	MaxPageLimit = 50
	// maxSearchedLibraries bounds the library IDs handed to the index, which binds one SQL
	// parameter per library. No real server comes near it; a search beyond it fails rather
	// than silently leaving libraries out.
	maxSearchedLibraries = 500
)

// Errors a caller can cause; all wrap domain.ErrInvalid. The HTTP adapter names the
// parameter of each.
var (
	// ErrQueryTooShort reports a query of fewer than MinQueryLength characters.
	ErrQueryTooShort = fmt.Errorf("query shorter than %d characters: %w", MinQueryLength, domain.ErrInvalid)
	// ErrUnknownType reports a Type that is not one of the constants.
	ErrUnknownType = fmt.Errorf("unknown search type: %w", domain.ErrInvalid)
	// ErrInvalidCursor reports a cursor Search did not issue for this type, or any cursor
	// with TypeTop, which is never paged.
	ErrInvalidCursor = fmt.Errorf("search cursor not valid: %w", domain.ErrInvalid)
)

// LibrarySource lists the libraries the caller may see, the same port as the rows use
// (M5-13); access control (M7) plugs in here.
type LibrarySource interface {
	// VisibleLibraries returns the visible libraries.
	VisibleLibraries(ctx context.Context) ([]library.ManagedLibrary, error)
}

// SummaryReader reads the summaries of items as listings carry them. Implemented by
// adapters/sqlite (CatalogReader.ItemSummaries).
type SummaryReader interface {
	// ItemSummaries returns the summaries of the given items in the given order; unknown
	// IDs and items the restriction hides are left out. The zero restriction allows none.
	ItemSummaries(ctx context.Context, itemIDs []string, restriction access.Restriction) ([]applibrary.ItemSummary, error)
}

// Dependencies are the ports of the Service; all are required.
type Dependencies struct {
	Libraries LibrarySource
	Index     Index
	Summaries SummaryReader
}

// Request is one search. Limit 0 means the default of the type; a larger limit than the type
// allows is clamped. Cursor is empty for the first page. Restriction is what the viewer may
// see; its zero value allows nothing.
type Request struct {
	Query       string
	Type        Type
	Limit       int
	Cursor      string
	Restriction access.Restriction
}

// Page is one page of hits, best match first. NextCursor is empty on the last page and
// always for TypeTop.
type Page struct {
	Items      []applibrary.ItemSummary
	NextCursor string
}

// Service answers searches over the catalog (task M5-15).
type Service struct {
	libraries LibrarySource
	index     Index
	summaries SummaryReader
}

// NewService returns the search service. A missing dependency wraps domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if dependencies.Libraries == nil || dependencies.Index == nil || dependencies.Summaries == nil {
		return nil, fmt.Errorf("search service: missing dependency: %w", domain.ErrInvalid)
	}

	return &Service{
		libraries: dependencies.Libraries,
		index:     dependencies.Index,
		summaries: dependencies.Summaries,
	}, nil
}

// pivotKinds returns the item kinds of a type; nil kinds for TypeTop mean every indexed kind.
func pivotKinds(searchType Type) ([]library.ItemKind, bool) {
	switch searchType {
	case TypeTop:
		return nil, true
	case TypeMovies:
		return []library.ItemKind{library.ItemKindMovie}, true
	case TypeShows:
		return []library.ItemKind{library.ItemKindShow}, true
	case TypeEpisodes:
		return []library.ItemKind{library.ItemKindEpisode}, true
	case TypeMusic:
		return []library.ItemKind{library.ItemKindArtist, library.ItemKindAlbum, library.ItemKindTrack}, true
	case TypePhotos:
		return []library.ItemKind{library.ItemKindPhotoAlbum, library.ItemKindPhoto}, true
	case TypeAudiobooks:
		return []library.ItemKind{library.ItemKindAudiobook}, true
	default:
		return nil, false
	}
}

// window is the slice of the ranked hits a request asks for.
type window struct {
	offset int
	limit  int
}

// windowOf checks the paging of a request. TypeTop takes no cursor and at most TopLimit hits;
// a pivot is clamped to MaxPageLimit.
func windowOf(request Request) (window, error) {
	if request.Limit < 0 {
		return window{}, fmt.Errorf("limit %d is negative: %w", request.Limit, domain.ErrInvalid)
	}

	if request.Type == TypeTop {
		if request.Cursor != "" {
			return window{}, fmt.Errorf("top hits are not paged: %w", ErrInvalidCursor)
		}

		return window{limit: clampLimit(request.Limit, TopLimit, TopLimit)}, nil
	}

	offset, err := decodeCursor(request.Cursor, request.Type)
	if err != nil {
		return window{}, err
	}

	return window{offset: offset, limit: clampLimit(request.Limit, DefaultPageLimit, MaxPageLimit)}, nil
}

func clampLimit(limit, defaultLimit, maxLimit int) int {
	if limit == 0 {
		return defaultLimit
	}

	return min(limit, maxLimit)
}

// Search returns one page of hits for the request. A query shorter than MinQueryLength wraps
// ErrQueryTooShort, an unknown type ErrUnknownType and a cursor not issued for the type
// ErrInvalidCursor (all domain.ErrInvalid). A viewer without visible libraries finds nothing.
//
// The index ranks at most MaxLimit hits per search, so a pivot ends there even when more
// items match: the best MaxLimit hits are what a results page shows.
func (service *Service) Search(ctx context.Context, request Request) (Page, error) {
	query := strings.TrimSpace(request.Query)
	if utf8.RuneCountInString(query) < MinQueryLength {
		return Page{}, fmt.Errorf("search: %w", ErrQueryTooShort)
	}

	kinds, known := pivotKinds(request.Type)
	if !known {
		return Page{}, fmt.Errorf("search: type %q: %w", request.Type, ErrUnknownType)
	}

	page, err := windowOf(request)
	if err != nil {
		return Page{}, fmt.Errorf("search: %w", err)
	}

	libraryIDs, err := service.visibleLibraryIDs(ctx)
	if err != nil {
		return Page{}, fmt.Errorf("search: %w", err)
	}

	if len(libraryIDs) == 0 {
		return Page{Items: []applibrary.ItemSummary{}}, nil
	}

	// One hit more than the page tells whether another page follows.
	fetched := min(page.offset+page.limit+1, MaxLimit)

	hits, err := service.index.Search(ctx, query, Filter{LibraryIDs: libraryIDs, Kinds: kinds}, fetched)
	if err != nil {
		return Page{}, fmt.Errorf("search: %w", indexFault(err))
	}

	return service.pageOf(ctx, request, page, hits)
}

// indexFault hides domain.ErrInvalid of the index: the request was checked before, so the
// index rejecting its filter or limit is a server bug, not a bad request.
func indexFault(err error) error {
	if errors.Is(err, domain.ErrInvalid) {
		return fmt.Errorf("index rejected a checked request: %v", err) //nolint:errorlint // the sentinel is hidden on purpose
	}

	return err
}

func (service *Service) visibleLibraryIDs(ctx context.Context) ([]string, error) {
	libraries, err := service.libraries.VisibleLibraries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list visible libraries: %w", err)
	}

	if len(libraries) > maxSearchedLibraries {
		return nil, fmt.Errorf("%d libraries exceed the %d one search covers", len(libraries), maxSearchedLibraries)
	}

	ids := make([]string, 0, len(libraries))
	for _, managed := range libraries {
		ids = append(ids, managed.ID)
	}

	return ids, nil
}

// pageOf cuts the page out of the ranked hits and loads its summaries in rank order.
func (service *Service) pageOf(ctx context.Context, request Request, page window, hits []Hit) (Page, error) {
	end := min(page.offset+page.limit, len(hits))
	if page.offset >= end {
		return Page{Items: []applibrary.ItemSummary{}}, nil
	}

	itemIDs := make([]string, 0, end-page.offset)
	for _, hit := range hits[page.offset:end] {
		itemIDs = append(itemIDs, hit.ItemID)
	}

	summaries, err := service.summaries.ItemSummaries(ctx, itemIDs, request.Restriction)
	if err != nil {
		return Page{}, fmt.Errorf("search: read hits: %w", err)
	}

	result := Page{Items: summaries}
	if result.Items == nil {
		result.Items = []applibrary.ItemSummary{}
	}

	if request.Type != TypeTop && len(hits) > end {
		result.NextCursor = encodeCursor(request.Type, end)
	}

	return result, nil
}

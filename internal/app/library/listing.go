package library

import (
	"context"
	"fmt"
	"slices"
	"strings"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ErrItemKindMismatch reports a requested item kind that is not the top-level kind of the
// library ("show" in a movies library). It wraps domain.ErrInvalid; the HTTP adapter names
// the type parameter in its problem.
var ErrItemKindMismatch = fmt.Errorf("item kind is not the library's top-level kind: %w", domain.ErrInvalid)

// FacetRequest asks for the facet counts of a library listing. Fields are the facet fields to
// count, in contract order without duplicates; Filter is the current filter. Restriction is
// what the viewer may see; its zero value denies everything (ADR 0032 §7).
type FacetRequest struct {
	Fields      []library.FacetField
	Filter      library.ListingFilter
	Restriction access.Restriction
}

// FacetValue is one value of a facet field with the number of top-level items carrying it.
type FacetValue struct {
	Value string
	Count int64
}

// Facet is the values of one field, ordered by value (numbers numerically, text
// case-insensitively, resolutions lowest first), at most MaxFacetValues of them; Truncated
// reports that more exist.
type Facet struct {
	Field     library.FacetField
	Values    []FacetValue
	Truncated bool
}

// MaxFacetValues is the most values one facet carries (API Facet.values maxItems).
const MaxFacetValues = 500

// LetterRequest asks for the letter index of the title-sorted listing narrowed by Filter.
type LetterRequest struct {
	Descending bool
	Filter     library.ListingFilter
	// Restriction is what the viewer may see; its zero value denies everything.
	Restriction access.Restriction
}

// CharacterCount is the number of listed items whose sort title starts with Character, as
// ListingReader reports them in list order.
type CharacterCount struct {
	Character string
	Count     int64
}

// LetterOffset is where the items of one A–Z bar letter start in the title-sorted list.
type LetterOffset struct {
	Letter string
	Offset int64
}

// LetterIndex is the start of every letter that occurs, in list order, with the item count.
type LetterIndex struct {
	Letters    []LetterOffset
	TotalCount int64
}

// ListFacets returns the facet counts of a library listing: per requested field (every field
// when none is requested) the values among the library's top-level items, each counted under
// the filter without that field's own values. Errors as ListItems; without a wired
// ListingReader the error wraps domain.ErrUnavailable.
func (service *Service) ListFacets(ctx context.Context, libraryID string, request FacetRequest) ([]Facet, error) {
	restriction, err := appaccess.LibraryRestriction(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list facets of library %q: %w", libraryID, err)
	}

	if service.listings == nil {
		return nil, fmt.Errorf("list facets of library %q: no listing reader: %w", libraryID, domain.ErrUnavailable)
	}

	filter, err := service.prepareFilter(ctx, libraryID, request.Filter)
	if err != nil {
		return nil, fmt.Errorf("list facets of library %q: %w", libraryID, err)
	}

	facets, err := service.listings.CountFacets(ctx, libraryID, FacetRequest{
		Fields: facetFieldsOf(request.Fields), Filter: filter,
		Restriction: restriction,
	})
	if err != nil {
		return nil, fmt.Errorf("list facets of library %q: %w", libraryID, err)
	}

	return facets, nil
}

// facetFieldsOf returns the requested fields in contract order without duplicates; none
// requested means every field.
func facetFieldsOf(requested []library.FacetField) []library.FacetField {
	all := library.FacetFields()
	if len(requested) == 0 {
		return all
	}

	return slices.DeleteFunc(all, func(field library.FacetField) bool { return !slices.Contains(requested, field) })
}

// ListLetters returns where each A–Z bar letter starts in the library's title-sorted listing
// narrowed by the filter: the first item of a letter, so letters folded from several first
// characters ("A" and "Ä") start at the first of them. Letters without items are left out.
// Errors as ListFacets.
func (service *Service) ListLetters(ctx context.Context, libraryID string, request LetterRequest) (LetterIndex, error) {
	restriction, err := appaccess.LibraryRestriction(ctx, libraryID)
	if err != nil {
		return LetterIndex{}, fmt.Errorf("list letters of library %q: %w", libraryID, err)
	}

	if service.listings == nil {
		return LetterIndex{}, fmt.Errorf("list letters of library %q: no listing reader: %w", libraryID, domain.ErrUnavailable)
	}

	filter, err := service.prepareFilter(ctx, libraryID, request.Filter)
	if err != nil {
		return LetterIndex{}, fmt.Errorf("list letters of library %q: %w", libraryID, err)
	}

	request.Filter = filter
	request.Restriction = restriction

	counts, err := service.listings.CountFirstCharacters(ctx, libraryID, request)
	if err != nil {
		return LetterIndex{}, fmt.Errorf("list letters of library %q: %w", libraryID, err)
	}

	return letterIndexOf(counts, request.Descending), nil
}

// letterIndexOf folds the first characters, in list order, into letters at their first
// position. The letters come in bar order, as the contract lists them: "#" then A–Z when
// ascending, Z–A then "#" when descending. In byte order of sort titles "#" items can also
// follow "z" (other scripts), so bar order and offset order may differ; the offsets stay the
// first positions.
func letterIndexOf(counts []CharacterCount, descending bool) LetterIndex {
	var index LetterIndex

	for _, count := range counts {
		letter := library.LetterOf(count.Character)

		seen := slices.ContainsFunc(index.Letters, func(offset LetterOffset) bool { return offset.Letter == letter })
		if !seen && count.Count > 0 {
			index.Letters = append(index.Letters, LetterOffset{Letter: letter, Offset: index.TotalCount})
		}

		index.TotalCount += count.Count
	}

	// LetterOther ("#") sorts before "A" in byte order, which is the ascending bar order.
	slices.SortFunc(index.Letters, func(left, right LetterOffset) int {
		if descending {
			return strings.Compare(right.Letter, left.Letter)
		}

		return strings.Compare(left.Letter, right.Letter)
	})

	return index
}

// prepareFilter checks the library and completes the filter for the reader: an unknown
// library wraps domain.ErrNotFound, a kind other than the library's top-level kind
// ErrItemKindMismatch. An omitted kind means the library's default top-level kind. For
// the quick filter "unavailable" it adds the library folders that are offline now.
func (service *Service) prepareFilter(ctx context.Context, libraryID string, filter library.ListingFilter) (library.ListingFilter, error) {
	// The readers cannot tell an unknown library from an empty one.
	managed, err := service.libraries.GetManaged(ctx, libraryID)
	if err != nil {
		return library.ListingFilter{}, storedDataError(err)
	}

	kinds := managed.Kind.TopLevelItemKinds()

	switch {
	case filter.Kind == "" && len(kinds) > 0:
		filter.Kind = kinds[0]
	case filter.Kind != "" && !slices.Contains(kinds, filter.Kind):
		return library.ListingFilter{}, fmt.Errorf("kind %q in a %s library: %w", filter.Kind, managed.Kind, ErrItemKindMismatch)
	}

	if filter.Quick == library.QuickUnavailable {
		filter.OfflineRootIDs = service.OfflineFolders(ctx, managed.Folders)
	}

	return filter, nil
}

// OfflineFolders returns the IDs of the folders that cannot be reached now; none without a
// RootChecker. The rows use case leaves the items in them out (M5-13).
func (service *Service) OfflineFolders(ctx context.Context, folders []library.Folder) []string {
	if service.roots == nil {
		return nil
	}

	paths := make([]string, 0, len(folders))
	for _, folder := range folders {
		paths = append(paths, folder.Path)
	}

	var offline []string

	for index, available := range service.roots.availableAll(ctx, paths) {
		if !available {
			offline = append(offline, folders[index].ID)
		}
	}

	return offline
}

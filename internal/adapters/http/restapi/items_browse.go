package restapi

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Bounds of the Browse query parameters (api/openapi.yaml, task M5-10). The generated server
// binds them but checks neither enums nor schema bounds, so the handlers do.
const (
	// maxFilterValues is the maxItems of every free-text and number field filter.
	maxFilterValues = 50
	// maxResolutionValues is the maxItems of the resolution filter.
	maxResolutionValues = 10
	// maxFacetFields is the maxItems of the facets field parameter.
	maxFacetFields = 20
	// maxGenreLength, maxContentRatingLength, maxCodecLength and maxLanguageLength are the
	// maxLength of one filter value in characters.
	maxGenreLength         = 200
	maxContentRatingLength = 100
	maxCodecLength         = 64
	maxLanguageLength      = 35
	// minFilterYear and maxFilterYear bound year and decade values; maxFilterDecade is the
	// last decade start below maxFilterYear.
	minFilterYear   = 1000
	maxFilterYear   = 9999
	maxFilterDecade = 9990
	// decadeLength is the number of years in a decade; a decade value is its first year.
	decadeLength = 10
	// maxLibraryPathLength is the maxLength of the library-relative folder path.
	maxLibraryPathLength = 4096
)

// Query parameter names a Browse problem points at (Problem.field).
const (
	fieldSort             = "sort"
	fieldOrder            = "order"
	fieldSeed             = "seed"
	fieldType             = "type"
	fieldQuick            = "quick"
	fieldGenre            = "genre"
	fieldYear             = "year"
	fieldDecade           = "decade"
	fieldContentRating    = "contentRating"
	fieldResolution       = "resolution"
	fieldVideoCodec       = "videoCodec"
	fieldAudioCodec       = "audioCodec"
	fieldContainer        = "container"
	fieldAudioLanguage    = "audioLanguage"
	fieldSubtitleLanguage = "subtitleLanguage"
	fieldFacetField       = "field"
	fieldPath             = "path"
)

// libraryPathSeparator separates the segments of a library-relative folder path.
const libraryPathSeparator = "/"

// listingFilters are the filters listLibraryItems, listLibraryFacets and listLibraryLetters
// share, so "the current filter" is checked the same way on all three.
type listingFilters struct {
	kind             *apigen.ItemKind
	quick            *apigen.ItemQuickFilter
	genre            *[]string
	year             *[]int32
	decade           *[]int32
	contentRating    *[]string
	resolution       *[]apigen.VideoResolution
	videoCodec       *[]string
	audioCodec       *[]string
	container        *[]string
	audioLanguage    *[]string
	subtitleLanguage *[]string
}

// invalidParameter reports a query parameter value outside the contract. A library
// FieldError maps to 400 "invalid" with Problem.field set to the parameter name.
func invalidParameter(field, reason string) error {
	return &library.FieldError{Field: library.Field(field), Reason: reason}
}

// validate checks every filter against the contract; the first violation wins.
func (filters listingFilters) validate() error {
	return errors.Join(
		checkItemKind(filters.kind),
		checkEnum(fieldQuick, filters.quick),
		checkTexts(fieldGenre, filters.genre, maxGenreLength),
		checkYears(fieldYear, filters.year, maxFilterYear, 1),
		checkYears(fieldDecade, filters.decade, maxFilterDecade, decadeLength),
		checkTexts(fieldContentRating, filters.contentRating, maxContentRatingLength),
		checkEnums(fieldResolution, filters.resolution, maxResolutionValues),
		checkTexts(fieldVideoCodec, filters.videoCodec, maxCodecLength),
		checkTexts(fieldAudioCodec, filters.audioCodec, maxCodecLength),
		checkTexts(fieldContainer, filters.container, maxCodecLength),
		checkTexts(fieldAudioLanguage, filters.audioLanguage, maxLanguageLength),
		checkTexts(fieldSubtitleLanguage, filters.subtitleLanguage, maxLanguageLength),
	)
}

// active reports whether any filter narrows the list; quick "all" and absent filters do not.
func (filters listingFilters) active() bool {
	quickFilters := filters.quick != nil && *filters.quick != apigen.ItemQuickFilterAll

	return quickFilters || filters.kind != nil || filters.genre != nil || filters.year != nil ||
		filters.decade != nil || filters.contentRating != nil || filters.resolution != nil ||
		filters.videoCodec != nil || filters.audioCodec != nil || filters.container != nil ||
		filters.audioLanguage != nil || filters.subtitleLanguage != nil
}

// checkItemKind accepts only the top-level kinds a library lists. Whether the kind is the
// one of the requested library is the use case's check (M5-11).
func checkItemKind(kind *apigen.ItemKind) error {
	if kind == nil {
		return nil
	}

	if *kind != apigen.ItemKindMovie && *kind != apigen.ItemKindShow {
		return invalidParameter(fieldType, "not a top-level item kind")
	}

	return nil
}

// validEnum is a generated enum type.
type validEnum interface {
	Valid() bool
}

func checkEnum[T validEnum](field string, value *T) error {
	if value != nil && !(*value).Valid() {
		return invalidParameter(field, "unknown value")
	}

	return nil
}

func checkEnums[T validEnum](field string, values *[]T, maxValues int) error {
	if values == nil {
		return nil
	}

	if len(*values) > maxValues {
		return invalidParameter(field, "too many values")
	}

	for _, value := range *values {
		if !value.Valid() {
			return invalidParameter(field, "unknown value")
		}
	}

	return nil
}

func checkTexts(field string, values *[]string, maxLength int) error {
	if values == nil {
		return nil
	}

	if len(*values) > maxFilterValues {
		return invalidParameter(field, "too many values")
	}

	for _, value := range *values {
		if length := utf8.RuneCountInString(value); length == 0 || length > maxLength {
			return invalidParameter(field, "empty or too long value")
		}
	}

	return nil
}

// checkYears checks year or decade values: within the year bounds and a multiple of step.
func checkYears(field string, values *[]int32, maxYear, step int32) error {
	if values == nil {
		return nil
	}

	if len(*values) > maxFilterValues {
		return invalidParameter(field, "too many values")
	}

	for _, value := range *values {
		if value < minFilterYear || value > maxYear || value%step != 0 {
			return invalidParameter(field, "value out of range")
		}
	}

	return nil
}

// checkSortOrder checks the order parameter of the Browse operations.
func checkSortOrder(order *apigen.SortOrder) error {
	return checkEnum(fieldOrder, order)
}

// checkLibraryPath rejects every library-relative path that could leave its folder or is
// not in the documented form: absolute, with a backslash, or with an empty, "." or ".."
// segment. The empty path (the top) is valid.
func checkLibraryPath(path *string) error {
	if path == nil || *path == "" {
		return nil
	}

	if utf8.RuneCountInString(*path) > maxLibraryPathLength {
		return invalidParameter(fieldPath, "path too long")
	}

	if strings.Contains(*path, `\`) {
		return invalidParameter(fieldPath, "path contains a backslash")
	}

	for segment := range strings.SplitSeq(*path, libraryPathSeparator) {
		if segment == "" || segment == "." || segment == ".." {
			return invalidParameter(fieldPath, fmt.Sprintf("path segment %q not allowed", segment))
		}
	}

	return nil
}

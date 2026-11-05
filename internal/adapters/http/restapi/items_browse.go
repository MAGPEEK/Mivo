package restapi

import (
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
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

// invalidQueryError reports a query parameter value outside the contract: 400 "invalid" with
// Problem.field naming the parameter (see queryProblem). It is local to this adapter, so the
// field names of library management (name, kind, folders) cannot turn it into their codes.
type invalidQueryError struct {
	field  string
	reason string
}

func (err *invalidQueryError) Error() string {
	return fmt.Sprintf("query parameter %s: %s", err.field, err.reason)
}

// Unwrap lets errors.Is find domain.ErrInvalid, like every other client mistake.
func (err *invalidQueryError) Unwrap() error {
	return domain.ErrInvalid
}

func invalidParameter(field, reason string) error {
	return &invalidQueryError{field: field, reason: reason}
}

// queryProblem maps an invalid query parameter, and a type that is not the library's
// top-level kind, to 400 "invalid" with the parameter as Problem.field. It reports false for
// every other error.
func queryProblem(err error) (apigen.Problem, bool) {
	var queryErr *invalidQueryError
	if errors.As(err, &queryErr) {
		return fieldRequestProblem(CodeInvalid, queryErr.field, "The value is not valid."), true
	}

	if errors.Is(err, applibrary.ErrItemKindMismatch) {
		return fieldRequestProblem(CodeInvalid, fieldType, "The type is not the top-level kind of this library."), true
	}

	return apigen.Problem{}, false
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

// checkItemKind accepts the kinds some library kind lists at its top (movie, show, artist,
// album, audiobook, photo_album, photo; library.IsTopLevelItemKind). Whether the kind
// belongs to the requested library is the use case's check (applibrary.ErrItemKindMismatch).
func checkItemKind(kind *apigen.ItemKind) error {
	if kind == nil {
		return nil
	}

	if !library.IsTopLevelItemKind(library.ItemKind(*kind)) {
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

// libraryPathOf parses the library-relative folder path (library.ParseFolderPath): every
// path that could leave its folder or is not in the documented form answers 400 "invalid"
// with field "path". Omitted and empty mean the top.
func libraryPathOf(path *string) (library.FolderPath, error) {
	if path == nil {
		return library.FolderPath{}, nil
	}

	parsed, err := library.ParseFolderPath(*path)
	if err != nil {
		return library.FolderPath{}, invalidParameter(fieldPath, err.Error())
	}

	return parsed, nil
}

// domainFilter converts checked filters to the use case's filter.
func (filters listingFilters) domainFilter() library.ListingFilter {
	var filter library.ListingFilter

	if filters.kind != nil {
		filter.Kind = library.ItemKind(*filters.kind)
	}

	if filters.quick != nil {
		filter.Quick = library.QuickFilter(*filters.quick)
	}

	filter.Genres = values(filters.genre)
	filter.Years = numbers(filters.year)
	filter.Decades = numbers(filters.decade)
	filter.ContentRatings = values(filters.contentRating)
	filter.Resolutions = convertAll(filters.resolution, func(value apigen.VideoResolution) library.Resolution { return library.Resolution(value) })
	filter.VideoCodecs = values(filters.videoCodec)
	filter.AudioCodecs = values(filters.audioCodec)
	filter.Containers = values(filters.container)
	filter.AudioLanguages = values(filters.audioLanguage)
	filter.SubtitleLanguages = values(filters.subtitleLanguage)

	return filter
}

func values(texts *[]string) []string {
	return convertAll(texts, func(text string) string { return text })
}

func numbers(years *[]int32) []int {
	return convertAll(years, func(year int32) int { return int(year) })
}

// convertAll converts an optional parameter list; omitted gives nil.
func convertAll[From, To any](values *[]From, convert func(From) To) []To {
	if values == nil {
		return nil
	}

	converted := make([]To, 0, len(*values))
	for _, value := range *values {
		converted = append(converted, convert(value))
	}

	return converted
}

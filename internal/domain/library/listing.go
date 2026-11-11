package library

import "github.com/magpeek/mivo/internal/domain/access"

// ItemSort is the sort key of a library listing (API ItemSort). The text values are part of
// the API contract.
type ItemSort string

// The sort keys of a library listing. Ties always follow by sort title, then by ID, so the
// order is total and keyset paging is stable.
const (
	SortByTitle         ItemSort = "title"
	SortByYear          ItemSort = "year"
	SortByReleaseDate   ItemSort = "releaseDate"
	SortByContentRating ItemSort = "contentRating"
	SortByDuration      ItemSort = "duration"
	SortByDateAdded     ItemSort = "dateAdded"
	SortByResolution    ItemSort = "resolution"
	SortByBitrate       ItemSort = "bitrate"
	SortByFileSize      ItemSort = "fileSize"
	SortByRandom        ItemSort = "random"
)

// IsValid reports whether the sort is one of the declared keys. The zero value is not valid;
// ListingOrder treats it as SortByTitle.
func (sort ItemSort) IsValid() bool {
	switch sort {
	case SortByTitle, SortByYear, SortByReleaseDate, SortByContentRating, SortByDuration, SortByDateAdded:
		return true
	case SortByResolution, SortByBitrate, SortByFileSize, SortByRandom:
		return true
	default:
		return false
	}
}

// QuickFilter is the toolbar filter of a library listing (API ItemQuickFilter).
type QuickFilter string

// The quick filters. The zero value filters nothing, like QuickAll.
const (
	QuickAll         QuickFilter = "all"
	QuickHDR         QuickFilter = "hdr"
	QuickDolbyVision QuickFilter = "dolbyVision"
	QuickHDR10Plus   QuickFilter = "hdr10plus"
	QuickAtmos       QuickFilter = "atmos"
	QuickUHD         QuickFilter = "uhd"
	QuickUnavailable QuickFilter = "unavailable"
	QuickDuplicates  QuickFilter = "duplicates"
)

// IsValid reports whether the quick filter is one of the declared values.
func (quick QuickFilter) IsValid() bool {
	switch quick {
	case QuickAll, QuickHDR, QuickDolbyVision, QuickHDR10Plus, QuickAtmos:
		return true
	case QuickUHD, QuickUnavailable, QuickDuplicates:
		return true
	default:
		return false
	}
}

// Active reports whether the quick filter narrows the list: neither the zero value nor
// QuickAll does.
func (quick QuickFilter) Active() bool {
	return quick != "" && quick != QuickAll
}

// FacetField names a filter field that facet counts are given for (API FacetField).
type FacetField string

// The facet fields in the order of the API contract.
const (
	FacetGenre            FacetField = "genre"
	FacetYear             FacetField = "year"
	FacetDecade           FacetField = "decade"
	FacetContentRating    FacetField = "contentRating"
	FacetResolution       FacetField = "resolution"
	FacetVideoCodec       FacetField = "videoCodec"
	FacetAudioCodec       FacetField = "audioCodec"
	FacetContainer        FacetField = "container"
	FacetAudioLanguage    FacetField = "audioLanguage"
	FacetSubtitleLanguage FacetField = "subtitleLanguage"
)

// FacetFields returns every facet field in contract order, the order of a facet answer.
func FacetFields() []FacetField {
	return []FacetField{
		FacetGenre, FacetYear, FacetDecade, FacetContentRating, FacetResolution,
		FacetVideoCodec, FacetAudioCodec, FacetContainer, FacetAudioLanguage, FacetSubtitleLanguage,
	}
}

// IsValid reports whether the field is one of the declared facet fields.
func (field FacetField) IsValid() bool {
	for _, known := range FacetFields() {
		if field == known {
			return true
		}
	}

	return false
}

// ListingFilter narrows a library listing. The values of one field combine with OR, the
// fields (and Quick) with AND; an empty field filters nothing. Kind is the requested
// top-level kind, empty for "the library's own". Decades hold the first year of each decade.
// Genres compare case-insensitively, every other text field exactly.
type ListingFilter struct {
	Kind              ItemKind
	Quick             QuickFilter
	Genres            []string
	Years             []int
	Decades           []int
	ContentRatings    []string
	Resolutions       []Resolution
	VideoCodecs       []string
	AudioCodecs       []string
	Containers        []string
	AudioLanguages    []string
	SubtitleLanguages []string
	// OfflineRootIDs are the library folders that cannot be reached now; QuickUnavailable
	// also matches the items whose primary file lies in one of them. The use case fills it.
	OfflineRootIDs []string
	// MatchState keeps only the items in one online match state; empty keeps every item.
	MatchState MatchStateFilter
}

// MatchStateFilter is the online match state a listing is narrowed to. Its values are the
// texts of match.MatchState, which this package cannot import (match depends on library).
type MatchStateFilter string

// Match states a listing can be narrowed to (ADR 0031 §9).
const (
	// MatchFilterAll keeps every item.
	MatchFilterAll MatchStateFilter = ""
	// MatchFilterMatched keeps the items linked to a provider entry.
	MatchFilterMatched MatchStateFilter = "matched"
	// MatchFilterUnmatched keeps the items without a match, also those never looked up.
	MatchFilterUnmatched MatchStateFilter = "unmatched"
	// MatchFilterDisabled keeps the items whose match was removed on purpose.
	MatchFilterDisabled MatchStateFilter = "disabled"
)

// IsValid reports whether the filter is one of the known states or empty.
func (filter MatchStateFilter) IsValid() bool {
	switch filter {
	case MatchFilterAll, MatchFilterMatched, MatchFilterUnmatched, MatchFilterDisabled:
		return true
	default:
		return false
	}
}

// Without returns the filter with the given facet field cleared, the filter a facet count of
// that field applies: choosing a further value of the same field widens the list.
func (filter ListingFilter) Without(field FacetField) ListingFilter {
	switch field {
	case FacetGenre:
		filter.Genres = nil
	case FacetYear:
		filter.Years = nil
	case FacetDecade:
		filter.Decades = nil
	case FacetContentRating:
		filter.ContentRatings = nil
	case FacetResolution:
		filter.Resolutions = nil
	case FacetVideoCodec:
		filter.VideoCodecs = nil
	case FacetAudioCodec:
		filter.AudioCodecs = nil
	case FacetContainer:
		filter.Containers = nil
	case FacetAudioLanguage:
		filter.AudioLanguages = nil
	case FacetSubtitleLanguage:
		filter.SubtitleLanguages = nil
	default:
	}

	return filter
}

// ListingOrder is the order of a library listing. The zero value is the default order: sort
// title ascending. Seed only matters for SortByRandom; HasSeed false lets the reader take the
// seed from the cursor.
type ListingOrder struct {
	Sort       ItemSort
	Descending bool
	Seed       int64
	HasSeed    bool
}

// Key returns the effective sort key: SortByTitle for the zero value.
func (order ListingOrder) Key() ItemSort {
	if order.Sort == "" {
		return SortByTitle
	}

	return order.Sort
}

// ListingQuery is the sort and filter of a library listing and the restriction of the viewer
// it is read for.
type ListingQuery struct {
	Filter ListingFilter
	Order  ListingOrder
	// Restriction is what the viewer may see (ADR 0032 §7). The zero value denies everything,
	// so a caller that forgets it gets no items instead of all.
	Restriction access.Restriction
}

// IsDescending reports whether the listing runs in descending order; SortByRandom ignores the
// direction.
func (order ListingOrder) IsDescending() bool {
	return order.Descending && order.Key() != SortByRandom
}

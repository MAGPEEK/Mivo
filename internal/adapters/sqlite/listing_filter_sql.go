package sqlite

import (
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// neverMatches is the condition of a quick filter whose property the catalog does not record
// yet (Dolby Vision, HDR10+ and Atmos are not detected, see media.HDRFormat).
const neverMatches = "0 = 1"

// noFacetField marks a condition no facet leaves out: the quick filter.
const noFacetField library.FacetField = ""

// filterCondition is the SQL condition of one active filter on the top-level items (alias
// "items"), the facet field it belongs to and the common tables it reads.
type filterCondition struct {
	field library.FacetField
	sql   string
	needs tableNeeds
}

// needsOf returns the common tables the conditions read.
func needsOf(conditions []filterCondition) tableNeeds {
	var needs tableNeeds
	for _, condition := range conditions {
		needs = needs.union(condition.needs)
	}

	return needs
}

// writeWithFilter writes the WITH clause the filter reads and returns its condition: the
// base condition of the scope and every filter condition, joined with AND.
func (scope listingScope) writeWithFilter(filter library.ListingFilter) string {
	conditions := scope.filterConditions(filter)
	scope.builder.writeWith(scope.commonTables(needsOf(conditions)))

	return joinConditions(scope.base, conditions)
}

// joinConditions joins the base condition and the filter conditions with AND.
func joinConditions(base string, conditions []filterCondition) string {
	parts := []string{base}
	for _, condition := range conditions {
		parts = append(parts, condition.sql)
	}

	return strings.Join(parts, " AND ")
}

// filterConditions returns the conditions of the active filters in facet field order, the
// quick filter last.
func (scope listingScope) filterConditions(filter library.ListingFilter) []filterCondition {
	conditions := append(scope.itemConditions(filter), scope.fileConditions(filter)...)
	conditions = append(conditions, scope.streamConditions(filter)...)

	if quick, found := scope.quickCondition(filter); found {
		conditions = append(conditions, quick)
	}

	return conditions
}

// itemConditions filter by the item's own metadata.
func (scope listingScope) itemConditions(filter library.ListingFilter) []filterCondition {
	builder := scope.builder

	var conditions []filterCondition

	if len(filter.Genres) > 0 {
		conditions = append(conditions, filterCondition{field: library.FacetGenre, sql: "items.id IN (SELECT item_tags.item_id FROM item_tags" +
			" JOIN tags ON tags.id = item_tags.tag_id WHERE tags.kind = 'genre'" +
			" AND tags.name COLLATE NOCASE IN (" + list(builder, filter.Genres) + "))"})
	}

	if len(filter.Years) > 0 {
		conditions = append(conditions, filterCondition{field: library.FacetYear, sql: "items.year IN (" + list(builder, filter.Years) + ")"})
	}

	if len(filter.Decades) > 0 {
		conditions = append(conditions, filterCondition{field: library.FacetDecade, sql: "items.year - items.year % 10 IN (" + list(builder, filter.Decades) + ")"})
	}

	if len(filter.ContentRatings) > 0 {
		conditions = append(conditions, filterCondition{field: library.FacetContentRating, sql: "items.content_rating IN (" + list(builder, filter.ContentRatings) + ")"})
	}

	if condition, found := matchStateCondition(builder, filter.MatchState); found {
		conditions = append(conditions, condition)
	}

	return conditions
}

// matchStateCondition narrows the items to one match state. An item without an item_match
// row counts as unmatched (migration 00005), so "unmatched" is the absence of any other
// state. It reports false for MatchFilterAll.
func matchStateCondition(builder *sqlBuilder, state library.MatchStateFilter) (filterCondition, bool) {
	switch state {
	case library.MatchFilterAll:
		return filterCondition{}, false
	case library.MatchFilterUnmatched:
		return filterCondition{field: noFacetField, sql: "items.id NOT IN (SELECT item_match.item_id FROM item_match" +
			" WHERE item_match.state <> " + builder.arg(string(library.MatchFilterUnmatched)) + ")"}, true
	case library.MatchFilterMatched, library.MatchFilterDisabled:
		return filterCondition{field: noFacetField, sql: "items.id IN (SELECT item_match.item_id FROM item_match" +
			" WHERE item_match.state = " + builder.arg(string(state)) + ")"}, true
	default:
		// The HTTP adapter rejects other values; an unknown state matches nothing rather
		// than everything.
		return filterCondition{field: noFacetField, sql: neverMatches}, true
	}
}

// fileConditions filter by the primary files and videos of the deciding members (facts).
func (scope listingScope) fileConditions(filter library.ListingFilter) []filterCondition {
	builder := scope.builder

	var conditions []filterCondition

	if len(filter.Resolutions) > 0 {
		conditions = append(conditions, factsCondition(library.FacetResolution, resolutionSQL()+" IN ("+list(builder, filter.Resolutions)+")"))
	}

	if len(filter.VideoCodecs) > 0 {
		conditions = append(conditions, factsCondition(library.FacetVideoCodec, "video_codec IN ("+list(builder, filter.VideoCodecs)+")"))
	}

	if len(filter.Containers) > 0 {
		conditions = append(conditions, factsCondition(library.FacetContainer, "container IN ("+list(builder, filter.Containers)+")"))
	}

	return conditions
}

func factsCondition(field library.FacetField, condition string) filterCondition {
	return filterCondition{field: field, sql: "items.id IN (SELECT top_id FROM facts WHERE " + condition + ")", needs: tableNeeds{members: true, decider: true, facts: true}}
}

// streamConditions filter by any stream of a given kind in the deciding files.
func (scope listingScope) streamConditions(filter library.ListingFilter) []filterCondition {
	builder := scope.builder

	var conditions []filterCondition

	if len(filter.AudioCodecs) > 0 {
		conditions = append(conditions, streamCondition(library.FacetAudioCodec, "audio", "codec", list(builder, filter.AudioCodecs)))
	}

	if len(filter.AudioLanguages) > 0 {
		conditions = append(conditions, streamCondition(library.FacetAudioLanguage, "audio", "language", list(builder, filter.AudioLanguages)))
	}

	if len(filter.SubtitleLanguages) > 0 {
		conditions = append(conditions, streamCondition(library.FacetSubtitleLanguage, "subtitle", "language", list(builder, filter.SubtitleLanguages)))
	}

	return conditions
}

func streamCondition(field library.FacetField, kind, column, placeholders string) filterCondition {
	return filterCondition{field: field, needs: tableNeeds{members: true, decider: true}, sql: "items.id IN (SELECT decider.top_id FROM decider" +
		" CROSS JOIN media_streams AS streams ON streams.media_file_id = decider.file_id" +
		" WHERE streams.kind = '" + kind + "' AND streams." + column + " IN (" + placeholders + "))"}
}

// quickCondition returns the condition of the quick filter; it reports false for none.
func (scope listingScope) quickCondition(filter library.ListingFilter) (filterCondition, bool) {
	switch filter.Quick {
	case library.QuickHDR:
		return factsCondition(noFacetField, "hdr <> ''"), true
	case library.QuickUHD:
		return factsCondition(noFacetField, resolutionSQL()+" IN ('"+string(library.Resolution4K)+"', '"+string(library.Resolution8K)+"')"), true
	case library.QuickUnavailable:
		return factsCondition(noFacetField, "probe_error IS NOT NULL OR root_id IN ("+list(scope.builder, filter.OfflineRootIDs)+")"), true
	case library.QuickDuplicates:
		return filterCondition{needs: tableNeeds{members: true}, sql: "items.id IN (SELECT top_id FROM members" +
			" WHERE (SELECT COUNT(*) FROM media_files WHERE media_files.item_id = members.item_id) > 1)"}, true
	case library.QuickDolbyVision, library.QuickHDR10Plus, library.QuickAtmos:
		return filterCondition{sql: neverMatches}, true
	case library.QuickAll:
		return filterCondition{}, false
	default:
		return filterCondition{}, false
	}
}

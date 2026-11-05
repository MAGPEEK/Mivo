package sqlite

import (
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// facetSpec is the grouped SELECT of one facet field over a table of listed items (id, year,
// content_rating): the value and item_count columns, their source, condition, grouping and
// order.
type facetSpec struct {
	columns, from, where, groupBy, orderBy string
}

// textOrderOf orders text values case-insensitively, then exactly, so the order is total.
func textOrderOf(column string) string {
	return column + " COLLATE NOCASE, " + column
}

// facetsQuery returns the SQL of every requested facet in one statement: rows of
// (field_index, value, item_count) ordered by field and value, at most rowLimit per field.
//
// flags evaluates every filter condition once per top-level item; the items listed for a
// field are those passing every condition but the field's own (listed_<n>). One statement
// computes the files (decider, facts) once for all fields, which is what keeps the facet
// counts of a large library fast.
func facetsQuery(target listingTarget, fields []library.FacetField, filter library.ListingFilter, rowLimit int) (string, []any) {
	builder := &sqlBuilder{}
	scope := newListingScope(builder, target)
	conditions := scope.filterConditions(filter)
	needs := needsOf(conditions)

	flags := []string{"items.id", "items.year", "items.content_rating"}
	for index, condition := range conditions {
		flags = append(flags, "("+condition.sql+") AS "+conditionColumn(index))
	}

	tables := []string{"flags AS MATERIALIZED (SELECT " + strings.Join(flags, ", ") + " FROM items WHERE " + scope.base + ")"}
	arms := make([]string, 0, len(fields))
	limit := builder.arg(rowLimit)

	for index, field := range fields {
		listed := "listed_" + strconv.Itoa(index)
		tables = append(tables, listed+" AS (SELECT id, year, content_rating FROM flags WHERE "+flagsWithout(conditions, field)+")")
		arms = append(arms, facetArmSQL(index, facetSpecOf(field, listed), limit))
		needs = needs.union(facetNeeds(field))
	}

	builder.writeWith(append(scope.commonTables(needs), tables...))
	builder.write("SELECT field_index, value, item_count FROM (", strings.Join(arms, "\nUNION ALL "), ") ORDER BY field_index, position")

	return builder.text.String(), builder.args
}

func conditionColumn(index int) string {
	return "condition_" + strconv.Itoa(index)
}

// flagsWithout returns the condition of the flags rows that pass every filter but the
// field's own.
func flagsWithout(conditions []filterCondition, field library.FacetField) string {
	parts := []string{"1"}

	for index, condition := range conditions {
		if condition.field != field {
			parts = append(parts, conditionColumn(index))
		}
	}

	return strings.Join(parts, " AND ")
}

func facetArmSQL(index int, spec facetSpec, limit string) string {
	return "SELECT " + strconv.Itoa(index) + " AS field_index, value, item_count, position FROM (SELECT " + spec.columns +
		", row_number() OVER (ORDER BY " + spec.orderBy + ") AS position FROM " + spec.from +
		" WHERE " + spec.where + " GROUP BY " + spec.groupBy + ") WHERE position <= " + limit
}

// facetNeeds returns the common tables a facet field reads.
func facetNeeds(field library.FacetField) tableNeeds {
	switch field {
	case library.FacetResolution, library.FacetVideoCodec, library.FacetContainer:
		return tableNeeds{members: true, decider: true, facts: true}
	case library.FacetAudioCodec, library.FacetAudioLanguage, library.FacetSubtitleLanguage:
		return tableNeeds{members: true, decider: true}
	case library.FacetGenre, library.FacetYear, library.FacetDecade, library.FacetContentRating:
		return tableNeeds{}
	default:
		return tableNeeds{}
	}
}

// facetSpecOf returns the grouped SELECT of a facet field over the listed table.
func facetSpecOf(field library.FacetField, listed string) facetSpec {
	inListed := " IN (SELECT id FROM " + listed + ")"

	switch field {
	case library.FacetGenre:
		return genreFacetSpec(inListed)
	case library.FacetYear:
		return listedFacetSpec(listed, "year", "year > 0")
	case library.FacetDecade:
		return listedFacetSpec(listed, "year - year % 10", "year > 0")
	case library.FacetContentRating:
		return facetSpec{
			columns: "content_rating AS value, COUNT(*) AS item_count", from: listed,
			where: "content_rating <> ''", groupBy: "content_rating", orderBy: textOrderOf("content_rating"),
		}
	case library.FacetResolution:
		return facetSpec{
			columns: "resolution AS value, COUNT(DISTINCT top_id) AS item_count",
			from:    "(SELECT top_id, " + resolutionSQL() + " AS resolution FROM facts WHERE top_id" + inListed + ")",
			where:   "resolution IS NOT NULL", groupBy: "resolution", orderBy: resolutionRankSQL("resolution"),
		}
	case library.FacetVideoCodec:
		return factsFacetSpec("video_codec", inListed)
	case library.FacetContainer:
		return factsFacetSpec("container", inListed)
	case library.FacetAudioCodec:
		return streamFacetSpec("audio", "codec", inListed)
	case library.FacetAudioLanguage:
		return streamFacetSpec("audio", "language", inListed)
	case library.FacetSubtitleLanguage:
		return streamFacetSpec("subtitle", "language", inListed)
	default:
		// Unreachable: the use case passes only library.FacetFields values (facetFieldsOf) and
		// the HTTP adapter rejects unknown fields with 400. A field added later needs its case.
		return streamFacetSpec("subtitle", "language", inListed)
	}
}

func genreFacetSpec(inListed string) facetSpec {
	return facetSpec{
		columns: "MIN(tags.name) AS value, COUNT(DISTINCT item_tags.item_id) AS item_count",
		from:    "item_tags JOIN tags ON tags.id = item_tags.tag_id",
		where:   "tags.kind = 'genre' AND item_tags.item_id" + inListed,
		groupBy: "tags.name COLLATE NOCASE", orderBy: textOrderOf("MIN(tags.name)"),
	}
}

// listedFacetSpec counts a number expression of the listed items, ordered numerically.
func listedFacetSpec(listed, expression, where string) facetSpec {
	return facetSpec{
		columns: "CAST(" + expression + " AS TEXT) AS value, COUNT(*) AS item_count", from: listed,
		where: where, groupBy: expression, orderBy: expression,
	}
}

func factsFacetSpec(column, inListed string) facetSpec {
	return facetSpec{
		columns: column + " AS value, COUNT(DISTINCT top_id) AS item_count", from: "facts",
		where: "top_id" + inListed + " AND " + column + " <> ''", groupBy: column, orderBy: textOrderOf(column),
	}
}

func streamFacetSpec(kind, column, inListed string) facetSpec {
	qualified := "streams." + column

	return facetSpec{
		columns: qualified + " AS value, COUNT(DISTINCT decider.top_id) AS item_count",
		from:    "decider CROSS JOIN media_streams AS streams ON streams.media_file_id = decider.file_id",
		where:   "decider.top_id" + inListed + " AND streams.kind = '" + kind + "' AND " + qualified + " <> ''",
		groupBy: qualified, orderBy: textOrderOf(qualified),
	}
}

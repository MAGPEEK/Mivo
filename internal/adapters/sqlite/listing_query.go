package sqlite

import (
	"github.com/magpeek/mivo/internal/domain/library"
)

// listingRequest is one page of a library listing: the target library, the query, the key
// after which the page starts (hasAfter false for the first page) and the rows to read.
type listingRequest struct {
	target   listingTarget
	query    library.ListingQuery
	after    listingKey
	hasAfter bool
	rowLimit int
}

// pageQuery returns the SQL of one listing page: rows of (id, sort_title, sort_value) in
// listing order. The title sort reads the partial index items_top_level in order.
//
// A filter that reads the files also returns the total count as a fourth column (counted
// true): the matching items are computed once and both counted and paged, because computing
// the files twice would double the cost. Without such a filter the count is a cheap query of
// its own (countQuery) and the page keeps its index scan.
func pageQuery(request listingRequest) (text string, args []any, counted bool) {
	builder := &sqlBuilder{}
	scope := newListingScope(builder, request.target)
	order := request.query.Order
	value := scope.sortValueSQL(order)
	titleOnly := order.Key() == library.SortByTitle
	conditions := scope.filterConditions(request.query.Filter)
	needs := needsOf(conditions)
	counted = needs.decider

	// Counted, listed is read twice and must be computed once; otherwise it is read once and
	// SQLite folds it into the page query, which keeps the index scan of the title sort.
	materialized := ""
	if counted {
		materialized = "MATERIALIZED "
	}

	builder.writeWith(append(scope.commonTables(needs), "listed AS "+materialized+"(SELECT items.id AS id, items.sort_title AS sort_title, "+
		value.sql+" AS sort_value FROM items WHERE "+joinConditions(scope.base, conditions)+")"))

	if counted {
		builder.write("SELECT id, sort_title, sort_value, (SELECT COUNT(*) FROM listed) FROM listed")
	} else {
		builder.write("SELECT id, sort_title, sort_value FROM listed")
	}

	if request.hasAfter {
		builder.write(" WHERE ", keysetSQL(builder, keysetRequest{
			key: request.after, value: value, titleOnly: titleOnly, descending: order.IsDescending(),
		}))
	}

	builder.write(" ORDER BY ", orderBySQL(value, titleOnly, order.IsDescending()), " LIMIT ", builder.arg(request.rowLimit))

	return builder.text.String(), builder.args, counted
}

type keysetRequest struct {
	key        listingKey
	value      sortValue
	titleOnly  bool
	descending bool
}

// keysetSQL returns the condition of the rows after the key in listing order. NULL values
// come last in both directions, so after a NULL key only NULL rows follow, and every NULL row
// follows a key with a value.
func keysetSQL(builder *sqlBuilder, request keysetRequest) string {
	comparison := " > "
	if request.descending {
		comparison = " < "
	}

	title, id := builder.arg(request.key.sortTitle), builder.arg(request.key.id)
	tieBreak := "(sort_title, id)" + comparison + "(" + title + ", " + id + ")"

	switch {
	case request.titleOnly:
		// Spelled out instead of a row value, so SQLite scans items_top_level as a range.
		bound := " >= "
		if request.descending {
			bound = " <= "
		}

		return "sort_title" + bound + title + " AND (sort_title" + comparison + title + " OR id" + comparison + id + ")"
	case request.value.nullable && request.key.value == nil:
		return "sort_value IS NULL AND " + tieBreak
	}

	after := "(sort_value, sort_title, id)" + comparison + "(" + builder.arg(request.key.value) + ", " + title + ", " + id + ")"
	if request.value.nullable {
		return "(sort_value IS NULL OR " + after + ")"
	}

	return after
}

// countQuery returns the SQL that counts the items matching the filter.
func countQuery(target listingTarget, filter library.ListingFilter) (string, []any) {
	builder := &sqlBuilder{}
	scope := newListingScope(builder, target)
	where := scope.writeWithFilter(filter)

	builder.write("SELECT COUNT(*) FROM items WHERE ", where)

	return builder.text.String(), builder.args
}

// firstCharactersQuery returns the SQL of the first characters of the sort titles in
// title-sort order with their counts. substr counts characters and GROUP BY compares with
// the BINARY collation of ORDER BY sort_title, so the groups follow the listing order.
func firstCharactersQuery(target listingTarget, filter library.ListingFilter, descending bool) (string, []any) {
	builder := &sqlBuilder{}
	scope := newListingScope(builder, target)
	direction := ""

	if descending {
		direction = " DESC"
	}

	where := scope.writeWithFilter(filter)
	builder.write(
		"SELECT substr(items.sort_title, 1, 1) AS first_character, COUNT(*) FROM items WHERE ",
		where, " GROUP BY first_character ORDER BY first_character", direction,
	)

	return builder.text.String(), builder.args
}

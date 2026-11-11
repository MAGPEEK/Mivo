package sqlite

import (
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/access"
)

// The viewer restriction of ADR 0032 section 7 as SQL. Every catalog read filters with the
// functions of this file (listing, facets, letters and folders since M7-12; rows, summaries
// and counts since M7-13), so the rule "allowed library, the item and every ancestor up to the
// maximum age, unrated chains hidden or not" is written once. The ancestors are not walked
// here: the adapter stores the visibility of the whole chain on each item (migration 00016). A restriction that limits nothing adds no SQL text, so an unrestricted read keeps its
// statement and its query plan.

// Columns the restriction compares, appended to an items alias.
const (
	libraryColumn = ".library_id"
	// visibilityAgeColumn is the highest own bucket of the item and its ancestors, NULL when
	// none of them is rated.
	visibilityAgeColumn = ".visibility_age_rating"
	// visibilityUnratedColumn is 1 when the item or an ancestor has no effective bucket.
	visibilityUnratedColumn = ".visibility_unrated"
)

// libraryAllowed reports whether the restriction allows the library. AllLibraries and
// DeniesAll are checked before LibraryIDs, whose empty list means either of them.
func libraryAllowed(restriction access.Restriction, libraryID string) bool {
	switch {
	case restriction.AllLibraries():
		return true
	case restriction.DeniesAll():
		return false
	default:
		return slices.Contains(restriction.LibraryIDs(), libraryID)
	}
}

// libraryConditionSQL returns the condition that the library ID in column is allowed: empty
// for all libraries, a condition that never matches for none.
func libraryConditionSQL(builder *sqlBuilder, restriction access.Restriction, column string) string {
	switch {
	case restriction.AllLibraries():
		return ""
	case restriction.DeniesAll():
		return neverMatches
	default:
		return column + " IN (" + list(builder, restriction.LibraryIDs()) + ")"
	}
}

// ageConditionSQL returns the condition that the items row of the alias and every ancestor
// of it pass the content limit (ADR 0032 section 7, ancestor visibility); empty without a
// limit. The alias may carry a unary plus, which keeps the condition out of SQLite's index
// choice (see visibleSQL).
//
// An unrated chain has no bucket to compare or is flagged unrated below a rated member, so
// hiding unrated titles tests the flag and showing them lets a NULL bucket pass.
func ageConditionSQL(builder *sqlBuilder, restriction access.Restriction, alias string) string {
	maxAge, limited := restriction.MaxAge()
	bucket, rated := alias+visibilityAgeColumn, alias+visibilityUnratedColumn+" = 0"

	switch {
	case limited && restriction.HideUnrated():
		return bucket + " <= " + builder.arg(int64(maxAge)) + " AND " + rated
	case limited:
		return "(" + bucket + " IS NULL OR " + bucket + " <= " + builder.arg(int64(maxAge)) + ")"
	case restriction.HideUnrated():
		return rated
	default:
		return ""
	}
}

// restrictionConditionSQL returns the whole restriction on the items row of the given alias,
// its parts joined with AND; empty when the restriction limits nothing.
func restrictionConditionSQL(builder *sqlBuilder, restriction access.Restriction, alias string) string {
	conditions := []string{
		libraryConditionSQL(builder, restriction, alias+libraryColumn),
		ageConditionSQL(builder, restriction, alias),
	}

	return strings.Join(slices.DeleteFunc(conditions, func(condition string) bool { return condition == "" }), " AND ")
}

// andSQL appends a condition to a WHERE clause; nothing for an empty condition.
func andSQL(condition string) string {
	if condition == "" {
		return ""
	}

	return " AND " + condition
}

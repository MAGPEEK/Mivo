package sqlite

import (
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/access"
)

// The viewer restriction of ADR 0032 section 7 as SQL. Every catalog read filters with the
// functions of this file (listing, facets, letters and folders since M7-12), so the rule
// "allowed library, effective age bucket up to the maximum, unrated hidden or not" is written
// once. A restriction that limits nothing adds no SQL text, so an unrestricted read keeps its
// statement and its query plan.

// Columns the restriction compares, appended to an items alias.
const (
	libraryColumn      = ".library_id"
	effectiveAgeColumn = ".effective_age_rating"
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

// ageConditionSQL returns the condition that the effective age bucket in column passes the
// content limit; empty without a limit. NULL is "unrated": a plain comparison drops it, so
// hiding unrated titles needs no extra term and showing them needs the IS NULL arm.
func ageConditionSQL(builder *sqlBuilder, restriction access.Restriction, column string) string {
	maxAge, limited := restriction.MaxAge()

	switch {
	case limited && restriction.HideUnrated():
		return column + " <= " + builder.arg(int64(maxAge))
	case limited:
		return "(" + column + " IS NULL OR " + column + " <= " + builder.arg(int64(maxAge)) + ")"
	case restriction.HideUnrated():
		return column + " IS NOT NULL"
	default:
		return ""
	}
}

// restrictionConditionSQL returns the whole restriction on the items row of the given alias,
// its parts joined with AND; empty when the restriction limits nothing.
func restrictionConditionSQL(builder *sqlBuilder, restriction access.Restriction, alias string) string {
	conditions := []string{
		libraryConditionSQL(builder, restriction, alias+libraryColumn),
		ageConditionSQL(builder, restriction, alias+effectiveAgeColumn),
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

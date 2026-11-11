package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/access"
)

// Counts under a viewer restriction (ADR 0032 section 7): child_count and leaf_count of a
// summary must cover visible members only, or a show whose episodes are hidden would tell how
// many there are. The generated listing queries count every member; for a restricted viewer
// the statement of this file replaces their counts and drops the items the viewer may not
// see. An unrestricted read never runs it, so its cost stays what it was.

// visibleCountsQuery returns the SQL of (id, child_count, leaf_count) of the given items that
// the restriction allows, counting like ListItemsWithCounts but only the children and
// episodes that pass the content limit. An episode's stored visibility covers its season, so
// the episodes of a hidden season are not counted.
func visibleCountsQuery(restriction access.Restriction, itemIDs []string) (string, []any) {
	builder := &sqlBuilder{}

	builder.write("SELECT items.id,",
		" (SELECT COUNT(*) FROM items AS children WHERE children.parent_id = items.id AND children.kind <> 'extra'",
		andSQL(ageConditionSQL(builder, restriction, "children")), "),",
		" (SELECT COUNT(*) FROM items AS seasons JOIN items AS episodes ON episodes.parent_id = seasons.id",
		" WHERE seasons.parent_id = items.id AND seasons.kind = 'season' AND episodes.kind = 'episode'",
		andSQL(ageConditionSQL(builder, restriction, "episodes")), ")",
		// The unary plus keeps the lookup on the primary key: without it SQLite reads every
		// visible item of the library through items_visibility to find a page of IDs.
		" FROM items WHERE items.id IN (", list(builder, itemIDs), ")",
		andSQL(restrictionConditionSQL(builder, restriction, "+items")))

	return builder.text.String(), builder.args
}

// restrictListedItems returns the listed items the restriction allows, with the counts of
// their visible members. A restriction that limits nothing returns the items unchanged
// without a query.
func restrictListedItems(
	ctx context.Context, transaction *sql.Tx, listed map[string]listedItem, restriction access.Restriction,
) (map[string]listedItem, error) {
	if restrictionConditionSQL(&sqlBuilder{}, restriction, "items") == "" || len(listed) == 0 {
		return listed, nil
	}

	itemIDs := make([]string, 0, len(listed))
	for id := range listed {
		itemIDs = append(itemIDs, id)
	}

	text, args := visibleCountsQuery(restriction, itemIDs)

	rows, err := transaction.QueryContext(ctx, text, args...)
	if err != nil {
		return nil, fmt.Errorf("count visible members: %w", err)
	}

	defer closeRows(rows)

	visible := make(map[string]listedItem, len(listed))

	for rows.Next() {
		var (
			id      string
			entry   listedItem
			counted listedItem
		)

		if err := rows.Scan(&id, &counted.childCount, &counted.leafCount); err != nil {
			return nil, fmt.Errorf("count visible members: %w", err)
		}

		entry = listed[id]
		entry.childCount, entry.leafCount = counted.childCount, counted.leafCount
		visible[id] = entry
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("count visible members: %w", err)
	}

	return visible, nil
}

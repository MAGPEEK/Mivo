package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain/access"
)

// ageRatingBackfillBatch bounds the stale rows one backfill transaction recomputes, so a first
// run on a large library does not hold the writer for the whole recomputation.
const ageRatingBackfillBatch = 500

// BackfillAgeRatings recomputes the age buckets of every item whose buckets were not computed
// from its current content_rating (ADR 0032 section 7): all items after migration 00014, and
// rows a writer changed without the adapter. It is idempotent and, when nothing is stale, costs
// one lookup in a partial index. Each batch runs in its own write transaction.
func (catalog *LibraryCatalog) BackfillAgeRatings(ctx context.Context) error {
	for {
		recomputed := 0

		err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
			var err error

			recomputed, err = backfillAgeRatingBatch(ctx, queries)

			return err
		})
		if err != nil {
			return fmt.Errorf("backfill age ratings: %w", err)
		}

		if recomputed < ageRatingBackfillBatch {
			return nil
		}
	}
}

// backfillAgeRatingBatch recomputes up to one batch of stale rows and reports how many it
// found. A row's subtree is propagated after its own bucket is set, so the order of the rows
// does not matter: each propagation recomputes the whole subtree from the current buckets.
func backfillAgeRatingBatch(ctx context.Context, queries *sqlcgen.Queries) (int, error) {
	stale, err := queries.ListStaleAgeRatings(ctx, ageRatingBackfillBatch)
	if err != nil {
		return 0, fmt.Errorf("list stale age ratings: %w", err)
	}

	for _, row := range stale {
		if err := storeAgeRating(ctx, queries, row.ID, row.ContentRating); err != nil {
			return 0, err
		}
	}

	return len(stale), nil
}

// storeAgeRating sets the bucket of an item's content rating and the effective bucket and
// ancestor visibility of the item and its subtree. It runs in the caller's transaction, so a
// show's rating and the buckets its seasons and episodes inherit change together.
func storeAgeRating(ctx context.Context, queries *sqlcgen.Queries, itemID, contentRating string) error {
	err := queries.SetItemAgeRating(ctx, sqlcgen.SetItemAgeRatingParams{
		AgeRating:       ageRatingColumn(contentRating),
		AgeRatingSource: sql.NullString{String: contentRating, Valid: true},
		ID:              itemID,
	})
	if err != nil {
		return fmt.Errorf("set age rating of item %q: %w", itemID, err)
	}

	return propagateEffectiveAgeRating(ctx, queries, itemID)
}

// propagateEffectiveAgeRating writes the effective bucket and the ancestor visibility
// (migration 00016) of every row in the item's subtree whose stored values are outdated. Any
// write that changes an item's own bucket or its parent must end here in the same
// transaction, or a descendant could stay visible below a hidden ancestor: the triggers of
// migration 00017 close only the changed row itself until the backfill runs.
func propagateEffectiveAgeRating(ctx context.Context, queries *sqlcgen.Queries, itemID string) error {
	outdated, err := queries.ListOutdatedAgeRatings(ctx, itemID)
	if err != nil {
		return fmt.Errorf("compute effective age ratings below item %q: %w", itemID, err)
	}

	for _, row := range outdated {
		visibility, err := nullableBucket(row.Visibility)
		if err != nil {
			return fmt.Errorf("visibility of item %q: %w", row.ID, err)
		}

		err = queries.SetEffectiveAgeRating(ctx, sqlcgen.SetEffectiveAgeRatingParams{
			EffectiveAgeRating: row.Effective, VisibilityAgeRating: visibility,
			VisibilityUnrated: row.Unrated, ID: row.ID,
		})
		if err != nil {
			return fmt.Errorf("set effective age rating of item %q: %w", row.ID, err)
		}
	}

	return nil
}

// nullableBucket converts a computed bucket, which sqlc cannot type (a CASE over two nullable
// columns): the driver yields an int64 or nil. Any other type is an error and never NULL,
// because a NULL bucket passes every maximum age.
func nullableBucket(value any) (sql.NullInt64, error) {
	switch bucket := value.(type) {
	case nil:
		return sql.NullInt64{}, nil
	case int64:
		return sql.NullInt64{Int64: bucket, Valid: true}, nil
	default:
		return sql.NullInt64{}, fmt.Errorf("computed bucket has type %T, want an integer or NULL", value)
	}
}

// ageRatingColumn is the stored form of a rating's bucket; NULL means unknown, which a content
// limit with hideUnrated hides.
func ageRatingColumn(contentRating string) sql.NullInt64 {
	bucket, known := access.ParseRating(contentRating)
	if !known {
		return sql.NullInt64{}
	}

	return sql.NullInt64{Int64: int64(bucket), Valid: true}
}

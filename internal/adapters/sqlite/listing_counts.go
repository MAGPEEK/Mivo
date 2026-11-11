package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
)

// CountFacets returns one facet per requested field, each counted under the filter without
// that field's own values, with one statement (see facetsQuery). A field with more than
// applibrary.MaxFacetValues values is cut and marked truncated. An unknown library wraps
// domain.ErrNotFound.
func (catalog *CatalogReader) CountFacets(ctx context.Context, libraryID string, request applibrary.FacetRequest) ([]applibrary.Facet, error) {
	facets := make([]applibrary.Facet, len(request.Fields))
	for index, field := range request.Fields {
		facets[index] = applibrary.Facet{Field: field, Values: []applibrary.FacetValue{}}
	}

	if len(request.Fields) == 0 {
		return facets, nil
	}

	err := inListingTransaction(ctx, catalog.reader, func(transaction *sql.Tx, _ *sqlcgen.Queries) error {
		target, err := completeTarget(ctx, transaction, listingTarget{
			libraryID: libraryID, kind: request.Filter.Kind, restriction: request.Restriction,
		})
		if err != nil {
			return err
		}

		text, args := facetsQuery(target, request.Fields, request.Filter, applibrary.MaxFacetValues+1)

		return readFacets(ctx, transaction, queryText{text: text, args: args}, facets)
	})
	if err != nil {
		return nil, fmt.Errorf("count facets of library %q: %w", libraryID, err)
	}

	return facets, nil
}

// queryText is a statement built at run time with its arguments.
type queryText struct {
	text string
	args []any
}

// readFacets fills facets (indexed like the requested fields) from the rows of facetsQuery.
func readFacets(ctx context.Context, transaction *sql.Tx, query queryText, facets []applibrary.Facet) error {
	rows, err := transaction.QueryContext(ctx, query.text, query.args...)
	if err != nil {
		return fmt.Errorf("read facets: %w", err)
	}

	defer closeRows(rows)

	for rows.Next() {
		var (
			index int
			value applibrary.FacetValue
		)

		if err := rows.Scan(&index, &value.Value, &value.Count); err != nil {
			return fmt.Errorf("read facet row: %w", err)
		}

		facet := &facets[index]
		if len(facet.Values) == applibrary.MaxFacetValues {
			facet.Truncated = true

			continue
		}

		facet.Values = append(facet.Values, value)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("read facet rows: %w", err)
	}

	return nil
}

// CountFirstCharacters returns the first characters of the sort titles of the filtered
// listing in title-sort order, with the number of items per character. An unknown library
// wraps domain.ErrNotFound.
func (catalog *CatalogReader) CountFirstCharacters(ctx context.Context, libraryID string, request applibrary.LetterRequest) ([]applibrary.CharacterCount, error) {
	var counts []applibrary.CharacterCount

	err := inListingTransaction(ctx, catalog.reader, func(transaction *sql.Tx, _ *sqlcgen.Queries) error {
		target, err := completeTarget(ctx, transaction, listingTarget{
			libraryID: libraryID, kind: request.Filter.Kind, restriction: request.Restriction,
		})
		if err != nil {
			return err
		}

		text, args := firstCharactersQuery(target, request.Filter, request.Descending)

		counts, err = readCharacterCounts(ctx, transaction, queryText{text: text, args: args})

		return err
	})
	if err != nil {
		return nil, fmt.Errorf("count first characters of library %q: %w", libraryID, err)
	}

	return counts, nil
}

func readCharacterCounts(ctx context.Context, transaction *sql.Tx, query queryText) ([]applibrary.CharacterCount, error) {
	rows, err := transaction.QueryContext(ctx, query.text, query.args...)
	if err != nil {
		return nil, fmt.Errorf("read first characters: %w", err)
	}

	defer closeRows(rows)

	var counts []applibrary.CharacterCount

	for rows.Next() {
		var count applibrary.CharacterCount
		if err := rows.Scan(&count.Character, &count.Count); err != nil {
			return nil, fmt.Errorf("read first character row: %w", err)
		}

		counts = append(counts, count)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read first character rows: %w", err)
	}

	return counts, nil
}

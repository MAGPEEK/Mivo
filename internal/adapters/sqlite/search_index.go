package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	appsearch "github.com/magpeek/mivo/internal/app/search"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

var _ appsearch.Index = (*SearchIndex)(nil)

// searchRankSQL orders hits by bm25 with per-column weights (title, original title, sort
// title, people, genres; FTS5 ranks lower values first): a word in the title counts most, so
// "matrix" lists The Matrix before a film with an actor named Matrix. Ties go to the item
// indexed first, so equal hits come back in a stable order; a tie-break by sort title would
// read the items row of every match, which doubles the time of a search with 50,000 matches.
const searchRankSQL = `bm25(item_search, 10.0, 5.0, 5.0, 2.0, 1.0), search_docs.doc_id`

// SearchIndex answers full-text searches over the catalog (task M5-14) from the FTS5 table
// item_search, which migration 00010 keeps in sync with the catalog through triggers.
type SearchIndex struct {
	reader *sql.DB
}

// NewSearchIndex returns the index on db. db must be migrated before the first call.
func NewSearchIndex(db *DB) *SearchIndex {
	return &SearchIndex{reader: db.reader}
}

// Search returns up to limit hits for query in the libraries and kinds of filter, best match
// first (see appsearch.Index). A limit outside 1..appsearch.MaxLimit or an unknown kind wraps
// domain.ErrInvalid.
func (index *SearchIndex) Search(ctx context.Context, query string, filter appsearch.Filter, limit int) ([]appsearch.Hit, error) {
	if err := validateSearchRequest(filter, limit); err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	expression := searchMatchExpression(query)
	if expression == "" || len(filter.LibraryIDs) == 0 {
		return []appsearch.Hit{}, nil
	}

	statement, args := searchStatement(expression, filter, limit)

	rows, err := index.reader.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("search: query index: %w", err)
	}

	hits, err := scanSearchHits(rows)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	return hits, nil
}

func validateSearchRequest(filter appsearch.Filter, limit int) error {
	if limit < 1 || limit > appsearch.MaxLimit {
		return fmt.Errorf("limit %d outside 1..%d: %w", limit, appsearch.MaxLimit, domain.ErrInvalid)
	}

	for _, kind := range filter.Kinds {
		if !kind.IsValid() {
			return fmt.Errorf("unknown item kind %q: %w", kind, domain.ErrInvalid)
		}
	}

	return nil
}

// searchStatement builds the query of one search. Only fixed SQL text and placeholders go into
// it; the match expression, library IDs and kinds are bound parameters.
func searchStatement(expression string, filter appsearch.Filter, limit int) (string, []any) {
	var builder sqlBuilder

	builder.write(`SELECT search_docs.item_id, search_docs.library_id, search_docs.kind
FROM item_search
JOIN search_docs ON search_docs.doc_id = item_search.rowid
WHERE item_search MATCH `, builder.arg(expression),
		` AND search_docs.library_id IN (`, list(&builder, filter.LibraryIDs), `)`)

	if len(filter.Kinds) > 0 {
		builder.write(` AND search_docs.kind IN (`, list(&builder, filter.Kinds), `)`)
	}

	builder.write(` ORDER BY `, searchRankSQL, ` LIMIT `, builder.arg(limit))

	return builder.text.String(), builder.args
}

func scanSearchHits(rows *sql.Rows) (hits []appsearch.Hit, err error) {
	defer func() {
		if closeErr := rows.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close hits: %w", closeErr)
		}
	}()

	hits = []appsearch.Hit{}

	for rows.Next() {
		var hit appsearch.Hit

		var kind string
		if err := rows.Scan(&hit.ItemID, &hit.LibraryID, &kind); err != nil {
			return nil, fmt.Errorf("read hit: %w", err)
		}

		if hit.Kind, err = library.ParseItemKind(kind); err != nil {
			return nil, undecodable(fmt.Sprintf("read hit %q", hit.ItemID), err)
		}

		hits = append(hits, hit)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read hits: %w", err)
	}

	return hits, nil
}

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// The viewer restriction for the generated queries (children, detail, music and photo reads).
// Their SQL is fixed, so the content limit reaches them as two bound values and each query
// repeats the conditions of ageConditionSQL in bound form (see queries/catalog_read.sql). The
// library part needs no SQL: these reads start from one item or library, which is checked
// here, and everything below an item lies in its library.

// contentLimit is the content limit of a restriction as the generated queries bind it. Its
// zero value is the strictest limit, so a read that forgets to set it shows nothing unrated
// and of the rated titles only those for all ages.
type contentLimit struct {
	// maxAge is the highest bucket the viewer may see; access.Age18 without a limit, which
	// every stored bucket passes.
	maxAge int64
	// allowUnrated is 1 unless unrated titles are hidden.
	allowUnrated int64
}

// contentLimitOf returns the bound form of the restriction's content limit.
func contentLimitOf(restriction access.Restriction) contentLimit {
	limit := contentLimit{maxAge: int64(access.Age18), allowUnrated: storedFlag(!restriction.HideUnrated())}
	if maxAge, limited := restriction.MaxAge(); limited {
		limit.maxAge = int64(maxAge)
	}

	return limit
}

// catalogView is what a read of the generated queries may see: the allowed libraries and the
// content limit of one viewer. Its zero value sees nothing.
type catalogView struct {
	restriction access.Restriction
	content     contentLimit
}

func viewOf(restriction access.Restriction) catalogView {
	return catalogView{restriction: restriction, content: contentLimitOf(restriction)}
}

// item reads an item the viewer may see. An unknown item, a hidden one and a descendant of a
// hidden one all wrap domain.ErrNotFound, so the answer does not tell them apart (ADR 0032
// section 7).
func (view catalogView) item(ctx context.Context, queries *sqlcgen.Queries, itemID string) (sqlcgen.Item, error) {
	item, err := queries.GetVisibleItem(ctx, sqlcgen.GetVisibleItemParams{
		ItemID: itemID, MaxAge: view.content.maxAge, AllowUnrated: view.content.allowUnrated,
	})
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !libraryAllowed(view.restriction, item.LibraryID)) {
		return sqlcgen.Item{}, fmt.Errorf("item %q: %w", itemID, domain.ErrNotFound)
	}

	if err != nil {
		return sqlcgen.Item{}, fmt.Errorf("read item %q: %w", itemID, err)
	}

	return item, nil
}

// itemOfKind is item with the kind check of the catalog's itemOfKind: an item of another
// kind wraps domain.ErrInvalid, but only when the viewer may see it.
func (view catalogView) itemOfKind(
	ctx context.Context, queries *sqlcgen.Queries, itemID string, kind library.ItemKind,
) (sqlcgen.Item, error) {
	item, err := view.item(ctx, queries, itemID)
	if err != nil {
		return sqlcgen.Item{}, err
	}

	if item.Kind != kind.String() {
		return sqlcgen.Item{}, fmt.Errorf("item %q is a %s, not a %s: %w", itemID, item.Kind, kind, domain.ErrInvalid)
	}

	return item, nil
}

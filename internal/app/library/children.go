package library

import (
	"context"
	"fmt"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// ChildPageRequest asks for one page of an item's children. Page carries limit and cursor as
// for ListItems; Descending reverses the whole child order (see ChildReader). A cursor is
// only valid for the direction that issued it.
type ChildPageRequest struct {
	Page       library.PageRequest
	Descending bool
}

// ListChildren returns one page of an item's children: the seasons of a show or the
// episodes of a season, in the order ChildReader documents; extras are never children. A
// leaf yields an empty page. A limit of 0 means the default; a limit outside 1 to
// library.MaxPageLimit and a cursor the catalog did not produce wrap domain.ErrInvalid, an
// unknown item wraps domain.ErrNotFound. Without a wired ChildReader the error wraps
// domain.ErrUnavailable.
func (service *Service) ListChildren(ctx context.Context, itemID string, request ChildPageRequest) (ItemPage, error) {
	restriction, err := appaccess.RestrictionFrom(ctx)
	if err != nil {
		return ItemPage{}, fmt.Errorf("list children of item %q: %w", itemID, err)
	}

	if service.children == nil {
		return ItemPage{}, fmt.Errorf("list children of item %q: no child reader: %w", itemID, domain.ErrUnavailable)
	}

	limit, err := library.NormalizeLimit(request.Page.Limit)
	if err != nil {
		return ItemPage{}, fmt.Errorf("list children of item %q: %w", itemID, err)
	}

	request.Page.Limit = limit

	// Unlike ListItems no existence check is needed: the reader knows the item and reports
	// an unknown or hidden one itself.
	children, err := service.children.ListChildren(ctx, itemID, request, restriction)
	if err != nil {
		return ItemPage{}, fmt.Errorf("list children of item %q: %w", itemID, err)
	}

	return children, nil
}

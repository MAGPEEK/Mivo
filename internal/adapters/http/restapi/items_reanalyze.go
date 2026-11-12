package restapi

import (
	"context"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// operationReanalyzeItem is the operation name of the re-analysis, as in api/openapi.yaml, for logs.
const operationReanalyzeItem = "reanalyzeItem"

// ItemReanalyzer probes the files of an item again in the background, implemented by
// app/scan.Service. Errors wrap domain.ErrNotFound (no such item or file), domain.ErrConflict
// (a run for the item is active), domain.ErrInvalid (a library kind without video probe) or
// domain.ErrUnavailable (service closed).
type ItemReanalyzer interface {
	Reanalyze(ctx context.Context, libraryID, itemID string) error
}

// ReanalyzeItem starts the re-analysis of an item and answers 202. The item is read through
// the viewer-scoped catalog first, so an item the viewer may not see answers 404 exactly like
// an unknown one and never reaches the use case. The management guard has already limited the
// route to administrators (ADR 0028).
func (api *API) ReanalyzeItem(ctx context.Context, request apigen.ReanalyzeItemRequestObject) (apigen.ReanalyzeItemResponseObject, error) {
	if api.reanalysis == nil {
		problem := notImplementedProblem()

		return apigen.ReanalyzeItemdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	itemID := request.ItemID.String()

	detail, err := api.items.GetItem(ctx, itemID)
	if err != nil {
		return nil, operationFailure(operationReanalyzeItem, err, http.StatusNotFound)
	}

	err = api.reanalysis.Reanalyze(ctx, detail.Item.LibraryID, itemID)
	if err != nil {
		return nil, operationFailure(operationReanalyzeItem, err,
			http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusServiceUnavailable)
	}

	return apigen.ReanalyzeItem202Response{}, nil
}

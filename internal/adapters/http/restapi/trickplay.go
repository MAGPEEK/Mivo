package restapi

import (
	"context"
	"math"
	"net/http"
	"strconv"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/trickplay"
)

// The route of one sprite sheet (getTrickplaySheet) is served by the stream adapter; the
// manifest lists the paths so clients never build them.
const (
	trickplaySheetPrefix = "/api/v1/media-files/"
	trickplaySheetInfix  = "/trickplay/"
	trickplaySheetSuffix = ".jpg"
)

// GetTrickplayManifest answers the layout of a file's trickplay. A file the profile may not
// see, one without video and one whose trickplay is not ready all answer 404, so the answer
// reveals nothing about files the viewer cannot see; the use case checks visibility.
func (api *API) GetTrickplayManifest(ctx context.Context, request apigen.GetTrickplayManifestRequestObject) (apigen.GetTrickplayManifestResponseObject, error) {
	if api.streaming.Trickplay == nil {
		problem := notImplementedProblem()

		return apigen.GetTrickplayManifestdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	fileID := request.FileID.String()

	manifest, err := api.streaming.Trickplay.Manifest(ctx, fileID)
	if err != nil {
		return nil, operationFailure(operationGetTrickplayManifest, err,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	}

	return apigen.GetTrickplayManifest200JSONResponse(newTrickplayManifest(fileID, manifest)), nil
}

// RequestTrickplay queues the generation and answers 202. A file that cannot have trickplay
// (no video, not probed, a disc) answers 422 "unsupported", a server that shuts down 503.
func (api *API) RequestTrickplay(ctx context.Context, request apigen.RequestTrickplayRequestObject) (apigen.RequestTrickplayResponseObject, error) {
	if api.streaming.Trickplay == nil {
		problem := notImplementedProblem()

		return apigen.RequestTrickplaydefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	if err := api.streaming.Trickplay.Request(ctx, request.FileID.String()); err != nil {
		return nil, operationFailure(operationRequestTrickplay, err,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
			http.StatusUnprocessableEntity, http.StatusServiceUnavailable)
	}

	return apigen.RequestTrickplay202Response{}, nil
}

func newTrickplayManifest(fileID string, manifest trickplay.Manifest) apigen.TrickplayManifest {
	sheetURLs := make([]string, 0, manifest.SheetCount)

	for sheet := range manifest.SheetCount {
		sheetURLs = append(sheetURLs, trickplaySheetPrefix+fileID+trickplaySheetInfix+strconv.Itoa(sheet)+trickplaySheetSuffix)
	}

	return apigen.TrickplayManifest{
		TileWidth: saturatedInt32(manifest.TileWidth), TileHeight: saturatedInt32(manifest.TileHeight),
		Columns: saturatedInt32(manifest.Columns), Rows: saturatedInt32(manifest.Rows),
		IntervalSeconds: durationSeconds(manifest.Interval),
		SheetCount:      saturatedInt32(len(sheetURLs)), SheetUrls: sheetURLs,
	}
}

// saturatedInt32 converts a layout number the generator bounds far below int32; the clamp only
// keeps the conversion defined.
func saturatedInt32(value int) int32 {
	return int32(min(max(value, 0), math.MaxInt32))
}

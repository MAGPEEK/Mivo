package restapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Operation names of the identify and refresh endpoints, as in api/openapi.yaml, for logs.
const (
	operationSearchIdentifyCandidates = "searchIdentifyCandidates"
	operationIdentifyItem             = "identifyItem"
	operationRemoveItemMatch          = "removeItemMatch"
	operationRefreshItemMetadata      = "refreshItemMetadata"
	operationRefreshLibraryMetadata   = "refreshLibraryMetadata"
)

// Bounds of the identify request bodies (IdentifySearchRequest, IdentifyRequest). The
// generated decoder checks none of them.
const (
	maxSearchQueryLength = 256
	maxProviderRefLength = 512
	minSearchYear        = 1800
	maxSearchYear        = 2200
	maxExternalIDLength  = 128
	// maxCandidateOverview is how many characters of a plot summary a candidate carries;
	// the dialog shows a few lines only.
	maxCandidateOverview = 500
	overviewEllipsis     = "…"
)

// Request properties an identify problem points at (Problem.field).
const (
	fieldQuery      = "query"
	fieldProviderID = "providerId"
	fieldProvider   = "provider"
	fieldExternalID = "externalId"
)

// SearchIdentifyCandidates answers the provider candidates for an item, best first. A body
// outside the contract or a kind without provider answers 400 "invalid", an unknown item
// 404, switched-off online metadata 409 "online_disabled" and an unreachable or rate-limiting
// provider 503 "provider_unavailable" with Retry-After.
func (api *API) SearchIdentifyCandidates(ctx context.Context, request apigen.SearchIdentifyCandidatesRequestObject) (apigen.SearchIdentifyCandidatesResponseObject, error) {
	if api.metadata.Identify == nil {
		problem := notImplementedProblem()

		return apigen.SearchIdentifyCandidatesdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	query, err := identifyQuery(request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationSearchIdentifyCandidates, err)
	}

	found, err := api.metadata.Identify.Search(ctx, request.ItemID.String(), query)
	if err != nil {
		return searchFailure(err)
	}

	candidates := make([]apigen.IdentifyCandidate, 0, len(found))
	for _, scored := range found {
		if candidate, ok := api.candidateResponse(scored); ok {
			candidates = append(candidates, candidate)
		}
	}

	return apigen.SearchIdentifyCandidates200JSONResponse{Candidates: candidates}, nil
}

func searchFailure(err error) (apigen.SearchIdentifyCandidatesResponseObject, error) {
	problem, ok := onlineProblem(err)
	switch {
	case ok && problem.Status == http.StatusConflict:
		return apigen.SearchIdentifyCandidates409ApplicationProblemPlusJSONResponse{
			OnlineDisabledApplicationProblemPlusJSONResponse: apigen.OnlineDisabledApplicationProblemPlusJSONResponse(problem),
		}, nil
	case ok:
		return apigen.SearchIdentifyCandidates503ApplicationProblemPlusJSONResponse{
			ProviderUnavailableApplicationProblemPlusJSONResponse: apigen.ProviderUnavailableApplicationProblemPlusJSONResponse{
				Body:    problem,
				Headers: apigen.ProviderUnavailableResponseHeaders{RetryAfter: problem.RetryAfterSeconds},
			},
		}, nil
	default:
		return nil, operationFailure(operationSearchIdentifyCandidates, err, http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable)
	}
}

// identifyQuery checks the search body against the contract.
func identifyQuery(body *apigen.SearchIdentifyCandidatesJSONRequestBody) (identify.Query, error) {
	if body == nil {
		return identify.Query{}, fmt.Errorf("missing body: %w", domain.ErrInvalid)
	}

	length := utf8.RuneCountInString(body.Query)
	if length < 1 || length > maxSearchQueryLength || !utf8.ValidString(body.Query) {
		return identify.Query{}, invalidParameter(fieldQuery, fmt.Sprintf("not 1 to %d characters", maxSearchQueryLength))
	}

	query := identify.Query{Text: body.Query}

	if body.Year != nil {
		if *body.Year < minSearchYear || *body.Year > maxSearchYear {
			return identify.Query{}, invalidParameter(fieldYear, fmt.Sprintf("not between %d and %d", minSearchYear, maxSearchYear))
		}

		query.Year = int(*body.Year)
	}

	if body.ProviderID != nil {
		if !utf8.ValidString(*body.ProviderID) || utf8.RuneCountInString(*body.ProviderID) > maxProviderRefLength {
			return identify.Query{}, invalidParameter(fieldProviderID, fmt.Sprintf("longer than %d characters", maxProviderRefLength))
		}

		query.ProviderRef = *body.ProviderID
	}

	return query, nil
}

// candidateResponse converts a scored candidate. An IMDb reference is offered under TMDB,
// which resolves IMDb IDs (identifyItem accepts it back the same way). Candidates of
// providers the API does not know are left out.
func (api *API) candidateResponse(scored match.Scored) (apigen.IdentifyCandidate, bool) {
	candidate := scored.Candidate

	provider, ok := candidateProvider(candidate.Ref.Provider)
	if !ok || candidate.Ref.ID == "" || len(candidate.Ref.ID) > maxExternalIDLength {
		return apigen.IdentifyCandidate{}, false
	}

	response := apigen.IdentifyCandidate{
		Provider:   provider,
		ExternalID: candidate.Ref.ID,
		Title:      candidate.Title,
		Confidence: min(max(scored.Confidence, 0), 1),
		Overview:   optionalString(shortenOverview(candidate.Overview)),
		PosterURL:  optionalString(api.posterURL(provider, candidate.Poster)),
	}

	if candidate.Year > 0 {
		year := int32(min(candidate.Year, maxSearchYear))
		response.Year = &year
	}

	return response, true
}

func candidateProvider(provider match.RefProvider) (apigen.MetadataProvider, bool) {
	switch provider {
	case match.RefTMDB, match.RefIMDb:
		return apigen.MetadataProviderTmdb, true
	case match.RefMusicBrainz:
		return apigen.MetadataProviderMusicbrainz, true
	default:
		return "", false
	}
}

// posterURL registers a TMDB poster with the proxy; other providers get no preview yet.
func (api *API) posterURL(provider apigen.MetadataProvider, poster string) string {
	if api.metadata.Posters == nil || poster == "" || provider != apigen.MetadataProviderTmdb {
		return ""
	}

	return api.metadata.Posters.PosterURL(enrich.ArtworkRef{
		Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: poster,
	})
}

// shortenOverview cuts a plot summary to maxCandidateOverview characters at a word
// boundary when one is near.
func shortenOverview(overview string) string {
	overview = strings.TrimSpace(overview)
	if utf8.RuneCountInString(overview) <= maxCandidateOverview {
		return overview
	}

	cut := string([]rune(overview)[:maxCandidateOverview])
	if space := strings.LastIndexByte(cut, ' '); space > len(cut)/2 {
		cut = cut[:space]
	}

	return strings.TrimRight(cut, " ,.;:") + overviewEllipsis
}

// IdentifyItem applies a candidate and answers the item as it is now. A body outside the
// contract, an ID that does not fit the item or a season or episode answers 400 "invalid",
// an unknown item 404 and switched-off online metadata 409 "online_disabled". It never
// contacts the provider, so it has no provider_unavailable answer.
func (api *API) IdentifyItem(ctx context.Context, request apigen.IdentifyItemRequestObject) (apigen.IdentifyItemResponseObject, error) {
	if api.metadata.Identify == nil {
		problem := notImplementedProblem()

		return apigen.IdentifyItemdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	provider, externalID, err := appliedCandidate(request.Body)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operationIdentifyItem, err)
	}

	itemID := request.ItemID.String()

	err = api.metadata.Identify.Apply(ctx, itemID, provider, externalID)
	if errors.Is(err, domain.ErrConflict) {
		return apigen.IdentifyItem409ApplicationProblemPlusJSONResponse{
			OnlineDisabledApplicationProblemPlusJSONResponse: apigen.OnlineDisabledApplicationProblemPlusJSONResponse(
				NewProblem(http.StatusConflict, CodeOnlineDisabled)),
		}, nil
	}

	if err != nil {
		return nil, operationFailure(operationIdentifyItem, err, http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	detail, err := api.currentItem(ctx, operationIdentifyItem, itemID)
	if err != nil {
		return nil, err
	}

	return apigen.IdentifyItem200JSONResponse(detail), nil
}

// appliedCandidate checks the identify body. A TMDB candidate may carry an IMDb ID (see
// candidateResponse); it is applied as the IMDb reference TMDB resolves.
func appliedCandidate(body *apigen.IdentifyItemJSONRequestBody) (match.RefProvider, string, error) {
	if body == nil {
		return "", "", fmt.Errorf("missing body: %w", domain.ErrInvalid)
	}

	externalID := strings.TrimSpace(body.ExternalID)
	if externalID == "" || len(externalID) > maxExternalIDLength || !utf8.ValidString(externalID) {
		return "", "", invalidParameter(fieldExternalID, fmt.Sprintf("not 1 to %d characters", maxExternalIDLength))
	}

	switch body.Provider {
	case apigen.MetadataProviderTmdb:
		if ref, err := match.ParseProviderRef(externalID); err == nil && ref.Provider == match.RefIMDb {
			return match.RefIMDb, externalID, nil
		}

		return match.RefTMDB, externalID, nil
	case apigen.MetadataProviderMusicbrainz:
		return match.RefMusicBrainz, externalID, nil
	case apigen.MetadataProviderCoverartarchive:
		return "", "", invalidParameter(fieldProvider, "the Cover Art Archive identifies nothing")
	default:
		return "", "", invalidParameter(fieldProvider, "unknown value")
	}
}

// RemoveItemMatch removes the online match of an item and answers the item as it is now. An
// unknown item answers 404, a kind without provider 400.
func (api *API) RemoveItemMatch(ctx context.Context, request apigen.RemoveItemMatchRequestObject) (apigen.RemoveItemMatchResponseObject, error) {
	if api.metadata.Identify == nil {
		problem := notImplementedProblem()

		return apigen.RemoveItemMatchdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	itemID := request.ItemID.String()

	if err := api.metadata.Identify.Unmatch(ctx, itemID); err != nil {
		return nil, operationFailure(operationRemoveItemMatch, err, http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	detail, err := api.currentItem(ctx, operationRemoveItemMatch, itemID)
	if err != nil {
		return nil, err
	}

	return apigen.RemoveItemMatch200JSONResponse(detail), nil
}

// currentItem reads the item detail answer after a change, with its match fields.
func (api *API) currentItem(ctx context.Context, operation, itemID string) (apigen.ItemDetail, error) {
	detail, err := api.items.GetItem(ctx, itemID)
	if err != nil {
		return apigen.ItemDetail{}, operationFailure(operation, err, http.StatusNotFound)
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		return apigen.ItemDetail{}, operationFailure(operation, err)
	}

	api.applyDetailMatch(ctx, &response)
	api.applyExternalTracks(ctx, &response, detail)

	return response, nil
}

// RefreshItemMetadata queues a refresh of an item and answers 202. An unknown item answers
// 404, a kind without provider 400 and switched-off online metadata 409.
func (api *API) RefreshItemMetadata(ctx context.Context, request apigen.RefreshItemMetadataRequestObject) (apigen.RefreshItemMetadataResponseObject, error) {
	if api.metadata.Refresh == nil {
		problem := notImplementedProblem()

		return apigen.RefreshItemMetadatadefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	err := api.metadata.Refresh.RefreshItem(ctx, request.ItemID.String())
	if errors.Is(err, domain.ErrConflict) {
		return apigen.RefreshItemMetadata409ApplicationProblemPlusJSONResponse{
			OnlineDisabledApplicationProblemPlusJSONResponse: apigen.OnlineDisabledApplicationProblemPlusJSONResponse(
				NewProblem(http.StatusConflict, CodeOnlineDisabled)),
		}, nil
	}

	if err != nil {
		return nil, operationFailure(operationRefreshItemMetadata, err, http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	return apigen.RefreshItemMetadata202Response{}, nil
}

// RefreshLibraryMetadata queues a refresh of a library and answers 202. An unknown library
// answers 404 and online metadata switched off globally or for the library 409.
func (api *API) RefreshLibraryMetadata(ctx context.Context, request apigen.RefreshLibraryMetadataRequestObject) (apigen.RefreshLibraryMetadataResponseObject, error) {
	if api.metadata.Refresh == nil {
		problem := notImplementedProblem()

		return apigen.RefreshLibraryMetadatadefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	err := api.metadata.Refresh.RefreshLibrary(ctx, request.LibraryID.String())
	if errors.Is(err, domain.ErrConflict) {
		return apigen.RefreshLibraryMetadata409ApplicationProblemPlusJSONResponse{
			OnlineDisabledApplicationProblemPlusJSONResponse: apigen.OnlineDisabledApplicationProblemPlusJSONResponse(
				NewProblem(http.StatusConflict, CodeOnlineDisabled)),
		}, nil
	}

	if err != nil {
		return nil, operationFailure(operationRefreshLibraryMetadata, err, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	return apigen.RefreshLibraryMetadata202Response{}, nil
}

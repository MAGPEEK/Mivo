package restapi

import (
	"context"
	"log/slog"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// matchStatuses reads the match of the items. The fields it feeds are optional, so a failed
// read is logged and leaves them out instead of failing the whole answer; without a
// MatchStatusReader it reports false.
func (api *API) matchStatuses(ctx context.Context, itemIDs []string) (map[string]match.ItemStatus, bool) {
	if api.metadata.Matches == nil || len(itemIDs) == 0 {
		return nil, false
	}

	statuses, err := api.metadata.Matches.ItemMatchStatuses(ctx, itemIDs)
	if err != nil {
		api.logger.WarnContext(ctx, "item match states unavailable",
			slog.String("request_id", requestid.FromContext(ctx)),
			slog.Any("error", err))

		return nil, false
	}

	return statuses, true
}

// applySummaryMatches fills matchState and metadataSources of every summary.
func (api *API) applySummaryMatches(ctx context.Context, items []apigen.ItemSummary) {
	itemIDs := make([]string, 0, len(items))
	for _, item := range items {
		itemIDs = append(itemIDs, item.ID.String())
	}

	statuses, ok := api.matchStatuses(ctx, itemIDs)
	if !ok {
		return
	}

	for index := range items {
		items[index].MatchState, items[index].MetadataSources = matchFields(statuses[items[index].ID.String()])
	}
}

// applyDetailMatch fills matchState and metadataSources of an item detail.
func (api *API) applyDetailMatch(ctx context.Context, detail *apigen.ItemDetail) {
	itemID := detail.ID.String()

	statuses, ok := api.matchStatuses(ctx, []string{itemID})
	if !ok {
		return
	}

	detail.MatchState, detail.MetadataSources = matchFields(statuses[itemID])
}

// matchFields converts a status; the zero status of an item without a stored match is
// unmatched (ADR 0031 §9).
func matchFields(status match.ItemStatus) (*apigen.MatchState, *apigen.MetadataSources) {
	state := apigen.MatchStateUnmatched
	if status.State.IsValid() {
		state = apigen.MatchState(status.State)
	}

	sources := status.Sources()
	if len(sources) == 0 {
		return &state, nil
	}

	return &state, &sources
}

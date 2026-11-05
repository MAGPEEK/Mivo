package sqlite

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// itemMatchStatusesSQL reads the stored matches of a JSON array of item IDs; one bound
// parameter serves any number of IDs.
const itemMatchStatusesSQL = `SELECT item_id, state, provider FROM item_match
WHERE item_id IN (SELECT value FROM json_each(?))`

// ItemMatchStatuses returns the match status of every listed item that has a stored match
// row. Items without a row, and unknown IDs, are left out: they are unmatched (migration
// 00005). Item answers read it to show matchState and metadataSources (ADR 0031 §9).
func (store *EnrichmentStore) ItemMatchStatuses(ctx context.Context, itemIDs []string) (map[string]match.ItemStatus, error) {
	statuses := make(map[string]match.ItemStatus, len(itemIDs))
	if len(itemIDs) == 0 {
		return statuses, nil
	}

	encoded, err := json.Marshal(itemIDs)
	if err != nil {
		return nil, fmt.Errorf("read match states: encode ids: %w", err)
	}

	rows, err := store.reader.QueryContext(ctx, itemMatchStatusesSQL, string(encoded))
	if err != nil {
		return nil, fmt.Errorf("read match states: %w", err)
	}

	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var itemID, state, provider string
		if err := rows.Scan(&itemID, &state, &provider); err != nil {
			return nil, fmt.Errorf("read match states: %w", err)
		}

		parsed, err := match.ParseMatchState(state)
		if err != nil {
			return nil, fmt.Errorf("read match state of item %q: %w", itemID, err)
		}

		statuses[itemID] = match.ItemStatus{State: parsed, Provider: match.RefProvider(provider)}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read match states: %w", err)
	}

	return statuses, nil
}

package identify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Unmatch removes the item's online match ("Remove match", ADR 0031 §9): the state becomes
// disabled, so no automatic match happens again until the user identifies the item; online
// data is dropped unless its field is locked; the provider_ids lock is released because it
// is the identify choice of the match being removed, and the provider IDs fall back to the
// NFO ones; the provider artwork rows and their files are deleted. Local data (NFO, local
// artwork, other locked fields) stays. A later scan keeps the NFO IDs in the record, but
// the item stays disabled. It asks no provider and works with online metadata switched off.
// An unknown item wraps domain.ErrNotFound, a kind without provider domain.ErrInvalid.
func (service *Service) Unmatch(ctx context.Context, itemID string) error {
	current, err := service.loadTarget(ctx, itemID)
	if err != nil {
		return fmt.Errorf("remove match of item %s: %w", itemID, err)
	}

	if err := service.deps.Matches.SaveMatchChange(ctx, itemID, removal(current.snapshot)); err != nil {
		return fmt.Errorf("remove match of item %s: save: %w", itemID, err)
	}

	dataPaths, err := service.deps.Artwork.DeleteProviderArtwork(ctx, itemID)
	if err != nil {
		return fmt.Errorf("remove match of item %s: %w", itemID, err)
	}

	service.removeFiles(ctx, itemID, dataPaths)

	return nil
}

// removal is the change Unmatch stores: local data and the other locked fields only, the
// provider IDs of the NFO (none without one) and no provider_ids lock.
func removal(snapshot enrich.ItemSnapshot) MatchChange {
	locks := metadata.NewLockSet(slices.DeleteFunc(snapshot.Locks.Fields(), func(field metadata.Field) bool {
		return field == metadata.FieldProviderIDs
	}))

	record := localRecord(snapshot, locks)
	record.ProviderIDs = slices.Clone(snapshot.Local.NFO.ProviderIDs)

	return MatchChange{
		Record: record,
		Locks:  locks,
		Match:  enrich.MatchResult{State: match.MatchStateDisabled},
	}
}

// removeFiles deletes the downloaded provider pictures. The rows are gone already, so a file
// that cannot be deleted is only an orphan in the data directory: it is logged, not failed.
func (service *Service) removeFiles(ctx context.Context, itemID string, dataPaths []string) {
	for _, dataPath := range dataPaths {
		err := service.deps.Files.RemoveDataFile(ctx, dataPath)
		if err == nil || errors.Is(err, domain.ErrNotFound) {
			continue
		}

		service.deps.Logger.LogAttrs(ctx, slog.LevelWarn, "could not delete provider artwork file",
			slog.String("item_id", itemID), slog.String("error", err.Error()))
	}
}

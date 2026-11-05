package identify

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Apply links the item to the provider entry the user picked (ADR 0031 §10): it stores the
// provider ID, locks provider_ids, sets the state matched and queues the enrichment (reason
// identify) of the item and, for a show, of its seasons and episodes. The earlier provider
// data is dropped at once except locked fields; the queued enrichment fills in the new
// entry's data and replaces the provider artwork, so until it ran (offline: until the
// provider is reachable) the earlier provider pictures stay. They are not deleted here
// because a locked artwork kind may have selected one. Other locks stay. No provider is asked,
// so applying works offline. Seasons and episodes are matched through their show and are
// refused with domain.ErrInvalid, like an ID of the wrong provider or kind; switched-off
// online metadata wraps domain.ErrConflict, an unknown item domain.ErrNotFound.
func (service *Service) Apply(ctx context.Context, itemID string, provider match.RefProvider, externalID string) error {
	current, err := service.loadTarget(ctx, itemID)
	if err != nil {
		return fmt.Errorf("identify item %s: %w", itemID, err)
	}

	// The kind is checked first: an episode stays invalid whatever the online switches say.
	ref, err := appliedRef(current, provider, externalID)
	if err != nil {
		return fmt.Errorf("identify item %s: %w", itemID, err)
	}

	if err := service.requireOnline(ctx, &current); err != nil {
		return fmt.Errorf("identify item %s: %w", itemID, err)
	}

	if err := service.deps.Matches.SaveMatchChange(ctx, itemID, identification(current.snapshot, ref)); err != nil {
		return fmt.Errorf("identify item %s: save: %w", itemID, err)
	}

	if err := service.enqueueWithChildren(ctx, current, enrich.ReasonIdentify); err != nil {
		return fmt.Errorf("identify item %s: %w", itemID, err)
	}

	return nil
}

// appliedRef checks that the item may be identified by the given ID and returns its
// reference. The error wraps domain.ErrInvalid.
func appliedRef(current target, provider match.RefProvider, externalID string) (match.ProviderRef, error) {
	kind := current.snapshot.Kind
	if kind == library.ItemKindSeason || kind == library.ItemKindEpisode {
		return match.ProviderRef{}, fmt.Errorf("a %s is matched through its show: %w", kind, domain.ErrInvalid)
	}

	parsed, err := match.ParseProviderRef(externalID)
	if err != nil {
		return match.ProviderRef{}, err
	}

	if parsed.Provider != provider {
		return match.ProviderRef{}, fmt.Errorf("%q is not a %s ID: %w", externalID, provider, domain.ErrInvalid)
	}

	return fitRef(current.family, parsed)
}

// identification is the change Apply stores. The record keeps only local data and locked
// fields; its provider IDs become the chosen one (the user's new choice replaces an earlier
// one, locked or not). A MusicBrainz ID has no place in a record and travels in the match
// reference, so the record's IDs stay then.
func identification(snapshot enrich.ItemSnapshot, ref match.ProviderRef) MatchChange {
	record := localRecord(snapshot, snapshot.Locks)

	if id, ok := ref.ProviderID(); ok {
		record.ProviderIDs = []metadata.ProviderID{id}
	} else {
		record.ProviderIDs = snapshot.Record.ProviderIDs
	}

	return MatchChange{
		Record: record,
		Locks:  metadata.NewLockSet(append(snapshot.Locks.Fields(), metadata.FieldProviderIDs)),
		Match: enrich.MatchResult{
			State:      match.MatchStateMatched,
			Provider:   ref.Provider,
			Ref:        ref,
			Confidence: exactConfidence,
		},
	}
}

// localRecord is the item's record without provider data: locked fields as stored, the
// rest from the NFO and the file name, in the merge order of ADR 0031 §8.
func localRecord(snapshot enrich.ItemSnapshot, locks metadata.LockSet) metadata.Record {
	return metadata.Merge(snapshot.Record, locks, []metadata.SourcedRecord{
		{Source: metadata.SourceNFO, Record: snapshot.Local.NFO},
		{Source: metadata.SourceFilename, Record: snapshot.Local.Filename},
	})
}

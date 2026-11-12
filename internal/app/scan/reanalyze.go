package scan

import (
	"context"
	"fmt"
	"log/slog"
	"slices"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// reanalysis is one accepted re-analysis: the files to probe again and everything the refresh
// afterwards needs, taken from the catalog before the request was answered.
type reanalysis struct {
	library library.ManagedLibrary
	itemID  string
	jobs    []probeJob
	// placed and chains describe every video of the library as the scan would place it, so the
	// refresher sees the item's files the way a scan would hand them over.
	placed []placedFile
	chains map[fileKey][]string
}

// Reanalyze probes the files of one item again in the background and updates the catalog
// (PL-35). It reuses the probe and record steps of the video scan, so the streams, the
// duration and a file that vanished (stored as probe_failed, which lists it as unavailable)
// follow the same rules as in a scan; the item and its ID stay. Only video libraries (movies
// and shows) are supported; any other kind wraps domain.ErrInvalid.
//
// Errors wrap domain.ErrNotFound for an unknown library or an item without files in it,
// domain.ErrConflict while a re-analysis of the item runs and domain.ErrUnavailable after
// Close. A failure of the background run is logged only.
func (service *Service) Reanalyze(ctx context.Context, libraryID, itemID string) error {
	if err := service.ensureOpen(); err != nil {
		return fmt.Errorf("reanalyze item %s: %w", itemID, err)
	}

	managed, err := service.libraries.GetLibrary(ctx, libraryID)
	if err != nil {
		return fmt.Errorf("reanalyze item %s: %w", itemID, err)
	}

	if managed.Kind != library.KindMovies && managed.Kind != library.KindShows {
		return fmt.Errorf("reanalyze item %s: library kind %q: %w", itemID, managed.Kind, domain.ErrInvalid)
	}

	plan, err := service.planReanalysis(ctx, managed, itemID)
	if err != nil {
		return fmt.Errorf("reanalyze item %s: %w", itemID, err)
	}

	done, err := service.claimReanalysis(itemID)
	if err != nil {
		return fmt.Errorf("reanalyze item %s: %w", itemID, err)
	}

	go service.runReanalysis(plan, done)

	return nil
}

// planReanalysis lists the stored files of the item (a show owns the files of its episodes)
// and places them like a scan does, from the stored paths of the whole library: a version
// label depends on the sibling files.
func (service *Service) planReanalysis(ctx context.Context, managed library.ManagedLibrary, itemID string) (reanalysis, error) {
	stored, err := service.catalogV2.PlacedFiles(ctx, managed.ID)
	if err != nil {
		return reanalysis{}, fmt.Errorf("load placed files: %w", err)
	}

	states := make([]library.FileState, 0, len(stored))
	for _, file := range stored {
		states = append(states, file.State)
	}

	placed, _ := placeVideos(managed.Kind, states)
	if !managed.Options.ScanExtras {
		placed, _ = withoutExtras(placed)
	}

	placementOf := make(map[fileKey]library.Placement, len(placed))
	for _, video := range placed {
		placementOf[keyOf(video.state)] = video.placement
	}

	plan := reanalysis{library: managed, itemID: itemID, placed: placed, chains: map[fileKey][]string{}}

	for index := range stored {
		file := stored[index]
		plan.chains[keyOf(file.State)] = file.ItemIDs

		if placement, ok := placementOf[keyOf(file.State)]; ok && slices.Contains(file.ItemIDs, itemID) {
			plan.jobs = append(plan.jobs, probeJob{state: file.State, placement: placement, placed: &file})
		}
	}

	if len(plan.jobs) == 0 {
		return reanalysis{}, fmt.Errorf("item %s has no file in library %s: %w", itemID, managed.ID, domain.ErrNotFound)
	}

	return plan, nil
}

// claimReanalysis marks the item as being re-analysed. The returned channel is closed by
// runReanalysis when the run ended.
func (service *Service) claimReanalysis(itemID string) (chan struct{}, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	if service.closed {
		return nil, fmt.Errorf("scan service is closed: %w", domain.ErrUnavailable)
	}

	if _, running := service.reanalyses[itemID]; running {
		return nil, fmt.Errorf("a re-analysis of the item is already running: %w", domain.ErrConflict)
	}

	done := make(chan struct{})
	service.reanalyses[itemID] = done

	return done, nil
}

// runReanalysis probes and records the files, then refreshes the item. It runs on the
// service's context, not on the request's, and ends with Close.
func (service *Service) runReanalysis(plan reanalysis, done chan struct{}) {
	defer func() {
		service.mutex.Lock()
		delete(service.reanalyses, plan.itemID)
		service.mutex.Unlock()
		close(done)
	}()

	if err := service.executeReanalysis(service.scanContext, plan); err != nil {
		service.logger.LogAttrs(service.logContext, slog.LevelWarn, "re-analysis failed",
			slog.String("library", plan.library.ID), slog.String("item", plan.itemID),
			slog.String("error", err.Error()))
	}
}

func (service *Service) executeReanalysis(ctx context.Context, plan reanalysis) error {
	scanTracker := newTracker(ctx, service.clock())
	defer scanTracker.cancel()

	itemKind, err := libraryItemKind(plan.library)
	if err != nil {
		return err
	}

	scanRun := service.newRunV2(plan.library, scanTracker)
	scanRun.base.itemKind = itemKind
	scanRun.withoutVideo = map[fileKey]library.FileState{}
	scanRun.wanted = plan.placed
	scanRun.chains = plan.chains

	if err := scanRun.base.probeAll(ctx, plan.jobs, scanRun.record); err != nil {
		return err
	}

	sidecars, err := scanRun.catalog.SidecarStates(ctx, plan.library.ID)
	if err != nil {
		return catalogFailure("load sidecar states", err)
	}

	return scanRun.refreshTouched(ctx, sidecars, false)
}

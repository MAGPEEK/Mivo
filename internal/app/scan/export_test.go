package scan

import (
	"context"
	"fmt"
)

// RefreshBelowSidecars returns the item IDs the refresh set collects for the changed sidecars
// of one folder. Both arguments use root-relative paths.
func RefreshBelowSidecars(changed []string, chains map[string][]string) []string {
	changedKeys := make([]fileKey, 0, len(changed))
	for _, changedPath := range changed {
		changedKeys = append(changedKeys, fileKey{path: changedPath})
	}

	chainKeys := make(map[fileKey][]string, len(chains))
	for filePath, chain := range chains {
		chainKeys[fileKey{path: filePath}] = chain
	}

	set := newRefreshSet()
	set.addBelowChangedSidecars(changedKeys, chainKeys)

	return set.sorted()
}

// WaitForScan blocks until the current scan of libraryID has finished or ctx ends. It returns
// at once when no scan was ever started. Tests use it instead of polling Status.
func (service *Service) WaitForScan(ctx context.Context, libraryID string) error {
	service.mutex.Lock()
	tracker := service.trackers[libraryID]
	service.mutex.Unlock()

	if tracker == nil {
		return nil
	}

	select {
	case <-tracker.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for scan of %s: %w", libraryID, ctx.Err())
	}
}

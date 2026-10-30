package scan

import (
	"context"
	"fmt"
)

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

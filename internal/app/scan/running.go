package scan

import (
	"cmp"
	"maps"
	"slices"
)

// Running is one scan in progress, for the activity view.
type Running struct {
	LibraryID   string
	LibraryName string
	Status      Status
}

// Running returns the scans that run now, oldest first. It reads the trackers the scans
// already keep, so the activity view duplicates no state.
func (service *Service) Running() []Running {
	service.mutex.Lock()
	trackers := maps.Clone(service.trackers)
	service.mutex.Unlock()

	var running []Running

	for libraryID, scanTracker := range trackers {
		if status, active := scanTracker.current(); active {
			running = append(running, Running{LibraryID: libraryID, LibraryName: scanTracker.libraryName, Status: status})
		}
	}

	slices.SortFunc(running, func(a, b Running) int {
		return cmp.Or(a.Status.StartedAt.Compare(b.Status.StartedAt), cmp.Compare(a.LibraryID, b.LibraryID))
	})

	return running
}

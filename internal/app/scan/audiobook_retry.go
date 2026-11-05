package scan

import "maps"

// A file is saved before the details that reference it can be written (the chapters of an
// audiobook part, the details of a photo), so a failed details write would leave a stored,
// unchanged file whose details the next scan never writes. The service remembers such files
// (memory only, like the options) and the next scan treats them as changed (BACKLOG, m3-13e
// notes). A library has one kind, so one set per library serves every pipeline.

// detailRetriesOf returns a copy of the files of the library whose details are due.
func (service *Service) detailRetriesOf(libraryID string) map[fileKey]struct{} {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return maps.Clone(service.detailRetries[libraryID])
}

// setDetailRetry remembers (due) or forgets the file whose details must be written again.
func (service *Service) setDetailRetry(libraryID string, key fileKey, due bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	if !due {
		delete(service.detailRetries[libraryID], key)

		return
	}

	if service.detailRetries[libraryID] == nil {
		service.detailRetries[libraryID] = map[fileKey]struct{}{}
	}

	service.detailRetries[libraryID][key] = struct{}{}
}

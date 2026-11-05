package scan

import "maps"

// A file is saved before its chapters can be written (they reference it), so a failed
// chapter write would leave a stored, unchanged file whose chapters the next scan never
// writes. The service remembers such files (memory only, like the options) and the next
// scan treats them as changed (BACKLOG, m3-13e notes).

// chapterRetriesOf returns a copy of the files of the library whose chapters are due.
func (service *Service) chapterRetriesOf(libraryID string) map[fileKey]struct{} {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return maps.Clone(service.chapterRetries[libraryID])
}

// setChapterRetry remembers (due) or forgets the file whose chapters must be written again.
func (service *Service) setChapterRetry(libraryID string, key fileKey, due bool) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	if !due {
		delete(service.chapterRetries[libraryID], key)

		return
	}

	if service.chapterRetries[libraryID] == nil {
		service.chapterRetries[libraryID] = map[fileKey]struct{}{}
	}

	service.chapterRetries[libraryID][key] = struct{}{}
}

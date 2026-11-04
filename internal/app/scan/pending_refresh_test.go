package scan_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

// requestedItems returns the item IDs of the last refresh request.
func (h *harness) requestedItems() []string {
	h.refresher.mutex.Lock()
	defer h.refresher.mutex.Unlock()

	return h.refresher.last.ItemIDs
}

func TestItemWithFilesInAnUnavailableFolderIsNotRefreshedUntilItReturns(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt(heatPath, 2)}})
	h.scan(t, multiID)

	shared := h.catalogV2.itemOf(heatPath)

	// A is lost while B changes: a refresh would merge file-name data over A's NFO metadata.
	h.walker.forget(firstPath)
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt(heatPath, 5), fileAt("Alien (1979).mkv", 3)}})
	h.scan(t, multiID)

	got := h.requestedItems()
	if len(got) != 1 || got[0] == shared {
		t.Errorf("requested %v, want only the new movie, not the shared %s", got, shared)
	}

	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.scan(t, multiID)

	if got := h.requestedItems(); !slices.Equal(got, []string{shared}) {
		t.Errorf("after the folder returned requested %v, want [%s]", got, shared)
	}

	h.scan(t, multiID)

	if got := h.requestedItems(); len(got) != 0 {
		t.Errorf("pending refresh was not cleared, requested %v", got)
	}
}

func TestOptionChangeStaysDueWhileAFolderIsUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2)}})
	h.scan(t, multiID)

	changed := multiFolder(library.KindMovies)
	changed.Options.LocalArtwork = false
	h.useLibrary(changed)
	h.walker.forget(firstPath)
	h.scan(t, multiID)

	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.scan(t, multiID)

	if got := h.requestedItems(); len(got) != 2 {
		t.Errorf("requested %v, want both items: the option change was not applied everywhere yet", got)
	}
}

func TestFilesWithoutVideoInAnUnavailableFolderAreNotProbedAgain(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)

	audio := videoDescription()
	audio.Streams = audio.Streams[1:]
	song := absolutePath(firstPath, "Song (2000).mkv")
	h.prober.script(song, probeOutcome{description: audio})
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt("Song (2000).mkv", 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2)}})
	h.scan(t, multiID)

	h.walker.forget(firstPath)
	h.scan(t, multiID)

	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt("Song (2000).mkv", 1)}})
	h.scan(t, multiID)

	count := 0

	for _, probed := range h.prober.probedPaths() {
		if probed == song {
			count++
		}
	}

	if count != 1 {
		t.Errorf("file without video probed %d times, want once", count)
	}
}

// A deleted library must not leave its remembered options behind: a library created later
// under the same ID starts like a new one, so an option change is not "due" for it.
func TestForgetDropsTheRememberedOptionsOfALibrary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	service := h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2)}})
	h.scan(t, multiID)

	changed := multiFolder(library.KindMovies)
	changed.Options.LocalArtwork = false
	h.useLibrary(changed)

	service.Forget(multiID)
	h.scan(t, multiID)

	if got := h.requestedItems(); len(got) != 0 {
		t.Errorf("requested %v after Forget, want none: the options are unknown again", got)
	}
}

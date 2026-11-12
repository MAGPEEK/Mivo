package scan_test

import (
	"context"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestRunningListsOnlyScansInProgressWithTheLibraryName(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.walker.started = make(chan struct{})
	h.walker.release = make(chan struct{})
	service := h.start(t)

	if got := service.Running(); len(got) != 0 {
		t.Fatalf("Running() before any scan = %+v, want none", got)
	}

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.walker.started, "walk start")

	got := service.Running()
	if len(got) != 1 || got[0].LibraryID != moviesID || got[0].LibraryName != "Movies" ||
		got[0].Status.State != scan.StateRunning || got[0].Status.StartedAt.IsZero() {
		t.Fatalf("Running() = %+v, want the movies scan, running", got)
	}

	close(h.walker.release)
	h.wait(t, moviesID)

	if got := service.Running(); len(got) != 0 {
		t.Errorf("Running() after the scan = %+v, want none", got)
	}
}

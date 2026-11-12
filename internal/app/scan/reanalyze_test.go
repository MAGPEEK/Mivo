package scan_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const reanalyzedMovie = "Heat (1995).mkv"

func (h *harness) waitReanalysis(t *testing.T, itemID string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.WaitForReanalysis(ctx, itemID); err != nil {
		t.Fatalf("WaitForReanalysis(%s) error = %v", itemID, err)
	}
}

// scannedMovie scans one movie and returns the ID of its item.
func scannedMovie(t *testing.T, h *harness) string {
	t.Helper()

	h.start(t)
	h.scanFiles(t, moviesID, moviesRoot, fileAt(reanalyzedMovie, 1))

	return h.catalogV2.itemOf(reanalyzedMovie)
}

func TestReanalyzeProbesTheFilesAgainAndRefreshesTheItem(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	itemID := scannedMovie(t, h)
	refreshes := h.refresher.callNum

	changed := videoDescription()
	changed.Duration = 2 * time.Hour
	h.prober.script(absolutePath(moviesRoot, reanalyzedMovie), probeOutcome{description: changed})

	if err := h.service.Reanalyze(context.Background(), moviesID, itemID); err != nil {
		t.Fatalf("Reanalyze() error = %v", err)
	}

	h.waitReanalysis(t, itemID)

	file, _ := h.catalogV2.savedFile(reanalyzedMovie)
	if file.Description.Duration != 2*time.Hour || file.ProbeError != media.ProbeErrorNone {
		t.Errorf("saved duration, probe error = %v, %q, want 2h, none", file.Description.Duration, file.ProbeError)
	}

	if got := h.catalogV2.count(library.ItemKindMovie); got != 1 {
		t.Errorf("movies = %d, want 1 (the item keeps its ID)", got)
	}

	if h.refresher.callNum != refreshes+1 || len(h.refresher.last.ItemIDs) != 1 || h.refresher.last.ItemIDs[0] != itemID {
		t.Errorf("refresh calls, items = %d, %v, want one more call for %s", h.refresher.callNum, h.refresher.last.ItemIDs, itemID)
	}
}

func TestReanalyzeOfAMissingFileMarksItUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	itemID := scannedMovie(t, h)
	h.resolver.errs[reanalyzedMovie] = domain.ErrNotFound

	if err := h.service.Reanalyze(context.Background(), moviesID, itemID); err != nil {
		t.Fatalf("Reanalyze() error = %v", err)
	}

	h.waitReanalysis(t, itemID)

	file, _ := h.catalogV2.savedFile(reanalyzedMovie)
	if file.ProbeError != media.ProbeErrorFailed {
		t.Errorf("probe error = %q, want %q", file.ProbeError, media.ProbeErrorFailed)
	}

	if got := h.catalogV2.count(library.ItemKindMovie); got != 1 {
		t.Errorf("movies = %d, want 1 (unavailable, not removed)", got)
	}
}

func TestReanalyzeRefusals(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	itemID := scannedMovie(t, h)

	tests := []struct {
		name      string
		libraryID string
		itemID    string
		want      error
	}{
		{name: "unknown item", libraryID: moviesID, itemID: "missing", want: domain.ErrNotFound},
		{name: "item of another library", libraryID: showsID, itemID: itemID, want: domain.ErrNotFound},
		{name: "unknown library", libraryID: "nope", itemID: itemID, want: domain.ErrNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := h.service.Reanalyze(context.Background(), test.libraryID, test.itemID)
			if !errors.Is(err, test.want) {
				t.Errorf("Reanalyze() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestReanalyzeWhileRunningConflicts(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	itemID := scannedMovie(t, h)
	h.prober.entered = make(chan struct{}, 1)
	h.prober.gate = make(chan struct{})

	if err := h.service.Reanalyze(context.Background(), moviesID, itemID); err != nil {
		t.Fatalf("Reanalyze() error = %v", err)
	}

	receive(t, h.prober.entered, "the probe")

	if err := h.service.Reanalyze(context.Background(), moviesID, itemID); !errors.Is(err, domain.ErrConflict) {
		t.Errorf("second Reanalyze() error = %v, want ErrConflict", err)
	}

	close(h.prober.gate)
	h.waitReanalysis(t, itemID)

	if err := h.service.Reanalyze(context.Background(), moviesID, itemID); err != nil {
		t.Errorf("Reanalyze() after the run error = %v", err)
	}

	h.waitReanalysis(t, itemID)
}

func TestReanalyzeRefusesAKindWithoutVideoPipeline(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.libraries.libraries = append(h.libraries.libraries, oneFolder("lib-music", "Music", "music", library.KindMusic))
	h.start(t)

	err := h.service.Reanalyze(context.Background(), "lib-music", "any")
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Reanalyze() error = %v, want ErrInvalid", err)
	}
}

func TestReanalyzeAfterCloseIsUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	itemID := scannedMovie(t, h)

	if err := h.service.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := h.service.Reanalyze(context.Background(), moviesID, itemID); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Reanalyze() error = %v, want ErrUnavailable", err)
	}
}

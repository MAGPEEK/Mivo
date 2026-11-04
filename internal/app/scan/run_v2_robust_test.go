package scan_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestV2EarlyEndStillRefreshesTouchedItems(t *testing.T) {
	t.Parallel()

	h := newHarnessV2(t)
	h.start(t)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Alien (1979).mkv", 1))

	// Heat is saved, then the removal of Alien fails: the scan ends after the save.
	h.catalogV2.removeErr = errors.New("disk full")
	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995).mkv", 2), fileAt("Heat (1995).nfo", 3))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeCatalogFailed)

	if len(h.refresher.last.Sidecars) != 1 {
		t.Errorf("early-end refresh sidecars = %v, want the walked set", h.refresher.last.Sidecars)
	}

	heat := h.catalogV2.itemOf("Heat (1995).mkv")
	if heat == "" || !slices.Contains(h.refresher.last.ItemIDs, heat) {
		t.Errorf("refreshed items = %v, want to contain the saved item %q", h.refresher.last.ItemIDs, heat)
	}

	if h.catalogV2.itemOf("Alien (1979).mkv") == "" {
		t.Error("a failed removal must leave the old file")
	}

	// The next scan converges: the removal goes through and nothing is saved twice.
	h.catalogV2.removeErr = nil
	status = h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995).mkv", 2), fileAt("Heat (1995).nfo", 3))

	assertCounts(t, status, scan.Counts{Unchanged: 1, Removed: 1})

	if h.catalogV2.itemOf("Alien (1979).mkv") != "" || h.catalogV2.count(library.ItemKindMovie) != 1 {
		t.Error("the next scan must remove the vanished file")
	}
}

func TestV2CancelDuringProbesRemovesNothingAndNextScanConverges(t *testing.T) {
	t.Parallel()

	h := newHarnessV2(t)
	h.start(t)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Alien (1979).mkv", 1), fileAt("Heat (1995).mkv", 2))

	h.prober.entered = make(chan struct{})
	h.prober.gate = make(chan struct{})
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("Heat (1995).mkv", 3)}})

	if _, err := h.service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.prober.entered, "the probe")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if h.catalogV2.itemOf("Alien (1979).mkv") == "" {
		t.Fatal("a cancelled scan must not remove files")
	}

	h.prober.entered, h.prober.gate = nil, nil
	h.start(t)

	status := h.scan(t, moviesID)
	assertCounts(t, status, scan.Counts{Updated: 1, Removed: 1})
}

func TestV2OrphanExtraIsKeptWhenTheWalkSkippedSomething(t *testing.T) {
	t.Parallel()

	h := newHarnessV2(t)
	h.start(t)

	movie := fileAt("Heat (1995)/Heat (1995).mkv", 1)
	extra := fileAt("Heat (1995)/Trailers/Teaser.mkv", 2)
	h.scanFiles(t, moviesID, moviesRoot, movie, extra)

	if h.catalogV2.count(library.ItemKindExtra) != 1 {
		t.Fatal("setup: the extra must be stored")
	}

	// The movie file could not be read: its extra looks like an orphan but is not gone.
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{extra}, skipped: []string{movie.RelativePath}})
	status := h.scan(t, moviesID)

	assertCounts(t, status, scan.Counts{Skipped: 1})

	if h.catalogV2.count(library.ItemKindExtra) != 1 || h.catalogV2.count(library.ItemKindMovie) != 1 {
		t.Error("nothing may be removed while the owner may lie in a skipped path")
	}

	// A complete walk without the movie does remove the orphan.
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{extra}})
	status = h.scan(t, moviesID)

	assertCounts(t, status, scan.Counts{Skipped: 1, Removed: 2})
}

func TestRefreshBelowSidecarsScalesWithFilesTimesDepth(t *testing.T) {
	t.Parallel()

	const (
		sidecars = 5000
		files    = 50000
	)

	changed := make([]string, 0, sidecars)
	for number := range sidecars {
		changed = append(changed, fmt.Sprintf("show %d/tvshow.nfo", number*10))
	}

	chains := make(map[string][]string, files)
	for number := range files {
		chains[fmt.Sprintf("show %d/Season 01/e%d.mkv", number/10, number)] = []string{fmt.Sprintf("item-%d", number)}
	}

	chains["root.mkv"] = []string{"root-item"}

	start := time.Now()
	got := scan.RefreshBelowSidecars(append(changed, "root.nfo"), chains)

	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("took %v, want well below 2s", elapsed)
	}

	// Every tenth of the 5000 show folders changed (10 files each), plus the root file.
	if want := files/10 + 1; len(got) != want {
		t.Errorf("refreshed items = %d, want %d", len(got), want)
	}
}

func TestRefreshBelowSidecarsRootSidecarCoversOnlyTheRoot(t *testing.T) {
	t.Parallel()

	chains := map[string][]string{"a.mkv": {"root"}, "dir/b.mkv": {"deep"}}

	if got := scan.RefreshBelowSidecars([]string{"movie.nfo"}, chains); !slices.Equal(got, []string{"root"}) {
		t.Errorf("got %v, want [root]", got)
	}
}

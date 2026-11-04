package scan_test

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	multiID     = "lib-multi"
	firstRoot   = "fa"
	secondRoot  = "fb"
	firstPath   = "a"
	secondPath  = "b"
	heatPath    = "Heat (1995)/Heat (1995).mkv"
	episodePath = "Severance/Season 01/Severance - S01E01 - Good News.mkv"
)

// multiFolder returns a library with two folders.
func multiFolder(kind library.Kind) library.ManagedLibrary {
	return library.ManagedLibrary{
		ID: multiID, Name: "Multi", Kind: kind, Options: library.DefaultOptions(),
		Folders: []library.Folder{{ID: firstRoot, Path: firstPath}, {ID: secondRoot, Path: secondPath}},
	}
}

// useLibrary makes the library source return only library.
func (h *harness) useLibrary(managed library.ManagedLibrary) {
	h.libraries.set(managed)
}

// storedFiles lists the stored files as "root:path", sorted.
func (catalog *memoryCatalogV2) storedFiles() []string {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	var files []string
	for _, file := range catalog.files {
		files = append(files, file.state.RootID+":"+file.state.RelativePath)
	}

	slices.Sort(files)

	return files
}

// storedSidecars lists the stored sidecars as "root:path", sorted.
func (catalog *memoryCatalogV2) storedSidecars() []string {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	var sidecars []string
	for _, state := range catalog.sidecars {
		sidecars = append(sidecars, state.RootID+":"+state.RelativePath)
	}

	slices.Sort(sidecars)

	return sidecars
}

func TestMultiFolderMoviesWithEqualFoldersBecomeOneItem(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1), fileAt("Heat (1995)/Heat (1995).nfo", 5)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt(heatPath, 2), fileAt("Alien (1979).mkv", 3)}})

	status := h.scan(t, multiID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 3})

	if got := h.catalogV2.count(library.ItemKindMovie); got != 2 {
		t.Errorf("movies = %d, want 2 (equal folders merge)", got)
	}

	if got := h.catalogV2.filesPerItem(); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("files per item = %v, want [1 2]", got)
	}

	want := []string{"fa:" + heatPath, "fb:Alien (1979).mkv", "fb:" + heatPath}
	if got := h.catalogV2.storedFiles(); !slices.Equal(got, want) {
		t.Errorf("stored files = %v, want %v", got, want)
	}

	if got := h.catalogV2.storedSidecars(); !slices.Equal(got, []string{"fa:Heat (1995)/Heat (1995).nfo"}) {
		t.Errorf("stored sidecars = %v, want the first folder's nfo with its folder id", got)
	}

	for _, probed := range []string{absolutePath(firstPath, heatPath), absolutePath(secondPath, heatPath)} {
		if !slices.Contains(h.prober.probed, probed) {
			t.Errorf("probed = %v, want %s (each file resolves below its own folder)", h.prober.probed, probed)
		}
	}
}

func TestMultiFolderShowsWithEqualFoldersBecomeOneShow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindShows))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(episodePath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{
		fileAt("Severance/Season 01/Severance - S01E02 - Half Loop.mkv", 2),
	}})

	assertState(t, h.scan(t, multiID), scan.StateIdle, scan.ErrorCodeNone)

	if shows, seasons, episodes := h.catalogV2.count(library.ItemKindShow), h.catalogV2.count(library.ItemKindSeason),
		h.catalogV2.count(library.ItemKindEpisode); shows != 1 || seasons != 1 || episodes != 2 {
		t.Errorf("shows, seasons, episodes = %d, %d, %d, want 1, 1, 2", shows, seasons, episodes)
	}
}

func TestMultiFolderRescanKeepsEqualPathsApartAndUnchanged(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt(heatPath, 2)}})
	h.scan(t, multiID)

	status := h.scan(t, multiID)
	assertCounts(t, status, scan.Counts{Unchanged: 2})

	// Removing the file of one folder must not touch the equally named file of the other.
	h.walker.set(secondPath, walkResult{})
	status = h.scan(t, multiID)

	assertCounts(t, status, scan.Counts{Unchanged: 1, Removed: 1})

	if got := h.catalogV2.storedFiles(); !slices.Equal(got, []string{"fa:" + heatPath}) {
		t.Errorf("stored files = %v, want only the first folder's", got)
	}
}

func TestUnavailableFolderKeepsItsEntriesAndTheOtherFolderIsScanned(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2), fileAt("Alien (1979).nfo", 3)}})
	h.scan(t, multiID)

	h.walker.forget(secondPath)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1), fileAt("Ronin (1998).mkv", 4)}})

	status := h.scan(t, multiID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 1, Unchanged: 1})

	if status.UnavailableFolders != 1 {
		t.Errorf("UnavailableFolders = %d, want 1", status.UnavailableFolders)
	}

	want := []string{"fa:" + heatPath, "fa:Ronin (1998).mkv", "fb:Alien (1979).mkv"}
	if got := h.catalogV2.storedFiles(); !slices.Equal(got, want) {
		t.Errorf("stored files = %v, want %v (the lost folder keeps its entries)", got, want)
	}

	if got := h.catalogV2.storedSidecars(); !slices.Equal(got, []string{"fb:Alien (1979).nfo"}) {
		t.Errorf("stored sidecars = %v, want the lost folder's nfo kept", got)
	}

	// The folder comes back: nothing was lost, nothing is probed again.
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2), fileAt("Alien (1979).nfo", 3)}})
	status = h.scan(t, multiID)

	assertCounts(t, status, scan.Counts{Unchanged: 3})

	if status.UnavailableFolders != 0 {
		t.Errorf("UnavailableFolders after recovery = %d, want 0", status.UnavailableFolders)
	}
}

func TestFolderLostDuringItsWalkKeepsItsEntries(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2)}})
	h.scan(t, multiID)

	// The second folder reports one file and then vanishes: the partial listing must not
	// count as proof that the other files are gone.
	h.walker.set(secondPath, walkResult{
		files:     []library.FileState{fileAt("Ronin (1998).mkv", 4)},
		failAfter: fmt.Errorf("share went away: %w", domain.ErrUnavailable),
	})

	status := h.scan(t, multiID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Unchanged: 1})

	want := []string{"fa:" + heatPath, "fb:Alien (1979).mkv"}
	if got := h.catalogV2.storedFiles(); !slices.Equal(got, want) {
		t.Errorf("stored files = %v, want %v", got, want)
	}
}

func TestAllFoldersUnavailableFailsTheScanAndKeepsTheCatalog(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2)}})
	h.scan(t, multiID)

	h.walker.forget(firstPath)
	h.walker.forget(secondPath)

	status := h.scan(t, multiID)

	assertState(t, status, scan.StateFailed, scan.ErrorCodeRootUnavailable)

	if status.UnavailableFolders != 2 {
		t.Errorf("UnavailableFolders = %d, want 2", status.UnavailableFolders)
	}

	if got := h.catalogV2.storedFiles(); len(got) != 2 {
		t.Errorf("stored files = %v, want both kept", got)
	}
}

func TestLibraryEditsApplyToTheNextScan(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.useLibrary(multiFolder(library.KindMovies))
	h.start(t)
	h.walker.set(firstPath, walkResult{files: []library.FileState{fileAt(heatPath, 1)}})
	h.walker.set(secondPath, walkResult{files: []library.FileState{fileAt("Alien (1979).mkv", 2)}})
	h.scan(t, multiID)

	// The second folder is removed from the library without a restart.
	edited := multiFolder(library.KindMovies)
	edited.Folders = edited.Folders[:1]
	h.useLibrary(edited)

	status := h.scan(t, multiID)

	assertCounts(t, status, scan.Counts{Unchanged: 1, Removed: 1}) // the removed folder's entries go

	if status.UnavailableFolders != 0 {
		t.Errorf("UnavailableFolders = %d, want 0", status.UnavailableFolders)
	}
}

func TestExtrasOptionOffIgnoresAndRemovesExtras(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	files := []library.FileState{fileAt("Heat (1995)/Heat (1995).mkv", 1), fileAt("Heat (1995)/trailer.mkv", 2)}
	assertCounts(t, h.scanFiles(t, moviesID, moviesRoot, files...), scan.Counts{Added: 2})

	if h.catalogV2.count(library.ItemKindExtra) != 1 {
		t.Fatal("setup: the extra must be stored")
	}

	managed := oneFolder(moviesID, "Movies", moviesRoot, library.KindMovies)
	managed.Options.ScanExtras = false
	h.useLibrary(managed)

	status := h.scan(t, moviesID)

	assertCounts(t, status, scan.Counts{Unchanged: 1, Removed: 1, Skipped: 1})

	if h.catalogV2.count(library.ItemKindExtra) != 0 {
		t.Error("extras must disappear when the option is switched off")
	}
}

func TestOptionChangeRefreshesEveryItemOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	files := []library.FileState{fileAt("Heat (1995).mkv", 1), fileAt("Alien (1979).mkv", 2)}
	h.scanFiles(t, moviesID, moviesRoot, files...)

	requested := func() []string {
		h.refresher.mutex.Lock()
		defer h.refresher.mutex.Unlock()

		return h.refresher.last.ItemIDs
	}

	h.scan(t, moviesID)

	if got := requested(); len(got) != 0 {
		t.Fatalf("unchanged rescan requested %v, want no items", got)
	}

	changed := oneFolder(moviesID, "Movies", moviesRoot, library.KindMovies)
	changed.Options.ReadNFO = false
	h.useLibrary(changed)
	h.scan(t, moviesID)

	if got := requested(); len(got) != 2 {
		t.Errorf("after the option change requested %v, want both items", got)
	}

	if last := h.refresher.last.Library.Options; last.ReadNFO {
		t.Errorf("refresh request options = %+v, want ReadNFO off", last)
	}

	h.scan(t, moviesID)

	if got := requested(); len(got) != 0 {
		t.Errorf("rescan with unchanged options requested %v, want no items", got)
	}
}

func TestCancelStopsARunningScanAndDeletesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)
	h.scanFiles(t, moviesID, moviesRoot, fileAt("a.mkv", 1), fileAt("b.mkv", 2))

	h.walker.set(moviesRoot, walkResult{})
	h.walker.started = make(chan struct{}, 1)
	h.walker.release = make(chan struct{})

	if _, err := h.service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.walker.started, "the walk to start")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.Cancel(ctx, moviesID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	status := h.service.Status(moviesID)
	assertState(t, status, scan.StateFailed, scan.ErrorCodeCanceled)

	if got := h.catalogV2.storedFiles(); len(got) != 2 {
		t.Errorf("stored files = %v, want both kept: the walk never completed", got)
	}

	// Another library is not affected, and a new scan of the canceled one works.
	close(h.walker.release)
	h.walker.started = nil
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
	assertState(t, h.scan(t, moviesID), scan.StateIdle, scan.ErrorCodeNone)
}

func TestCancelOnlyStopsTheNamedLibrary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.walker.set(showsRoot, walkResult{})
	h.walker.started = make(chan struct{}, 2)
	h.walker.release = make(chan struct{})
	h.start(t)

	for _, id := range []string{moviesID, showsID} {
		if _, err := h.service.Start(context.Background(), id); err != nil {
			t.Fatalf("Start(%s) error = %v", id, err)
		}
	}

	receive(t, h.walker.started, "first walk")
	receive(t, h.walker.started, "second walk")

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.service.Cancel(ctx, showsID); err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}

	assertState(t, h.service.Status(showsID), scan.StateFailed, scan.ErrorCodeCanceled)

	if state := h.service.Status(moviesID).State; state != scan.StateRunning {
		t.Errorf("movies state = %q, want it still running", state)
	}
}

func TestCancelAndWaitIdleWithoutAScanReturnAtOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	service := h.start(t)

	if err := service.Cancel(context.Background(), moviesID); err != nil {
		t.Errorf("Cancel() without a scan error = %v", err)
	}

	if err := service.WaitIdle(context.Background(), moviesID); err != nil {
		t.Errorf("WaitIdle() without a scan error = %v", err)
	}
}

func TestWaitIdleAndCancelAreBoundedByTheirContext(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{})
	h.walker.started = make(chan struct{}, 1)
	h.walker.release = make(chan struct{})
	h.walker.stubborn = true
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	receive(t, h.walker.started, "the walk to start")

	expired, cancel := context.WithCancel(context.Background())
	cancel()

	if err := service.WaitIdle(expired, moviesID); err == nil {
		t.Error("WaitIdle() of a running scan with a done context returned nil")
	}

	if err := service.Cancel(expired, moviesID); err == nil {
		t.Error("Cancel() of a stuck scan with a done context returned nil")
	}

	close(h.walker.release)

	ctx, stop := context.WithTimeout(context.Background(), testTimeout)
	defer stop()

	if err := service.WaitIdle(ctx, moviesID); err != nil {
		t.Errorf("WaitIdle() after the walk was released error = %v", err)
	}
}

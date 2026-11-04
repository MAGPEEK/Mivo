package scan_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// scanMovies sets the files of the movies library and scans it.
func (h *harness) scanFiles(t *testing.T, libraryID, root string, files ...library.FileState) scan.Status {
	t.Helper()

	h.walker.set(root, walkResult{files: files})

	return h.scan(t, libraryID)
}

func TestV2MovieFolderWithVersionsAndTrailer(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	status := h.scanFiles(t, moviesID, moviesRoot,
		fileAt("Heat (1995)/Heat (1995) - Theatrical.mkv", 1),
		fileAt("Heat (1995)/Heat (1995) - Director's Cut.mkv", 2),
		fileAt("Heat (1995)/trailer.mkv", 3),
		fileAt("Heat (1995)/Heat (1995).nfo", 4),
		fileAt("Alien (1979).mkv", 5),
		fileAt("Alien (1979)-sample.mkv", 6),
	)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 4, Skipped: 1})

	if got := h.catalogV2.count(library.ItemKindMovie); got != 2 {
		t.Errorf("movies = %d, want 2", got)
	}

	if got := h.catalogV2.count(library.ItemKindExtra); got != 1 {
		t.Errorf("extras = %d, want 1", got)
	}

	if got := h.catalogV2.filesPerItem(); !slices.Equal(got, []int{1, 1, 2}) {
		t.Errorf("files per item = %v, want [1 1 2]", got)
	}

	if len(h.catalogV2.sidecars) != 1 {
		t.Errorf("sidecars = %d, want 1", len(h.catalogV2.sidecars))
	}
}

func TestV2ExtraWithoutParentMovieIsSkipped(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/trailer.mkv", 1))

	assertCounts(t, status, scan.Counts{Skipped: 1})

	if got := h.catalogV2.count(library.ItemKindExtra) + h.catalogV2.count(library.ItemKindMovie); got != 0 {
		t.Errorf("items = %d, want 0", got)
	}
}

func TestV2RescanProbesAndMovesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	files := []library.FileState{
		fileAt("Heat (1995)/Heat (1995).mkv", 1),
		fileAt("Heat (1995)/Heat (1995).nfo", 2),
	}
	h.scanFiles(t, moviesID, moviesRoot, files...)

	probes := len(h.prober.probedPaths())
	status := h.scanFiles(t, moviesID, moviesRoot, files...)

	assertCounts(t, status, scan.Counts{Unchanged: 1})

	if got := len(h.prober.probedPaths()); got != probes {
		t.Errorf("probes after rescan = %d, want %d", got, probes)
	}

	if h.catalogV2.moves != 0 || h.catalogV2.saves != 1 || h.catalogV2.sidecarWrites != 1 {
		t.Errorf("moves, saves, sidecar writes = %d, %d, %d, want 0, 1, 1",
			h.catalogV2.moves, h.catalogV2.saves, h.catalogV2.sidecarWrites)
	}

	if len(h.refresher.last.ItemIDs) != 0 {
		t.Errorf("refreshed items = %v, want none", h.refresher.last.ItemIDs)
	}
}

func TestV2FolderRenameMovesWithoutProbeAndKeepsFileRemovesOldMovie(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/Heat (1995).mkv", 1))
	probes := len(h.prober.probedPaths())

	// Same file state under a new path is a new file; an unchanged path with a new
	// placement is a move: the version label changes with the name.
	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/Heat (1995).mkv", 1))
	assertCounts(t, status, scan.Counts{Unchanged: 1})

	status = h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995)/Heat (1995) - Remastered.mkv", 1))

	if got := len(h.prober.probedPaths()); got != probes+1 {
		t.Errorf("probes = %d, want %d", got, probes+1)
	}

	assertCounts(t, status, scan.Counts{Added: 1, Removed: 1})

	if got := h.catalogV2.count(library.ItemKindMovie); got != 1 {
		t.Errorf("movies = %d, want 1", got)
	}
}

func TestV2ShowsLibrary(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	status := h.scanFiles(t, showsID, showsRoot,
		fileAt("Severance/Season 01/Severance - S01E01 - Good News.mkv", 1),
		fileAt("Severance/Season 01/Severance - S01E02.mkv", 2),
		fileAt("Severance/Season 01/Severance - S01E03-E04.mkv", 3),
		fileAt("Severance/Season 00/Severance - S00E01 - Special.mkv", 4),
		fileAt("Severance/tvshow.nfo", 5),
	)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

	if got := h.catalogV2.count(library.ItemKindShow); got != 1 {
		t.Errorf("shows = %d, want 1", got)
	}

	if got := h.catalogV2.count(library.ItemKindSeason); got != 2 {
		t.Errorf("seasons = %d, want 2", got)
	}

	if got := h.catalogV2.count(library.ItemKindEpisode); got != 4 {
		t.Errorf("episodes = %d, want 4", got)
	}
}

func TestV2RenamedEpisodeKeepsItsID(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	h.scanFiles(t, showsID, showsRoot, fileAt("Severance/Season 01/Severance - S01E01 - Good News.mkv", 1))
	before := h.catalogV2.itemOf("Severance/Season 01/Severance - S01E01 - Good News.mkv")

	status := h.scanFiles(t, showsID, showsRoot, fileAt("Severance/Season 01/Severance - S01E01 - Renamed.mkv", 1))
	after := h.catalogV2.itemOf("Severance/Season 01/Severance - S01E01 - Renamed.mkv")

	if before == "" || before != after {
		t.Errorf("episode item before, after = %q, %q, want the same", before, after)
	}

	assertCounts(t, status, scan.Counts{Added: 1, Removed: 1})

	if got := h.catalogV2.count(library.ItemKindEpisode); got != 1 {
		t.Errorf("episodes = %d, want 1", got)
	}
}

func TestV2ShowFolderRenameMovesFilesAndRemovesOldShow(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	h.scanFiles(t, showsID, showsRoot, fileAt("Old Name/Season 01/Old Name - S01E01.mkv", 1))
	h.scanFiles(t, showsID, showsRoot, fileAt("Old Name/Season 01/Old Name - S01E01.mkv", 1))

	// The file keeps its path and state; only a different folder structure above moves it. A
	// moved file is simulated by a stored placement key that no longer matches.
	for _, file := range h.catalogV2.files {
		file.placementKey = "stale"
	}

	status := h.scanFiles(t, showsID, showsRoot, fileAt("Old Name/Season 01/Old Name - S01E01.mkv", 1))

	assertCounts(t, status, scan.Counts{Updated: 1})

	if h.catalogV2.moves != 1 {
		t.Errorf("moves = %d, want 1", h.catalogV2.moves)
	}

	if got := h.catalogV2.count(library.ItemKindShow); got != 1 {
		t.Errorf("shows = %d, want 1 (no leftover)", got)
	}
}

func TestV2AbsoluteNumberedShowIsOneShowOneSeason(t *testing.T) {
	t.Parallel()

	const episodes = 131

	h := newHarness(t)
	h.start(t)

	files := make([]library.FileState, 0, episodes)
	for number := 1; number <= episodes; number++ {
		files = append(files, fileAt(fmt.Sprintf("One Piece/One Piece - %03d.mkv", number), int64(number)))
	}

	status := h.scanFiles(t, showsID, showsRoot, files...)

	assertCounts(t, status, scan.Counts{Added: episodes})

	if h.catalogV2.count(library.ItemKindShow) != 1 || h.catalogV2.count(library.ItemKindSeason) != 1 ||
		h.catalogV2.count(library.ItemKindEpisode) != episodes {
		t.Errorf("show, season, episode = %d, %d, %d, want 1, 1, %d",
			h.catalogV2.count(library.ItemKindShow), h.catalogV2.count(library.ItemKindSeason),
			h.catalogV2.count(library.ItemKindEpisode), episodes)
	}
}

func TestV2RefreshSetCoversSidecarChanges(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	movie := fileAt("Heat (1995)/Heat (1995).mkv", 1)
	other := fileAt("Alien (1979)/Alien (1979).mkv", 2)
	h.scanFiles(t, moviesID, moviesRoot, movie, other, fileAt("Heat (1995)/Heat (1995).nfo", 3))

	if len(h.refresher.last.ItemIDs) != 2 {
		t.Fatalf("first refresh = %v, want both movies", h.refresher.last.ItemIDs)
	}

	h.scanFiles(t, moviesID, moviesRoot, movie, other, fileAt("Heat (1995)/Heat (1995).nfo", 33))

	want := []string{h.catalogV2.itemOf(movie.RelativePath)}
	if !slices.Equal(h.refresher.last.ItemIDs, want) {
		t.Errorf("refresh after sidecar change = %v, want %v", h.refresher.last.ItemIDs, want)
	}

	h.scanFiles(t, moviesID, moviesRoot, movie, other)

	if !slices.Equal(h.refresher.last.ItemIDs, want) || len(h.catalogV2.sidecars) != 0 {
		t.Errorf("after sidecar removal: refresh %v sidecars %d", h.refresher.last.ItemIDs, len(h.catalogV2.sidecars))
	}
}

func TestV2SafetyRules(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.start(t)

	h.scanFiles(t, moviesID, moviesRoot, fileAt("Locked/Heat (1995)/Heat (1995).mkv", 1), fileAt("Alien (1979).mkv", 2))

	h.walker.set(moviesRoot, walkResult{skipped: []string{"Locked"}})
	status := h.scan(t, moviesID)

	assertCounts(t, status, scan.Counts{Removed: 1})

	h.walker.forget(moviesRoot)
	status = h.scan(t, moviesID)
	assertState(t, status, scan.StateFailed, scan.ErrorCodeRootUnavailable)

	if got := h.catalogV2.count(library.ItemKindMovie); got != 1 {
		t.Errorf("movies = %d, want 1: lost root must not empty the catalog", got)
	}
}

func TestV2RefreshFailureFailsScanAndKeepsSidecarsUnsaved(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.refresher.err = errors.New("boom")
	h.start(t)

	status := h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995).mkv", 1), fileAt("Heat (1995).nfo", 2))

	assertState(t, status, scan.StateFailed, scan.ErrorCodeFailed)

	if len(h.catalogV2.sidecars) != 0 {
		t.Errorf("sidecars saved = %d, want 0 so the next scan retries", len(h.catalogV2.sidecars))
	}
}

func TestV2DefaultsAndFinished(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.refresher = nil

	var (
		mutex    sync.Mutex
		finished []string
	)

	h.finished = func(_ context.Context, libraryID string) {
		mutex.Lock()
		defer mutex.Unlock()

		finished = append(finished, libraryID)
	}

	h.start(t)
	h.scanFiles(t, moviesID, moviesRoot, fileAt("Heat (1995).mkv", 1))
	h.walker.forget(showsRoot)
	h.scan(t, showsID)

	mutex.Lock()
	defer mutex.Unlock()

	if !slices.Equal(finished, []string{moviesID, showsID}) {
		t.Errorf("finished = %v, want both libraries (also after a failed scan)", finished)
	}
}

func TestFinishedIsCalledAfterEveryScan(t *testing.T) {
	t.Parallel()

	h := newHarness(t)

	called := 0
	h.finished = func(context.Context, string) { called++ }

	h.start(t)
	h.walker.set(moviesRoot, walkResult{})
	h.scan(t, moviesID)

	if called != 1 {
		t.Errorf("finished calls = %d, want 1", called)
	}
}

func TestV2ValidationNeedsACatalog(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.catalogV2 = nil

	if _, err := scan.NewService(h.dependencies()); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("NewService without any catalog error = %v, want ErrInvalid", err)
	}
}

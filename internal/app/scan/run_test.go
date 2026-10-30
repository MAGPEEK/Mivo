package scan_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestFirstScanAddsEveryVideoFile(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{
		fileAt("Über_Film.2019.mkv", 10), fileAt("Sub/The.Big.Movie.mp4", 20),
	}})
	h.start(t)

	status := h.scan(t, moviesID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 2})

	if status.StartedAt.IsZero() || !status.FinishedAt.After(status.StartedAt) {
		t.Errorf("started, finished = %s, %s, want a finish after the start", status.StartedAt, status.FinishedAt)
	}

	stored := h.catalog.byPath()
	assertAddedFile(t, stored["Über_Film.2019.mkv"], fileAt("Über_Film.2019.mkv", 10), "Über Film 2019")
	assertAddedFile(t, stored["Sub/The.Big.Movie.mp4"], fileAt("Sub/The.Big.Movie.mp4", 20), "The Big Movie")

	wantProbed := []string{
		absolutePath(moviesRoot, "Sub/The.Big.Movie.mp4"), absolutePath(moviesRoot, "Über_Film.2019.mkv"),
	}
	if got := h.prober.probedPaths(); !slices.Equal(got, wantProbed) {
		t.Errorf("probed %q, want %q", got, wantProbed)
	}
}

func assertAddedFile(t *testing.T, record scan.ProbedFile, state library.FileState, title string) {
	t.Helper()

	item, file := record.Item, record.File

	if item.ID == "" || file.ID == "" || item.ID == file.ID || file.ItemID != item.ID {
		t.Errorf("%s: item id %q, file id %q, file item id %q, want distinct new ids that link",
			state.RelativePath, item.ID, file.ID, file.ItemID)
	}

	if item.LibraryID != moviesID || file.LibraryID != moviesID || item.Kind != library.ItemKindMovie {
		t.Errorf("%s: library ids %q/%q, kind %q, want %q and movie",
			state.RelativePath, item.LibraryID, file.LibraryID, item.Kind, moviesID)
	}

	if item.Title != title || item.SortTitle != library.SortTitle(title) {
		t.Errorf("%s: title %q, sort title %q, want %q", state.RelativePath, item.Title, item.SortTitle, title)
	}

	if file.RelativePath != state.RelativePath || file.Size != state.Size || !file.ModifiedAt.Equal(state.ModifiedAt) {
		t.Errorf("%s: stored state %q/%d/%s, want %+v",
			state.RelativePath, file.RelativePath, file.Size, file.ModifiedAt, state)
	}

	if !file.Probed() || len(file.Description.Streams) != len(videoDescription().Streams) {
		t.Errorf("%s: probe error %q with %d streams, want the probed description",
			state.RelativePath, file.ProbeError, len(file.Description.Streams))
	}

	if item.CreatedAt.IsZero() || record.ProbedAt.IsZero() {
		t.Errorf("%s: created %s, probed %s, want clock times", state.RelativePath, item.CreatedAt, record.ProbedAt)
	}
}

func TestShowsLibraryCreatesEpisodes(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(showsRoot, walkResult{files: []library.FileState{fileAt("Show/S01E01.mkv", 1)}})
	h.start(t)

	h.scan(t, showsID)

	if got := h.catalog.byPath()["Show/S01E01.mkv"].Item.Kind; got != library.ItemKindEpisode {
		t.Errorf("item kind = %q, want episode", got)
	}
}

func TestRescanOfUnchangedFilesProbesNothing(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
	h.start(t)
	h.scan(t, moviesID)

	// The walker reports a finer time than the catalog keeps; that is not a change.
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{
		fileAt("a.mkv", 1),
		{RelativePath: "b.mkv", Size: 2, ModifiedAt: fileAt("b.mkv", 2).ModifiedAt.Add(time.Microsecond)},
	}})

	status := h.scan(t, moviesID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Unchanged: 2})

	if probed := len(h.prober.probedPaths()); probed != 2 {
		t.Errorf("prober called %d times over both scans, want 2", probed)
	}

	if saves := h.catalog.saveCount(); saves != 2 {
		t.Errorf("catalog saved %d times over both scans, want 2", saves)
	}
}

func TestChangedFileIsProbedAgainAndKeepsItsIDs(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
	h.start(t)
	h.scan(t, moviesID)
	before := h.catalog.byPath()["a.mkv"]

	changed := fileAt("a.mkv", 100)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{changed, fileAt("b.mkv", 2)}})

	status := h.scan(t, moviesID)

	assertCounts(t, status, scan.Counts{Updated: 1, Unchanged: 1})

	after := h.catalog.byPath()["a.mkv"]
	if after.File.ID != before.File.ID || after.Item.ID != before.Item.ID || after.File.ItemID != before.Item.ID {
		t.Errorf("ids after change = file %q item %q, want file %q item %q",
			after.File.ID, after.Item.ID, before.File.ID, before.Item.ID)
	}

	if after.File.Size != changed.Size || !after.File.ModifiedAt.Equal(changed.ModifiedAt) {
		t.Errorf("stored size, mtime = %d, %s, want %d, %s",
			after.File.Size, after.File.ModifiedAt, changed.Size, changed.ModifiedAt)
	}
}

func TestVanishedFileIsRemoved(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
	h.start(t)
	h.scan(t, moviesID)

	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("b.mkv", 2)}})

	status := h.scan(t, moviesID)

	assertCounts(t, status, scan.Counts{Removed: 1, Unchanged: 1})

	if got := h.catalog.paths(); !slices.Equal(got, []string{"b.mkv"}) {
		t.Errorf("catalog paths = %q, want [b.mkv]", got)
	}
}

func TestFilesBelowSkippedPathsAreKept(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{
		fileAt("Locked/a.mkv", 1), fileAt("Locked/Deep/b.mkv", 2), fileAt("Lockedness.mkv", 3),
		fileAt("unreadable.mkv", 4), fileAt("gone/c.mkv", 5), fileAt("kept.mkv", 6),
	}})
	h.start(t)
	h.scan(t, moviesID)

	h.walker.set(moviesRoot, walkResult{
		files:   []library.FileState{fileAt("kept.mkv", 6)},
		skipped: []string{"Locked", "unreadable.mkv"},
	})

	status := h.scan(t, moviesID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Removed: 2, Unchanged: 1})

	want := []string{"Locked/Deep/b.mkv", "Locked/a.mkv", "kept.mkv", "unreadable.mkv"}
	if got := h.catalog.paths(); !slices.Equal(got, want) {
		t.Errorf("catalog paths = %q, want %q", got, want)
	}
}

func TestLostRootDeletesNothing(t *testing.T) {
	t.Parallel()

	lost := fmt.Errorf("walk: root went away: %w", domain.ErrUnavailable)

	tests := []struct {
		name   string
		result walkResult
		exists bool
	}{
		{name: "root missing before the walk", exists: false},
		{name: "root lost after some files", result: walkResult{
			files: []library.FileState{fileAt("a.mkv", 1)}, failAfter: lost,
		}, exists: true},
		{name: "root lost before any file", result: walkResult{failAfter: lost}, exists: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
			h.start(t)
			h.scan(t, moviesID)

			if test.exists {
				h.walker.set(moviesRoot, test.result)
			} else {
				h.walker.forget(moviesRoot)
			}

			status := h.scan(t, moviesID)

			assertState(t, status, scan.StateFailed, scan.ErrorCodeRootUnavailable)
			assertCounts(t, status, scan.Counts{})

			if got := h.catalog.paths(); !slices.Equal(got, []string{"a.mkv", "b.mkv"}) {
				t.Errorf("catalog paths = %q, want both files kept", got)
			}
		})
	}
}

func TestProbeErrorsAreStoredAndCountedWithoutAbortingTheScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want media.ProbeError
	}{
		{name: "unsupported", err: fmt.Errorf("probe x.mkv: %w", domain.ErrUnsupported), want: media.ProbeErrorFailed},
		{name: "timed out", err: fmt.Errorf("probe x.mkv: %w", context.DeadlineExceeded), want: media.ProbeErrorTimedOut},
		{name: "unknown error", err: errors.New("something odd"), want: media.ProbeErrorFailed},
		{name: "invalid description", want: media.ProbeErrorFailed},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("bad.mkv", 1), fileAt("good.mkv", 2)}})

			outcome := probeOutcome{err: test.err}
			if test.err == nil {
				outcome.description = videoDescription()
				outcome.description.Duration = -time.Second
			}

			h.prober.script(absolutePath(moviesRoot, "bad.mkv"), outcome)
			h.start(t)

			status := h.scan(t, moviesID)

			assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
			assertCounts(t, status, scan.Counts{Added: 1, Failed: 1})

			bad := h.catalog.byPath()["bad.mkv"]
			if bad.File.ProbeError != test.want || len(bad.File.Description.Streams) != 0 {
				t.Errorf("stored probe error %q with %d streams, want %q and none",
					bad.File.ProbeError, len(bad.File.Description.Streams), test.want)
			}

			if bad.Item.Title != "bad" {
				t.Errorf("failed file item title = %q, want bad", bad.Item.Title)
			}

			if !strings.Contains(h.logs.String(), "bad.mkv") {
				t.Error("log does not mention the failed file")
			}
		})
	}
}

func TestFailedFileIsRetriedOnlyWhenItChanges(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("bad.mkv", 1)}})
	h.prober.script(absolutePath(moviesRoot, "bad.mkv"), probeOutcome{err: domain.ErrUnsupported})
	h.start(t)
	h.scan(t, moviesID)

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Unchanged: 1})

	h.prober.script(absolutePath(moviesRoot, "bad.mkv"), probeOutcome{description: videoDescription()})
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("bad.mkv", 5)}})

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Updated: 1})

	if got := h.catalog.byPath()["bad.mkv"].File; !got.Probed() {
		t.Errorf("probe error after successful re-probe = %q, want none", got.ProbeError)
	}
}

func TestFileWithoutVideoStreamIsSkipped(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("music.mkv", 1), fileAt("was-video.mkv", 2)}})
	h.prober.script(absolutePath(moviesRoot, "music.mkv"), probeOutcome{description: audioOnlyDescription()})
	h.start(t)

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Added: 1, Skipped: 1})

	if got := h.catalog.paths(); !slices.Equal(got, []string{"was-video.mkv"}) {
		t.Errorf("catalog paths = %q, want only the video", got)
	}

	// A stored video that turns out to have no video stream any more leaves the catalog.
	h.prober.script(absolutePath(moviesRoot, "was-video.mkv"), probeOutcome{description: audioOnlyDescription()})
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("music.mkv", 1), fileAt("was-video.mkv", 3)}})

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Skipped: 2})

	if got := h.catalog.paths(); len(got) != 0 {
		t.Errorf("catalog paths = %q, want none", got)
	}
}

func TestMissingProberAbortsTheScanWithoutMarkingFiles(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{
		fileAt("a.mkv", 1), fileAt("b.mkv", 2), fileAt("c.mkv", 3), fileAt("d.mkv", 4),
	}})

	for _, name := range []string{"a.mkv", "b.mkv", "c.mkv", "d.mkv"} {
		h.prober.script(absolutePath(moviesRoot, name), probeOutcome{
			err: fmt.Errorf("start ffprobe: %w", domain.ErrUnavailable),
		})
	}

	h.start(t)

	status := h.scan(t, moviesID)

	assertState(t, status, scan.StateFailed, scan.ErrorCodeProberUnavailable)
	assertCounts(t, status, scan.Counts{})

	if saves := h.catalog.saveCount(); saves != 0 {
		t.Errorf("catalog saved %d files, want 0", saves)
	}
}

func TestProbeCanceledByTheProberStopsWithoutRecordingFailures(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.prober.script(absolutePath(moviesRoot, "a.mkv"), probeOutcome{err: fmt.Errorf("probe a.mkv: %w", context.Canceled)})
	h.start(t)

	status := h.scan(t, moviesID)

	assertState(t, status, scan.StateFailed, scan.ErrorCodeCanceled)
	assertCounts(t, status, scan.Counts{})

	if saves := h.catalog.saveCount(); saves != 0 {
		t.Errorf("catalog saved %d files, want 0", saves)
	}
}

func TestRemovingAFileThatLostItsVideoStreamCanFail(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.start(t)
	h.scan(t, moviesID)

	h.prober.script(absolutePath(moviesRoot, "a.mkv"), probeOutcome{description: audioOnlyDescription()})
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 2)}})
	h.catalog.removeErr = errors.New("disk I/O error")

	assertState(t, h.scan(t, moviesID), scan.StateFailed, scan.ErrorCodeCatalogFailed)
}

func TestAPathTheWalkerReportsTwiceIsProbedOnce(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("a.mkv", 1)}})
	h.start(t)

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Added: 1})

	if probed := len(h.prober.probedPaths()); probed != 1 {
		t.Errorf("prober called %d times, want 1", probed)
	}
}

func TestResolveErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantState scan.State
		wantCode  scan.ErrorCode
		wantCount scan.Counts
	}{
		{
			name: "file vanished", err: fmt.Errorf("resolve: %w", domain.ErrNotFound),
			wantState: scan.StateIdle, wantCount: scan.Counts{Added: 1, Failed: 1},
		},
		{
			name: "link leaves the root", err: fmt.Errorf("resolve: %w", domain.ErrForbidden),
			wantState: scan.StateIdle, wantCount: scan.Counts{Added: 1, Failed: 1},
		},
		{
			name: "other error", err: errors.New("resolver bug"),
			wantState: scan.StateIdle, wantCount: scan.Counts{Added: 1, Failed: 1},
		},
		{
			name: "root lost", err: fmt.Errorf("resolve: %w", domain.ErrUnavailable),
			wantState: scan.StateFailed, wantCode: scan.ErrorCodeRootUnavailable,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
			h.resolver.errs["a.mkv"] = test.err
			h.start(t)

			status := h.scan(t, moviesID)

			if test.wantState == scan.StateFailed {
				assertState(t, status, test.wantState, test.wantCode)

				if status.Counts.Failed != 0 {
					t.Errorf("failed count = %d after an abort, want 0", status.Counts.Failed)
				}

				return
			}

			assertState(t, status, test.wantState, test.wantCode)
			assertCounts(t, status, test.wantCount)

			stored, found := h.catalog.byPath()["a.mkv"]
			if !found || stored.File.ProbeError != media.ProbeErrorFailed || len(stored.File.Description.Streams) != 0 {
				t.Errorf("unresolvable file stored = %v with %+v, want probe_failed without streams", found, stored.File)
			}
		})
	}
}

func TestAChangedFileThatBecameUnresolvableLosesItsPlayableDescription(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
	h.start(t)
	h.scan(t, moviesID)
	before := h.catalog.byPath()["a.mkv"]

	h.resolver.errs["a.mkv"] = fmt.Errorf("resolve: %w", domain.ErrForbidden)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 2)}})

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Failed: 1})

	after := h.catalog.byPath()["a.mkv"].File
	if after.ID != before.File.ID || after.ProbeError != media.ProbeErrorFailed || len(after.Description.Streams) != 0 {
		t.Errorf("stored after the change = %+v, want file %s with probe_failed and no streams", after, before.File.ID)
	}
}

func TestUnchangedFilesWithoutVideoAreNotProbedAgain(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("music.mkv", 1), fileAt("movie.mkv", 2)}})
	h.prober.script(absolutePath(moviesRoot, "music.mkv"), probeOutcome{description: audioOnlyDescription()})
	h.start(t)

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Added: 1, Skipped: 1})
	assertCounts(t, h.scan(t, moviesID), scan.Counts{Unchanged: 1, Skipped: 1})

	if probed := len(h.prober.probedPaths()); probed != 2 {
		t.Errorf("prober called %d times over two scans, want 2", probed)
	}

	// A changed file is probed again and may turn out to be a video after all.
	h.prober.script(absolutePath(moviesRoot, "music.mkv"), probeOutcome{description: videoDescription()})
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("music.mkv", 3), fileAt("movie.mkv", 2)}})

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Added: 1, Unchanged: 1})

	// A file that vanished is forgotten: when it comes back unchanged it is probed again.
	h.prober.script(absolutePath(moviesRoot, "other.mkv"), probeOutcome{description: audioOnlyDescription()})
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("other.mkv", 4)}})
	h.scan(t, moviesID)
	h.walker.set(moviesRoot, walkResult{})
	h.scan(t, moviesID)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("other.mkv", 4)}})

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Skipped: 1})

	if probed := len(h.prober.probedPaths()); probed != 5 {
		t.Errorf("prober called %d times in total, want 5", probed)
	}
}

func TestSkippedPathsWithATrailingSlashProtectTheirFiles(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("Locked/a.mkv", 1), fileAt("gone.mkv", 2)}})
	h.start(t)
	h.scan(t, moviesID)

	h.walker.set(moviesRoot, walkResult{skipped: []string{"Locked/"}})

	assertCounts(t, h.scan(t, moviesID), scan.Counts{Removed: 1})

	if got := h.catalog.paths(); !slices.Equal(got, []string{"Locked/a.mkv"}) {
		t.Errorf("catalog paths = %q, want [Locked/a.mkv]", got)
	}
}

func TestAFailedScanLogsNoAbsolutePathAndTheCodeOnce(t *testing.T) {
	t.Parallel()

	const hiddenRoot = "/srv/nas-share/movies"

	tests := []struct {
		name    string
		prepare func(*harness)
		code    scan.ErrorCode
	}{
		{name: "root lost", code: scan.ErrorCodeRootUnavailable, prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{failAfter: fmt.Errorf("open %s: %w", hiddenRoot, domain.ErrUnavailable)})
		}},
		{name: "walker bug", code: scan.ErrorCodeFailed, prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{failAfter: fmt.Errorf("read %s/x: bad", hiddenRoot)})
		}},
		{name: "root lost at resolve", code: scan.ErrorCodeRootUnavailable, prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
			h.resolver.errs["a.mkv"] = fmt.Errorf("open %s: %w", hiddenRoot, domain.ErrUnavailable)
		}},
		{name: "prober missing", code: scan.ErrorCodeProberUnavailable, prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
			h.prober.script(absolutePath(moviesRoot, "a.mkv"), probeOutcome{
				err: fmt.Errorf("start %s/ffprobe: %w", hiddenRoot, domain.ErrUnavailable),
			})
		}},
		{name: "resolver forbids", code: scan.ErrorCodeNone, prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
			h.resolver.errs["a.mkv"] = fmt.Errorf("resolve %s/a.mkv: %w", hiddenRoot, domain.ErrForbidden)
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			test.prepare(h)
			h.start(t)

			status := h.scan(t, moviesID)
			logs := h.logs.String()

			if status.ErrorCode != test.code {
				t.Errorf("error code = %q, want %q", status.ErrorCode, test.code)
			}

			if strings.Contains(logs, hiddenRoot) {
				t.Errorf("log contains the absolute root:\n%s", logs)
			}

			if test.code != scan.ErrorCodeNone && strings.Count(logs, string(test.code)) != 1 {
				t.Errorf("log names %q %d times, want once:\n%s", test.code, strings.Count(logs, string(test.code)), logs)
			}
		})
	}
}

func TestCatalogFailuresFailTheScan(t *testing.T) {
	t.Parallel()

	broken := errors.New("disk I/O error")

	tests := []struct {
		name   string
		mutate func(*memoryCatalog)
	}{
		{name: "file states", mutate: func(c *memoryCatalog) { c.statesErr = broken }},
		{name: "save", mutate: func(c *memoryCatalog) { c.saveErr = broken }},
		{name: "remove", mutate: func(c *memoryCatalog) { c.removeErr = broken }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("old.mkv", 1)}})
			h.start(t)
			h.scan(t, moviesID)

			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("new.mkv", 2), fileAt("new2.mkv", 3)}})
			test.mutate(h.catalog)

			status := h.scan(t, moviesID)

			assertState(t, status, scan.StateFailed, scan.ErrorCodeCatalogFailed)

			if !strings.Contains(h.logs.String(), broken.Error()) {
				t.Errorf("log does not contain the cause %q", broken)
			}
		})
	}
}

func TestUnexpectedFailuresFailTheScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(*harness)
	}{
		{name: "walker error", prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{failAfter: errors.New("walker bug")})
		}},
		{name: "id generator", prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
			h.ids.err = errors.New("no entropy")
		}},
		{name: "unknown library kind", prepare: func(h *harness) {
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})
			h.libraries.libraries[0].Kind = "music"
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			test.prepare(h)
			h.start(t)

			status := h.scan(t, moviesID)

			assertState(t, status, scan.StateFailed, scan.ErrorCodeFailed)

			if paths := h.catalog.paths(); len(paths) != 0 {
				t.Errorf("catalog paths = %q, want none", paths)
			}
		})
	}
}

func TestRescanAfterAFailureStartsFresh(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{failAfter: fmt.Errorf("walk: %w", domain.ErrUnavailable)})
	h.start(t)
	assertState(t, h.scan(t, moviesID), scan.StateFailed, scan.ErrorCodeRootUnavailable)

	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1)}})

	status := h.scan(t, moviesID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)
	assertCounts(t, status, scan.Counts{Added: 1})
}

func TestProbesRunWithBoundedConcurrency(t *testing.T) {
	t.Parallel()

	const fileCount = 8

	h := newHarness(t)

	files := make([]library.FileState, 0, fileCount)
	for index := range fileCount {
		files = append(files, fileAt(fmt.Sprintf("file-%d.mkv", index), int64(index+1)))
	}

	h.walker.set(moviesRoot, walkResult{files: files})
	h.prober.gate = make(chan struct{})
	h.prober.entered = make(chan struct{}, fileCount)
	service := h.start(t)

	if _, err := service.Start(context.Background(), moviesID); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	for range scan.MaxConcurrentProbes {
		receive(t, h.prober.entered, "concurrent probe")
	}

	running := service.Status(moviesID)
	assertState(t, running, scan.StateRunning, scan.ErrorCodeNone)

	close(h.prober.gate)
	h.wait(t, moviesID)

	_, maxActive, _ := h.prober.stats()
	if maxActive != scan.MaxConcurrentProbes {
		t.Errorf("max concurrent probes = %d, want %d", maxActive, scan.MaxConcurrentProbes)
	}

	assertCounts(t, service.Status(moviesID), scan.Counts{Added: fileCount})

	if scan.MaxConcurrentProbes != 2 {
		t.Errorf("MaxConcurrentProbes = %d, want 2 (ADR 0022)", scan.MaxConcurrentProbes)
	}
}

func TestFinishedScanIsLoggedWithCountsAndRelativePathsOnly(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("Sub/bad.mkv", 1), fileAt("good.mkv", 2)}})
	h.prober.script(absolutePath(moviesRoot, "Sub/bad.mkv"), probeOutcome{err: domain.ErrUnsupported})
	h.start(t)
	h.scan(t, moviesID)

	logs := h.logs.String()

	for _, want := range []string{
		"level=INFO", "library scan finished", "added=1", "failed=1", "library=" + moviesID,
		"level=DEBUG", "Sub/bad.mkv", "probe_failed",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("log lacks %q:\n%s", want, logs)
		}
	}

	if strings.Contains(logs, fakeRootPrefix) {
		t.Errorf("log contains an absolute path:\n%s", logs)
	}

	if finished := strings.Count(logs, "library scan finished"); finished != 1 {
		t.Errorf("%d finish lines, want 1", finished)
	}
}

func TestTheCatalogFakeEnforcesTheAdapterRules(t *testing.T) {
	t.Parallel()

	moment := time.Date(2026, time.October, 30, 18, 0, 0, 0, time.UTC)
	valid := scan.ProbedFile{
		Item: library.Item{
			ID: "item", LibraryID: moviesID, Kind: library.ItemKindMovie,
			Title: "A", SortTitle: "a", CreatedAt: moment,
		},
		File: media.File{
			ID: "file", ItemID: "item", LibraryID: moviesID, RelativePath: "a.mkv",
			Description: videoDescription(),
		},
		ProbedAt: moment,
	}

	tests := []struct {
		name   string
		mutate func(*scan.ProbedFile)
	}{
		{name: "zero probed time", mutate: func(r *scan.ProbedFile) { r.ProbedAt = time.Time{} }},
		{name: "invalid file", mutate: func(r *scan.ProbedFile) { r.File.Size = -1 }},
		{name: "libraries differ", mutate: func(r *scan.ProbedFile) { r.Item.LibraryID = showsID }},
		{name: "file points elsewhere", mutate: func(r *scan.ProbedFile) { r.File.ItemID = "other" }},
		{name: "new item without creation time", mutate: func(r *scan.ProbedFile) { r.Item.CreatedAt = time.Time{} }},
		{name: "new item with blank title", mutate: func(r *scan.ProbedFile) { r.Item.Title = " " }},
		{name: "new item of the wrong kind", mutate: func(r *scan.ProbedFile) { r.Item.Kind = library.ItemKindEpisode }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			catalog := newHarness(t).catalog
			record := valid
			test.mutate(&record)

			if err := catalog.SaveProbedFile(context.Background(), record); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("SaveProbedFile() error = %v, want ErrInvalid", err)
			}
		})
	}

	catalog := newHarness(t).catalog
	if err := catalog.SaveProbedFile(context.Background(), valid); err != nil {
		t.Fatalf("SaveProbedFile(valid) error = %v", err)
	}

	if err := catalog.RemoveFiles(context.Background(), showsID, []string{"file", "unknown"}); err != nil {
		t.Errorf("RemoveFiles() of foreign and unknown ids error = %v, want nil (ignored)", err)
	}

	if paths := catalog.paths(); len(paths) != 1 || len(catalog.violationList()) != 2 {
		t.Errorf("after a foreign remove: paths %q, violations %q, want the file kept and 2 violations",
			paths, catalog.violationList())
	}
}

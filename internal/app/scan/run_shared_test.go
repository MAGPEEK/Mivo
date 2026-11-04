package scan_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// These tests cover the probing, failure and logging behaviour every scan shares; the
// placement rules are tested in run_v2_test.go.

func TestProbeErrorsAreCountedWithoutAbortingTheScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "unsupported", err: fmt.Errorf("probe x.mkv: %w", domain.ErrUnsupported)},
		{name: "timed out", err: fmt.Errorf("probe x.mkv: %w", context.DeadlineExceeded)},
		{name: "unknown error", err: errors.New("something odd")},
		{name: "invalid description"},
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

			if !strings.Contains(h.logs.String(), "bad.mkv") {
				t.Error("log does not mention the failed file")
			}
		})
	}
}

func TestMissingProberAbortsTheScanWithoutSavingFiles(t *testing.T) {
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

	if h.catalogV2.saves != 0 {
		t.Errorf("catalog saved %d files, want 0", h.catalogV2.saves)
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
		{name: "file vanished", err: fmt.Errorf("resolve: %w", domain.ErrNotFound), wantState: scan.StateIdle, wantCount: scan.Counts{Added: 1, Failed: 1}},
		{name: "link leaves the root", err: fmt.Errorf("resolve: %w", domain.ErrForbidden), wantState: scan.StateIdle, wantCount: scan.Counts{Added: 1, Failed: 1}},
		{name: "other error", err: errors.New("resolver bug"), wantState: scan.StateIdle, wantCount: scan.Counts{Added: 1, Failed: 1}},
		{name: "root lost", err: fmt.Errorf("resolve: %w", domain.ErrUnavailable), wantState: scan.StateFailed, wantCode: scan.ErrorCodeRootUnavailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t)
			h.walker.set(moviesRoot, walkResult{files: []library.FileState{fileAt("a.mkv", 1), fileAt("b.mkv", 2)}})
			h.resolver.errs["a.mkv"] = test.err
			h.start(t)

			status := h.scan(t, moviesID)

			assertState(t, status, test.wantState, test.wantCode)

			if test.wantState == scan.StateFailed {
				if status.Counts.Failed != 0 {
					t.Errorf("failed count = %d after an abort, want 0", status.Counts.Failed)
				}

				return
			}

			assertCounts(t, status, test.wantCount)
		})
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

	assertState(t, service.Status(moviesID), scan.StateRunning, scan.ErrorCodeNone)

	close(h.prober.gate)
	h.wait(t, moviesID)

	if _, maxActive, _ := h.prober.stats(); maxActive != scan.MaxConcurrentProbes {
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

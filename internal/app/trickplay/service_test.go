package trickplay_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/trickplay"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
)

const waitLimit = 5 * time.Second

type harness struct {
	service   *trickplay.Service
	files     *fakeFiles
	generator *fakeGenerator
	store     *fakeStore
	cache     *fakeCache
	sessions  *fakeSessions
	clock     *fakeClock
	logs      *syncBuffer
	options   trickplay.Options
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		files:     &fakeFiles{files: map[string]appplayback.PlayableFile{"f1": videoFile("f1", "lib")}},
		generator: newGenerator(),
		store:     &fakeStore{stored: map[string]trickplay.Stored{}},
		cache:     &fakeCache{},
		sessions:  &fakeSessions{},
		clock:     &fakeClock{now: epoch()},
		logs:      &syncBuffer{},
	}
	h.options = trickplay.Options{
		Files: h.files, Paths: fakePaths{}, Generator: h.generator, Store: h.store, Cache: h.cache,
		Sessions: h.sessions, Clock: h.clock, Logger: newLogger(h.logs), MaxSessions: 1,
	}

	return h
}

// start builds the service from the current options and runs it.
func (h *harness) start(t *testing.T) {
	t.Helper()

	service, err := trickplay.New(h.options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	h.service = service

	if err := service.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	t.Cleanup(service.Close)
}

func eventually(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(waitLimit)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(time.Millisecond)
	}
}

func (h *harness) status(t *testing.T, fileID string) trickplay.Status {
	t.Helper()

	status, err := h.service.Status(viewerContext(access.AllLibraries()), fileID)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	return status
}

func (h *harness) waitStatus(t *testing.T, fileID string, want trickplay.Status) {
	t.Helper()
	eventually(t, "status "+string(want), func() bool { return h.status(t, fileID) == want })
}

func TestRequestGeneratesAndManifestBecomesReadable(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	if _, err := h.service.Manifest(ctx, "f1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Manifest before generation = %v, want ErrNotFound", err)
	}

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatalf("Request: %v", err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	manifest, err := h.service.Manifest(ctx, "f1")
	if err != nil || manifest.SheetCount != 3 {
		t.Fatalf("Manifest = %+v, %v", manifest, err)
	}

	call := <-h.generator.calls
	if call.source != "/lib/f1.mkv" || call.destination != filepath.Join(cacheRoot, "f1") {
		t.Errorf("generator called with %+v", call)
	}
}

func TestSheetNumbersMapFromZeroBasedToOneBasedFiles(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	tests := []struct {
		sheet   int
		want    string
		wantErr bool
	}{
		{sheet: 0, want: "sheet-001.jpg"},
		{sheet: 2, want: "sheet-003.jpg"},
		{sheet: 3, wantErr: true},
		{sheet: -1, wantErr: true},
	}

	for _, test := range tests {
		path, err := h.service.Sheet(ctx, "f1", test.sheet)
		if test.wantErr {
			if !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("sheet %d: err = %v, want ErrNotFound", test.sheet, err)
			}

			continue
		}

		if err != nil || path != filepath.Join(cacheRoot, "f1", test.want) {
			t.Errorf("sheet %d = %q, %v; want %s", test.sheet, path, err, test.want)
		}
	}
}

func TestDestinationComesFromTheCatalogID(t *testing.T) {
	h := newHarness(t)
	source := videoFile("source-id", "lib")
	h.files.files["alias"] = source
	h.files.set(source)
	h.start(t)

	if err := h.service.Request(viewerContext(access.AllLibraries()), "alias"); err != nil {
		t.Fatal(err)
	}

	call := <-h.generator.calls
	if !strings.HasSuffix(call.destination, "source-id") || strings.Contains(call.destination, "alias") {
		t.Errorf("destination %q derived from request input", call.destination)
	}
}

func TestRequestsAreDeduplicated(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	for range 3 {
		if err := h.service.Request(ctx, "f1"); err != nil {
			t.Fatal(err)
		}
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond)

	if got := h.generator.callCount(); got != 1 {
		t.Errorf("generator ran %d times, want 1", got)
	}
}

func TestFullQueueDropsRequestsWithoutError(t *testing.T) {
	h := newHarness(t)
	h.generator.block = true

	for i := range trickplay.MaxPendingJobs + 3 {
		h.files.set(videoFile(fmt.Sprintf("q%d", i), "lib"))
	}

	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	// The first job runs and blocks, so the queue holds the rest.
	for i := range trickplay.MaxPendingJobs + 3 {
		if err := h.service.Request(ctx, fmt.Sprintf("q%d", i)); err != nil {
			t.Fatalf("Request %d: %v", i, err)
		}

		if i == 0 {
			<-h.generator.started
		}
	}

	last := fmt.Sprintf("q%d", trickplay.MaxPendingJobs+2)
	if got := h.status(t, last); got != trickplay.StatusAbsent {
		t.Errorf("dropped request status = %s, want absent", got)
	}

	if got := h.status(t, "q1"); got != trickplay.StatusPending {
		t.Errorf("queued request status = %s, want pending", got)
	}
}

func TestBusyServerDelaysTheJob(t *testing.T) {
	h := newHarness(t)
	h.sessions.busyPolls = 3
	h.start(t)

	if err := h.service.Request(viewerContext(access.AllLibraries()), "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	if got := h.clock.waitCount(); got != 3 {
		t.Errorf("waited %d times, want 3", got)
	}

	if h.clock.waits[0] != trickplay.IdlePollInterval {
		t.Errorf("poll interval = %s", h.clock.waits[0])
	}
}

func TestMoreThanOneSessionNeverWaits(t *testing.T) {
	h := newHarness(t)
	h.sessions.always = true
	h.options.MaxSessions = 2
	h.start(t)

	if err := h.service.Request(viewerContext(access.AllLibraries()), "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	if h.sessions.pollCount() != 0 || h.clock.waitCount() != 0 {
		t.Error("job polled although max_sessions > 1")
	}
}

func TestJobGivesUpAfterAnHourAndMayBeRequestedAgain(t *testing.T) {
	h := newHarness(t)
	h.sessions.always = true
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	eventually(t, "job dropped", func() bool {
		return h.clock.Now().Sub(epoch()) >= trickplay.IdleGiveUpAfter && h.status(t, "f1") == trickplay.StatusAbsent
	})

	if h.generator.callCount() != 0 {
		t.Error("generated although the server stayed busy")
	}

	h.sessions.mu.Lock()
	h.sessions.always = false
	h.sessions.mu.Unlock()

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)
}

func TestChangedFileInvalidatesAndRegenerates(t *testing.T) {
	h := newHarness(t)
	h.store.stored["f1"] = trickplay.Stored{Key: "100-" + fmt.Sprint(epoch().UnixNano()), Manifest: trickplay.Manifest{SheetCount: 1}}
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	if _, err := h.service.Manifest(ctx, "f1"); err != nil {
		t.Fatalf("fresh manifest: %v", err)
	}

	changed := videoFile("f1", "lib")
	changed.File.ModifiedAt = epoch().Add(time.Hour)
	h.files.set(changed)

	if _, err := h.service.Manifest(ctx, "f1"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("stale manifest err = %v, want ErrNotFound", err)
	}

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	if h.generator.callCount() != 1 {
		t.Errorf("generator ran %d times", h.generator.callCount())
	}
}

func TestReadyRequestDoesNotGenerateAgain(t *testing.T) {
	h := newHarness(t)
	h.store.stored["f1"] = trickplay.Stored{Key: "100-" + fmt.Sprint(epoch().UnixNano()), Manifest: trickplay.Manifest{SheetCount: 1}}
	h.start(t)

	if err := h.service.Request(viewerContext(access.AllLibraries()), "f1"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(20 * time.Millisecond)

	if h.generator.callCount() != 0 {
		t.Error("generated an existing trickplay")
	}
}

func TestFailureIsRememberedAndLoggedOnce(t *testing.T) {
	h := newHarness(t)
	h.generator.err = fmt.Errorf("boom: %w", domain.ErrUnsupported)
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusFailed)

	for range 2 {
		if err := h.service.Request(ctx, "f1"); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(20 * time.Millisecond)

	if h.generator.callCount() != 1 {
		t.Errorf("generator ran %d times after a failure", h.generator.callCount())
	}

	if got := strings.Count(h.logs.String(), "trickplay generation failed"); got != 1 {
		t.Errorf("warn logged %d times, want 1", got)
	}
}

func TestShutdownCancelsARunningGeneration(t *testing.T) {
	h := newHarness(t)
	h.generator.block = true
	h.start(t)
	ctx := viewerContext(access.AllLibraries())

	if err := h.service.Request(ctx, "f1"); err != nil {
		t.Fatal(err)
	}

	<-h.generator.started

	closed := make(chan struct{})

	go func() {
		h.service.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(waitLimit):
		t.Fatal("Close did not return")
	}

	if got := h.status(t, "f1"); got != trickplay.StatusAbsent {
		t.Errorf("status after cancel = %s, want absent", got)
	}

	if strings.Contains(h.logs.String(), "failed") {
		t.Error("cancellation was logged as a failure")
	}

	if err := h.service.Request(ctx, "f1"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Request after Close = %v, want ErrUnavailable", err)
	}
}

func TestEvictionRemovesLeastRecentlyUsedAndKeepsTheNewEntry(t *testing.T) {
	h := newHarness(t)
	h.options.MaxCacheBytes = 100
	h.cache.entries = []trickplay.Entry{
		{FileID: "new", Bytes: 60, LastUsed: epoch()},
		{FileID: "old", Bytes: 50, LastUsed: epoch().Add(-2 * time.Hour)},
		{FileID: "mid", Bytes: 50, LastUsed: epoch().Add(-time.Hour)},
	}
	h.files.set(videoFile("old", "lib"))
	h.files.set(videoFile("mid", "lib"))
	h.files.set(videoFile("new", "lib"))
	h.start(t)

	if err := h.service.Request(viewerContext(access.AllLibraries()), "f1"); err != nil {
		t.Fatal(err)
	}

	h.waitStatus(t, "f1", trickplay.StatusReady)

	removed := h.cache.removedIDs()
	if len(removed) != 2 || removed[0] != "old" || removed[1] != "mid" {
		t.Errorf("removed %v, want [old mid]", removed)
	}
}

func TestStartPrunesVanishedFilesAndStagingLeftovers(t *testing.T) {
	h := newHarness(t)
	h.cache.entries = []trickplay.Entry{{FileID: "f1"}, {FileID: "gone"}}
	h.start(t)

	if !h.cache.swept {
		t.Error("staging directories not swept")
	}

	if removed := h.cache.removedIDs(); len(removed) != 1 || removed[0] != "gone" {
		t.Errorf("removed %v, want [gone]", removed)
	}
}

func TestStartReportsASweepFailure(t *testing.T) {
	h := newHarness(t)
	h.cache.sweepErr = errors.New("disk")

	service, err := trickplay.New(h.options)
	if err != nil {
		t.Fatal(err)
	}

	if err := service.Start(context.Background()); err == nil {
		t.Fatal("Start succeeded despite the sweep failure")
	}
}

func TestAccessIsCheckedBeforeAnything(t *testing.T) {
	h := newHarness(t)
	h.start(t)
	hidden := viewerContext(access.OnlyLibraries("other"))

	if _, err := h.service.Manifest(hidden, "f1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Manifest hidden = %v, want ErrNotFound", err)
	}

	if _, err := h.service.Sheet(hidden, "f1", 0); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Sheet hidden = %v, want ErrNotFound", err)
	}

	if _, err := h.service.Status(hidden, "f1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Status hidden = %v, want ErrNotFound", err)
	}

	if err := h.service.Request(hidden, "f1"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Request hidden = %v, want ErrNotFound", err)
	}

	if err := h.service.Request(context.Background(), "f1"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("Request without viewer = %v, want ErrUnauthenticated", err)
	}

	if h.generator.callCount() != 0 {
		t.Error("generator ran for a hidden file")
	}
}

func TestRequestRejectsFilesWithoutTrickplay(t *testing.T) {
	disc := videoFile("disc", "lib")
	disc.File.Layout = media.LayoutBDMV
	unprobed := videoFile("unprobed", "lib")
	unprobed.File.ProbeError = media.ProbeErrorFailed
	audio := videoFile("audio", "lib")
	audio.File.Description.Streams[0].Kind = media.StreamKindAudio

	for _, file := range []string{"disc", "unprobed", "audio"} {
		h := newHarness(t)
		h.files.set(disc)
		h.files.set(unprobed)
		h.files.set(audio)
		h.start(t)

		err := h.service.Request(viewerContext(access.AllLibraries()), file)
		if !errors.Is(err, domain.ErrUnsupported) {
			t.Errorf("%s: err = %v, want ErrUnsupported", file, err)
		}
	}
}

func TestRequestWithoutGeneratorIsUnavailable(t *testing.T) {
	h := newHarness(t)
	h.options.Generator = nil
	h.start(t)

	err := h.service.Request(viewerContext(access.AllLibraries()), "f1")
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("err = %v, want ErrUnavailable", err)
	}
}

func TestNewRejectsMissingOptions(t *testing.T) {
	if _, err := trickplay.New(trickplay.Options{}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}

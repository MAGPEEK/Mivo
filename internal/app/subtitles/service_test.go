package subtitles_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	fileID    = "file-1"
	libraryID = "library-1"
)

type harness struct {
	t         *testing.T
	service   *subtitles.Service
	cache     *memoryCache
	converter *fakeConverter
	files     *fakeFiles
	statter   *fakeStatter
	external  *fakeExternal
	root      string
	cacheDir  string
}

func externalSubtitle(textBased bool) media.ExternalSubtitle {
	return media.ExternalSubtitle{
		RelativePath: "Film.en.srt", Format: media.SubtitleFormatSubRip, Language: "en", TextBased: textBased,
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()

	harness := &harness{t: t, root: t.TempDir(), cacheDir: t.TempDir(), cache: newMemoryCache()}
	harness.converter = &fakeConverter{cache: harness.cache}
	harness.statter = &fakeStatter{stat: subtitles.FileStat{Size: 10, ModifiedAt: time.Unix(500, 0)}}
	harness.external = &fakeExternal{subtitles: []media.ExternalSubtitle{externalSubtitle(true)}}
	harness.files = &fakeFiles{
		file: subtitles.PlayableFile{
			LibraryRoot: harness.root,
			File: media.File{
				ID: fileID, LibraryID: libraryID, RelativePath: "Film.mkv", Size: 1000, ModifiedAt: time.Unix(100, 0),
				Description: media.Description{Streams: []media.Stream{
					{Index: 0, Kind: media.StreamKindVideo, Codec: "h264"},
					{Index: 1, Kind: media.StreamKindAudio, Codec: "aac"},
					{Index: 2, Kind: media.StreamKindSubtitle, Codec: "subrip"},
					{Index: 3, Kind: media.StreamKindSubtitle, Codec: "hdmv_pgs_subtitle"},
					{Index: 4, Kind: media.StreamKindSubtitle, Codec: "ass"},
				}},
			},
		},
		known: map[string]bool{fileID: true},
	}

	harness.rebuild()

	return harness
}

func (harness *harness) rebuild() {
	harness.t.Helper()

	service, err := subtitles.NewService(subtitles.Dependencies{
		Files: harness.files, External: harness.external, Resolver: joinResolver{}, Statter: harness.statter,
		Converter: harness.converter, Cache: harness.cache, CacheDir: harness.cacheDir,
		Now: func() time.Time { return time.Unix(100_000, 0) },
	})
	if err != nil {
		harness.t.Fatalf("NewService: %v", err)
	}

	harness.service = service
}

func viewerContext() context.Context {
	return appaccess.WithViewer(context.Background(), access.Viewer{
		Libraries: access.AllLibraries(), Limit: access.NoLimit(),
	})
}

func (harness *harness) webVTT(ctx context.Context, trackID string) (subtitles.WebVTT, error) {
	return harness.service.WebVTT(ctx, fileID, trackID)
}

func TestEmbeddedTrackIsConvertedByOrdinalAndCached(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)

	result, err := harness.webVTT(viewerContext(), "s:4")
	if err != nil {
		t.Fatalf("WebVTT: %v", err)
	}

	defer closeQuietly(result.Content)

	body, _ := io.ReadAll(result.Content)
	if string(body) != "WEBVTT\n" || result.Size != int64(len(body)) {
		t.Errorf("body %q size %d", body, result.Size)
	}

	call := harness.converter.lastCall()
	if call.track != (subtitles.Track{Ordinal: 1 + 1, Codec: "ass"}) {
		t.Errorf("track = %+v, want ordinal 2 (third subtitle stream) codec ass", call.track)
	}

	if call.source != filepath.Join(harness.root, "Film.mkv") {
		t.Errorf("source = %q", call.source)
	}

	if _, err := harness.webVTT(viewerContext(), "s:4"); err != nil {
		t.Fatalf("second WebVTT: %v", err)
	}

	if got := harness.converter.calls.Load(); got != 1 {
		t.Errorf("conversions = %d, want 1 (second request is a cache hit)", got)
	}
}

func TestDestinationIsBelowTheCacheAndNeverTheRequestText(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)

	if _, err := harness.webVTT(viewerContext(), "s:2"); err != nil {
		t.Fatalf("WebVTT: %v", err)
	}

	destination := harness.converter.lastCall().destination
	wantDirectory := filepath.Join(harness.cacheDir, "subtitles", fileID)

	if filepath.Dir(destination) != wantDirectory || filepath.Ext(destination) != ".vtt" {
		t.Errorf("destination = %q, want a .vtt in %q", destination, wantDirectory)
	}

	if strings.Contains(filepath.Base(destination), "s:2") || strings.Contains(destination, "Film") {
		t.Errorf("destination %q contains request or source text", destination)
	}
}

func TestExternalTrackIsConvertedFromItsFile(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	trackID := externalSubtitle(true).TrackID()

	if _, err := harness.webVTT(viewerContext(), trackID); err != nil {
		t.Fatalf("WebVTT: %v", err)
	}

	call := harness.converter.lastCall()
	if !call.track.External || call.source != filepath.Join(harness.root, "Film.en.srt") {
		t.Errorf("call = %+v", call)
	}
}

func TestChangedSourceMissesTheCache(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	trackID := externalSubtitle(true).TrackID()
	embedded := "s:2"

	tests := []struct {
		name   string
		track  string
		change func()
	}{
		{"media size", embedded, func() { harness.files.file.File.Size++ }},
		{"media mtime", embedded, func() { harness.files.file.File.ModifiedAt = time.Unix(101, 0) }},
		{"subtitle size", trackID, func() { harness.statter.stat.Size++ }},
		{"subtitle mtime", trackID, func() { harness.statter.stat.ModifiedAt = time.Unix(501, 0) }},
	}

	for _, test := range tests {

		for range 2 {
			if _, err := harness.webVTT(viewerContext(), test.track); err != nil {
				t.Fatalf("%s: WebVTT: %v", test.name, err)
			}
		}

		test.change()
		before := harness.converter.calls.Load()

		if _, err := harness.webVTT(viewerContext(), test.track); err != nil {
			t.Fatalf("%s: WebVTT after change: %v", test.name, err)
		}

		if got := harness.converter.calls.Load() - before; got != 1 {
			t.Errorf("%s: conversions = %d, want a new one after the change", test.name, got)
		}
	}
}

func TestInvalidAndUnknownTracks(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.external.subtitles = []media.ExternalSubtitle{
		externalSubtitle(true),
		{RelativePath: "Film.sup", Format: media.SubtitleFormatPGS, TextBased: false},
	}

	tests := []struct {
		name    string
		trackID string
		want    error
	}{
		{"embedded picture", "s:3", domain.ErrInvalid},
		{"external picture", harness.external.subtitles[1].TrackID(), domain.ErrInvalid},
		{"unknown embedded", "s:9", domain.ErrNotFound},
		{"audio stream", "s:1", domain.ErrNotFound},
		{"unknown external", "x:000000000000", domain.ErrNotFound},
		{"path as track", "../../etc/passwd", domain.ErrNotFound},
	}

	for _, test := range tests {
		if _, err := harness.webVTT(viewerContext(), test.trackID); !errors.Is(err, test.want) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.want)
		}
	}

	if harness.converter.calls.Load() != 0 {
		t.Error("a refused track must not start a conversion")
	}
}

func TestDiscEmbeddedTrackIsInvalid(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.files.file.File.Layout = media.LayoutISO

	if _, err := harness.webVTT(viewerContext(), "s:2"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

func TestAccessIsCheckedAndFailsClosed(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	otherLibrary := appaccess.WithViewer(context.Background(), access.Viewer{
		Libraries: access.OnlyLibraries("other"), Limit: access.NoLimit(),
	})

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"no viewer", context.Background(), domain.ErrUnauthenticated},
		{"zero viewer", appaccess.WithViewer(context.Background(), access.Viewer{}), domain.ErrNotFound},
		{"hidden library", otherLibrary, domain.ErrNotFound},
	}

	for _, test := range tests {
		if _, err := harness.webVTT(test.ctx, "s:2"); !errors.Is(err, test.want) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.want)
		}
	}

	if _, err := harness.service.WebVTT(viewerContext(), "other-file", "s:2"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown file: error = %v", err)
	}

	if harness.converter.calls.Load() != 0 {
		t.Error("a denied viewer must not start a conversion")
	}
}

func TestUnsafeFileIDIsRefused(t *testing.T) {
	t.Parallel()

	for _, id := range []string{"", "..", "a/b", `a\b`, "c:x"} {
		harness := newHarness(t)
		harness.files.file.File.ID = id

		service, err := subtitles.NewService(subtitles.Dependencies{
			Files: harness.files, External: harness.external, Resolver: joinResolver{}, Statter: harness.statter,
			Converter: harness.converter, Cache: harness.cache, CacheDir: harness.cacheDir,
		})
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}

		if _, err := service.WebVTT(viewerContext(), id, "s:2"); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("id %q: error = %v, want ErrInvalid", id, err)
		}
	}

	if _, err := newHarnessWithCacheDir("relative"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("relative cache dir: error = %v", err)
	}

	if _, err := subtitles.NewService(subtitles.Dependencies{CacheDir: t.TempDir()}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("missing ports: error = %v", err)
	}
}

func newHarnessWithCacheDir(dir string) (*subtitles.Service, error) {
	cache := newMemoryCache()

	return subtitles.NewService(subtitles.Dependencies{
		Files: &fakeFiles{}, External: fakeExternal{}, Resolver: joinResolver{}, Statter: &fakeStatter{},
		Converter: &fakeConverter{cache: cache}, Cache: cache, CacheDir: dir,
	})
}

func TestConcurrentRequestsConvertOnce(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.converter.gate = make(chan struct{})

	const requests = 8

	var group sync.WaitGroup

	errs := make(chan error, requests)

	for range requests {
		group.Add(1)

		go func() {
			defer group.Done()

			result, err := harness.webVTT(viewerContext(), "s:2")
			if err == nil {
				closeQuietly(result.Content)
			}

			errs <- err
		}()
	}

	waitFor(t, func() bool { return harness.converter.calls.Load() >= 1 })
	time.Sleep(50 * time.Millisecond)
	close(harness.converter.gate)
	group.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("WebVTT: %v", err)
		}
	}

	if got := harness.converter.calls.Load(); got != 1 {
		t.Errorf("conversions = %d, want 1", got)
	}
}

func TestAtMostTwoConversionsRunAtOnce(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.converter.gate = make(chan struct{})
	harness.files.file.File.Description.Streams = nil

	for index := range 5 {
		harness.files.file.File.Description.Streams = append(harness.files.file.File.Description.Streams,
			media.Stream{Index: index, Kind: media.StreamKindSubtitle, Codec: "subrip"})
	}

	var group sync.WaitGroup

	for _, trackID := range []string{"s:0", "s:1", "s:2", "s:3", "s:4"} {
		group.Add(1)

		go func() {
			defer group.Done()

			if result, err := harness.webVTT(viewerContext(), trackID); err == nil {
				closeQuietly(result.Content)
			}
		}()
	}

	waitFor(t, func() bool { return harness.converter.running.Load() == 2 })
	time.Sleep(50 * time.Millisecond)

	if got := harness.converter.running.Load(); got != 2 {
		t.Errorf("running = %d, want 2", got)
	}

	close(harness.converter.gate)
	group.Wait()

	if peak := harness.converter.peak.Load(); peak > 2 {
		t.Errorf("peak = %d, want at most 2", peak)
	}
}

func TestCanceledWaiterReturnsItsContextError(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.converter.gate = make(chan struct{})

	defer close(harness.converter.gate)

	ctx, cancel := context.WithCancel(viewerContext())
	done := make(chan error, 1)

	go func() {
		_, err := harness.webVTT(ctx, "s:2")
		done <- err
	}()

	waitFor(t, func() bool { return harness.converter.calls.Load() == 1 })
	cancel()

	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestConverterErrorIsReturnedAndNotCached(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.converter.err = domain.ErrUnsupported

	if _, err := harness.webVTT(viewerContext(), "s:2"); !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}

	harness.converter.err = nil

	if _, err := harness.webVTT(viewerContext(), "s:2"); err != nil {
		t.Errorf("retry: %v", err)
	}
}

func TestStoringBeyondTheCapEvictsTheOldest(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	harness.cache.put(filepath.Join(harness.cacheDir, "subtitles", "old", "a.vtt"),
		make([]byte, subtitles.MaxCacheBytes-10), time.Unix(1, 0))
	harness.cache.put(filepath.Join(harness.cacheDir, "subtitles", "newer", "b.vtt"),
		make([]byte, 5), time.Unix(2, 0))
	harness.converter.size = 100

	if _, err := harness.webVTT(viewerContext(), "s:2"); err != nil {
		t.Fatalf("WebVTT: %v", err)
	}

	if _, err := harness.cache.Open(context.Background(), filepath.Join(harness.cacheDir, "subtitles", "old", "a.vtt")); err == nil {
		t.Error("the oldest entry survived the cap")
	}

	if _, err := harness.cache.Open(context.Background(), harness.converter.lastCall().destination); err != nil {
		t.Errorf("the new entry was evicted: %v", err)
	}
}

func TestSweepRemovesUnknownFilesAndStaleTempFiles(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	base := filepath.Join(harness.cacheDir, "subtitles")
	now := time.Unix(100_000, 0)

	keep := []string{
		filepath.Join(base, fileID, "a.vtt"),
		filepath.Join(base, fileID, "fresh.vtt.tmp"),
	}
	drop := []string{
		filepath.Join(base, "gone", "b.vtt"),
		filepath.Join(base, fileID, "stale.vtt.tmp"),
	}

	for _, path := range keep {
		harness.cache.put(path, []byte("x"), now.Add(-time.Minute))
	}

	harness.cache.put(drop[0], []byte("x"), now)
	harness.cache.put(drop[1], []byte("x"), now.Add(-time.Hour))

	if err := harness.service.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	left := strings.Join(harness.cache.paths(), "\n")
	for _, path := range keep {
		if !strings.Contains(left, path) {
			t.Errorf("%s was removed", path)
		}
	}

	for _, path := range drop {
		if strings.Contains(left, path) {
			t.Errorf("%s was kept", path)
		}
	}
}

func TestSweepTrimsToTheCap(t *testing.T) {
	t.Parallel()

	harness := newHarness(t)
	base := filepath.Join(harness.cacheDir, "subtitles", fileID)

	harness.cache.put(filepath.Join(base, "old.vtt"), make([]byte, subtitles.MaxCacheBytes), time.Unix(1, 0))
	harness.cache.put(filepath.Join(base, "new.vtt"), make([]byte, 10), time.Unix(2, 0))

	if err := harness.service.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if paths := harness.cache.paths(); len(paths) != 1 || !strings.HasSuffix(paths[0], "new.vtt") {
		t.Errorf("left = %v, want only new.vtt", paths)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not reached in time")
		}

		time.Sleep(time.Millisecond)
	}
}

func closeQuietly(closer io.Closer) { _ = closer.Close() }

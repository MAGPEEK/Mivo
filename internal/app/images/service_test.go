package images_test

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	posterID  = "0f8fad5b-d9cb-469f-a165-70867728950e"
	logoID    = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	posterTag = "0123456789abcdef"
	logoTag   = "fedcba9876543210"
	// joinTimeout bounds how long a test waits for goroutines to reach a state.
	joinTimeout = 5 * time.Second
	// quietPeriod is how long a test watches for something that must not happen.
	quietPeriod = 50 * time.Millisecond
)

type fixture struct {
	source   *fakeSource
	renderer *fakeRenderer
	cache    *memoryCache
	service  *images.Service
}

func newFixture(t *testing.T, renderer *fakeRenderer) *fixture {
	t.Helper()

	source := &fakeSource{artworks: map[string]artwork.Stored{
		posterID: {
			ID: posterID, Kind: artwork.KindPoster, Source: artwork.SourceLocal,
			LibraryRoot: "/lib", Location: artwork.LocalLocation("Movie/poster.jpg"), Tag: posterTag,
		},
		logoID: {
			ID: logoID, Kind: artwork.KindLogo, Source: artwork.SourceEmbedded,
			LibraryRoot: "/lib", Location: artwork.EmbeddedLocation("Movie/movie.mkv", 3), Tag: logoTag,
		},
	}}
	cache := newMemoryCache()
	service := images.NewService(images.Dependencies{
		Source: source, Resolver: prefixResolver{}, Renderer: renderer, Cache: cache,
	})

	return &fixture{source: source, renderer: renderer, cache: cache, service: service}
}

func readAll(t *testing.T, image images.Image) string {
	t.Helper()

	defer func() { _ = image.Content.Close() }()

	data, err := io.ReadAll(image.Content)
	if err != nil {
		t.Fatalf("read image: %v", err)
	}

	return string(data)
}

func TestImageRendersOnMissAndServesHitFromCache(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, &fakeRenderer{})
	request := images.Request{ID: posterID, Width: artwork.Width320, Tag: posterTag}

	for round := range 2 {
		image, err := fixture.service.Image(t.Context(), request)
		if err != nil {
			t.Fatalf("round %d: Image: %v", round, err)
		}

		if got := readAll(t, image); got != "img:/lib/Movie/poster.jpg" {
			t.Errorf("round %d: content = %q", round, got)
		}

		if image.Size != int64(len("img:/lib/Movie/poster.jpg")) || image.ContentType != "image/jpeg" ||
			image.ETag != `"0123456789abcdef-320"` || !image.Immutable {
			t.Errorf("round %d: image = %+v", round, image)
		}
	}

	if calls := fixture.renderer.calls(); calls != 1 {
		t.Errorf("renders = %d, want 1", calls)
	}

	want := artwork.RenderRequest{
		AbsolutePath: "/lib/Movie/poster.jpg", StreamIndex: artwork.WholeFile,
		MaxWidth: artwork.Width320, Format: artwork.FormatJPEG,
	}
	if fixture.renderer.requests[0] != want {
		t.Errorf("render request = %+v, want %+v", fixture.renderer.requests[0], want)
	}
}

func TestImageRendersLogoAsPNGWithStreamIndex(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, &fakeRenderer{})

	image, err := fixture.service.Image(t.Context(), images.Request{ID: logoID, Width: artwork.Width640})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

	_ = image.Content.Close()

	if image.ContentType != "image/png" || image.Immutable {
		t.Errorf("image = %+v, want PNG and not immutable", image)
	}

	request := fixture.renderer.requests[0]
	if request.StreamIndex != 3 || request.Format != artwork.FormatPNG || request.MaxWidth != artwork.Width640 {
		t.Errorf("render request = %+v", request)
	}
}

func TestImageTagMismatchIsNotImmutable(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, &fakeRenderer{})

	for _, tag := range []string{"", "1111111111111111"} {
		image, err := fixture.service.Image(t.Context(), images.Request{ID: posterID, Width: artwork.Width160, Tag: tag})
		if err != nil {
			t.Fatalf("Image(tag %q): %v", tag, err)
		}

		_ = image.Content.Close()

		if image.Immutable || image.ETag != `"0123456789abcdef-160"` {
			t.Errorf("tag %q: image = %+v", tag, image)
		}
	}
}

func TestImageErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		id       string
		prepare  func(*fixture, *images.Dependencies)
		wantErr  error
		wantKept bool
	}{
		{name: "unknown id", id: "missing", wantErr: domain.ErrNotFound},
		{
			name: "renderer unsupported", id: posterID, wantErr: domain.ErrUnsupported,
			prepare: func(f *fixture, _ *images.Dependencies) { f.renderer.err = domain.ErrUnsupported },
		},
		{
			name: "renderer unavailable", id: posterID, wantErr: domain.ErrUnavailable,
			prepare: func(f *fixture, _ *images.Dependencies) { f.renderer.err = domain.ErrUnavailable },
		},
		{
			name: "source escapes root", id: posterID, wantErr: domain.ErrForbidden,
			prepare: func(_ *fixture, deps *images.Dependencies) {
				deps.Resolver = prefixResolver{err: domain.ErrForbidden}
			},
		},
		{
			name: "data directory source", id: posterID, wantErr: domain.ErrUnsupported,
			prepare: func(f *fixture, _ *images.Dependencies) {
				stored := f.source.artworks[posterID]
				stored.Location = artwork.DataLocation("artwork/x.jpg")
				f.source.artworks[posterID] = stored
			},
		},
		{
			name: "cache broken", id: posterID, wantErr: domain.ErrUnavailable,
			prepare: func(f *fixture, _ *images.Dependencies) { f.cache.openErr = domain.ErrUnavailable },
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			fixture := newFixture(t, &fakeRenderer{})
			deps := images.Dependencies{
				Source: fixture.source, Resolver: prefixResolver{}, Renderer: fixture.renderer, Cache: fixture.cache,
			}

			if testCase.prepare != nil {
				testCase.prepare(fixture, &deps)
			}

			service := images.NewService(deps)

			_, err := service.Image(t.Context(), images.Request{ID: testCase.id, Width: artwork.Width320})
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("err = %v, want %v", err, testCase.wantErr)
			}

			if fixture.cache.size() != 0 {
				t.Errorf("cache holds %d entries after a failure, want 0", fixture.cache.size())
			}
		})
	}
}

func TestImageServesRenderingWhenStoreFails(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, &fakeRenderer{})
	fixture.cache.storeErr = errors.New("disk full")

	image, err := fixture.service.Image(t.Context(), images.Request{ID: posterID, Width: artwork.Width320})
	if err != nil {
		t.Fatalf("Image: %v", err)
	}

	if got := readAll(t, image); got != "img:/lib/Movie/poster.jpg" {
		t.Errorf("content = %q", got)
	}
}

func TestImageLimitsConcurrentRenders(t *testing.T) {
	t.Parallel()

	const requests = 5

	renderer := &fakeRenderer{started: make(chan struct{}, requests), release: make(chan struct{})}
	fixture := newFixture(t, renderer)

	var group sync.WaitGroup
	for index := range requests {
		group.Go(func() {
			width := artwork.Widths()[index]

			image, err := fixture.service.Image(context.Background(), images.Request{ID: posterID, Width: width})
			if err != nil {
				t.Errorf("Image(%d): %v", width, err)

				return
			}

			_ = image.Content.Close()
		})
	}

	waitFor(t, renderer.started, 2)

	select {
	case <-renderer.started:
		t.Fatal("a third render started while two were running")
	case <-time.After(quietPeriod):
	}

	close(renderer.release)
	group.Wait()

	if renderer.maxRunning != 2 || renderer.calls() != requests {
		t.Errorf("max running = %d, calls = %d; want 2 and %d", renderer.maxRunning, renderer.calls(), requests)
	}
}

func TestImageSharesOneRenderPerKey(t *testing.T) {
	t.Parallel()

	const requests = 4

	renderer := &fakeRenderer{started: make(chan struct{}, requests), release: make(chan struct{})}
	fixture := newFixture(t, renderer)
	request := images.Request{ID: posterID, Width: artwork.Width480}
	key := images.Key{ID: posterID, Tag: posterTag, Width: artwork.Width480, Format: artwork.FormatJPEG}
	results := make(chan string, requests)

	for range requests {
		go func() {
			image, err := fixture.service.Image(context.Background(), request)
			if err != nil {
				results <- err.Error()

				return
			}

			results <- readAll(t, image)
		}()
	}

	waitFor(t, renderer.started, 1)
	waitUntil(t, func() bool { return fixture.service.Waiting(key) == requests })
	close(renderer.release)

	for range requests {
		if got := <-results; got != "img:/lib/Movie/poster.jpg" {
			t.Errorf("result = %q", got)
		}
	}

	if calls := renderer.calls(); calls != 1 {
		t.Errorf("renders = %d, want 1", calls)
	}
}

func TestImageWaitingRequestHonoursContext(t *testing.T) {
	t.Parallel()

	renderer := &fakeRenderer{started: make(chan struct{}, 2), release: make(chan struct{})}
	fixture := newFixture(t, renderer)
	request := images.Request{ID: posterID, Width: artwork.Width960}
	ctx, cancel := context.WithCancel(t.Context())
	errs := make(chan error, 1)

	go func() {
		_, err := fixture.service.Image(ctx, request)
		errs <- err
	}()

	waitFor(t, renderer.started, 1)
	cancel()

	if err := <-errs; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}

	// The render goes on for other requests and fills the cache.
	close(renderer.release)
	waitUntil(t, func() bool { return fixture.cache.size() == 1 })
}

func TestImageSlotWaitTimesOutAsUnavailable(t *testing.T) {
	t.Parallel()

	renderer := &fakeRenderer{started: make(chan struct{}, 2), release: make(chan struct{}), ignoreContext: true}
	fixture := newFixture(t, renderer)
	service := images.NewServiceWithLimits(images.Dependencies{
		Source: fixture.source, Resolver: prefixResolver{}, Renderer: renderer, Cache: fixture.cache,
	}, 1, quietPeriod)

	go func() {
		image, err := service.Image(context.Background(), images.Request{ID: posterID, Width: artwork.Width160})
		if err == nil {
			_ = image.Content.Close()
		}
	}()

	waitFor(t, renderer.started, 1)

	_, err := service.Image(t.Context(), images.Request{ID: logoID, Width: artwork.Width160})
	if !errors.Is(err, domain.ErrUnavailable) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want ErrUnavailable and DeadlineExceeded", err)
	}

	close(renderer.release)
}

func TestPurgeKeepsOnlyCurrentTags(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, &fakeRenderer{})
	ctx := t.Context()
	current := images.Key{ID: posterID, Tag: posterTag, Width: artwork.Width320, Format: artwork.FormatJPEG}
	stale := images.Key{ID: posterID, Tag: "1111111111111111", Width: artwork.Width320, Format: artwork.FormatJPEG}
	gone := images.Key{ID: "deleted", Tag: posterTag, Width: artwork.Width320, Format: artwork.FormatJPEG}

	for _, key := range []images.Key{current, stale, gone} {
		if err := fixture.cache.Store(ctx, key, []byte("x")); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	if err := fixture.service.Purge(ctx); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if _, ok := fixture.cache.entries[current]; !ok || fixture.cache.size() != 1 {
		t.Errorf("entries after purge = %v, want only the current one", fixture.cache.entries)
	}
}

func TestPurgeFailsWhenTagsCannotBeRead(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t, &fakeRenderer{})
	fixture.source.tagsErr = domain.ErrUnavailable

	if err := fixture.service.Purge(t.Context()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func waitFor(t *testing.T, events <-chan struct{}, count int) {
	t.Helper()

	for range count {
		select {
		case <-events:
		case <-time.After(joinTimeout):
			t.Fatalf("timed out waiting for %d events", count)
		}
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(joinTimeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}

		time.Sleep(time.Millisecond)
	}
}

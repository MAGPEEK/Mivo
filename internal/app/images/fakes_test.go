package images_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

type fakeSource struct {
	artworks map[string]artwork.Stored
	tagsErr  error
}

func (source *fakeSource) Artwork(_ context.Context, id string) (artwork.Stored, error) {
	stored, ok := source.artworks[id]
	if !ok {
		return artwork.Stored{}, fmt.Errorf("artwork %s: %w", id, domain.ErrNotFound)
	}

	return stored, nil
}

func (source *fakeSource) ArtworkTags(context.Context) (map[string]string, error) {
	if source.tagsErr != nil {
		return nil, source.tagsErr
	}

	tags := make(map[string]string, len(source.artworks))
	for id, stored := range source.artworks {
		tags[id] = stored.Tag
	}

	return tags, nil
}

type prefixResolver struct {
	err error
}

func (resolver prefixResolver) Resolve(root, relativePath string) (string, error) {
	if resolver.err != nil {
		return "", resolver.err
	}

	return root + "/" + relativePath, nil
}

// fakeRenderer records requests and the highest number of renders running at once. When
// release is set, every render blocks until it receives a value.
type fakeRenderer struct {
	mutex      sync.Mutex
	requests   []artwork.RenderRequest
	running    int
	maxRunning int
	started    chan struct{}
	release    chan struct{}
	err        error
	// ignoreContext keeps a render blocked past its deadline, holding the render slot.
	ignoreContext bool
}

func (renderer *fakeRenderer) Render(ctx context.Context, request artwork.RenderRequest) (artwork.Rendered, error) {
	renderer.mutex.Lock()
	renderer.requests = append(renderer.requests, request)
	renderer.running++
	renderer.maxRunning = max(renderer.maxRunning, renderer.running)
	renderer.mutex.Unlock()

	defer func() {
		renderer.mutex.Lock()
		renderer.running--
		renderer.mutex.Unlock()
	}()

	if renderer.started != nil {
		renderer.started <- struct{}{}
	}

	if renderer.release != nil {
		done := ctx.Done()
		if renderer.ignoreContext {
			done = nil
		}

		select {
		case <-renderer.release:
		case <-done:
			return artwork.Rendered{}, ctx.Err()
		}
	}

	if renderer.err != nil {
		return artwork.Rendered{}, renderer.err
	}

	return artwork.Rendered{Data: []byte("img:" + request.AbsolutePath), Format: request.Format}, nil
}

func (renderer *fakeRenderer) calls() int {
	renderer.mutex.Lock()
	defer renderer.mutex.Unlock()

	return len(renderer.requests)
}

type memoryCache struct {
	mutex    sync.Mutex
	entries  map[images.Key][]byte
	storeErr error
	openErr  error
}

func newMemoryCache() *memoryCache {
	return &memoryCache{entries: make(map[images.Key][]byte)}
}

func (cache *memoryCache) Open(_ context.Context, key images.Key) (images.Cached, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if cache.openErr != nil {
		return images.Cached{}, cache.openErr
	}

	data, ok := cache.entries[key]
	if !ok {
		return images.Cached{}, fmt.Errorf("cache %v: %w", key, domain.ErrNotFound)
	}

	return images.Cached{Content: readSeekNopCloser{bytes.NewReader(data)}, Size: int64(len(data))}, nil
}

func (cache *memoryCache) Store(_ context.Context, key images.Key, data []byte) error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if cache.storeErr != nil {
		return cache.storeErr
	}

	cache.entries[key] = data

	return nil
}

func (cache *memoryCache) Purge(_ context.Context, keep func(id, tag string) bool) error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	for key := range cache.entries {
		if !keep(key.ID, key.Tag) {
			delete(cache.entries, key)
		}
	}

	return nil
}

func (cache *memoryCache) size() int {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	return len(cache.entries)
}

type readSeekNopCloser struct {
	io.ReadSeeker
}

func (readSeekNopCloser) Close() error { return nil }

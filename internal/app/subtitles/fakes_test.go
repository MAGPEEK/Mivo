package subtitles_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/magpeek/mivo/internal/app/subtitles"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
)

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

// memoryCache is an in-memory Cache; paths are the keys.
type memoryCache struct {
	mutex sync.Mutex
	files map[string]cachedBytes
}

type cachedBytes struct {
	data    []byte
	modTime time.Time
}

func newMemoryCache() *memoryCache { return &memoryCache{files: map[string]cachedBytes{}} }

func (cache *memoryCache) put(path string, data []byte, modTime time.Time) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	cache.files[path] = cachedBytes{data: data, modTime: modTime}
}

func (cache *memoryCache) paths() []string {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	paths := make([]string, 0, len(cache.files))
	for path := range cache.files {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	return paths
}

func (cache *memoryCache) Open(_ context.Context, path string) (subtitles.CacheFile, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	entry, ok := cache.files[path]
	if !ok {
		return subtitles.CacheFile{}, fmt.Errorf("open %s: %w", path, domain.ErrNotFound)
	}

	return subtitles.CacheFile{
		Content: nopCloser{bytes.NewReader(entry.data)}, Size: int64(len(entry.data)), ModTime: entry.modTime,
	}, nil
}

func (cache *memoryCache) List(_ context.Context, _ string) ([]subtitles.CacheEntry, error) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	var entries []subtitles.CacheEntry

	for path, entry := range cache.files {
		entries = append(entries, subtitles.CacheEntry{
			Path: path, Directory: filepath.Base(filepath.Dir(path)), Name: filepath.Base(path),
			Size: int64(len(entry.data)), ModTime: entry.modTime,
		})
	}

	return entries, nil
}

func (cache *memoryCache) Remove(_ context.Context, path string) error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	delete(cache.files, path)

	return nil
}

// fakeConverter writes a fixed WebVTT into the cache and records its calls.
type fakeConverter struct {
	cache *memoryCache
	calls atomic.Int32
	// gate, when set, blocks every conversion until it is closed.
	gate chan struct{}
	// size is the length of the written file; 0 means a short default.
	size    int
	err     error
	mutex   sync.Mutex
	last    convertCall
	running atomic.Int32
	peak    atomic.Int32
}

type convertCall struct {
	source      string
	track       subtitles.Track
	destination string
}

func (converter *fakeConverter) ConvertWebVTT(_ context.Context, source string, track subtitles.Track, destination string) error {
	converter.calls.Add(1)

	running := converter.running.Add(1)
	defer converter.running.Add(-1)

	for {
		peak := converter.peak.Load()
		if running <= peak || converter.peak.CompareAndSwap(peak, running) {
			break
		}
	}

	converter.mutex.Lock()
	converter.last = convertCall{source: source, track: track, destination: destination}
	converter.mutex.Unlock()

	if converter.gate != nil {
		<-converter.gate
	}

	if converter.err != nil {
		return converter.err
	}

	data := []byte("WEBVTT\n")
	if converter.size > 0 {
		data = bytes.Repeat([]byte("x"), converter.size)
	}

	converter.cache.put(destination, data, time.Unix(1_000, 0))

	return nil
}

func (converter *fakeConverter) lastCall() convertCall {
	converter.mutex.Lock()
	defer converter.mutex.Unlock()

	return converter.last
}

type fakeFiles struct {
	file    subtitles.PlayableFile
	known   map[string]bool
	lookups int
}

func (files *fakeFiles) PlayableFile(_ context.Context, fileID string, restriction access.Restriction) (subtitles.PlayableFile, error) {
	if fileID != files.file.File.ID || !restriction.AllowsLibrary(files.file.File.LibraryID) {
		return subtitles.PlayableFile{}, fmt.Errorf("file %s: %w", fileID, domain.ErrNotFound)
	}

	return files.file, nil
}

func (files *fakeFiles) FileKnown(_ context.Context, fileID string) (bool, error) {
	files.lookups++

	return files.known[fileID], nil
}

type fakeExternal struct{ subtitles []media.ExternalSubtitle }

func (external fakeExternal) ExternalSubtitles(context.Context, string) ([]media.ExternalSubtitle, error) {
	return external.subtitles, nil
}

// joinResolver joins root and relative path without touching the disk.
type joinResolver struct{ missing string }

func (resolver joinResolver) Resolve(root, relativePath string) (string, error) {
	if relativePath == resolver.missing {
		return "", fmt.Errorf("resolve %s: %w", relativePath, domain.ErrNotFound)
	}

	return filepath.Join(root, filepath.FromSlash(relativePath)), nil
}

type fakeStatter struct{ stat subtitles.FileStat }

func (statter *fakeStatter) Stat(context.Context, string) (subtitles.FileStat, error) {
	return statter.stat, nil
}

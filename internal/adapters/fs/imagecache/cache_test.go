package imagecache_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/imagecache"
	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	artworkID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	otherID   = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	tagA      = "0123456789abcdef"
	tagB      = "fedcba9876543210"
)

func newCache(t *testing.T) (*imagecache.Cache, string) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "images")

	cache, err := imagecache.New(root, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return cache, root
}

func key(id, tag string, width artwork.Width) images.Key {
	return images.Key{ID: id, Tag: tag, Width: width, Format: artwork.FormatJPEG}
}

func read(t *testing.T, cache *imagecache.Cache, entry images.Key) string {
	t.Helper()

	cached, err := cache.Open(t.Context(), entry)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = cached.Content.Close() }()

	data, err := io.ReadAll(cached.Content)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if cached.Size != int64(len(data)) {
		t.Errorf("size = %d, want %d", cached.Size, len(data))
	}

	return string(data)
}

func TestNewRejectsRelativeRoot(t *testing.T) {
	t.Parallel()

	if _, err := imagecache.New("images", nil); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestStoreAndOpen(t *testing.T) {
	t.Parallel()

	cache, root := newCache(t)
	entry := images.Key{ID: artworkID, Tag: tagA, Width: artwork.Width320, Format: artwork.FormatPNG}

	if _, err := cache.Open(t.Context(), entry); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Open before Store: err = %v, want ErrNotFound", err)
	}

	for _, content := range []string{"first", "second"} {
		if err := cache.Store(t.Context(), entry, []byte(content)); err != nil {
			t.Fatalf("Store: %v", err)
		}

		if got := read(t, cache, entry); got != content {
			t.Errorf("content = %q, want %q", got, content)
		}
	}

	want := filepath.Join(root, "0f", artworkID, tagA+"-320.png")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file %s: %v", want, err)
	}

	files, err := os.ReadDir(filepath.Dir(want))
	if err != nil || len(files) != 1 {
		t.Errorf("entry directory holds %d files (err %v), want only the picture", len(files), err)
	}
}

func TestStoreConcurrentWritersLeaveOneCompleteFile(t *testing.T) {
	t.Parallel()

	const writers = 8

	cache, _ := newCache(t)
	entry := key(artworkID, tagA, artwork.Width640)
	payload := make([]byte, 64*1024)

	var group sync.WaitGroup
	for index := range byte(writers) {
		group.Go(func() {
			data := make([]byte, len(payload))
			for position := range data {
				data[position] = index
			}

			if err := cache.Store(context.Background(), entry, data); err != nil {
				t.Errorf("Store: %v", err)
			}
		})
	}

	group.Wait()

	got := read(t, cache, entry)
	if len(got) != len(payload) {
		t.Fatalf("length = %d, want %d", len(got), len(payload))
	}

	for position := range got {
		if got[position] != got[0] {
			t.Fatal("cached file mixes the data of several writers")
		}
	}
}

func TestCraftedKeysAreRejected(t *testing.T) {
	t.Parallel()

	cache, root := newCache(t)
	testCases := []struct {
		name  string
		entry images.Key
	}{
		{name: "traversal id", entry: key("../../../../etc/passwd-000000000000", tagA, artwork.Width320)},
		{name: "separator id", entry: key(`0f8fad5b\d9cb-469f-a165-70867728950e`, tagA, artwork.Width320)},
		{name: "uppercase id", entry: key("0F8FAD5B-D9CB-469F-A165-70867728950E", tagA, artwork.Width320)},
		{name: "short id", entry: key("0f", tagA, artwork.Width320)},
		{name: "traversal tag", entry: key(artworkID, "../../../../x", artwork.Width320)},
		{name: "uppercase tag", entry: key(artworkID, "0123456789ABCDEF", artwork.Width320)},
		{name: "odd width", entry: key(artworkID, tagA, artwork.Width(321))},
		{name: "unknown format", entry: images.Key{ID: artworkID, Tag: tagA, Width: artwork.Width320, Format: "gif"}},
	}

	for _, testCase := range testCases {
		if err := cache.Store(t.Context(), testCase.entry, []byte("x")); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: Store err = %v, want ErrInvalid", testCase.name, err)
		}

		if _, err := cache.Open(t.Context(), testCase.entry); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: Open err = %v, want ErrInvalid", testCase.name, err)
		}
	}

	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("rejected keys created the cache directory: %v", err)
	}
}

func TestPurgeRemovesStaleEntriesAndKeepsForeignFiles(t *testing.T) {
	t.Parallel()

	cache, root := newCache(t)
	ctx := t.Context()
	current := key(artworkID, tagA, artwork.Width320)
	stale := key(artworkID, tagB, artwork.Width320)
	gone := key(otherID, tagA, artwork.Width160)

	for _, entry := range []images.Key{current, stale, gone} {
		if err := cache.Store(ctx, entry, []byte("x")); err != nil {
			t.Fatalf("Store: %v", err)
		}
	}

	foreign := []string{
		filepath.Join(root, "README.txt"),
		filepath.Join(root, "0f", "notes.txt"),
		filepath.Join(root, "0f", artworkID, "thumbs.db"),
	}
	writeFiles(t, foreign...)

	freshTemp := filepath.Join(root, "0f", artworkID, ".tmp-fresh")
	oldTemp := filepath.Join(root, "0f", artworkID, ".tmp-old")
	writeFiles(t, freshTemp, oldTemp)

	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldTemp, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	keep := func(id, tag string) bool { return id == artworkID && tag == tagA }
	if err := cache.Purge(ctx, keep); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	assertExists(t, true, append(foreign, freshTemp)...)
	assertExists(t, false, oldTemp, filepath.Join(root, "7c"))

	if got := read(t, cache, current); got != "x" {
		t.Errorf("current entry = %q", got)
	}

	if _, err := cache.Open(ctx, stale); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("stale entry: err = %v, want ErrNotFound", err)
	}
}

func TestPurgeOfMissingRootSucceeds(t *testing.T) {
	t.Parallel()

	cache, _ := newCache(t)

	if err := cache.Purge(t.Context(), func(string, string) bool { return false }); err != nil {
		t.Fatalf("Purge: %v", err)
	}
}

func TestPurgeHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	cache, _ := newCache(t)
	if err := cache.Store(t.Context(), key(artworkID, tagA, artwork.Width320), []byte("x")); err != nil {
		t.Fatalf("Store: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := cache.Purge(ctx, func(string, string) bool { return false }); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestStoreAfterPurgeRemovedTheDirectory(t *testing.T) {
	t.Parallel()

	cache, _ := newCache(t)
	ctx := t.Context()
	entry := key(artworkID, tagA, artwork.Width320)

	if err := cache.Store(ctx, entry, []byte("x")); err != nil {
		t.Fatalf("Store: %v", err)
	}

	if err := cache.Purge(ctx, func(string, string) bool { return false }); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	if err := cache.Store(ctx, entry, []byte("y")); err != nil {
		t.Fatalf("Store after purge: %v", err)
	}

	if got := read(t, cache, entry); got != "y" {
		t.Errorf("content = %q", got)
	}
}

func TestPurgeWhileAnEntryIsBeingServed(t *testing.T) {
	t.Parallel()

	cache, _ := newCache(t)
	ctx := t.Context()
	entry := key(artworkID, tagA, artwork.Width320)

	if err := cache.Store(ctx, entry, []byte("served")); err != nil {
		t.Fatalf("Store: %v", err)
	}

	cached, err := cache.Open(ctx, entry)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = cached.Content.Close() }()

	if err := cache.Purge(ctx, func(string, string) bool { return false }); err != nil {
		t.Fatalf("Purge: %v", err)
	}

	data, err := io.ReadAll(cached.Content)
	if err != nil || string(data) != "served" {
		t.Errorf("read during purge = %q, %v", data, err)
	}
}

func writeFiles(t *testing.T, paths ...string) {
	t.Helper()

	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}

		if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
}

func assertExists(t *testing.T, want bool, paths ...string) {
	t.Helper()

	for _, path := range paths {
		_, err := os.Stat(path)
		if exists := err == nil; exists != want {
			t.Errorf("%s exists = %v, want %v", path, exists, want)
		}
	}
}

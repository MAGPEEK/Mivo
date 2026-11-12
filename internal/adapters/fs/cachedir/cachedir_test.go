package cachedir_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/cachedir"
	"github.com/magpeek/mivo/internal/app/trickplay"
	"github.com/magpeek/mivo/internal/domain"
)

const (
	fileOne = "11111111-1111-4111-8111-111111111111"
	fileTwo = "22222222-2222-4222-8222-222222222222"
)

func write(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSubtitleCacheOpensListsAndRemovesItsFiles(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	root := filepath.Join(cacheDir, "subtitles")
	cache, err := cachedir.NewSubtitles(cacheDir)
	if err != nil {
		t.Fatalf("NewSubtitles: %v", err)
	}

	cached := filepath.Join(root, fileOne, "a.vtt")
	write(t, cached, "WEBVTT")
	write(t, filepath.Join(root, fileOne, "b.vtt.tmp"), "x")
	write(t, filepath.Join(root, "not-an-id", "c.vtt"), "ignored")
	write(t, filepath.Join(root, "stray.vtt"), "ignored")

	file, err := cache.Open(t.Context(), cached)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	body, _ := io.ReadAll(file.Content)
	_ = file.Content.Close()

	if string(body) != "WEBVTT" || file.Size != int64(len(body)) {
		t.Errorf("Open = %q size %d", body, file.Size)
	}

	entries, err := cache.List(t.Context(), root)
	if err != nil || len(entries) != 2 {
		t.Fatalf("List = %v, %v, want the two files of the file folder", entries, err)
	}

	for _, entry := range entries {
		if entry.Directory != fileOne || filepath.Dir(entry.Path) != filepath.Join(root, fileOne) {
			t.Errorf("entry = %+v, want the folder %s", entry, fileOne)
		}
	}

	if err := cache.Remove(t.Context(), cached); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if err := cache.Remove(t.Context(), cached); err != nil {
		t.Errorf("Remove of a removed file = %v, want nil", err)
	}

	if _, err := cache.Open(t.Context(), cached); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Open after Remove = %v, want not found", err)
	}
}

// TestSubtitleCacheNeverLeavesItsFolder: the cache only touches <cache>/subtitles/<fileID>/<name>.
func TestSubtitleCacheNeverLeavesItsFolder(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache, err := cachedir.NewSubtitles(cacheDir)
	if err != nil {
		t.Fatalf("NewSubtitles: %v", err)
	}

	outside := filepath.Join(cacheDir, "secret.txt")
	write(t, outside, "secret")
	write(t, filepath.Join(cacheDir, "subtitles", fileOne, "a.vtt"), "WEBVTT")

	root := filepath.Join(cacheDir, "subtitles")
	paths := []string{
		outside,
		filepath.Join(root, "..", "secret.txt"),
		filepath.Join(root, fileOne, "..", "..", "secret.txt"),
		filepath.Join(root, "not-an-id", "a.vtt"),
		filepath.Join(root, fileOne),
		filepath.Join(root, fileOne, "deeper", "a.vtt"),
		filepath.Join(cacheDir, "trickplay", fileOne, "a.vtt"),
		"relative.vtt",
	}

	for _, path := range paths {
		if _, err := cache.Open(t.Context(), path); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("Open(%q) = %v, want forbidden", path, err)
		}

		if err := cache.Remove(t.Context(), path); !errors.Is(err, domain.ErrForbidden) {
			t.Errorf("Remove(%q) = %v, want forbidden", path, err)
		}
	}

	if _, err := cache.List(t.Context(), cacheDir); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("List(cache dir) = %v, want forbidden", err)
	}

	if _, err := os.Stat(outside); err != nil {
		t.Errorf("the file outside the cache is gone: %v", err)
	}
}

func TestSubtitleCacheListsNothingBeforeTheFirstConversion(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache, _ := cachedir.NewSubtitles(cacheDir)

	entries, err := cache.List(t.Context(), filepath.Join(cacheDir, "subtitles"))
	if err != nil || len(entries) != 0 {
		t.Errorf("List = %v, %v, want none", entries, err)
	}
}

func TestCachesRefuseARelativeCacheDirectory(t *testing.T) {
	t.Parallel()

	if _, err := cachedir.NewSubtitles("relative"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("NewSubtitles = %v, want invalid", err)
	}

	if _, err := cachedir.NewTrickplay("relative"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("NewTrickplay = %v, want invalid", err)
	}
}

func TestTrickplayStoreRoundTripsTheManifest(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache, _ := cachedir.NewTrickplay(cacheDir)

	if _, err := cache.Load(t.Context(), fileOne); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Load before Save = %v, want not found", err)
	}

	want := trickplay.Stored{
		Key: "10-20",
		Manifest: trickplay.Manifest{
			TileWidth: 320, TileHeight: 180, Columns: 10, Rows: 10, Interval: 10 * time.Second, SheetCount: 3,
		},
	}

	if err := cache.Save(t.Context(), fileOne, want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := cache.Load(t.Context(), fileOne)
	if err != nil || got != want {
		t.Errorf("Load = %+v, %v, want %+v", got, err, want)
	}

	if cache.Directory(fileOne) != filepath.Join(cacheDir, "trickplay", fileOne) {
		t.Errorf("Directory = %s", cache.Directory(fileOne))
	}

	if err := cache.Touch(t.Context(), fileOne); err != nil {
		t.Errorf("Touch: %v", err)
	}
}

// TestTrickplayCacheRefusesIDsThatAreNoFileIDs: nothing but a canonical UUID becomes a folder
// name, and the lookup of a bad ID stays inside the cache.
func TestTrickplayCacheRefusesIDsThatAreNoFileIDs(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	cache, _ := cachedir.NewTrickplay(cacheDir)
	secret := filepath.Join(cacheDir, "secret", "manifest.json")
	write(t, secret, `{"key":"k"}`)

	for _, id := range []string{"", "..", "../secret", "secret", `..\secret`, "11111111-1111-4111-8111-11111111111", "A1111111-1111-4111-8111-111111111111"} {
		rel, err := filepath.Rel(filepath.Join(cacheDir, "trickplay"), cache.Directory(id))
		if err != nil || !filepath.IsLocal(rel) {
			t.Errorf("Directory(%q) = %s leaves the cache", id, cache.Directory(id))
		}

		if _, err := cache.Load(t.Context(), id); !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("Load(%q) = %v, want not found", id, err)
		}

		if err := cache.Save(t.Context(), id, trickplay.Stored{}); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Save(%q) = %v, want invalid", id, err)
		}

		if err := cache.Remove(t.Context(), id); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Remove(%q) = %v, want invalid", id, err)
		}
	}

	if _, err := os.Stat(secret); err != nil {
		t.Errorf("a file outside the cache is gone: %v", err)
	}
}

func TestTrickplayCacheListsRemovesAndSweepsStaging(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	root := filepath.Join(cacheDir, "trickplay")
	cache, _ := cachedir.NewTrickplay(cacheDir)

	write(t, filepath.Join(root, fileOne, "sheet-001.jpg"), "12345")
	write(t, filepath.Join(root, fileOne, ".trickplay-abc", "sheet-001.jpg"), "partial")
	write(t, filepath.Join(root, fileTwo, "sheet-001.jpg"), "123")
	write(t, filepath.Join(root, "other", "sheet-001.jpg"), "ignored")

	if err := cache.Save(t.Context(), fileOne, trickplay.Stored{Key: "k"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if err := cache.SweepStaging(t.Context()); err != nil {
		t.Fatalf("SweepStaging: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, fileOne, ".trickplay-abc")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("staging directory survived the sweep: %v", err)
	}

	entries, err := cache.Entries(t.Context())
	if err != nil || len(entries) != 2 {
		t.Fatalf("Entries = %v, %v, want the two file folders", entries, err)
	}

	for _, entry := range entries {
		if entry.Bytes == 0 || entry.LastUsed.IsZero() {
			t.Errorf("entry %+v lacks its size or last use", entry)
		}
	}

	if err := cache.Remove(t.Context(), fileTwo); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if err := cache.Remove(t.Context(), fileTwo); err != nil {
		t.Errorf("Remove of a removed entry = %v, want nil", err)
	}

	if _, err := os.Stat(filepath.Join(root, "other", "sheet-001.jpg")); err != nil {
		t.Errorf("a foreign folder was touched: %v", err)
	}
}

func TestTrickplayCacheOfAnEmptyCacheDirectory(t *testing.T) {
	t.Parallel()

	cache, _ := cachedir.NewTrickplay(t.TempDir())

	if entries, err := cache.Entries(t.Context()); err != nil || len(entries) != 0 {
		t.Errorf("Entries = %v, %v, want none", entries, err)
	}

	if err := cache.SweepStaging(t.Context()); err != nil {
		t.Errorf("SweepStaging = %v, want nil", err)
	}

	if err := cache.Remove(t.Context(), fileOne); err != nil {
		t.Errorf("Remove = %v, want nil", err)
	}
}

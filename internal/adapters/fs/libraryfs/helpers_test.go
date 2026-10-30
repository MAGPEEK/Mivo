package libraryfs_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	testFileMode = 0o600
	testDirMode  = 0o750
	testContent  = "video bytes"
	// Log keys the adapter uses; the tests check that paths are logged relative to the root.
	pathKey = "path"
)

// logRecord is the part of a slog.Record the tests assert on.
type logRecord struct {
	level   slog.Level
	message string
	attrs   map[string]slog.Value
}

// recordingHandler keeps every record at any level in memory.
type recordingHandler struct {
	mutex   *sync.Mutex
	records *[]logRecord
}

func (recordingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (handler recordingHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := map[string]slog.Value{}
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value
		return true
	})

	handler.mutex.Lock()
	defer handler.mutex.Unlock()

	*handler.records = append(*handler.records, logRecord{level: record.Level, message: record.Message, attrs: attrs})

	return nil
}

// WithAttrs and WithGroup are not used by the adapter; returning the handler unchanged is
// enough for these tests.
func (handler recordingHandler) WithAttrs([]slog.Attr) slog.Handler { return handler }

func (handler recordingHandler) WithGroup(string) slog.Handler { return handler }

// logs returns the records written so far.
type logs func() []logRecord

// newFileSystem returns the adapter with a logger that records every level.
func newFileSystem(t *testing.T) (*libraryfs.FileSystem, logs) {
	t.Helper()

	handler := recordingHandler{mutex: &sync.Mutex{}, records: &[]logRecord{}}

	fileSystem, err := libraryfs.New(slog.New(handler))
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	snapshot := func() []logRecord {
		handler.mutex.Lock()
		defer handler.mutex.Unlock()

		return slices.Clone(*handler.records)
	}

	return fileSystem, snapshot
}

// newLibrary returns an empty library root inside t.TempDir() next to an "outside" directory
// that holds outside/secret.mkv, the file no access through the root may reach.
func newLibrary(t *testing.T) (string, string) {
	t.Helper()

	base := t.TempDir()
	root := filepath.Join(base, "library")
	outside := filepath.Join(base, "outside")

	mkdir(t, root)
	writeFile(t, filepath.Join(outside, "secret.mkv"), "secret")

	return root, outside
}

// writeFiles creates every slash-separated relative path below root with testContent.
func writeFiles(t *testing.T, root string, relativePaths ...string) {
	t.Helper()

	for _, relativePath := range relativePaths {
		writeFile(t, filepath.Join(root, filepath.FromSlash(relativePath)), testContent)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()

	mkdir(t, filepath.Dir(path))

	if err := os.WriteFile(path, []byte(content), testFileMode); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(path, testDirMode); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}

// symlinkOrSkip creates a symlink or skips the test where the OS does not allow it (Windows
// without developer mode or administrator rights).
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()

	mkdir(t, filepath.Dir(link))

	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks not permitted on this Windows account: %v", err)
		}

		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

// walkPaths walks root and returns the reported relative paths in walk order.
func walkPaths(t *testing.T, fileSystem *libraryfs.FileSystem, root string) []string {
	t.Helper()

	paths, _ := walkWithSkipped(t, fileSystem, root)

	return paths
}

// walkWithSkipped walks root and returns the reported relative paths in walk order and the
// paths Walk could not read.
func walkWithSkipped(t *testing.T, fileSystem *libraryfs.FileSystem, root string) ([]string, []string) {
	t.Helper()

	var paths []string

	skipped, err := fileSystem.Walk(t.Context(), root, func(state library.FileState) error {
		paths = append(paths, state.RelativePath)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk(%s) error = %v", root, err)
	}

	return paths, skipped
}

// noVisit is a visit function for walks whose files do not matter.
func noVisit(library.FileState) error { return nil }

// countRecords counts the records with the level and message.
func countRecords(records []logRecord, level slog.Level, message string) int {
	count := 0

	for _, record := range records {
		if record.level == level && record.message == message {
			count++
		}
	}

	return count
}

// assertPaths compares reported paths with the expected ones, ignoring order.
func assertPaths(t *testing.T, got []string, want ...string) {
	t.Helper()

	got = slices.Sorted(slices.Values(got))
	want = slices.Sorted(slices.Values(want))

	if !slices.Equal(got, want) {
		t.Errorf("walked paths = %q, want %q", got, want)
	}
}

// assertLogged checks that a record with the level and root-relative path attribute exists.
func assertLogged(t *testing.T, records []logRecord, level slog.Level, relativePath string) {
	t.Helper()

	for _, record := range records {
		if record.level == level && record.attrs[pathKey].String() == relativePath {
			return
		}
	}

	t.Errorf("no %s record with path %q in %+v", level, relativePath, records)
}

// assertNoAbsolutePathsLogged checks that no attribute of any record contains the base
// directory of the test, so logs only ever show root-relative paths.
func assertNoAbsolutePathsLogged(t *testing.T, records []logRecord, base string) {
	t.Helper()

	for _, record := range records {
		for key, value := range record.attrs {
			if containsFold(value.String(), base) {
				t.Errorf("record %q attribute %s = %q contains the absolute path %s",
					record.message, key, value.String(), base)
			}
		}
	}
}

// containsFold reports whether text contains part, ignoring case (Windows paths).
func containsFold(text, part string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(part))
}

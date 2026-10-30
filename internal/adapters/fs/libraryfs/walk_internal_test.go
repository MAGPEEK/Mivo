package libraryfs

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

// pathRecord is a log record reduced to what these tests check.
type pathRecord struct {
	level   slog.Level
	message string
	path    string
}

// pathRecorder keeps the level and path attribute of every record.
type pathRecorder struct {
	mutex   *sync.Mutex
	records *[]pathRecord
}

func (pathRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (recorder pathRecorder) Handle(_ context.Context, record slog.Record) error {
	path := ""

	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == pathLogKey {
			path = attr.Value.String()
		}

		return true
	})

	recorder.mutex.Lock()
	defer recorder.mutex.Unlock()

	*recorder.records = append(*recorder.records, pathRecord{level: record.Level, message: record.Message, path: path})

	return nil
}

func (recorder pathRecorder) WithAttrs([]slog.Attr) slog.Handler { return recorder }

func (recorder pathRecorder) WithGroup(string) slog.Handler { return recorder }

// injectedWalkResult is what one walk with an injected identity function produced.
type injectedWalkResult struct {
	paths   []string
	skipped []string
	records []pathRecord
}

// walkWithIdentity walks a fresh library with the files below, identifying directories with
// identify instead of the file system.
func walkWithIdentity(t *testing.T, identify func(*os.File) (directoryID, error), files ...string) injectedWalkResult {
	t.Helper()

	root := t.TempDir()
	for _, file := range files {
		path := filepath.Join(root, filepath.FromSlash(file))
		if err := os.MkdirAll(filepath.Dir(path), internalTestDirMode); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}

		if err := os.WriteFile(path, []byte("video"), internalTestFileMode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	libraryRoot, err := os.OpenRoot(root)
	if err != nil {
		t.Fatalf("open root: %v", err)
	}

	defer closeRoot(libraryRoot)

	var result injectedWalkResult

	recorder := pathRecorder{mutex: &sync.Mutex{}, records: &[]pathRecord{}}
	walker := newWalk(libraryRoot, slog.New(recorder), func(state library.FileState) error {
		result.paths = append(result.paths, state.RelativePath)
		return nil
	})
	walker.identify = identify

	if err := walker.run(t.Context()); err != nil {
		t.Fatalf("run error = %v", err)
	}

	result.skipped = walker.skipped
	result.records = *recorder.records

	return result
}

// isLibraryRoot reports whether a directory opened by the walk is the library root: the root
// is opened as ".", every other directory by its relative path.
func isLibraryRoot(directory *os.File) bool {
	return filepath.Base(directory.Name()) == rootDirectory
}

func (result injectedWalkResult) logged(level slog.Level, path string) bool {
	return slices.ContainsFunc(result.records, func(record pathRecord) bool {
		return record.level == level && record.path == path
	})
}

func TestWalkReportsADirectoryWhoseIdentityCollidesInTheRealTree(t *testing.T) {
	t.Parallel()

	// ReFS, SMB or FUSE file systems may report one id for different directories. A collision
	// in the real tree must not silently drop a subtree: the scan would delete its entries.
	colliding := func(*os.File) (directoryID, error) {
		return directoryID{device: 1, indexLow: 42}, nil
	}

	result := walkWithIdentity(t, colliding, "top.mkv", "a/movie.mkv", "b/clip.mp4")

	if !slices.Equal(result.paths, []string{"top.mkv"}) {
		t.Errorf("walked paths = %q, want only the root's file", result.paths)
	}

	if !slices.Equal(result.skipped, []string{"a", "b"}) {
		t.Errorf("skipped = %q, want both colliding directories", result.skipped)
	}

	for _, path := range []string{"a", "b"} {
		if !result.logged(slog.LevelWarn, path) {
			t.Errorf("no warning for the colliding directory %s in %+v", path, result.records)
		}
	}
}

func TestWalkListsEveryDirectoryWithAnUnknownIdentity(t *testing.T) {
	t.Parallel()

	for name, unknown := range map[string]directoryID{
		"zero":           {},
		"zero on device": {device: 7},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			identify := func(*os.File) (directoryID, error) { return unknown, nil }

			result := walkWithIdentity(t, identify, "top.mkv", "a/movie.mkv", "a/b/clip.mp4", "c/film.webm")

			want := []string{"a/b/clip.mp4", "a/movie.mkv", "c/film.webm", "top.mkv"}
			if !slices.Equal(result.paths, want) || len(result.skipped) != 0 {
				t.Errorf("walk = (%q, skipped %q), want (%q, none)", result.paths, result.skipped, want)
			}
		})
	}
}

func TestWalkSkipsDirectoriesItCannotIdentify(t *testing.T) {
	t.Parallel()

	failing := func(directory *os.File) (directoryID, error) {
		switch filepath.Base(directory.Name()) {
		case "broken":
			return directoryID{}, errors.New("identity not readable")
		case "vanished":
			return directoryID{}, fs.ErrNotExist
		default:
			return directoryID{device: 1, indexLow: uint64(len(directory.Name()))}, nil
		}
	}

	result := walkWithIdentity(t, failing, "ok/movie.mkv", "broken/movie.mkv", "vanished/movie.mkv")

	if !slices.Equal(result.paths, []string{"ok/movie.mkv"}) {
		t.Errorf("walked paths = %q, want only ok/movie.mkv", result.paths)
	}

	// An unreadable directory is reported, one that disappeared is not: its entries may go.
	if !slices.Equal(result.skipped, []string{"broken"}) {
		t.Errorf("skipped = %q, want [broken]", result.skipped)
	}

	if !result.logged(slog.LevelWarn, "broken") || !result.logged(slog.LevelDebug, "vanished") {
		t.Errorf("records = %+v, want a warning for broken and a debug record for vanished", result.records)
	}
}

func TestWalkRecognisesADuplicateBeforeTheDepthLimit(t *testing.T) {
	t.Parallel()

	// One directory below the limit shares the root's id: as a link duplicate it is no failure.
	deep := strings.Repeat("d/", maxDirectoryDepth+1)

	identify := func(directory *os.File) (directoryID, error) {
		if isLibraryRoot(directory) || strings.HasSuffix(filepath.ToSlash(directory.Name()), strings.TrimSuffix(deep, "/")) {
			return directoryID{device: 1, indexLow: 1}, nil
		}

		return directoryID{device: 1, indexLow: uint64(len(directory.Name()))}, nil
	}

	result := walkWithIdentity(t, identify, deep+"movie.mkv")

	deepest := strings.TrimSuffix(deep, "/")

	// The real tree reports the collision (warn + skipped) instead of the depth limit.
	if !slices.Equal(result.skipped, []string{deepest}) {
		t.Errorf("skipped = %q, want the deepest directory once", result.skipped)
	}

	want := pathRecord{level: slog.LevelWarn, message: collisionMessage, path: deepest}
	if !slices.Contains(result.records, want) {
		t.Errorf("records = %+v, want %+v", result.records, want)
	}
}

func TestDirectoryIDKnown(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		identity directoryID
		want     bool
	}{
		{identity: directoryID{}, want: false},
		{identity: directoryID{device: 3}, want: false},
		{identity: directoryID{indexLow: 1}, want: true},
		{identity: directoryID{indexHigh: 1}, want: true},
	}

	for _, testCase := range testCases {
		if got := testCase.identity.known(); got != testCase.want {
			t.Errorf("%+v.known() = %t, want %t", testCase.identity, got, testCase.want)
		}
	}
}

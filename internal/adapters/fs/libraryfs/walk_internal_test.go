package libraryfs

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

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

// walkTimeout bounds a walk of these tests, so a walk that multiplies itself fails the test
// instead of hanging it.
const walkTimeout = 10 * time.Second

// walkWithIdentity walks a fresh library with the files below, identifying directories with
// identify instead of the file system.
func walkWithIdentity(t *testing.T, identify func(*os.File) (directoryID, error), files ...string) injectedWalkResult {
	t.Helper()

	return walkRootWithIdentity(t, newInternalLibrary(t, files...), identify)
}

// newInternalLibrary creates a library root holding the files below.
func newInternalLibrary(t *testing.T, files ...string) string {
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

	return root
}

// symlinkInLibraryOrSkip creates the link (a slash-separated path below root) to target, or
// skips the test where the OS does not allow symlinks (Windows without developer mode).
func symlinkInLibraryOrSkip(t *testing.T, root, link, target string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(link))
	if err := os.MkdirAll(filepath.Dir(path), internalTestDirMode); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}

	if err := os.Symlink(filepath.FromSlash(target), path); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks not permitted on this Windows account: %v", err)
		}

		t.Fatalf("symlink %s -> %s: %v", link, target, err)
	}
}

// walkRootWithIdentity walks root, identifying directories with identify instead of the file
// system, and fails the test when the walk does not end within walkTimeout.
func walkRootWithIdentity(t *testing.T, root string, identify func(*os.File) (directoryID, error)) injectedWalkResult {
	t.Helper()

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

	// The walk checks its context before every entry, so a walk that multiplies itself ends
	// with the deadline instead of hanging the test.
	ctx, cancel := context.WithTimeout(t.Context(), walkTimeout)
	defer cancel()

	if err := walker.run(ctx); err != nil {
		t.Fatalf("run error = %v (after %d paths)", err, len(result.paths))
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

// Without directory ids the walk cannot tell a link back to an ancestor from a new folder.
// Following such links would list the ancestor again with every level, multiplying the
// pending links; link-reached directories with an unknown identity are therefore skipped.
func TestWalkSkipsLinkedDirectoriesWithAnUnknownIdentity(t *testing.T) {
	t.Parallel()

	links := []string{"links/up-1", "links/up-2", "links/up-3"}

	root := newInternalLibrary(t, "top.mkv", "a/movie.mkv", "a/b/clip.mp4")
	for _, link := range links {
		symlinkInLibraryOrSkip(t, root, link, "..")
	}

	unknown := func(*os.File) (directoryID, error) { return directoryID{device: 7}, nil }

	result := walkRootWithIdentity(t, root, unknown)

	want := []string{"a/b/clip.mp4", "a/movie.mkv", "top.mkv"}
	if !slices.Equal(result.paths, want) {
		const shown = 10
		t.Errorf("walked %d paths, first %q, want each real-tree file once: %q",
			len(result.paths), result.paths[:min(len(result.paths), shown)], want)
	}

	if !slices.Equal(result.skipped, links) {
		t.Errorf("skipped = %q, want the links %q", result.skipped, links)
	}

	for _, link := range links {
		want := pathRecord{level: slog.LevelInfo, message: unknownLinkedDirectoryMessage, path: link}
		if !slices.Contains(result.records, want) {
			t.Errorf("records = %+v, want %+v", result.records, want)
		}
	}
}

// The admit rules without a file system, so they also run where symlinks need privileges.
func TestAdmitListsUnknownIdentitiesOnlyInTheRealTree(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		at          location
		wantAdmit   bool
		wantSkipped []string
		wantLevel   slog.Level
	}{
		{name: "real tree", at: location{relativePath: "a", depth: 1}, wantAdmit: true},
		{
			name: "through a link", at: location{relativePath: "links/up", depth: 2, throughLink: true},
			wantSkipped: []string{"links/up"}, wantLevel: slog.LevelInfo,
		},
	}

	for _, testCase := range testCases {
		recorder := pathRecorder{mutex: &sync.Mutex{}, records: &[]pathRecord{}}
		walker := newWalk(nil, slog.New(recorder), func(library.FileState) error { return nil })

		admitted := walker.admit(testCase.at, directoryID{device: 7})

		if admitted != testCase.wantAdmit || !slices.Equal(walker.skipped, testCase.wantSkipped) {
			t.Errorf("%s: admit = %t, skipped %q, want %t, %q",
				testCase.name, admitted, walker.skipped, testCase.wantAdmit, testCase.wantSkipped)
		}

		wantRecords := []pathRecord{}
		if !testCase.wantAdmit {
			wantRecords = []pathRecord{{level: testCase.wantLevel, message: unknownLinkedDirectoryMessage, path: testCase.at.relativePath}}
		}

		if !slices.Equal(*recorder.records, wantRecords) {
			t.Errorf("%s: records = %+v, want %+v", testCase.name, *recorder.records, wantRecords)
		}

		if len(walker.visited) != 0 {
			t.Errorf("%s: visited = %v, want an unknown identity never recorded", testCase.name, walker.visited)
		}
	}
}

func TestWalkListsALinkedDirectoryWithAKnownIdentityOnce(t *testing.T) {
	t.Parallel()

	root := newInternalLibrary(t, "a/movie.mkv", "b/clip.mp4")
	symlinkInLibraryOrSkip(t, root, "b/to-a", "../a")
	symlinkInLibraryOrSkip(t, root, "b/up", "..")

	result := walkRootWithIdentity(t, root, identifyDirectory)

	if want := []string{"a/movie.mkv", "b/clip.mp4"}; !slices.Equal(result.paths, want) {
		t.Errorf("walked paths = %q, want %q", result.paths, want)
	}

	if len(result.skipped) != 0 {
		t.Errorf("skipped = %q, want none: a walked directory is no gap", result.skipped)
	}

	for _, link := range []string{"b/to-a", "b/up"} {
		want := pathRecord{level: slog.LevelDebug, message: alreadyWalkedMessage, path: link}
		if !slices.Contains(result.records, want) {
			t.Errorf("records = %+v, want %+v", result.records, want)
		}
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

	// The deepest directory, beyond the depth limit, shares the root's id. The identity is
	// checked first, so the walk reports it as a collision of the real tree, not as too deep;
	// reached through a link, the same check makes it a harmless duplicate at any depth.
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

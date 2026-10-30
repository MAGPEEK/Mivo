package libraryfs_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// alreadyWalkedMessage is the debug message for a directory reached a second time.
const alreadyWalkedMessage = "skipping directory that was already walked"

func TestNewRejectsANilLogger(t *testing.T) {
	t.Parallel()

	if _, err := libraryfs.New(nil); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("New(nil) error = %v, want ErrInvalid", err)
	}
}

func TestWalkReportsVideoFilesAndSkipsHiddenAndSystemEntries(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	writeFiles(t, root,
		"Movie.mkv",
		"UPPER.MP4",
		"Shows/Show A/Season 1/S01E01.m2ts",
		"Shows/Show A/Season 1/S01E01.srt",
		"Shows/Show A/Season 1/notes.txt",
		"Amélie (2001).mkv",
		"日本語/映画.MKV",
		"Ünïcödé/Ω.webm",
		"@eaDir/Movie.mkv",
		"Sub/@eaDir/thumb.mkv",
		"#recycle/old.mkv",
		"#snapshot/old.mkv",
		"$RECYCLE.BIN/old.mkv",
		"System Volume Information/old.mkv",
		"lost+found/old.mkv",
		".hidden/secret.mkv",
		".hidden.mkv",
		"Sub/._Movie.mkv",
		"Sub/.@__thumb/thumb.mkv",
		"Sub/Clip.mov",
	)
	mkdir(t, filepath.Join(root, "Empty", "Nested"))

	assertPaths(t, walkPaths(t, fileSystem, root),
		"Movie.mkv",
		"UPPER.MP4",
		"Shows/Show A/Season 1/S01E01.m2ts",
		"Amélie (2001).mkv",
		"日本語/映画.MKV",
		"Ünïcödé/Ω.webm",
		"Sub/Clip.mov",
	)
}

func TestWalkReportsSizeAndModificationTime(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	path := filepath.Join(root, "Folder", "movie.mkv")
	writeFile(t, path, "0123456789")

	modified := time.Date(2024, time.March, 9, 20, 15, 30, 0, time.UTC)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}

	var states []library.FileState

	_, err := fileSystem.Walk(t.Context(), root, func(state library.FileState) error {
		states = append(states, state)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk error = %v", err)
	}

	want := library.FileState{RelativePath: "Folder/movie.mkv", Size: 10, ModifiedAt: modified}
	if len(states) != 1 || library.Changed(states[0], want) {
		t.Errorf("Walk states = %+v, want [%+v]", states, want)
	}
}

func TestWalkVisitsDirectoriesInNameOrder(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "c.mkv", "a/z.mkv", "b.mkv", "a/y.mkv")

	got := walkPaths(t, fileSystem, root)
	want := []string{"a/y.mkv", "a/z.mkv", "b.mkv", "c.mkv"}

	if len(got) != len(want) {
		t.Fatalf("walked paths = %q, want %q", got, want)
	}

	for index := range want {
		if got[index] != want[index] {
			t.Errorf("walked paths = %q, want %q", got, want)
		}
	}
}

func TestWalkStopsWhenTheContextIsCancelled(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "a.mkv", "b.mkv", "c/d.mkv", "c/e.mkv", "f.mkv")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	visited := 0

	_, err := fileSystem.Walk(ctx, root, func(library.FileState) error {
		visited++
		cancel()

		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Walk error = %v, want context.Canceled", err)
	}

	if visited != 1 {
		t.Errorf("visited %d files after cancellation, want 1", visited)
	}
}

func TestWalkWithACancelledContextVisitsNothing(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "a.mkv")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := fileSystem.Walk(ctx, root, func(state library.FileState) error {
		t.Errorf("visited %s with a cancelled context", state.RelativePath)
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Walk error = %v, want context.Canceled", err)
	}
}

func TestWalkStopsWithTheErrorOfVisit(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "a.mkv", "b.mkv")

	errStop := errors.New("stop")
	visited := 0

	_, err := fileSystem.Walk(t.Context(), root, func(library.FileState) error {
		visited++
		return errStop
	})

	if !errors.Is(err, errStop) || visited != 1 {
		t.Errorf("Walk = (%v, %d visits), want (errStop, 1)", err, visited)
	}
}

func TestWalkReportsAnUnusableRootAsUnavailable(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	base := t.TempDir()
	file := filepath.Join(base, "file.mkv")
	writeFile(t, file, testContent)

	for name, root := range map[string]string{
		"missing":        filepath.Join(base, "missing"),
		"a file":         file,
		"below a file":   filepath.Join(file, "library"),
		"missing parent": filepath.Join(base, "gone", "library"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := fileSystem.Walk(t.Context(), root, noVisit)
			if !errors.Is(err, domain.ErrUnavailable) {
				t.Errorf("Walk(%s) error = %v, want ErrUnavailable", root, err)
			}
		})
	}
}

func TestWalkRejectsInvalidArguments(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)

	if _, err := fileSystem.Walk(t.Context(), root, nil); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Walk with a nil visit error = %v, want ErrInvalid", err)
	}

	_, err := fileSystem.Walk(t.Context(), "library", noVisit)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("Walk with a relative root error = %v, want ErrInvalid", err)
	}
}

func TestWalkFollowsSymlinksThatStayInsideTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "real/movie.mkv")

	// A hidden folder is not walked by itself, so its content is reported under the link.
	writeFiles(t, root, ".store/films/hidden.mkv", ".store/films/nested/deeper.mp4")

	// A file link is an alias the user made on purpose and is reported under its own path.
	symlinkOrSkip(t, filepath.Join("..", "real", "movie.mkv"), filepath.Join(root, "links", "alias.mkv"))
	// A link whose own name is no video is not reported even if its target is one.
	symlinkOrSkip(t, filepath.Join("..", "real", "movie.mkv"), filepath.Join(root, "links", "alias"))
	// The directory link is followed after the real tree, including its plain subfolders.
	symlinkOrSkip(t, filepath.Join(".store", "films"), filepath.Join(root, "a-films"))
	// A directory link to a folder of the real tree is skipped: its files keep the real path,
	// even though "links" sorts before "real".
	symlinkOrSkip(t, filepath.Join("..", "real"), filepath.Join(root, "links", "films"))

	assertPaths(t, walkPaths(t, fileSystem, root),
		"real/movie.mkv", "links/alias.mkv", "a-films/hidden.mkv", "a-films/nested/deeper.mp4")

	records := logs()
	assertNoWarnings(t, records)
	assertLogged(t, records, slog.LevelDebug, "links/films")
}

func TestWalkListsEveryCrossLinkedDirectoryOnce(t *testing.T) {
	t.Parallel()

	const folderCount = 4

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)

	// "By genre" layouts: every folder links to every other folder. Following the links
	// naively would list folders folderCount! times and report every file many times.
	var want []string

	for folder := range folderCount {
		file := fmt.Sprintf("folder-%d/movie-%d.mkv", folder, folder)
		writeFiles(t, root, file)
		want = append(want, file)
	}

	for folder := range folderCount {
		for other := range folderCount {
			if other != folder {
				symlinkOrSkip(t, filepath.Join("..", fmt.Sprintf("folder-%d", other)),
					filepath.Join(root, fmt.Sprintf("folder-%d", folder), fmt.Sprintf("link-%d", other)))
			}
		}
	}

	got := walkPaths(t, fileSystem, root)
	if len(got) != folderCount {
		t.Errorf("walk reported %d files, want each of the %d files once: %q", len(got), folderCount, got)
	}

	assertPaths(t, got, want...)

	// Each link is opened once to learn that its target was walked already, and not listed.
	links := folderCount * (folderCount - 1)
	if count := countRecords(logs(), slog.LevelDebug, alreadyWalkedMessage); count != links {
		t.Errorf("%d directories skipped as already walked, want one per link (%d)", count, links)
	}
}

func TestWalkStopsAtTheDepthLimit(t *testing.T) {
	t.Parallel()

	// One level deeper than the limit of the adapter (libraryfs.maxDirectoryDepth).
	const tooDeep = 65

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)

	deepest := strings.Repeat("d/", tooDeep)
	allowed := strings.Repeat("d/", tooDeep-1)
	writeFiles(t, root, allowed+"ok.mkv", deepest+"too-deep.mkv")

	paths, skipped := walkWithSkipped(t, fileSystem, root)

	assertPaths(t, paths, allowed+"ok.mkv")
	assertLogged(t, logs(), slog.LevelWarn, strings.TrimSuffix(deepest, "/"))

	if len(skipped) != 1 || skipped[0] != strings.TrimSuffix(deepest, "/") {
		t.Errorf("skipped = %q, want only the directory below the limit", skipped)
	}
}

func TestWalkSkipsAndLogsSymlinksThatLeaveTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, outside := newLibrary(t)
	writeFiles(t, root, "real/movie.mkv")

	escapes := map[string]string{
		"relative.mkv":    filepath.Join("..", "outside", "secret.mkv"),
		"absolute.mkv":    filepath.Join(outside, "secret.mkv"),
		"relative-dir":    filepath.Join("..", "outside"),
		"absolute-dir":    outside,
		"deep/climb.mkv":  filepath.Join("..", "..", "outside", "secret.mkv"),
		"broken.mkv":      "missing.mkv",
		"absolute-in.mkv": filepath.Join(root, "real", "movie.mkv"), // os.Root refuses every absolute target
		"absolute-in-dir": filepath.Join(root, "real"),
		"self-loop.mkv":   "self-loop.mkv",
		"deep/above-root": filepath.Join("..", ".."),
	}
	for link, target := range escapes {
		symlinkOrSkip(t, target, filepath.Join(root, filepath.FromSlash(link)))
	}

	assertPaths(t, walkPaths(t, fileSystem, root), "real/movie.mkv")

	records := logs()
	for link := range escapes {
		assertLogged(t, records, slog.LevelInfo, link)
	}

	assertNoAbsolutePathsLogged(t, records, filepath.Dir(root))
}

func TestWalkTerminatesOnDirectorySymlinkLoops(t *testing.T) {
	t.Parallel()

	fileSystem, logs := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "a/movie.mkv", "b/clip.mp4")

	symlinkOrSkip(t, ".", filepath.Join(root, "to-root"))
	symlinkOrSkip(t, "..", filepath.Join(root, "a", "up"))
	// Two directories pointing at each other: a/to-b -> b, b/to-a -> a.
	symlinkOrSkip(t, filepath.Join("..", "b"), filepath.Join(root, "a", "to-b"))
	symlinkOrSkip(t, filepath.Join("..", "a"), filepath.Join(root, "b", "to-a"))

	got := walkPaths(t, fileSystem, root)

	// Every directory is reached through the real tree first, so each file keeps its real path
	// and every link is skipped as a directory that was walked already.
	assertPaths(t, got, "a/movie.mkv", "b/clip.mp4")

	records := logs()
	for _, link := range []string{"to-root", "a/up", "a/to-b", "b/to-a"} {
		assertLogged(t, records, slog.LevelDebug, link)
	}

	assertNoWarnings(t, records)
}

func assertNoWarnings(t *testing.T, records []logRecord) {
	t.Helper()

	for _, record := range records {
		if record.level >= slog.LevelWarn {
			t.Errorf("unexpected %s record %q %v", record.level, record.message, record.attrs)
		}
	}
}

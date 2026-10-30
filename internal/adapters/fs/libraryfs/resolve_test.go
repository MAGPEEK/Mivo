package libraryfs_test

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/fs/libraryfs"
	"github.com/magpeek/mivo/internal/domain"
)

// accessCase is one way of reaching a file in a library: Resolve for FFmpeg, Open for the
// direct-play handler. Both must agree on every rule.
type accessCase struct {
	name   string
	access func(fileSystem *libraryfs.FileSystem, root, relativePath string) (string, error)
}

func accessCases() []accessCase {
	return []accessCase{
		{name: "Resolve", access: func(fileSystem *libraryfs.FileSystem, root, relativePath string) (string, error) {
			return fileSystem.Resolve(root, relativePath)
		}},
		{name: "Open", access: func(fileSystem *libraryfs.FileSystem, root, relativePath string) (string, error) {
			file, err := fileSystem.Open(root, relativePath)
			if err != nil {
				return "", err
			}

			defer func() { _ = file.Close() }()

			content, err := io.ReadAll(file)

			return string(content), err
		}},
	}
}

// assertAccessFails checks that both Resolve and Open reject the path with the sentinel.
func assertAccessFails(t *testing.T, root, relativePath string, want error) {
	t.Helper()

	fileSystem, _ := newFileSystem(t)
	assertAccessFailsWith(t, fileSystem, accessAttempt{root: root, relativePath: relativePath, want: want})
}

// accessAttempt is a path that Resolve and Open must refuse with the error want.
type accessAttempt struct {
	root         string
	relativePath string
	want         error
}

// assertAccessFailsWith is assertAccessFails with a given file system; it reports with
// t.Errorf only, so it may run in a goroutine.
func assertAccessFailsWith(t *testing.T, fileSystem *libraryfs.FileSystem, attempt accessAttempt) {
	t.Helper()

	for _, accessCase := range accessCases() {
		got, err := accessCase.access(fileSystem, attempt.root, attempt.relativePath)
		if !errors.Is(err, attempt.want) {
			t.Errorf("%s(%q) = (%q, %v), want error %v", accessCase.name, attempt.relativePath, got, err, attempt.want)
		}
	}
}

func TestResolveReturnsTheEvaluatedAbsolutePath(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/Amélie (2001).mkv")

	got, err := fileSystem.Resolve(root, "Movies/Amélie (2001).mkv")
	if err != nil {
		t.Fatalf("Resolve error = %v", err)
	}

	want := evalSymlinks(t, filepath.Join(root, "Movies", "Amélie (2001).mkv"))
	if got != want || !filepath.IsAbs(got) {
		t.Errorf("Resolve = %q, want the absolute path %q", got, want)
	}
}

func TestOpenReturnsTheFileContent(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/日本語.mkv")

	file, err := fileSystem.Open(root, "Movies/日本語.mkv")
	if err != nil {
		t.Fatalf("Open error = %v", err)
	}

	defer func() { _ = file.Close() }()

	content, err := io.ReadAll(file)
	if err != nil || string(content) != testContent {
		t.Errorf("read = (%q, %v), want %q", content, err, testContent)
	}
}

func TestResolveAndOpenRejectPathsThatAreNotCanonicalOrLocal(t *testing.T) {
	t.Parallel()

	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/movie.mkv")

	for _, relativePath := range []string{
		"",
		"..",
		"../outside/secret.mkv",
		"Movies/../../outside/secret.mkv",
		"Movies/../movie.mkv",
		"./Movies/movie.mkv",
		"Movies/./movie.mkv",
		"Movies//movie.mkv",
		"Movies/movie.mkv/",
		"/Movies/movie.mkv",
		`Movies\movie.mkv`,
		`..\outside\secret.mkv`,
		"C:/Movies/movie.mkv",
		"C:movie.mkv",
		"//server/share/movie.mkv",
		"Movies/movie.mkv\x00.txt",
	} {
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			assertAccessFails(t, root, relativePath, domain.ErrForbidden)
		})
	}
}

func TestResolveAndOpenReportMissingFilesAndDirectoriesAsNotFound(t *testing.T) {
	t.Parallel()

	root, _ := newLibrary(t)
	writeFiles(t, root, "Movies/movie.mkv")

	for _, relativePath := range []string{"missing.mkv", "Movies", "Movies/missing.mkv", "Missing/movie.mkv", "Movies/movie.mkv/inner.mkv"} {
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			assertAccessFails(t, root, relativePath, domain.ErrNotFound)
		})
	}
}

func TestResolveAndOpenReportAnUnusableRoot(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	writeFile(t, filepath.Join(base, "file"), testContent)

	assertAccessFails(t, filepath.Join(base, "missing"), "movie.mkv", domain.ErrUnavailable)
	assertAccessFails(t, filepath.Join(base, "file"), "movie.mkv", domain.ErrUnavailable)
	assertAccessFails(t, "relative-root", "movie.mkv", domain.ErrInvalid)
}

func TestResolveAndOpenFollowSymlinksInsideTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "real/movie.mkv")
	symlinkOrSkip(t, filepath.Join("..", "real", "movie.mkv"), filepath.Join(root, "links", "alias.mkv"))
	symlinkOrSkip(t, filepath.Join("..", "real"), filepath.Join(root, "links", "films"))

	want := evalSymlinks(t, filepath.Join(root, "real", "movie.mkv"))

	for _, relativePath := range []string{"links/alias.mkv", "links/films/movie.mkv"} {
		for _, accessCase := range accessCases() {
			got, err := accessCase.access(fileSystem, root, relativePath)
			if err != nil {
				t.Errorf("%s(%q) error = %v", accessCase.name, relativePath, err)
				continue
			}

			if accessCase.name == "Resolve" && got != want {
				t.Errorf("Resolve(%q) = %q, want the link target %q", relativePath, got, want)
			}
		}
	}
}

func TestResolveAndOpenRefuseSymlinksThatLeaveTheRoot(t *testing.T) {
	t.Parallel()

	root, outside := newLibrary(t)
	writeFiles(t, root, "real/movie.mkv")

	symlinkOrSkip(t, filepath.Join("..", "outside", "secret.mkv"), filepath.Join(root, "relative.mkv"))
	symlinkOrSkip(t, filepath.Join(outside, "secret.mkv"), filepath.Join(root, "absolute.mkv"))
	symlinkOrSkip(t, filepath.Join("..", "outside"), filepath.Join(root, "out"))
	symlinkOrSkip(t, filepath.Join(root, "real", "movie.mkv"), filepath.Join(root, "absolute-in.mkv"))

	for _, relativePath := range []string{"relative.mkv", "absolute.mkv", "out/secret.mkv", "absolute-in.mkv"} {
		t.Run(relativePath, func(t *testing.T) {
			t.Parallel()

			assertAccessFails(t, root, relativePath, domain.ErrForbidden)
		})
	}
}

func TestResolveEvaluatesSymlinksOfTheRoot(t *testing.T) {
	t.Parallel()

	fileSystem, _ := newFileSystem(t)
	root, _ := newLibrary(t)
	writeFiles(t, root, "movie.mkv")

	linkedRoot := filepath.Join(filepath.Dir(root), "linked-library")
	symlinkOrSkip(t, root, linkedRoot)

	got, err := fileSystem.Resolve(linkedRoot, "movie.mkv")
	if err != nil {
		t.Fatalf("Resolve through a linked root error = %v", err)
	}

	if want := evalSymlinks(t, filepath.Join(root, "movie.mkv")); got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}
}

func evalSymlinks(t *testing.T, path string) string {
	t.Helper()

	evaluated, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", path, err)
	}

	return evaluated
}

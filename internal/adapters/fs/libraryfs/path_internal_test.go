package libraryfs

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	internalTestDirMode  = 0o750
	internalTestFileMode = 0o600
)

func TestClassifyRootError(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string
		err  error
		want error
	}{
		{name: "missing", err: fs.ErrNotExist, want: domain.ErrNotFound},
		{name: "file used as a directory", err: syscall.ENOTDIR, want: domain.ErrNotFound},
		{name: "permission", err: fs.ErrPermission, want: domain.ErrForbidden},
		{name: "escape or anything unknown", err: errors.New("path escapes from parent"), want: domain.ErrForbidden},
	}

	for _, transient := range transientErrors() {
		testCases = append(testCases, struct {
			name string
			err  error
			want error
		}{name: "transient " + transient.Error(), err: transient, want: domain.ErrUnavailable})
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			wrapped := &fs.PathError{Op: "statat", Path: "movie.mkv", Err: testCase.err}
			if got := classifyRootError(wrapped); !errors.Is(got, testCase.want) {
				t.Errorf("classifyRootError(%v) = %v, want %v", wrapped, got, testCase.want)
			}
		})
	}
}

func TestIsWithin(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		root       string
		target     string
		ignoreCase bool
		want       bool
	}{
		{name: "child", root: "/media", target: "/media/movie.mkv", want: true},
		{name: "nested child", root: "/media", target: "/media/a/b/movie.mkv", want: true},
		{name: "root with trailing separator", root: "/", target: "/movie.mkv", want: true},
		{name: "the root itself", root: "/media", target: "/media", want: false},
		{name: "sibling with the root as prefix", root: "/media", target: "/media-other/movie.mkv", want: false},
		{name: "parent", root: "/media/library", target: "/media/movie.mkv", want: false},
		{name: "other case, case-sensitive", root: "/media", target: "/MEDIA/movie.mkv", want: false},
		{name: "other case, case-insensitive", root: "/media", target: "/MEDIA/movie.mkv", ignoreCase: true, want: true},
		{name: "sibling, case-insensitive", root: "/media", target: "/MEDIA2/movie.mkv", ignoreCase: true, want: false},
		{name: "shorter target", root: "/media/library", target: "/media", ignoreCase: true, want: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got := isWithin(testCase.root, testCase.target, testCase.ignoreCase, '/')
			if got != testCase.want {
				t.Errorf("isWithin(%q, %q, %t) = %t, want %t",
					testCase.root, testCase.target, testCase.ignoreCase, got, testCase.want)
			}
		})
	}
}

func TestIsWithinWindowsPaths(t *testing.T) {
	t.Parallel()

	const separator = '\\'

	if !isWithin(`C:\Media`, `c:\media\Movie.mkv`, true, separator) {
		t.Error(`isWithin(C:\Media, c:\media\Movie.mkv) = false, want true`)
	}

	if !isWithin(`C:\`, `C:\Movie.mkv`, true, separator) {
		t.Error(`isWithin(C:\, C:\Movie.mkv) = false, want true`)
	}

	if isWithin(`C:\Media`, `C:\Media2\Movie.mkv`, true, separator) {
		t.Error(`isWithin(C:\Media, C:\Media2\Movie.mkv) = true, want false`)
	}
}

// Resolve only reaches evaluateInsideRoot after os.Root accepted the path, so its own
// containment check is tested directly: it must refuse a link that leaves the root by itself.
func TestEvaluateInsideRootRefusesALinkLeavingTheRoot(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	root := filepath.Join(base, "library")
	outside := filepath.Join(base, "outside.mkv")

	if err := os.Mkdir(root, internalTestDirMode); err != nil {
		t.Fatalf("create %s: %v", root, err)
	}

	if err := os.WriteFile(outside, []byte("secret"), internalTestFileMode); err != nil {
		t.Fatalf("write %s: %v", outside, err)
	}

	if got, err := evaluateInsideRoot(filepath.Join(base, "missing"), "escape.mkv"); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("evaluateInsideRoot with a missing root = (%q, %v), want ErrUnavailable", got, err)
	}

	if err := os.Symlink(filepath.Join("..", "outside.mkv"), filepath.Join(root, "escape.mkv")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlinks not permitted on this Windows account: %v", err)
		}

		t.Fatalf("symlink: %v", err)
	}

	if got, err := evaluateInsideRoot(root, "escape.mkv"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("evaluateInsideRoot = (%q, %v), want ErrForbidden", got, err)
	}
}

func TestCheckWindowsSegments(t *testing.T) {
	t.Parallel()

	for _, accepted := range []string{"movie.mkv", "a/b/movie.mkv", "Amélie (2001).mkv", ".x/y", "a. b/c"} {
		if err := checkWindowsSegments(accepted); err != nil {
			t.Errorf("checkWindowsSegments(%q) error = %v, want nil", accepted, err)
		}
	}

	for _, rejected := range []string{
		"movie.mkv.", "movie.mkv ", "dir./movie.mkv", "dir /movie.mkv", "movie.mkv:stream", "a/b:c/d", "movie.mkv::$DATA",
	} {
		if err := checkWindowsSegments(rejected); !errors.Is(err, errUnsafeOnWindows) {
			t.Errorf("checkWindowsSegments(%q) error = %v, want errUnsafeOnWindows", rejected, err)
		}
	}
}

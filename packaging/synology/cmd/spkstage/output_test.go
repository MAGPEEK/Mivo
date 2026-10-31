package main

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/packaging/synology/internal/spkmeta"
)

// outputFixture is the file system of one checkReplaceable case: inputs of a staging run in a
// temporary directory and a staged project next to them.
type outputFixture struct {
	root      string
	repo      string
	binary    string
	ffmpegDir string
	project   string
}

func newOutputFixture(t *testing.T) outputFixture {
	t.Helper()

	root := t.TempDir()
	fixture := outputFixture{
		root:      root,
		repo:      filepath.Join(root, "repo"),
		binary:    filepath.Join(root, "work", "bin", "server"),
		ffmpegDir: filepath.Join(root, "work", "ffmpeg"),
		project:   filepath.Join(root, "stage", "x86_64"),
	}

	writeFile(t, filepath.Join(fixture.repo, licensePath), []byte("GPL\n"), testFileMode)
	writeFile(t, filepath.Join(fixture.repo, filepath.FromSlash(packageTemplatesDir), "INFO.sh"), []byte("#!/bin/bash\n"), testFileMode)
	writeFile(t, fixture.binary, []byte("ELF"), testExecutableMode)
	writeFile(t, filepath.Join(fixture.ffmpegDir, ffmpegName), []byte("ELF"), testExecutableMode)
	writeFile(t, filepath.Join(fixture.project, projectMarkerPath), []byte(projectMarkerText), testFileMode)
	writeFile(t, filepath.Join(fixture.project, buildEnvPath), []byte("ID='x'\n"), testFileMode)

	return fixture
}

func (fixture outputFixture) inputs() []namedPath {
	return stageInputs(options{repo: fixture.repo, binary: fixture.binary, ffmpegDir: fixture.ffmpegDir})
}

func TestCheckReplaceable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// out returns the -out value; it may change the fixture first.
		out         func(t *testing.T, fixture outputFixture) string
		force       bool
		wantErr     error
		wantReplace bool
	}{
		{name: "new directory", out: func(_ *testing.T, f outputFixture) string { return filepath.Join(f.root, "new", "x86_64") }},
		{name: "staged project with -force", out: func(_ *testing.T, f outputFixture) string { return f.project }, force: true, wantReplace: true},
		{name: "staged project without -force", out: func(_ *testing.T, f outputFixture) string { return f.project }, wantErr: errProjectNotEmpty},
		{name: "empty directory", out: emptyDirectory, wantReplace: true},
		{name: "file system root", out: func(_ *testing.T, f outputFixture) string { return volumeRoot(f.root) }, force: true, wantErr: errUnsafeOutput},
		{name: "the repository", out: func(_ *testing.T, f outputFixture) string { return f.repo }, force: true, wantErr: errUnsafeOutput},
		{name: "a parent of the inputs", out: func(_ *testing.T, f outputFixture) string { return filepath.Join(f.root, "work") }, force: true, wantErr: errUnsafeOutput},
		{name: "the binary", out: func(_ *testing.T, f outputFixture) string { return f.binary }, force: true, wantErr: errUnsafeOutput},
		{name: "inside the FFmpeg directory", out: func(_ *testing.T, f outputFixture) string { return filepath.Join(f.ffmpegDir, "out") }, wantErr: errUnsafeOutput},
		{name: "new directory inside the templates", out: func(_ *testing.T, f outputFixture) string {
			return filepath.Join(f.repo, filepath.FromSlash(packageTemplatesDir), "a", "b")
		}, wantErr: errUnsafeOutput},
		{name: "input inside a staged project", out: func(t *testing.T, f outputFixture) string {
			directory := filepath.Dir(f.binary)
			writeFile(t, filepath.Join(directory, projectMarkerPath), []byte(projectMarkerText), testFileMode)
			writeFile(t, filepath.Join(directory, buildEnvPath), []byte("ID='x'\n"), testFileMode)

			return directory
		}, force: true, wantErr: errUnsafeOutput},
		{name: "directory without marker", out: func(t *testing.T, f outputFixture) string {
			removeFile(t, filepath.Join(f.project, projectMarkerPath))

			return f.project
		}, force: true, wantErr: errUnsafeOutput},
		{name: "marker without build.env", out: func(t *testing.T, f outputFixture) string {
			removeFile(t, filepath.Join(f.project, buildEnvPath))

			return f.project
		}, force: true, wantErr: errUnsafeOutput},
		{name: "a file", out: func(_ *testing.T, f outputFixture) string {
			return filepath.Join(f.project, buildEnvPath)
		}, force: true, wantErr: errUnexpectedInput},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newOutputFixture(t)
			out := test.out(t, fixture)

			target, err := checkReplaceable(out, fixture.inputs(), test.force)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("checkReplaceable(%s) error = %v, want %v", out, err, test.wantErr)
			}

			if err == nil && target.replace != test.wantReplace {
				t.Errorf("replace = %t, want %t", target.replace, test.wantReplace)
			}

			if err != nil && !strings.Contains(err.Error(), "-out") {
				t.Errorf("error %q does not name -out", err)
			}
		})
	}
}

func emptyDirectory(t *testing.T, fixture outputFixture) string {
	t.Helper()

	directory := filepath.Join(fixture.root, "empty")
	if err := os.Mkdir(directory, testDirectoryMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	return directory
}

func volumeRoot(path string) string {
	return filepath.VolumeName(path) + string(filepath.Separator)
}

func removeFile(t *testing.T, file string) {
	t.Helper()

	if err := os.Remove(file); err != nil {
		t.Fatalf("remove %s: %v", file, err)
	}
}

// TestCheckReplaceableFollowsLinks proves that the checks run on the resolved path: a link to
// the repository is the repository.
func TestCheckReplaceableFollowsLinks(t *testing.T) {
	t.Parallel()

	fixture := newOutputFixture(t)
	link := filepath.Join(fixture.root, "link")

	if err := os.Symlink(fixture.repo, link); err != nil {
		t.Skipf("cannot create a symbolic link here: %v", err)
	}

	for _, out := range []string{link, filepath.Join(link, "packaging")} {
		if _, err := checkReplaceable(out, fixture.inputs(), true); !errors.Is(err, errUnsafeOutput) {
			t.Errorf("checkReplaceable(%s) error = %v, want %v", out, err, errUnsafeOutput)
		}
	}
}

// TestFailedReplaceKeepsTheOldProject breaks the write after validation: the old project must
// survive unchanged and no temporary directory may stay behind.
func TestFailedReplaceKeepsTheOldProject(t *testing.T) {
	t.Parallel()

	fixture := newOutputFixture(t)

	target, err := checkReplaceable(fixture.project, fixture.inputs(), true)
	if err != nil {
		t.Fatalf("checkReplaceable: %v", err)
	}

	entries := []entry{
		{path: "a", mode: regularMode, content: []byte("a")},
		{path: "b", mode: regularMode, source: filepath.Join(fixture.root, "missing")},
	}

	if err := writeAtomically(target, entries, time.Unix(testEpoch, 0)); err == nil {
		t.Fatal("writeAtomically succeeded with a missing source")
	}

	if got := readStaged(t, fixture.project, buildEnvPath); got != "ID='x'\n" {
		t.Errorf("old project changed: build.env = %q", got)
	}

	assertNoTemporaryDirectories(t, filepath.Dir(fixture.project))
}

func assertNoTemporaryDirectories(t *testing.T, parent string) {
	t.Helper()

	items, err := os.ReadDir(parent)
	if err != nil {
		t.Fatalf("read %s: %v", parent, err)
	}

	for _, item := range items {
		if strings.HasPrefix(item.Name(), temporaryPrefix) {
			t.Errorf("temporary directory %s left in %s", item.Name(), parent)
		}
	}
}

func TestForceReplacesAStagedProject(t *testing.T) {
	t.Parallel()

	opts := testOptions(t, "x86_64", writeInputs(t, "x86_64", fakeSourceSelected))
	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
		t.Fatalf("first stage: %v", err)
	}

	leftover := filepath.Join(opts.out, "leftover")
	writeFile(t, leftover, []byte("x"), testFileMode)

	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); !errors.Is(err, errProjectNotEmpty) {
		t.Fatalf("second stage without -force: error = %v, want %v", err, errProjectNotEmpty)
	}

	opts.force = true
	if _, err := stage(opts, spkmeta.DefaultPackage(), io.Discard); err != nil {
		t.Fatalf("stage with -force: %v", err)
	}

	if _, err := os.Stat(leftover); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("-force kept %s", leftover)
	}

	assertNoTemporaryDirectories(t, filepath.Dir(opts.out))
}

package main

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// Test inputs: modes of the fake files and a fixed file time.
const (
	testExecutableMode fs.FileMode = 0o755
	testFileMode       fs.FileMode = 0o644
	testDirectoryMode  fs.FileMode = 0o755
	testEpoch                      = 1767225600 // 2026-01-01T00:00:00Z
	testVersion                    = "0.1.0-alpha.2"
)

// Licence files of the fake FFmpeg directory, as fetch.sh writes them for jellyfin builds.
const (
	fakeLicenseGPL     = "COPYING.GPLv3"
	fakeLicenseMD      = "LICENSE.md"
	fakeSourceSelected = "FFmpeg build bundled for the DSM architecture family\n\nStatus:               selected\n"
	fakeSourceInterim  = "FFmpeg build bundled for the DSM architecture family\n\nStatus:               interim\n"
)

// repoRoot is the repository root, four levels above this package.
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("locate repository root: %v", err)
	}

	return root
}

// fakeELF returns a minimal ELF header of the class and machine, enough for debug/elf.
func fakeELF(t *testing.T, class elf.Class, machine elf.Machine) []byte {
	t.Helper()

	return fakeELFFile(t, elfSpec{class: class, machine: machine, fileType: elf.ET_EXEC})
}

// elfSpec describes a fake ELF file: header fields and the types of its program headers.
type elfSpec struct {
	class    elf.Class
	machine  elf.Machine
	fileType elf.Type
	programs []elf.ProgType
}

// ELF header and program header sizes per class.
const (
	elf64HeaderSize  = 64
	elf64ProgramSize = 56
	elf32HeaderSize  = 52
	elf32ProgramSize = 32
)

// fakeELFFile encodes the header and the program headers (all empty) of spec.
func fakeELFFile(t *testing.T, spec elfSpec) []byte {
	t.Helper()

	var ident [elf.EI_NIDENT]byte

	copy(ident[:], elf.ELFMAG)
	ident[elf.EI_CLASS] = byte(spec.class)
	ident[elf.EI_DATA] = byte(elf.ELFDATA2LSB)
	ident[elf.EI_VERSION] = byte(elf.EV_CURRENT)

	parts := []any{}
	count := uint16(len(spec.programs)) //nolint:gosec // a test passes a handful of program headers

	if spec.class == elf.ELFCLASS64 {
		header := elf.Header64{
			Ident: ident, Type: uint16(spec.fileType), Machine: uint16(spec.machine),
			Version: uint32(elf.EV_CURRENT), Ehsize: elf64HeaderSize, Phentsize: elf64ProgramSize, Phnum: count,
		}
		if count > 0 {
			header.Phoff = elf64HeaderSize
		}

		parts = append(parts, header)
		for _, program := range spec.programs {
			parts = append(parts, elf.Prog64{Type: uint32(program)}) //nolint:gosec // ELF program types are 32-bit
		}
	} else {
		header := elf.Header32{
			Ident: ident, Type: uint16(spec.fileType), Machine: uint16(spec.machine),
			Version: uint32(elf.EV_CURRENT), Ehsize: elf32HeaderSize, Phentsize: elf32ProgramSize, Phnum: count,
		}
		if count > 0 {
			header.Phoff = elf32HeaderSize
		}

		parts = append(parts, header)
		for _, program := range spec.programs {
			parts = append(parts, elf.Prog32{Type: uint32(program)}) //nolint:gosec // ELF program types are 32-bit
		}
	}

	var buffer bytes.Buffer

	for _, part := range parts {
		if err := binary.Write(&buffer, binary.LittleEndian, part); err != nil {
			t.Fatalf("encode ELF: %v", err)
		}
	}

	return buffer.Bytes()
}

func identityOf(t *testing.T, family string) elfIdentity {
	t.Helper()

	identity, err := familyELF(family)
	if err != nil {
		t.Fatalf("familyELF(%q): %v", family, err)
	}

	return identity
}

func writeFile(t *testing.T, file string, content []byte, mode fs.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(file), testDirectoryMode); err != nil {
		t.Fatalf("create parent of %s: %v", file, err)
	}

	if err := os.WriteFile(file, content, mode); err != nil { //nolint:gosec // paths built by the tests in temporary directories
		t.Fatalf("write %s: %v", file, err)
	}

	if err := os.Chmod(file, mode); err != nil {
		t.Fatalf("chmod %s: %v", file, err)
	}
}

// testInputs are the fake binary and FFmpeg directory of one family.
type testInputs struct {
	binary    string
	ffmpegDir string
}

// writeInputs creates a fake server binary and a fake fetch.sh directory for the family.
func writeInputs(t *testing.T, family, sourceNotice string) testInputs {
	t.Helper()

	directory := t.TempDir()
	identity := identityOf(t, family)
	executable := fakeELF(t, identity.class, identity.machine)
	inputs := testInputs{binary: filepath.Join(directory, "server"), ffmpegDir: filepath.Join(directory, "ffmpeg")}

	writeFile(t, inputs.binary, executable, testExecutableMode)
	writeFile(t, filepath.Join(inputs.ffmpegDir, ffmpegName), executable, testExecutableMode)
	writeFile(t, filepath.Join(inputs.ffmpegDir, ffprobeName), executable, testExecutableMode)
	writeFile(t, filepath.Join(inputs.ffmpegDir, fakeLicenseGPL), []byte("GNU GPL v3\n"), testFileMode)
	writeFile(t, filepath.Join(inputs.ffmpegDir, fakeLicenseMD), []byte("FFmpeg licence\n"), testFileMode)
	writeFile(t, filepath.Join(inputs.ffmpegDir, sourceNoticeName), []byte(sourceNotice), testFileMode)
	writeFile(t, filepath.Join(inputs.ffmpegDir, ".fetch-manifest"), []byte("ffmpeg\nffprobe\n"), testFileMode)

	return inputs
}

// testOptions are valid options for the family that write into a new temporary directory.
func testOptions(t *testing.T, family string, inputs testInputs) options {
	t.Helper()

	return options{
		family:    family,
		version:   testVersion,
		binary:    inputs.binary,
		ffmpegDir: inputs.ffmpegDir,
		out:       filepath.Join(t.TempDir(), "project"),
		repo:      repoRoot(t),
		modTime:   time.Unix(testEpoch, 0).UTC(),
	}
}

// copyRepository copies LICENSE and the package templates into a new directory, so a test can
// break a template without touching the repository.
func copyRepository(t *testing.T) string {
	t.Helper()

	source := repoRoot(t)
	target := t.TempDir()
	templates := filepath.Join(source, filepath.FromSlash(packageTemplatesDir))

	copyFile(t, filepath.Join(source, licensePath), filepath.Join(target, licensePath))

	err := filepath.WalkDir(templates, func(current string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil || item.IsDir() {
			return walkErr
		}

		relative, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}

		copyFile(t, current, filepath.Join(target, relative))

		return nil
	})
	if err != nil {
		t.Fatalf("copy templates: %v", err)
	}

	return target
}

func copyFile(t *testing.T, source, target string) {
	t.Helper()

	content, err := os.ReadFile(source) //nolint:gosec // test copies the repository's own files
	if err != nil {
		t.Fatalf("read %s: %v", source, err)
	}

	writeFile(t, target, content, testFileMode)
}

// treeEntry is one path of a staged tree with its mode; directories end with "/".
type treeEntry struct {
	path string
	mode fs.FileMode
}

// diskTree lists the staged directory. Windows file systems have no Unix modes, so modes are
// zero there.
func diskTree(t *testing.T, root string) []treeEntry {
	t.Helper()

	var tree []treeEntry

	err := filepath.WalkDir(root, func(current string, item fs.DirEntry, walkErr error) error {
		if walkErr != nil || current == root {
			return walkErr
		}

		relative, err := filepath.Rel(root, current)
		if err != nil {
			return err
		}

		info, err := item.Info()
		if err != nil {
			return err
		}

		name := filepath.ToSlash(relative)
		if item.IsDir() {
			name += "/"
		}

		var mode fs.FileMode
		if runtime.GOOS != "windows" {
			mode = info.Mode().Perm()
		}

		tree = append(tree, treeEntry{path: name, mode: mode})

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}

	return tree
}

// plannedTree lists the planned files plus their directories, sorted like diskTree.
func plannedTree(entries []entry) []treeEntry {
	seen := map[string]bool{}

	var tree []treeEntry

	for _, item := range entries {
		tree = append(tree, treeEntry{path: item.path, mode: item.mode})

		for directory := filepath.ToSlash(filepath.Dir(item.path)); directory != "."; directory = filepath.ToSlash(filepath.Dir(directory)) {
			if !seen[directory] {
				seen[directory] = true
				tree = append(tree, treeEntry{path: directory + "/", mode: directoryMode})
			}
		}
	}

	sortTree(tree)

	return tree
}

// sortTree orders like filepath.WalkDir: a directory's entries follow it.
func sortTree(tree []treeEntry) {
	sort.Slice(tree, func(left, right int) bool {
		return strings.TrimSuffix(tree[left].path, "/") < strings.TrimSuffix(tree[right].path, "/")
	})
}

func readStaged(t *testing.T, root, name string) string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name))) //nolint:gosec // staged file of the test
	if err != nil {
		t.Fatalf("read staged %s: %v", name, err)
	}

	return string(content)
}

// snapshot captures path, mode, time and content of every staged file for comparing two runs.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()

	result := map[string]string{}

	for _, item := range diskTree(t, root) {
		file := filepath.Join(root, filepath.FromSlash(item.path))

		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("stat %s: %v", file, err)
		}

		description := item.mode.String() + " " + info.ModTime().UTC().Format(time.RFC3339)

		if !info.IsDir() {
			handle, err := os.Open(file) //nolint:gosec // staged file of the test
			if err != nil {
				t.Fatalf("open %s: %v", file, err)
			}

			content, err := io.ReadAll(handle)
			_ = handle.Close()

			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}

			description += " " + string(content)
		}

		result[item.path] = description
	}

	return result
}

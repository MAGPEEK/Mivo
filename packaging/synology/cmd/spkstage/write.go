package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// The marker that identifies a directory written by spkstage. -force replaces only directories
// that carry it next to build.env, so a mistyped -out cannot delete anything else.
const (
	projectMarkerPath = ".spkstage"
	projectMarkerText = "Toolkit project staged by spkstage; spkstage -force may replace this directory.\n"
)

// temporaryPrefix names the sibling directory a run writes into before it renames it to -out.
const temporaryPrefix = ".spkstage-"

var (
	// errProjectNotEmpty reports an existing project directory with content, without -force.
	errProjectNotEmpty = errors.New("project directory exists and is not empty (pass -force to replace it)")
	// errUnsafeOutput reports a project directory that spkstage must not write or replace.
	errUnsafeOutput = errors.New("refusing to use this directory")
)

// namedPath is an input of the run with the flag that names it.
type namedPath struct {
	label string
	path  string
}

// stageInputs lists what the run reads; -out may neither be, contain nor lie inside one of them.
func stageInputs(opts options) []namedPath {
	return []namedPath{
		{label: "-repo", path: filepath.Join(opts.repo, filepath.FromSlash(packageTemplatesDir))},
		{label: "-repo", path: filepath.Join(opts.repo, licensePath)},
		{label: "-binary", path: opts.binary},
		{label: "-ffmpeg-dir", path: opts.ffmpegDir},
	}
}

// outputTarget is the resolved project directory and whether an existing one is replaced.
type outputTarget struct {
	path    string
	replace bool
}

// checkReplaceable resolves -out (following symbolic links) and decides whether the run may
// write it: never a file system root, never a path that is, contains or lies inside an input,
// and an existing non-empty directory only with force and only if it is a staged project.
func checkReplaceable(out string, inputs []namedPath, force bool) (outputTarget, error) {
	target, err := resolvePath(out)
	if err != nil {
		return outputTarget{}, fmt.Errorf("-out %s: %w", out, err)
	}

	if err := checkLocation(out, target, inputs); err != nil {
		return outputTarget{}, err
	}

	replace, err := checkExisting(out, target, force)
	if err != nil {
		return outputTarget{}, err
	}

	return outputTarget{path: target, replace: replace}, nil
}

func checkLocation(out, target string, inputs []namedPath) error {
	if filepath.Dir(target) == target {
		return fmt.Errorf("-out %s is a file system root: %w", out, errUnsafeOutput)
	}

	for _, input := range inputs {
		resolved, err := resolvePath(input.path)
		if err != nil {
			return fmt.Errorf("%s %s: %w", input.label, input.path, err)
		}

		if within(target, resolved) || within(resolved, target) {
			return fmt.Errorf("-out %s overlaps %s %s: %w", out, input.label, input.path, errUnsafeOutput)
		}
	}

	return nil
}

// checkExisting reports whether target exists and must be replaced.
func checkExisting(out, target string, force bool) (bool, error) {
	info, err := os.Stat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("-out %s: %w", out, err)
	}

	if !info.IsDir() {
		return false, fmt.Errorf("-out %s is not a directory: %w", out, errUnexpectedInput)
	}

	items, err := os.ReadDir(target)

	switch {
	case err != nil:
		return false, fmt.Errorf("-out %s: %w", out, err)
	case len(items) == 0:
		return true, nil
	case !force:
		return false, fmt.Errorf("-out %s: %w", out, errProjectNotEmpty)
	case !isStagedProject(target):
		return false, fmt.Errorf("-out %s holds no %s and %s, so it is not a staged project: %w",
			out, projectMarkerPath, buildEnvPath, errUnsafeOutput)
	}

	return true, nil
}

func isStagedProject(directory string) bool {
	for _, name := range []string{projectMarkerPath, buildEnvPath} {
		info, err := os.Lstat(filepath.Join(directory, name))
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}

	return true
}

// resolvePath returns the absolute path with every symbolic link resolved. For a path that does
// not exist yet, the longest existing ancestor is resolved and the rest appended.
func resolvePath(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", fmt.Errorf("absolute path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil {
		return resolved, nil
	}

	parent := filepath.Dir(absolute)
	if !errors.Is(err, fs.ErrNotExist) || parent == absolute {
		return "", fmt.Errorf("resolve links: %w", err)
	}

	resolvedParent, err := resolvePath(parent)
	if err != nil {
		return "", err
	}

	return filepath.Join(resolvedParent, filepath.Base(absolute)), nil
}

// within reports whether child is parent or lies below it. Windows and macOS file systems
// ignore case, so the comparison does as well there.
func within(child, parent string) bool {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		child, parent = strings.ToLower(child), strings.ToLower(parent)
	}

	relative, err := filepath.Rel(parent, child)

	return err == nil && !filepath.IsAbs(relative) && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// writeAtomically writes the project into a temporary sibling of the target and renames it
// into place, so a failed run leaves the old project (or nothing) behind, never a partial tree.
func writeAtomically(target outputTarget, entries []entry, modTime time.Time) error {
	parent := filepath.Dir(target.path)
	if err := os.MkdirAll(parent, directoryMode); err != nil {
		return fmt.Errorf("-out: create %s: %w", parent, err)
	}

	temporary, err := os.MkdirTemp(parent, temporaryPrefix+"*")
	if err != nil {
		return fmt.Errorf("-out: create a temporary directory in %s: %w", parent, err)
	}

	if err := fillProject(temporary, target, entries, modTime); err != nil {
		return errors.Join(err, removeTemporary(temporary))
	}

	return nil
}

func fillProject(temporary string, target outputTarget, entries []entry, modTime time.Time) error {
	if err := setDirectoryMode(temporary); err != nil {
		return err
	}

	if err := writeProject(temporary, entries, modTime); err != nil {
		return err
	}

	if target.replace {
		if err := os.RemoveAll(target.path); err != nil {
			return fmt.Errorf("-out: remove the old project %s: %w", target.path, err)
		}
	}

	if err := os.Rename(temporary, target.path); err != nil {
		return fmt.Errorf("-out: move the project to %s: %w", target.path, err)
	}

	return nil
}

func removeTemporary(temporary string) error {
	if err := os.RemoveAll(temporary); err != nil {
		return fmt.Errorf("remove %s: %w", temporary, err)
	}

	return nil
}

// setDirectoryMode applies directoryMode regardless of the umask.
func setDirectoryMode(directory string) error {
	if err := os.Chmod(directory, directoryMode); err != nil {
		return fmt.Errorf("set mode of %s: %w", directory, err)
	}

	return nil
}

// writeProject writes the entries in path order, then sets one modification time on every
// file and directory, the directories last so writing their children cannot change it.
func writeProject(out string, entries []entry, modTime time.Time) error {
	sorted := slices.Clone(entries)
	slices.SortFunc(sorted, func(left, right entry) int { return strings.Compare(left.path, right.path) })

	directories := map[string]bool{".": true}

	for _, item := range sorted {
		if err := ensureParents(out, item.path, directories); err != nil {
			return err
		}

		if err := writeEntry(out, item); err != nil {
			return err
		}

		if err := setTime(filepath.Join(out, filepath.FromSlash(item.path)), modTime); err != nil {
			return err
		}
	}

	ordered := make([]string, 0, len(directories))
	for directory := range directories {
		ordered = append(ordered, directory)
	}

	// Deepest first, so a parent's time is set after its children's.
	slices.SortFunc(ordered, func(left, right string) int {
		return strings.Compare(right, left)
	})

	for _, directory := range ordered {
		if err := setTime(filepath.Join(out, filepath.FromSlash(directory)), modTime); err != nil {
			return err
		}
	}

	return nil
}

// ensureParents creates the missing parent directories of a project path with directoryMode.
func ensureParents(out, file string, directories map[string]bool) error {
	var missing []string

	for directory := path.Dir(file); !directories[directory]; directory = path.Dir(directory) {
		missing = append(missing, directory)
	}

	for _, directory := range slices.Backward(missing) {
		if err := os.Mkdir(filepath.Join(out, filepath.FromSlash(directory)), directoryMode); err != nil {
			return fmt.Errorf("create %s: %w", directory, err)
		}

		if err := setDirectoryMode(filepath.Join(out, filepath.FromSlash(directory))); err != nil {
			return err
		}

		directories[directory] = true
	}

	return nil
}

// writeEntry creates one file with its mode; the umask does not apply because the mode is
// set again explicitly.
func writeEntry(out string, item entry) error {
	target := filepath.Join(out, filepath.FromSlash(item.path))

	file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, item.mode) //nolint:gosec // the project path is built from fixed names
	if err != nil {
		return fmt.Errorf("create %s: %w", item.path, err)
	}

	writeErr := fillFile(file, item)
	closeErr := file.Close()

	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("write %s: %w", item.path, err)
	}

	if err := os.Chmod(target, item.mode); err != nil {
		return fmt.Errorf("set mode of %s: %w", item.path, err)
	}

	return nil
}

func fillFile(file *os.File, item entry) error {
	if item.content != nil {
		_, err := file.Write(item.content)

		return err
	}

	source, err := os.Open(item.source)
	if err != nil {
		return fmt.Errorf("open %s: %w", item.source, err)
	}

	_, copyErr := io.Copy(file, source)

	return errors.Join(copyErr, source.Close())
}

func setTime(target string, modTime time.Time) error {
	if err := os.Chtimes(target, modTime, modTime); err != nil {
		return fmt.Errorf("set time of %s: %w", target, err)
	}

	return nil
}

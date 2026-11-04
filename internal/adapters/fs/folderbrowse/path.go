package folderbrowse

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	isWindows       = runtime.GOOS == "windows"
	relativeDivider = "/"
	rootDirectory   = "."
)

// nativePath validates a root-relative path ("" is the root itself) and returns it with
// native separators, ready for os.Root. Rules follow libraryfs.nativeRelativePath.
func nativePath(relativePath string) (string, error) {
	if relativePath == "" {
		return rootDirectory, nil
	}

	if _, err := library.ParseRelativePath(relativePath); err != nil {
		return "", err
	}

	native := filepath.FromSlash(relativePath)
	if !filepath.IsLocal(native) {
		return "", fmt.Errorf("path %q is not local: %w", relativePath, domain.ErrInvalid)
	}

	for segment := range strings.SplitSeq(relativePath, relativeDivider) {
		if library.IsHiddenFolderName(segment) {
			return "", fmt.Errorf("segment %q is a hidden or system folder: %w", segment, domain.ErrInvalid)
		}

		if isWindows && (strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") ||
			strings.Contains(segment, ":") || isDeviceName(segment)) {
			return "", fmt.Errorf("segment %q is unsafe on Windows: %w", segment, domain.ErrInvalid)
		}
	}

	return native, nil
}

// isDeviceName reports a Windows device name ("CON", "COM1", "CON.txt"): Win32 opens the
// device for it, whatever follows the first dot.
func isDeviceName(segment string) bool {
	base, _, _ := strings.Cut(segment, ".")

	switch upper := strings.ToUpper(strings.TrimRight(base, " ")); upper {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	default:
		return len(upper) == len("COM1") && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) &&
			upper[3] >= '1' && upper[3] <= '9'
	}
}

// hiddenSegment reports, on Windows, whether a segment of the path carries the hidden or
// system attribute. Unreadable segments are left to the later open, which reports them.
func hiddenSegment(opened *os.Root, native string) bool {
	if !isWindows || native == rootDirectory {
		return false
	}

	prefix := ""

	for segment := range strings.SplitSeq(native, string(filepath.Separator)) {
		prefix = filepath.Join(prefix, segment)

		info, err := opened.Lstat(prefix)
		if err != nil {
			return false
		}

		if hiddenInfo(info) {
			return true
		}
	}

	return false
}

// isReservedResolved is isReserved plus a check of the link-resolved path (a link into a
// reserved directory, Windows 8.3 short names). Reserved directories are resolved too, so a
// root below a symlinked prefix still matches.
func (b *Browser) isReservedResolved(absolute string) bool {
	if b.isReserved(absolute) {
		return true
	}

	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return false
	}

	for _, reserved := range b.reserved {
		if sameOrInside(reserved, resolved) {
			return true
		}

		if resolvedReserved, evalErr := filepath.EvalSymlinks(reserved); evalErr == nil &&
			sameOrInside(resolvedReserved, resolved) {
			return true
		}
	}

	return false
}

func joinRelative(directory, name string) string {
	if directory == "" {
		return name
	}

	return directory + relativeDivider + name
}

// isReserved reports whether absolute is a reserved directory or lies inside one.
func (b *Browser) isReserved(absolute string) bool {
	for _, reserved := range b.reserved {
		if sameOrInside(reserved, absolute) {
			return true
		}
	}

	return false
}

func sameOrInside(directory, target string) bool {
	if isWindows {
		directory, target = strings.ToLower(directory), strings.ToLower(target)
	}

	if directory == target {
		return true
	}

	prefix := directory
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}

	return strings.HasPrefix(target, prefix)
}

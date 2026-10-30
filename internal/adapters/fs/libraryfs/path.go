package libraryfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const (
	// isWindows enables the Windows-only path rules; it is a constant, so the rules stay
	// testable on every OS through checkWindowsSegments.
	isWindows = runtime.GOOS == "windows"
	// relativeSeparator separates the segments of a catalog path on every OS.
	relativeSeparator = "/"
	// rootDirectory names the library root itself inside its os.Root.
	rootDirectory = "."
	// windowsStreamSeparator starts an alternate data stream name ("movie.mkv:stream").
	windowsStreamSeparator = ":"
)

// errUnsafeOnWindows reports a segment that Win32 would rewrite or treat as a stream.
var errUnsafeOnWindows = errors.New("segment is not safe on Windows")

// nativeRelativePath checks a catalog path against the domain rule and the rules of the host
// OS and returns it with native separators.
func nativeRelativePath(relativePath string) (string, error) {
	if _, err := library.ParseRelativePath(relativePath); err != nil {
		return "", err
	}

	native := filepath.FromSlash(relativePath)
	if !filepath.IsLocal(native) {
		return "", fmt.Errorf("path %q is not a local path on %s", relativePath, runtime.GOOS)
	}

	if isWindows {
		if err := checkWindowsSegments(relativePath); err != nil {
			return "", err
		}
	}

	return native, nil
}

// checkWindowsSegments rejects segments that end in a dot or space, which Win32 strips so that
// "movie.mkv." opens "movie.mkv", and segments with a colon, which name an alternate data
// stream. filepath.IsLocal already refuses colons; the check is repeated so the rule does not
// depend on that detail.
func checkWindowsSegments(relativePath string) error {
	for segment := range strings.SplitSeq(relativePath, relativeSeparator) {
		if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
			return fmt.Errorf("%w: %q ends in a dot or space", errUnsafeOnWindows, segment)
		}

		if strings.Contains(segment, windowsStreamSeparator) {
			return fmt.Errorf("%w: %q names an alternate data stream", errUnsafeOnWindows, segment)
		}
	}

	return nil
}

// joinRelative appends an entry name to the catalog path of its directory.
func joinRelative(directory, name string) string {
	if directory == rootDirectory {
		return name
	}

	return directory + relativeSeparator + name
}

// isWithin reports whether target lies strictly below root. Both must be clean absolute paths
// with symlinks evaluated. Windows compares case-insensitively because NTFS does; a case
// variant that changes the byte length fails the comparison, which errs on the safe side.
func isWithin(root, target string, ignoreCase bool, separator byte) bool {
	prefix := root
	if !strings.HasSuffix(prefix, string(separator)) {
		prefix += string(separator)
	}

	if len(target) <= len(prefix) {
		return false
	}

	head := target[:len(prefix)]
	if ignoreCase {
		return strings.EqualFold(head, prefix)
	}

	return head == prefix
}

// openLibraryRoot opens an absolute library root. A root that is missing, not a directory or
// not readable is reported as domain.ErrUnavailable: NAS shares may mount after the server
// starts (ADR 0022).
func openLibraryRoot(root string) (*os.Root, error) {
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("library root %q: must be an absolute path: %w", root, domain.ErrInvalid)
	}

	opened, err := os.OpenRoot(root)
	if err != nil {
		return nil, fmt.Errorf("open library root %s: %w: %w", root, domain.ErrUnavailable, err)
	}

	return opened, nil
}

func closeRoot(root *os.Root) {
	_ = root.Close()
}

func closeFile(file *os.File) {
	_ = file.Close()
}

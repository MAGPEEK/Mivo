package library

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
)

// MaxFolderPathLength is the longest library folder path in characters (API
// listLibraryFolders path).
const MaxFolderPathLength = 4096

// segmentStreamSeparator names an alternate data stream on Windows ("folder:stream"); it has
// no place in a folder name the server lists.
const segmentStreamSeparator = ":"

// FolderPath is a folder of a library as the folder view names it: the ID of a library folder
// followed by the names of the directories below it. The zero value is the top, which lists
// the library folders themselves.
type FolderPath struct {
	FolderID string
	Segments []string
}

// ParseFolderPath reads a library-relative folder path: empty for the top, else a folder ID
// and directory names separated by "/". It refuses everything that could name another place
// than a plain directory below the folder: more than MaxFolderPathLength characters, invalid
// UTF-8, control characters (NUL included), a backslash, a leading "/", an empty, "." or ".."
// segment, a ":" in a segment and Windows reserved device names (CON, NUL, COM1, …). The error
// wraps domain.ErrInvalid.
func ParseFolderPath(text string) (FolderPath, error) {
	if text == "" {
		return FolderPath{}, nil
	}

	if err := checkFolderPathText(text); err != nil {
		return FolderPath{}, fmt.Errorf("parse folder path: %w", err)
	}

	segments := strings.Split(text, pathSeparator)
	for _, segment := range segments {
		if err := checkFolderSegment(segment); err != nil {
			return FolderPath{}, fmt.Errorf("parse folder path: %w", err)
		}
	}

	return FolderPath{FolderID: segments[0], Segments: segments[1:]}, nil
}

func checkFolderPathText(text string) error {
	switch {
	case utf8.RuneCountInString(text) > MaxFolderPathLength:
		return fmt.Errorf("path longer than %d characters: %w", MaxFolderPathLength, domain.ErrInvalid)
	case !utf8.ValidString(text):
		return fmt.Errorf("path is not valid UTF-8: %w", domain.ErrInvalid)
	case strings.ContainsFunc(text, unicode.IsControl):
		return fmt.Errorf("path contains a control character: %w", domain.ErrInvalid)
	case strings.ContainsRune(text, backslashCharacter):
		return fmt.Errorf("path contains a backslash: %w", domain.ErrInvalid)
	case strings.HasPrefix(text, pathSeparator):
		return fmt.Errorf("path is absolute: %w", domain.ErrInvalid)
	default:
		return nil
	}
}

func checkFolderSegment(segment string) error {
	if err := checkSegment(segment); err != nil {
		return err
	}

	if strings.Contains(segment, segmentStreamSeparator) {
		return fmt.Errorf("path segment %q contains a colon: %w", segment, domain.ErrInvalid)
	}

	if isWindowsDeviceName(segment) {
		return fmt.Errorf("path segment %q is a reserved device name: %w", segment, domain.ErrInvalid)
	}

	return nil
}

// isWindowsDeviceName reports names Win32 maps to a device whatever the folder: CON, PRN,
// AUX, NUL, COM1–9 and LPT1–9 (also with superscript digits), in any case, with any
// extension and with trailing spaces before it ("nul .txt").
func isWindowsDeviceName(segment string) bool {
	stem, _, _ := strings.Cut(segment, ".")
	stem = strings.ToUpper(strings.TrimRight(stem, " "))

	if slices.Contains([]string{"CON", "PRN", "AUX", "NUL"}, stem) {
		return true
	}

	prefix, digit, found := cutDevicePrefix(stem)

	return found && (prefix == "COM" || prefix == "LPT") && strings.ContainsRune("123456789¹²³", digit)
}

// cutDevicePrefix splits "COM1" into "COM" and '1'; it reports false for any other shape.
func cutDevicePrefix(stem string) (string, rune, bool) {
	const devicePrefixLength = 3

	if len(stem) <= devicePrefixLength {
		return "", 0, false
	}

	digit, size := utf8.DecodeRuneInString(stem[devicePrefixLength:])
	if devicePrefixLength+size != len(stem) {
		return "", 0, false
	}

	return stem[:devicePrefixLength], digit, true
}

// IsTop reports whether the path is the top of the library.
func (path FolderPath) IsTop() bool {
	return path.FolderID == ""
}

// Relative returns the directory below the library folder, segments joined by "/"; empty
// for the library folder itself and for the top.
func (path FolderPath) Relative() string {
	return strings.Join(path.Segments, pathSeparator)
}

// String returns the path as the API carries it; empty for the top.
func (path FolderPath) String() string {
	if path.IsTop() {
		return ""
	}

	return strings.Join(append([]string{path.FolderID}, path.Segments...), pathSeparator)
}

// Parent returns the folder above; it reports false at the top. The parent of a library
// folder is the top.
func (path FolderPath) Parent() (FolderPath, bool) {
	if path.IsTop() {
		return FolderPath{}, false
	}

	if len(path.Segments) == 0 {
		return FolderPath{}, true
	}

	return FolderPath{FolderID: path.FolderID, Segments: slices.Clone(path.Segments[:len(path.Segments)-1])}, true
}

// Child returns the path of a directory or file name directly below this folder. Below the
// top the name is a library folder ID.
func (path FolderPath) Child(name string) FolderPath {
	if path.IsTop() {
		return FolderPath{FolderID: name}
	}

	return FolderPath{FolderID: path.FolderID, Segments: append(slices.Clone(path.Segments), name)}
}

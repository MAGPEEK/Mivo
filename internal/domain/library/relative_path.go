package library

import (
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Characters and segments with a special meaning in file paths.
const (
	pathSeparator          = "/"
	backslashCharacter     = '\\'
	nulCharacter           = '\x00'
	driveSeparator         = ':'
	driveLetterPrefixBytes = 2
	currentDirectory       = "."
	parentDirectory        = ".."
)

// ParseRelativePath validates a path relative to a library root as it is stored in the
// catalog: segments separated by "/", no empty, "." or ".." segments, no backslash, no NUL
// byte and nothing absolute ("/x", "C:/x", "C:x", UNC). The canonical form is the only
// accepted form, so one file never has two stored paths; the input is returned unchanged.
// The error wraps domain.ErrInvalid.
func ParseRelativePath(text string) (string, error) {
	if err := checkRelativePath(text); err != nil {
		return "", fmt.Errorf("parse relative path %q: %w", text, err)
	}

	return text, nil
}

func checkRelativePath(text string) error {
	switch {
	case text == "":
		return fmt.Errorf("path is empty: %w", domain.ErrInvalid)
	case strings.ContainsRune(text, nulCharacter):
		return fmt.Errorf("path contains a NUL byte: %w", domain.ErrInvalid)
	case strings.ContainsRune(text, backslashCharacter):
		return fmt.Errorf("path contains a backslash: %w", domain.ErrInvalid)
	case strings.HasPrefix(text, pathSeparator), hasDriveLetter(text):
		return fmt.Errorf("path is absolute: %w", domain.ErrInvalid)
	}

	for segment := range strings.SplitSeq(text, pathSeparator) {
		if err := checkSegment(segment); err != nil {
			return err
		}
	}

	return nil
}

func checkSegment(segment string) error {
	switch segment {
	case "":
		return fmt.Errorf("path has an empty segment: %w", domain.ErrInvalid)
	case currentDirectory, parentDirectory:
		return fmt.Errorf("path has a %q segment: %w", segment, domain.ErrInvalid)
	default:
		return nil
	}
}

// hasDriveLetter reports a Windows drive prefix ("C:"), which makes a path absolute or
// drive-relative. A Linux file named "a:b.mkv" is rejected too; that rare name is the price
// for never handing a drive path to a Windows host.
func hasDriveLetter(text string) bool {
	return len(text) >= driveLetterPrefixBytes &&
		isASCIILetter(text[0]) && text[1] == driveSeparator
}

func isASCIILetter(character byte) bool {
	return ('a' <= character && character <= 'z') || ('A' <= character && character <= 'Z')
}

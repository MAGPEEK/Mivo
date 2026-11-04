package library

import (
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// FolderReason says why a folder is rejected.
type FolderReason string

// Folder rejection reasons; the HTTP adapter maps each to a problem code.
const (
	ReasonDuplicate FolderReason = "duplicate" // same folder twice in the library
	ReasonNested    FolderReason = "nested"    // one folder of the library inside another
	ReasonOverlap   FolderReason = "overlap"   // equal, inside or containing another library's folder
	ReasonReserved  FolderReason = "reserved"  // equal, inside or containing a server directory
	ReasonPath      FolderReason = "path"      // blank, NUL, not absolute or containing "." / ".." elements
)

// isCleanAbsolute accepts a leading "/" or "\" (also UNC), or a drive letter "X:"
// followed by a separator or nothing, and rejects blank/NUL paths and "." / ".." elements
// (split on both separators, stricter than needed on POSIX, so traversal cannot slip by).
func isCleanAbsolute(path string) bool {
	if validateRoot(path) != nil {
		return false
	}

	absolute := strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`)
	if !absolute && len(path) >= 2 && path[1] == ':' && isLetters(path[:1], 1, 1) {
		absolute = len(path) == 2 || path[2] == '/' || path[2] == '\\'
	}

	if !absolute {
		return false
	}

	for _, element := range strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' }) {
		if element == "." || element == ".." {
			return false
		}
	}

	return true
}

// FolderError reports a rejected folder. It wraps domain.ErrInvalid.
type FolderError struct {
	Index          int
	Reason         FolderReason
	OtherLibraryID string // set for ReasonOverlap
}

// Error implements error.
func (e *FolderError) Error() string {
	if e.OtherLibraryID != "" {
		return fmt.Sprintf("folder %d: %s (library %s)", e.Index, e.Reason, e.OtherLibraryID)
	}

	return fmt.Sprintf("folder %d: %s", e.Index, e.Reason)
}

// Unwrap lets errors.Is match domain.ErrInvalid.
func (e *FolderError) Unwrap() error { return domain.ErrInvalid }

// FolderRules is the context the folder checks need. Paths are cleaned and absolute.
// Separator rule: "/" always separates; "\" separates only when CaseInsensitive is set
// (Windows mode), otherwise it is an ordinary name character.
type FolderRules struct {
	CaseInsensitive bool
	Reserved        []string // config, data, cache and transcode directories
	OtherLibraries  []ManagedLibrary
}

// Check applies the folder rules to the candidate. Other libraries with the candidate's
// own ID are skipped, so an update does not collide with itself.
func (rules FolderRules) Check(candidate ManagedLibrary) error {
	for index, folder := range candidate.Folders {
		if !isCleanAbsolute(folder.Path) {
			return &FolderError{Index: index, Reason: ReasonPath}
		}

		if err := rules.checkWithin(candidate.Folders, index); err != nil {
			return err
		}

		if err := rules.checkOthers(candidate.ID, folder, index); err != nil {
			return err
		}
	}

	return nil
}

func (rules FolderRules) checkWithin(folders []Folder, index int) error {
	current := pathElements(folders[index].Path, rules.CaseInsensitive)

	for earlier := range index {
		other := pathElements(folders[earlier].Path, rules.CaseInsensitive)

		switch {
		case sameElements(current, other):
			return &FolderError{Index: index, Reason: ReasonDuplicate}
		case related(current, other):
			return &FolderError{Index: index, Reason: ReasonNested}
		}
	}

	return nil
}

func (rules FolderRules) checkOthers(ownID string, folder Folder, index int) error {
	current := pathElements(folder.Path, rules.CaseInsensitive)

	for _, reserved := range rules.Reserved {
		if related(current, pathElements(reserved, rules.CaseInsensitive)) {
			return &FolderError{Index: index, Reason: ReasonReserved}
		}
	}

	for _, other := range rules.OtherLibraries {
		if ownID != "" && other.ID == ownID {
			continue
		}

		for _, otherFolder := range other.Folders {
			if related(current, pathElements(otherFolder.Path, rules.CaseInsensitive)) {
				return &FolderError{Index: index, Reason: ReasonOverlap, OtherLibraryID: other.ID}
			}
		}
	}

	return nil
}

func related(a, b []string) bool { return hasPrefix(a, b) || hasPrefix(b, a) }

func sameElements(a, b []string) bool { return len(a) == len(b) && hasPrefix(a, b) }

// hasPrefix reports whether prefix is a per-element prefix of path (equal counts).
func hasPrefix(path, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}

	for i, element := range prefix {
		if path[i] != element {
			return false
		}
	}

	return true
}

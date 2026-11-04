package scan

import (
	"path"

	"github.com/magpeek/mivo/internal/domain/library"
)

// fileKey identifies a file or sidecar inside a library: the folder it lies in and its path
// relative to that folder. The same relative path in two folders names two files (ADR 0028).
type fileKey struct {
	rootID string
	path   string
}

func keyOf(state library.FileState) fileKey {
	return fileKey{rootID: state.RootID, path: state.RelativePath}
}

// directory returns the key of the folder that holds the file.
func (key fileKey) directory() fileKey {
	return fileKey{rootID: key.rootID, path: path.Dir(key.path)}
}

// skipSet lists, per folder ID, the root-relative paths a walk could not read. A folder that
// was unavailable as a whole is covered by wholeFolder.
type skipSet map[string][]string

// wholeFolder is the skipped path that stands for a complete folder (see belowAny).
const wholeFolder = "."

func (skipped skipSet) add(rootID string, paths ...string) {
	if len(paths) > 0 {
		skipped[rootID] = append(skipped[rootID], paths...)
	}
}

// covers reports whether the file lies at or below a path its folder's walk could not read.
func (skipped skipSet) covers(state library.FileState) bool {
	return belowAny(state.RelativePath, skipped[state.RootID])
}

// folderRoot returns the path of the folder with rootID. Catalogs store the folder of every row
// (M3-19h), so an empty or unknown ID names no folder.
func folderRoot(scanned library.ManagedLibrary, rootID string) (string, bool) {
	for _, folder := range scanned.Folders {
		if folder.ID == rootID {
			return folder.Path, true
		}
	}

	return "", false
}

// folderRanks maps folder IDs to their position, so files of several folders sort by folder
// order first (the primary file of a movie lies in the first folder that has it).
func folderRanks(folders []library.Folder) map[string]int {
	ranks := make(map[string]int, len(folders))
	for position, folder := range folders {
		ranks[folder.ID] = position
	}

	return ranks
}

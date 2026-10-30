package config

import (
	"fmt"

	"github.com/magpeek/mivo/internal/platform/paths"
)

// Relations of two directories, as they read in the overlap errors.
const (
	relationSame     = "is the same directory as"
	relationInside   = "lies inside"
	relationContains = "contains"
)

// serverDirectory is a directory the server reads its configuration from or writes to. A
// library root must neither be, contain nor lie inside one: the server would scan its own
// files, and a purge of the transcode directory could reach media (ADR 0022).
type serverDirectory struct {
	description string
	path        string
}

func serverDirectories(cfg Config) []serverDirectory {
	return []serverDirectory{
		{description: "the config directory", path: cfg.ConfigDir},
		{description: "the data directory (" + keyPathsData + ")", path: cfg.Paths.Data},
		{description: "the cache directory (" + keyPathsCache + ")", path: cfg.Paths.Cache},
		{description: "the transcode directory (" + keyPathsTranscode + ")", path: cfg.Paths.Transcode},
	}
}

// checkLibraryRoots rejects duplicate and nested roots and roots that overlap a server
// directory. goos decides the comparison: case-insensitive on Windows. Roots are not
// required to exist, so symlinks are not evaluated here; the walker confines itself to
// each root at scan time.
func checkLibraryRoots(libraries []configuredLibrary, cfg Config, goos string) error {
	directories := serverDirectories(cfg)

	for index, entry := range libraries {
		if err := checkAgainstServerDirectories(entry, directories, goos); err != nil {
			return err
		}

		if err := checkAgainstEarlierRoots(entry, libraries[:index], goos); err != nil {
			return err
		}
	}

	return nil
}

func checkAgainstServerDirectories(entry configuredLibrary, directories []serverDirectory, goos string) error {
	root := entry.library.Path

	for _, directory := range directories {
		if relation, overlaps := relationOf(goos, root, directory.path); overlaps {
			return fmt.Errorf("%s: %w: library root %q %s %s %q",
				entry.origin, ErrInvalid, root, relation, directory.description, directory.path)
		}
	}

	return nil
}

func checkAgainstEarlierRoots(entry configuredLibrary, earlier []configuredLibrary, goos string) error {
	root := entry.library.Path

	for _, other := range earlier {
		if relation, overlaps := relationOf(goos, root, other.library.Path); overlaps {
			return fmt.Errorf("%s: %w: library root %q %s the root %q of %s",
				entry.origin, ErrInvalid, root, relation, other.library.Path, other.label)
		}
	}

	return nil
}

// relationOf describes how path relates to other, and whether they overlap at all.
func relationOf(goos, path, other string) (relation string, overlaps bool) {
	inside, contains := paths.Within(goos, other, path), paths.Within(goos, path, other)

	switch {
	case inside && contains:
		return relationSame, true
	case inside:
		return relationInside, true
	case contains:
		return relationContains, true
	default:
		return "", false
	}
}

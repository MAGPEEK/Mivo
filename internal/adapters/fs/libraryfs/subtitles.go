package libraryfs

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// MaxSubtitlesPerVideo bounds the external subtitles kept for one video, so a folder full of
	// subtitle files cannot flood the catalog.
	MaxSubtitlesPerVideo = 64
	// MaxSubtitleBytes is the largest subtitle file that is listed; the file content is not
	// read here, the limit keeps absurd files out of the track list (ADR 0030 §3).
	MaxSubtitleBytes int64 = 5 << 20
)

// subtitleFolderNames are the folders below a video's folder that hold its subtitle files; the
// match ignores case, like library.ParseSubtitleName does.
func subtitleFolderNames() []string {
	return []string{"subs", "subtitles"}
}

// subtitleCandidate is a file that may be a subtitle: its path relative to the video folder as
// found on disk (opens the file) and in Unicode NFC (what the name parser and the track ID use).
type subtitleCandidate struct {
	native string
	name   string
}

// ListExternalSubtitles returns the external subtitle files of the videos of one folder: files
// next to the videos and one level down in a "Subs" or "Subtitles" folder (ADR 0030 §3).
// folder is the catalog path of the videos' folder ("" for the root); videoBases are the file
// names of the videos without extension. The result maps a video base to its subtitles sorted
// by path, at most MaxSubtitlesPerVideo each, with library-relative RelativePath in NFC.
//
// A file that matches several bases (a video "Movie" and a video "Movie.2") belongs to the
// longest one. Reads stay inside the library root (os.Root); links that leave it, hidden
// entries, files that are not regular and files larger than MaxSubtitleBytes are ignored.
// The content of a subtitle is not read.
//
// Errors: an unsafe folder wraps domain.ErrForbidden, a missing one domain.ErrNotFound, an
// unreachable root domain.ErrUnavailable; a done ctx returns its error.
func (fileSystem *FileSystem) ListExternalSubtitles(
	ctx context.Context, root, folder string, videoBases []string,
) (map[string][]media.ExternalSubtitle, error) {
	native := rootDirectory

	if folder != "" {
		var err error
		if native, err = checkedRelativePath(folder); err != nil {
			return nil, fmt.Errorf("list subtitles of %q: %w", folder, err)
		}
	}

	libraryRoot, err := openLibraryRoot(root)
	if err != nil {
		return nil, fmt.Errorf("list subtitles of %q: %w", folder, err)
	}

	defer closeRoot(libraryRoot)

	lister := subtitleLister{fileSystem: fileSystem, libraryRoot: libraryRoot, root: root, bases: videoBases}

	candidates, err := lister.candidates(ctx, native)
	if err != nil {
		return nil, fmt.Errorf("list subtitles of %q: %w", folder, err)
	}

	return groupSubtitles(candidates, videoBases, norm.NFC.String(folder)), nil
}

// subtitleLister reads the directories of one ListExternalSubtitles call.
type subtitleLister struct {
	fileSystem  *FileSystem
	libraryRoot *os.Root
	root        string
	bases       []string
}

// candidates returns the subtitle files of the folder and of its subtitle folders. The folder
// itself must be readable; a subtitle folder that is not is skipped.
func (lister subtitleLister) candidates(ctx context.Context, native string) ([]subtitleCandidate, error) {
	entries, err := readDirectory(lister.libraryRoot, lister.root, native)
	if err != nil {
		return nil, err
	}

	found := lister.filesOf(ctx, native, "", entries)

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("list subtitles: %w", err)
		}

		if !isSubtitleFolder(entry.Name()) || isSkippedName(entry.Name()) {
			continue
		}

		child := joinNative(native, entry.Name())

		children, err := readDirectory(lister.libraryRoot, lister.root, child)
		if err != nil {
			lister.fileSystem.logger.LogAttrs(ctx, slog.LevelDebug, "skipping unreadable subtitle folder",
				slog.String(pathLogKey, filepath.ToSlash(child)), slog.Any(errorLogKey, err))

			continue
		}

		found = append(found, lister.filesOf(ctx, child, entry.Name()+relativeSeparator, children)...)
	}

	return found, nil
}

// filesOf keeps the entries that may be subtitles of one of the videos and pass the file
// checks. prefix is the path of the directory below the video folder, "" for the folder itself.
func (lister subtitleLister) filesOf(ctx context.Context, directory, prefix string, entries []fs.DirEntry) []subtitleCandidate {
	var found []subtitleCandidate

	for _, entry := range entries {
		if ctx.Err() != nil {
			return found
		}

		name := norm.NFC.String(prefix + entry.Name())
		if isSkippedName(entry.Name()) || !matchesAnyBase(lister.bases, name) {
			continue
		}

		if lister.isUsableFile(directory, entry) {
			found = append(found, subtitleCandidate{native: joinNative(directory, entry.Name()), name: name})
		}
	}

	return found
}

// isUsableFile reports a regular file of at most MaxSubtitleBytes. A link counts through
// os.Root, which refuses links that leave the root.
func (lister subtitleLister) isUsableFile(directory string, entry fs.DirEntry) bool {
	if entry.IsDir() {
		return false
	}

	info, err := lister.libraryRoot.Stat(joinNative(directory, entry.Name()))
	if err != nil {
		return false
	}

	return info.Mode().IsRegular() && info.Size() <= MaxSubtitleBytes
}

// groupSubtitles gives every candidate to its longest matching base, parses each group and
// places the result below the video folder.
func groupSubtitles(candidates []subtitleCandidate, bases []string, folder string) map[string][]media.ExternalSubtitle {
	groups := map[string][]string{}

	for _, candidate := range candidates {
		if base, ok := longestMatchingBase(bases, candidate.name); ok {
			groups[base] = append(groups[base], candidate.name)
		}
	}

	result := make(map[string][]media.ExternalSubtitle, len(groups))

	for base, names := range groups {
		subtitles := library.ExternalSubtitles(base, names)
		subtitles = subtitles[:min(len(subtitles), MaxSubtitlesPerVideo)]

		for position := range subtitles {
			subtitles[position] = subtitles[position].Under(folder)
		}

		if len(subtitles) > 0 {
			result[base] = subtitles
		}
	}

	return result
}

// longestMatchingBase returns the longest of the bases the file name belongs to.
func longestMatchingBase(bases []string, name string) (string, bool) {
	longest, found := "", false

	for _, base := range bases {
		if _, ok := library.ParseSubtitleName(base, name); ok && (!found || len(base) > len(longest)) {
			longest, found = base, true
		}
	}

	return longest, found
}

func matchesAnyBase(bases []string, name string) bool {
	return slices.ContainsFunc(bases, func(base string) bool {
		_, ok := library.ParseSubtitleName(base, name)

		return ok
	})
}

func isSubtitleFolder(name string) bool {
	return slices.ContainsFunc(subtitleFolderNames(), func(folder string) bool {
		return strings.EqualFold(name, folder)
	})
}

// joinNative appends an entry name to a native directory path of an os.Root.
func joinNative(directory, name string) string {
	if directory == rootDirectory {
		return name
	}

	return directory + string(os.PathSeparator) + name
}

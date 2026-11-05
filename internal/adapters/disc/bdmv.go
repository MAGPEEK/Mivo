package disc

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain/media"
)

// Blu-ray folder names (compared lower-cased) and accepted extensions; AVCHD discs use the
// short ".mpl"/".mts" forms.
const (
	bdmvFolderName     = "bdmv"
	playlistFolderName = "playlist"
	streamFolderName   = "stream"
	playlistExtension  = ".mpls"
	avchdPlaylistExt   = ".mpl"
	streamExtension    = ".m2ts"
	avchdStreamExt     = ".mts"
)

// clipFolder names a clip folder below BDMV and the extensions of the files it indexes.
// needsData skips empty files (a 0-byte stream clip is as unplayable as a missing one).
type clipFolder struct {
	name, extension, avchdExtension string
	needsData                       bool
}

// selectBDMV picks the longest playlist that repeats no clip and whose clips all exist.
// Ties keep the lowest playlist number. Unreadable or unparsable playlists are skipped.
func selectBDMV(files *fileTree) ([]titleClip, error) {
	base, err := files.discFolder(bdmvFolderName)
	if err != nil {
		return nil, fmt.Errorf("select Blu-ray title: %w", err)
	}
	streams, err := files.indexClips(base, clipFolder{streamFolderName, streamExtension, avchdStreamExt, true})
	if err != nil {
		return nil, fmt.Errorf("select Blu-ray title: %w", err)
	}
	playlists, err := files.indexClips(base, clipFolder{playlistFolderName, playlistExtension, avchdPlaylistExt, false})
	if err != nil {
		return nil, fmt.Errorf("select Blu-ray title: %w", err)
	}

	var best []titleClip
	var bestDuration time.Duration
	for _, playlistPath := range sortedValues(playlists) {
		clips, err := files.playlistClips(playlistPath, streams)
		if err != nil {
			return nil, fmt.Errorf("select Blu-ray title: %w", err)
		}
		if duration := totalDuration(clips); len(clips) > 0 && (best == nil || duration > bestDuration) {
			best, bestDuration = clips, duration
		}
	}
	if best == nil {
		return nil, fmt.Errorf("select Blu-ray title: no playable playlist: %w", media.ErrDiscNoTitle)
	}

	return best, nil
}

// playlistClips returns the clips of a playable playlist, nil for a skipped one, or an error
// that aborts the whole disc (cancellation, exhausted budget).
func (files *fileTree) playlistClips(playlistPath string, streams map[string]string) ([]titleClip, error) {
	data, err := files.read(playlistPath, maxPlaylistSize)
	if err != nil {
		if isSkippable(err) {
			return nil, nil
		}
		return nil, err
	}
	list, err := parsePlaylist(data)
	if err != nil || list.repeatsClip() {
		return nil, nil //nolint:nilerr // an unparsable playlist is skipped, not fatal
	}

	clips := make([]titleClip, 0, len(list))
	for _, entry := range list {
		clipPath, ok := streams[entry.clipID]
		if !ok {
			return nil, nil // a missing clip makes the playlist unplayable
		}
		clips = append(clips, titleClip{path: clipPath, duration: ticksToDuration(entry.out - entry.in)})
	}

	return clips, nil
}

// indexClips maps the clip ids of the files "ddddd<ext>" in the clip folder below base to their
// tree paths. A missing folder yields an empty index; for names that differ only in case the
// first in sorted order wins.
func (files *fileTree) indexClips(base string, folder clipFolder) (map[string]string, error) {
	dir, found, err := files.childDir(base, folder.name)
	if err != nil || !found {
		return map[string]string{}, err
	}
	entries, err := files.list(dir)
	if err != nil {
		return nil, err
	}

	index := make(map[string]string, len(entries))
	for _, entry := range entries {
		id, ok := clipIDOf(entry, folder)
		if _, taken := index[id]; ok && !taken && (!folder.needsData || hasData(entry)) {
			index[id] = joinPath(dir, entry.Name())
		}
	}

	return index, nil
}

// hasData reports a listed file with a positive size; an unreadable entry counts as empty.
func hasData(entry fs.DirEntry) bool {
	info, err := entry.Info()

	return err == nil && info.Size() > 0
}

// clipIDOf returns the clip id of a non-directory entry named "ddddd<ext>" (any case).
func clipIDOf(entry fs.DirEntry, folder clipFolder) (string, bool) {
	if entry.IsDir() {
		return "", false
	}
	name := lowerASCII(entry.Name())
	for _, extension := range []string{folder.extension, folder.avchdExtension} {
		id, ok := strings.CutSuffix(name, extension)
		if ok && isClipID(id) {
			return id, true
		}
	}

	return "", false
}

func totalDuration(clips []titleClip) time.Duration {
	var total time.Duration
	for _, clip := range clips {
		total += clip.duration
	}

	return total
}

// sortedValues returns the paths of an id index in id order (ids have a fixed width).
func sortedValues(index map[string]string) []string {
	ids := make([]string, 0, len(index))
	for id := range index {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		paths = append(paths, index[id])
	}

	return paths
}

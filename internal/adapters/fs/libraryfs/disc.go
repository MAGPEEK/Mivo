package libraryfs

import (
	"context"
	"io/fs"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Disc folder and index file names, compared without case: discs copied from Windows or ripped
// by various tools use VIDEO_TS and video_ts alike (ADR 0029).
const (
	videoTSFolder = "video_ts"
	videoTSIndex  = "video_ts.ifo"
	bdmvFolder    = "bdmv"
	bdmvIndex     = "index.bdmv"
	isoExtension  = ".iso"
)

// isISOName reports whether a file name is a disc image.
func isISOName(name string) bool {
	return strings.EqualFold(path.Ext(name), isoExtension)
}

// discLayoutOf returns the layout and index file name of a folder name that can be a disc.
func discLayoutOf(folderName string) (media.Layout, string, bool) {
	switch strings.ToLower(folderName) {
	case videoTSFolder:
		return media.LayoutVideoTS, videoTSIndex, true
	case bdmvFolder:
		return media.LayoutBDMV, bdmvIndex, true
	default:
		return "", "", false
	}
}

// directoryOrDisc reports a disc folder as one entry, or walks any other directory.
func (w *walk) directoryOrDisc(ctx context.Context, at location) error {
	if reported, err := w.reportDisc(at); reported || err != nil {
		return err
	}

	return w.directory(ctx, at)
}

// reportDisc reports a BDMV or VIDEO_TS folder that holds its index file as one disc, so its
// inner files are not walked. A folder without the index, or one that cannot be inspected, is
// not reported and is walked like a normal directory (which logs what is unreadable).
//
// The folder's identity is recorded like a walked directory's, so a link with another name to
// the same disc (walked after the real tree) is recognised as already seen and not listed as
// a normal directory. A disc directly in the library root is reported although
// naming.PlaceDisc rejects it later; the scan treats it as unmatched (M3-16h).
func (w *walk) reportDisc(at location) (bool, error) {
	layout, indexName, ok := discLayoutOf(path.Base(at.relativePath))
	if !ok {
		return false, nil
	}

	info, identity, found := w.findIndex(at.relativePath, indexName)
	if !found {
		return false, nil
	}

	if identity.known() {
		if _, seen := w.visited[identity]; seen {
			w.duplicate(at)
			return true, nil
		}

		w.visited[identity] = struct{}{}
	}

	return true, w.emitVideo(at.relativePath, info, layout)
}

// findIndex looks for the regular index file in a folder, whatever its case, and returns the
// folder's identity. It goes through the root, so a link to a file outside the library never
// counts.
func (w *walk) findIndex(folder, indexName string) (fs.FileInfo, directoryID, bool) {
	directory, err := w.root.OpenFile(folder, openDirectoryFlags, 0)
	if err != nil {
		return nil, directoryID{}, false
	}

	defer closeFile(directory)

	identity, err := w.identify(directory)
	if err != nil {
		return nil, directoryID{}, false
	}

	entries, err := directory.ReadDir(readAllEntries)
	if err != nil {
		return nil, directoryID{}, false
	}

	for _, entry := range entries {
		if !strings.EqualFold(entry.Name(), indexName) {
			continue
		}

		info, err := w.root.Stat(joinRelative(folder, entry.Name()))
		if err == nil && info.Mode().IsRegular() {
			return info, identity, true
		}
	}

	return nil, directoryID{}, false
}

// isoFile reports a disc image. An empty file cannot be one (a truncated copy, say); it is
// recorded as skipped so the scan keeps a catalog entry the file had before.
func (w *walk) isoFile(relativePath string, info fs.FileInfo) error {
	if info.Size() <= 0 {
		w.logger.Info("skipping empty disc image", pathAttr(relativePath))
		w.skipped = append(w.skipped, relativePath)

		return nil
	}

	return w.emitVideo(relativePath, info, media.LayoutISO)
}

// emitVideo hands a disc to the visitor. For a folder, info is that of its index file: the
// scan then notices a re-authored disc without reading the whole folder.
func (w *walk) emitVideo(relativePath string, info fs.FileInfo, layout media.Layout) error {
	return w.report(library.FileState{
		RelativePath:   relativePath,
		NormalizedPath: normalizedPath(relativePath),
		Class:          library.FileClassVideo,
		Size:           info.Size(),
		ModifiedAt:     info.ModTime(),
		Layout:         layout,
	})
}

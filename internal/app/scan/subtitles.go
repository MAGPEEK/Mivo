package scan

import (
	"context"
	"log/slog"
	"path"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// maxExternalSubtitles bounds the external subtitles stored per video (ADR 0030 §3). The
// lister applies the same limit; the scan does not trust it.
const maxExternalSubtitles = 64

// SubtitleLister finds the external subtitle files of the videos in one folder. Implemented by
// adapters/fs/libraryfs (ListExternalSubtitles).
type SubtitleLister interface {
	// ListExternalSubtitles returns, per video base (file name without extension, in NFC), the
	// subtitles next to the videos of folder (a path relative to root, "" for the root) and in
	// its Subs or Subtitles folder. RelativePath is relative to root. A vanished or unsafe
	// folder wraps domain.ErrNotFound or domain.ErrForbidden, an unusable root
	// domain.ErrUnavailable.
	ListExternalSubtitles(ctx context.Context, root, folder string, videoBases []string) (map[string][]media.ExternalSubtitle, error)
}

// SubtitleCatalog stores the external subtitles of media files. Implemented by adapters/sqlite.
type SubtitleCatalog interface {
	// ExternalSubtitles returns the stored subtitles of the library's files by file ID; files
	// without subtitles are absent.
	ExternalSubtitles(ctx context.Context, libraryID string) (map[string][]media.ExternalSubtitle, error)
	// ReplaceExternalSubtitles makes the given list the complete set of the file's subtitles;
	// an empty list removes them all. Errors wrap domain.ErrNotFound for an unknown file.
	ReplaceExternalSubtitles(ctx context.Context, fileID string, subtitles []media.ExternalSubtitle) error
}

// SubtitlePorts are the optional ports of the external subtitle step; without both the scan
// leaves the stored subtitles alone.
type SubtitlePorts struct {
	Lister  SubtitleLister
	Catalog SubtitleCatalog
}

func (ports SubtitlePorts) complete() bool {
	return ports.Lister != nil && ports.Catalog != nil
}

// folderVideos are the videos of one folder, by the base of their file name.
type folderVideos struct {
	rootID string
	folder string
	// idsByBase lists the file IDs of the videos with the same base name.
	idsByBase map[string][]string
}

func (videos folderVideos) bases() []string {
	bases := make([]string, 0, len(videos.idsByBase))
	for base := range videos.idsByBase {
		bases = append(bases, base)
	}

	slices.Sort(bases)

	return bases
}

// scanSubtitles records the external subtitles of every video this scan placed. A folder whose
// listing fails keeps its stored subtitles; only a cancelled scan or a failing catalog ends the
// step. Unchanged sets are not written.
func (scanRun *runV2) scanSubtitles(ctx context.Context, files walked) error {
	ports := scanRun.base.service.subtitles
	if !ports.complete() {
		return nil
	}

	stored, err := scanRun.catalog.PlacedFiles(ctx, scanRun.base.library.ID)
	if err != nil {
		return catalogFailure("load placed files for subtitles", err)
	}

	known, err := ports.Catalog.ExternalSubtitles(ctx, scanRun.base.library.ID)
	if err != nil {
		return catalogFailure("load external subtitles", err)
	}

	for _, videos := range scanRun.videosByFolder(stored, files.skipped) {
		if err := scanRun.syncFolderSubtitles(ctx, videos, known); err != nil {
			return err
		}
	}

	return nil
}

// videosByFolder groups the placed single-file videos by folder, in a stable order. Disc
// folders and folders the walk could not read are left out.
func (scanRun *runV2) videosByFolder(stored []PlacedFileState, skipped skipSet) []folderVideos {
	idsByKey := make(map[fileKey]string, len(stored))
	for _, file := range stored {
		idsByKey[keyOf(file.State)] = file.ID
	}

	groups := map[fileKey]*folderVideos{}

	for _, video := range scanRun.wanted {
		state := video.state

		id, saved := idsByKey[keyOf(state)]
		if !saved || skipped.covers(state) || (state.Layout != "" && state.Layout != media.LayoutFile) {
			continue
		}

		directory := keyOf(state).directory()

		group := groups[directory]
		if group == nil {
			group = &folderVideos{rootID: state.RootID, folder: folderOf(directory.path), idsByBase: map[string][]string{}}
			groups[directory] = group
		}

		base := videoBase(state)
		group.idsByBase[base] = append(group.idsByBase[base], id)
	}

	ordered := make([]folderVideos, 0, len(groups))
	for _, group := range groups {
		ordered = append(ordered, *group)
	}

	slices.SortFunc(ordered, func(left, right folderVideos) int {
		return strings.Compare(left.rootID+"\x00"+left.folder, right.rootID+"\x00"+right.folder)
	})

	return ordered
}

// syncFolderSubtitles lists one folder and writes the subtitle sets that differ from the stored.
func (scanRun *runV2) syncFolderSubtitles(
	ctx context.Context, videos folderVideos, known map[string][]media.ExternalSubtitle,
) error {
	base := scanRun.base

	root, found := folderRoot(base.library, videos.rootID)
	if !found {
		return nil
	}

	listed, err := base.service.subtitles.Lister.ListExternalSubtitles(ctx, root, videos.folder, videos.bases())
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		base.service.logger.LogAttrs(ctx, slog.LevelWarn, "scan kept the stored subtitles of a folder it could not list",
			slog.String("library", base.library.ID), slog.String("path", videos.folder), slog.Any("error", err))

		return nil
	}

	for videoName, ids := range videos.idsByBase {
		wanted := listed[videoName]
		wanted = wanted[:min(len(wanted), maxExternalSubtitles)]

		for _, id := range ids {
			if err := scanRun.replaceIfChanged(ctx, id, known[id], wanted); err != nil {
				return err
			}
		}
	}

	return nil
}

func (scanRun *runV2) replaceIfChanged(ctx context.Context, fileID string, stored, wanted []media.ExternalSubtitle) error {
	if slices.Equal(stored, wanted) {
		return nil
	}

	if err := scanRun.base.service.subtitles.Catalog.ReplaceExternalSubtitles(ctx, fileID, slices.Clone(wanted)); err != nil {
		return catalogFailure("replace external subtitles", err)
	}

	return nil
}

// videoBase returns the file name of a video without its extension, the base external subtitle
// names must start with. PathForParsing is Unicode NFC already, so decomposed names match.
func videoBase(state library.FileState) string {
	name := path.Base(state.PathForParsing())

	return strings.TrimSuffix(name, path.Ext(name))
}

// folderOf returns the folder of a video as the lister expects it: "" for the library root.
func folderOf(directory string) string {
	if directory == "." {
		return ""
	}

	return directory
}

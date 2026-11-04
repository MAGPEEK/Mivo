package naming

import (
	"fmt"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Disc root names and the stand-in extension of the virtual video file (ADR 0029).
const (
	videoTSName      = "video_ts"
	bdmvName         = "bdmv"
	isoExtension     = ".iso"
	virtualDiscExt   = ".disc"
	singleDiscVideos = 1
)

// PlaceDisc places a disc like a video file of its own name: "Movie (2019)/BDMV" and
// "Movie (2019)/VIDEO_TS" are named by the folder holding them, "Movie (2019).iso" by the
// file. In a movies library the disc belongs to its folder movie; in a shows library the
// episode patterns are looked for next to the disc's folder, so
// "Show/Season 1/Show S01E02/VIDEO_TS" is parsed as "Show/Season 1/Show S01E02.disc".
func PlaceDisc(kind library.Kind, discPath string) (Result, error) {
	virtual, err := discVideoPath(kind, discPath)
	if err != nil {
		return Result{}, err
	}

	return Place(kind, FileContext{Path: virtual, DirectoryVideoCount: singleDiscVideos})
}

func discVideoPath(kind library.Kind, discPath string) (string, error) {
	if _, err := library.ParseRelativePath(discPath); err != nil {
		return "", fmt.Errorf("place disc: %w", err)
	}

	base := path.Base(discPath)
	if strings.EqualFold(path.Ext(base), isoExtension) {
		return discPath, nil
	}

	lower := strings.ToLower(base)
	parent := path.Dir(discPath)

	if (lower != videoTSName && lower != bdmvName) || parent == rootDirectory {
		return "", fmt.Errorf("place disc %q: not a disc image or disc folder: %w", discPath, domain.ErrInvalid)
	}

	name := path.Base(parent) + virtualDiscExt
	if kind == library.KindMovies {
		return path.Join(parent, name), nil
	}

	return path.Join(path.Dir(parent), name), nil
}

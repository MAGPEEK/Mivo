package naming

import (
	"fmt"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
)

// unkeyedNamePrefix marks the match key of a name without letters or digits, which
// NormalizeKey would reduce to nothing.
const unkeyedNamePrefix = "name:"

// placeInPhotos places a photo or video file of a photos library (ADR 0029): every
// directory on the path is a photo album nested like the folder tree, the file is a photo
// below the last album, or a top-level photo in the library root. The photo's match key is
// its normalised file name including the extension, so photo.jpg and photo.png are two
// photos; a RAW partner is attached by the scan (photo.GroupFiles), not here.
//
// Known limits: a RAW file without a partner is keyed by its own extension, so when its
// JPEG appears later the item's key changes from "img 1 cr2" to "img 1 jpg". And because
// NormalizeKey turns punctuation into spaces, "a.b.jpg" and "a b.jpg" in one folder get the
// same key and collide (likewise two albums "A_B" and "A B").
func placeInPhotos(file FileContext) (Result, error) {
	if _, err := library.ParseRelativePath(file.Path); err != nil {
		return Result{}, fmt.Errorf("place: %w", err)
	}

	segments := strings.Split(file.Path, "/")
	levels := make([]library.Level, 0, len(segments))

	for _, directory := range segments[:len(segments)-1] {
		levels = append(levels, library.Level{
			Kind: library.ItemKindPhotoAlbum, MatchKey: photoKey(directory), Title: directory,
		})
	}

	base := segments[len(segments)-1]
	levels = append(levels, library.Level{
		Kind: library.ItemKindPhoto, MatchKey: photoKey(base), Title: library.TitleFromFileName(base),
	})

	placement := library.Placement{Levels: levels}
	if err := placement.Validate(); err != nil {
		return Result{}, fmt.Errorf("place %q: %w", file.Path, err)
	}

	return Result{Outcome: OutcomePlaced, Placement: placement}, nil
}

func photoKey(name string) string {
	if key := library.NormalizeKey(name); key != "" {
		return key
	}

	return unkeyedNamePrefix + strings.ToLower(path.Base(name))
}

package tmdb

import (
	"fmt"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	// maxArtworkPerKind bounds the pictures offered per kind; the first is the selection.
	maxArtworkPerKind = 5

	// Image sizes per kind (ADR 0031 §11): fixed widths for posters and backdrops; other kinds
	// use the original, which the artwork fetcher limits to 15 MiB.
	sizePoster   = "w780"
	sizeBackdrop = "w1280"
	sizeOriginal = "original"

	maxFilePathLength = 128
)

// ImageURL builds the image.tmdb.org address of a TMDB picture with the fixed size for its
// kind. The artwork fetcher (M6-08) calls it; refs from other providers or with a path that
// is not a plain TMDB file name fail with domain.ErrInvalid.
func (c *Client) ImageURL(ref enrich.ArtworkRef) (string, error) {
	if ref.Provider != enrich.ArtworkProviderTMDB || !isFilePath(ref.Path) {
		return "", fmt.Errorf("tmdb image url for %s picture: %w", ref.Provider, domain.ErrInvalid)
	}

	return c.imageBaseURL + "/" + imageSize(ref.Kind) + ref.Path, nil
}

func imageSize(kind artwork.Kind) string {
	switch kind {
	case artwork.KindPoster:
		return sizePoster
	case artwork.KindBackdrop:
		return sizeBackdrop
	case artwork.KindThumb, artwork.KindLogo, artwork.KindBanner:
		// Listed so the exhaustive linter notices a new kind; it shares the default size.
		return sizeOriginal
	default:
		return sizeOriginal
	}
}

// isFilePath accepts TMDB's "/abc123.jpg": one leading slash and a file name of letters,
// digits, '_', '-' and dots, never "..". Anything else cannot be turned into a URL safely.
func isFilePath(path string) bool {
	if len(path) < 2 || len(path) > maxFilePathLength || path[0] != '/' {
		return false
	}

	name := path[1:]
	for index := range len(name) {
		if !isFileNameByte(name[index]) {
			return false
		}
	}

	return name[0] != '.'
}

func isFileNameByte(character byte) bool {
	return (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
		(character >= '0' && character <= '9') || character == '_' || character == '-' || character == '.'
}

// artworkRefs lists the pictures of an entry, the primary one of each kind first, without
// duplicates and with at most maxArtworkPerKind per kind.
func artworkRefs(primaryPoster, primaryBackdrop string, pictures images) []enrich.ArtworkRef {
	var refs []enrich.ArtworkRef

	refs = append(refs, kindRefs(artwork.KindPoster, primaryPoster, pictures.Posters)...)
	refs = append(refs, kindRefs(artwork.KindBackdrop, primaryBackdrop, pictures.Backdrops)...)
	refs = append(refs, kindRefs(artwork.KindLogo, "", pictures.Logos)...)

	return refs
}

func kindRefs(kind artwork.Kind, primary string, more []image) []enrich.ArtworkRef {
	paths := make([]string, 0, len(more)+1)
	paths = append(paths, primary)

	for _, picture := range more {
		paths = append(paths, picture.FilePath)
	}

	var refs []enrich.ArtworkRef

	seen := map[string]bool{}

	for _, path := range paths {
		if len(refs) == maxArtworkPerKind {
			break
		}

		if !isFilePath(path) || seen[path] {
			continue
		}

		seen[path] = true

		refs = append(refs, enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: kind, Path: path})
	}

	return refs
}

// singleRef is the one picture of a season (poster) or episode (still).
func singleRef(kind artwork.Kind, path string) []enrich.ArtworkRef {
	return kindRefs(kind, path, nil)
}

package library

import (
	"path"
	"strings"
)

// KindPhotos is the library kind for photo collections (ADR 0029).
const KindPhotos Kind = "photos"

// Photo item kinds: every directory is an album, every image (or video) a photo.
const (
	ItemKindPhotoAlbum ItemKind = "photo_album"
	ItemKindPhoto      ItemKind = "photo"
)

// FileClassPhoto is an image file that is itself a library item (a photo), as opposed to
// FileClassImage, which is artwork describing another item.
const FileClassPhoto FileClass = "photo"

// ImageClass groups image extensions by how much a browser can do with the file.
type ImageClass string

// Known image classes (ADR 0029).
const (
	// ImageClassWeb is displayed by browsers as is.
	ImageClassWeb ImageClass = "web"
	// ImageClassHEIC is HEIC/HEIF and needs a decoder or a conversion.
	ImageClassHEIC ImageClass = "heic"
	// ImageClassTIFF is TIFF and needs a conversion.
	ImageClassTIFF ImageClass = "tiff"
	// ImageClassRAW is a camera RAW file; its embedded preview is shown if there is one.
	ImageClassRAW ImageClass = "raw"
)

// ClassifyImage returns the image class of a file name by its extension, ignoring case.
// ok is false for every other file.
func ClassifyImage(name string) (class ImageClass, ok bool) {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".avif":
		return ImageClassWeb, true
	case ".heic", ".heif":
		return ImageClassHEIC, true
	case ".tiff", ".tif":
		return ImageClassTIFF, true
	case ".cr2", ".cr3", ".nef", ".arw", ".dng", ".orf", ".rw2", ".raf":
		return ImageClassRAW, true
	default:
		return "", false
	}
}

// classifyPhotoFile is the photos-library branch of ClassifyFileIn: videos stay videos and
// every image is a photo (FileClassPhoto), not artwork. Other files are not decided here.
func classifyPhotoFile(name string) (class FileClass, ok bool) {
	if IsVideoFileName(name) {
		return FileClassVideo, true
	}

	if _, isImage := ClassifyImage(name); isImage {
		return FileClassPhoto, true
	}

	return "", false
}

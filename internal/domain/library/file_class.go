package library

import (
	"fmt"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// FileClass says what a library file is to a scan (ADR 0027 §9): a video to probe and place,
// or a sidecar (NFO or image) that only describes the items next to it. The text values are
// stored in the database.
type FileClass string

// Known file classes.
const (
	FileClassVideo FileClass = "video"
	FileClassNFO   FileClass = "nfo"
	FileClassImage FileClass = "image"
)

// ClassifyFile returns the class of a file by its extension, ignoring case: the video
// extensions of IsVideoFileName, ".nfo", and the local artwork extensions of ADR 0027 §8
// (jpg, jpeg, png, webp, tbn). Any other file is not a library file and ok is false.
func ClassifyFile(name string) (class FileClass, ok bool) {
	extension := strings.ToLower(path.Ext(name))

	switch {
	case isVideoExtension(extension):
		return FileClassVideo, true
	case extension == ".nfo":
		return FileClassNFO, true
	case isImageExtension(extension):
		return FileClassImage, true
	default:
		return "", false
	}
}

// ParseFileClass accepts the exact text of a known file class, for example when an adapter
// reads a stored row back. The error wraps domain.ErrInvalid.
func ParseFileClass(text string) (FileClass, error) {
	class := FileClass(text)
	if !class.IsValid() {
		return "", fmt.Errorf("parse file class %q: unknown class: %w", text, domain.ErrInvalid)
	}

	return class, nil
}

// IsValid reports whether the file class is one of the known classes.
func (class FileClass) IsValid() bool {
	switch class {
	case FileClassVideo, FileClassNFO, FileClassImage, FileClassAudio:
		return true
	default:
		return false
	}
}

// String returns the text value of the file class.
func (class FileClass) String() string {
	return string(class)
}

// isImageExtension reports a lowercase local artwork extension. ".tbn" is Kodi's legacy
// thumbnail name for a JPEG or PNG file.
func isImageExtension(extension string) bool {
	switch extension {
	case ".jpg", ".jpeg", ".png", ".webp", ".tbn":
		return true
	default:
		return false
	}
}

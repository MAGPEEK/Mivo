package imagecache

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	// uuidLength is the length of the canonical UUID text of an artwork ID.
	uuidLength = 36
	// prefixLength is the number of leading ID characters that name the fan-out directory,
	// so no single directory holds every artwork.
	prefixLength = 2
	// nameSeparator separates tag and width in a cache file name.
	nameSeparator = "-"
)

// isUUID reports whether text is a UUID in canonical lowercase form. Only such IDs become
// path parts, so a crafted ID can never contain a separator or "..".
func isUUID(text string) bool {
	if len(text) != uuidLength {
		return false
	}

	for position := range len(text) {
		switch position {
		case 8, 13, 18, 23: //nolint:mnd // hyphen offsets of the canonical UUID text
			if text[position] != '-' {
				return false
			}
		default:
			if !isLowerHex(text[position]) {
				return false
			}
		}
	}

	return true
}

// isTag reports whether text is a cache tag: artwork.TagLength lowercase hex digits.
func isTag(text string) bool {
	if len(text) != artwork.TagLength {
		return false
	}

	for position := range len(text) {
		if !isLowerHex(text[position]) {
			return false
		}
	}

	return true
}

func isLowerHex(character byte) bool {
	return ('0' <= character && character <= '9') || ('a' <= character && character <= 'f')
}

func isPrefix(text string) bool {
	return len(text) == prefixLength && isLowerHex(text[0]) && isLowerHex(text[1])
}

// validateKey rejects every key that must not become a path.
func validateKey(key images.Key) error {
	if !isUUID(key.ID) {
		return fmt.Errorf("image cache key: id %q is not a canonical UUID: %w", key.ID, domain.ErrInvalid)
	}

	if !isTag(key.Tag) {
		return fmt.Errorf("image cache key: tag %q is not a cache tag: %w", key.Tag, domain.ErrInvalid)
	}

	if _, err := artwork.ParseWidth(key.Width.String()); err != nil {
		return fmt.Errorf("image cache key: %w", err)
	}

	if key.Format != artwork.FormatJPEG && key.Format != artwork.FormatPNG {
		return fmt.Errorf("image cache key: format %q: %w", key.Format, domain.ErrInvalid)
	}

	return nil
}

// entryDirectory returns <root>/<id[0:2]>/<id> of a validated ID.
func entryDirectory(root, id string) string {
	return filepath.Join(root, id[:prefixLength], id)
}

// fileName returns <tag>-<width>.<ext> of a validated key.
func fileName(key images.Key) string {
	return key.Tag + nameSeparator + key.Width.String() + "." + key.Format.Extension()
}

// parseFileName returns the tag of a cache file name, or false for a foreign file.
func parseFileName(name string) (string, bool) {
	tag, rest, found := strings.Cut(name, nameSeparator)
	if !found || !isTag(tag) {
		return "", false
	}

	widthText, extension, found := strings.Cut(rest, ".")
	if !found {
		return "", false
	}

	if _, err := artwork.ParseWidth(widthText); err != nil {
		return "", false
	}

	if extension != artwork.FormatJPEG.Extension() && extension != artwork.FormatPNG.Extension() {
		return "", false
	}

	return tag, true
}

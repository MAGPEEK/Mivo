package library

import (
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	minLanguageLength = 2
	maxLanguageLength = 3
	regionLength      = 2
)

// ParseMetadataLanguage normalises a metadata language: empty means the server default;
// otherwise "ll" or "lll" with an optional "-RR" region, returned as "ll[l][-RR]". Any other
// text wraps domain.ErrInvalid.
func ParseMetadataLanguage(text string) (string, error) {
	if text == "" {
		return "", nil
	}

	language, region, hasRegion := strings.Cut(text, "-")
	if !isLetters(language, minLanguageLength, maxLanguageLength) ||
		(hasRegion && !isLetters(region, regionLength, regionLength)) {
		return "", fmt.Errorf("parse metadata language %q: %w", text, domain.ErrInvalid)
	}

	normalised := strings.ToLower(language)
	if hasRegion {
		normalised += "-" + strings.ToUpper(region)
	}

	return normalised, nil
}

func isLetters(text string, minLength, maxLength int) bool {
	if len(text) < minLength || len(text) > maxLength {
		return false
	}

	for _, char := range text {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
			return false
		}
	}

	return true
}

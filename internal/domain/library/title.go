package library

import (
	"path"
	"strings"
	"unicode"
)

// untitledTitle is the last-resort title for a blank file name, so an item never has an
// empty title in lists and sort order.
const untitledTitle = "Untitled"

// TitleFromFileName derives a display title from a file name: the extension is removed,
// "." and "_" become spaces, runs of whitespace collapse to one space and the result is
// trimmed. If nothing is left (e.g. "___.mkv"), the trimmed file name itself is the title.
// It never returns an empty string. The name parser of milestone 3 replaces this rule.
func TitleFromFileName(fileName string) string {
	stem := strings.TrimSuffix(fileName, path.Ext(fileName))
	spaced := strings.Map(separatorToSpace, stem)

	if title := strings.Join(strings.Fields(spaced), " "); title != "" {
		return title
	}

	if trimmed := strings.TrimSpace(fileName); trimmed != "" {
		return trimmed
	}

	return untitledTitle
}

// SortTitle returns the case-folded form of a title, so "Über", "über" and "ÜBER" sort
// together. Each rune is mapped to upper and then to lower case, an approximation of
// Unicode simple case folding that also unifies variants such as the Greek final sigma and
// the long s. The standard library has no full case folding (ß stays ß).
func SortTitle(title string) string {
	return strings.Map(foldRune, title)
}

func separatorToSpace(character rune) rune {
	if character == '.' || character == '_' {
		return ' '
	}

	return character
}

func foldRune(character rune) rune {
	return unicode.ToLower(unicode.ToUpper(character))
}

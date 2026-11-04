package library

import (
	"path"
	"strings"
	"unicode"
)

// untitledTitle is the last-resort title for a blank file name, so an item never has an
// empty title in lists and sort order (the catalog rejects empty titles).
//
// The scan cannot reach it: a blank name has no video extension and is never catalogued.
// It stays because TitleFromFileName is total and must not return "" for any input; it is
// not translated because titles are catalog data, not UI text. The helper is retired with
// the M2 scan in M3-10, so no i18n placeholder is introduced for it.
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

// SortTitle returns the sort form of a title (UX LB-22): one leading English or German
// article (The, A, An, Der, Die, Das, Ein, Eine) is dropped when it is a whole word followed
// by whitespace and more title, then the text is case-folded so "Über", "über" and "ÜBER"
// sort together. "Die Hard" therefore sorts under H; articles per metadata language come
// with milestone 6 (ADR 0027 §6).
//
// Case folding maps each rune to upper and then to lower case, an approximation of Unicode
// simple case folding that also unifies variants such as the Greek final sigma and the long
// s. The standard library has no full case folding (ß stays ß). The input must be NFC
// (ADR 0027 §3: adapters normalise at the boundary).
func SortTitle(title string) string {
	return strings.Map(foldRune, withoutLeadingArticle(title))
}

// withoutLeadingArticle drops the first word of a title if it is an article and something
// other than whitespace follows it; otherwise the title is returned unchanged.
func withoutLeadingArticle(title string) string {
	wordEnd := strings.IndexFunc(title, unicode.IsSpace)
	if wordEnd <= 0 || !isLeadingArticle(title[:wordEnd]) {
		return title
	}

	rest := strings.TrimLeftFunc(title[wordEnd:], unicode.IsSpace)
	if rest == "" {
		return title
	}

	return rest
}

func isLeadingArticle(word string) bool {
	for _, article := range []string{"the", "a", "an", "der", "die", "das", "ein", "eine"} {
		if strings.EqualFold(word, article) {
			return true
		}
	}

	return false
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

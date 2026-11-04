package metadata

import (
	"strings"
	"unicode"

	"github.com/magpeek/mivo/internal/domain/library"
)

// SortTitleFor returns the sort key of an item (ADR 0027 §6, UX LB-22). An explicit sort
// title (NFO <sorttitle>, later the edit dialog) is only case-folded: the user chose it, so
// no article is dropped. Without one, the key is library.SortTitle of the title.
func SortTitleFor(record Record) string {
	if strings.TrimSpace(record.SortTitle) != "" {
		return strings.Map(foldRune, record.SortTitle)
	}

	return library.SortTitle(record.Title)
}

// foldRune applies the same case folding as library.SortTitle, so explicit and derived sort
// titles order together.
func foldRune(character rune) rune {
	return unicode.ToLower(unicode.ToUpper(character))
}

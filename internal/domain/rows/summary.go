package rows

import (
	"strings"
	"unicode/utf8"
)

// MaxSummaryLineRunes bounds the hero's summary line (API HeroItem.summaryLine, at most 300
// characters): about one line of the hero on a TV.
const MaxSummaryLineRunes = 160

// ellipsis marks a cut summary line.
const ellipsis = "…"

// SummaryLine returns the start of a summary for one line: whitespace runs (line breaks
// included) become single spaces, and a longer text is cut at the last word boundary that
// fits, followed by an ellipsis; a single over-long word is cut inside. Empty for an empty
// summary.
func SummaryLine(summary string) string {
	text := strings.Join(strings.Fields(summary), " ")
	if utf8.RuneCountInString(text) <= MaxSummaryLineRunes {
		return text
	}

	runes := []rune(text)
	cut := string(runes[:MaxSummaryLineRunes-utf8.RuneCountInString(ellipsis)])

	if space := strings.LastIndex(cut, " "); space > 0 {
		cut = cut[:space]
	}

	return strings.TrimRight(cut, " ,;:.-") + ellipsis
}

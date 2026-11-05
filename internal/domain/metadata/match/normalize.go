package match

import (
	"slices"
	"strings"
	"unicode"

	"github.com/magpeek/mivo/internal/domain/library"
)

// leadingArticles are dropped from the start of a title so "The Matrix" and "Matrix" agree.
// "a" and "an" are English, "der/die/das" German, "le/la" French.
//
//nolint:gochecknoglobals // read-only lookup table
var leadingArticles = []string{"the", "a", "an", "der", "die", "das", "le", "la"}

// combiningCompositions maps a combining mark to the Latin letters it composes with and the
// precomposed result at the same index. The domain is standard-library only, so this small
// table stands in for NFC on the accents of European titles; the adapters normalise fully
// (golang.org/x/text) at the boundary and other scripts pass through unchanged.
//
//nolint:gochecknoglobals // read-only lookup table
var combiningCompositions = map[rune][2]string{
	'̀': {"AEIOUaeiou", "ÀÈÌÒÙàèìòù"},
	'́': {"AEIOUYaeiouyCcLlNnRrSsZz", "ÁÉÍÓÚÝáéíóúýĆćĹĺŃńŔŕŚśŹź"},
	'̂': {"AEIOUaeiouCcGgHhJjSsWwYy", "ÂÊÎÔÛâêîôûĈĉĜĝĤĥĴĵŜŝŴŵŶŷ"},
	'̃': {"ANOanoIiUu", "ÃÑÕãñõĨĩŨũ"},
	'̈': {"AEIOUaeiouyY", "ÄËÏÖÜäëïöüÿŸ"},
	'̊': {"AaUu", "ÅåŮů"},
	'̧': {"CcSsTt", "ÇçŞşŢţ"},
	'̌': {"CcDdEeNnRrSsTtZz", "ČčĎďĚěŇňŘřŠšŤťŽž"},
}

// NormalizeTitle reduces a title to the form two spellings of the same title share: Latin
// accents composed (NFC), case-folded, "&" read as "and", punctuation and spacing reduced to
// single spaces and a leading article dropped. Accents stay: "Léon" and "Leon" differ. CJK
// titles keep every character. A title that is only an article keeps it.
func NormalizeTitle(title string) string {
	folded := strings.ReplaceAll(strings.ToLower(composeAccents(title)), "ß", "ss")
	spelled := strings.ReplaceAll(folded, "&", " and ")
	tokens := strings.Fields(library.NormalizeKey(spelled))

	if len(tokens) > 1 && slices.Contains(leadingArticles, tokens[0]) {
		tokens = tokens[1:]
	}

	return strings.Join(tokens, " ")
}

// composeAccents replaces base letter plus combining mark with the precomposed letter for the
// marks in combiningCompositions.
func composeAccents(text string) string {
	var builder strings.Builder

	builder.Grow(len(text))

	var pending rune

	for _, character := range text {
		if composed, ok := composeWith(pending, character); ok {
			builder.WriteRune(composed)

			pending = 0

			continue
		}

		if pending != 0 {
			builder.WriteRune(pending)
		}

		pending = character
	}

	if pending != 0 {
		builder.WriteRune(pending)
	}

	return builder.String()
}

func composeWith(base, mark rune) (rune, bool) {
	pair, known := combiningCompositions[mark]
	if !known || base == 0 || !unicode.IsLetter(base) {
		return 0, false
	}

	index := strings.IndexRune(pair[0], base)
	if index < 0 {
		return 0, false
	}

	composed := []rune(pair[1])

	return composed[runeIndex(pair[0], index)], true
}

// runeIndex converts the byte offset of a rune in text to its rune position.
func runeIndex(text string, byteOffset int) int {
	return len([]rune(text[:byteOffset]))
}

// colonMainTitle returns the normalised part of a title before its first colon, or "" when
// there is no colon or nothing before it.
func colonMainTitle(title string) string {
	main, _, found := strings.Cut(title, ":")
	if !found {
		return ""
	}

	return NormalizeTitle(main)
}

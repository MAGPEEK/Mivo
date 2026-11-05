package library

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// LetterOther is the letter of every sort title that does not start with a Latin letter:
// digits, symbols and other scripts.
const LetterOther = "#"

// latinVariant lists the lowercase Latin letters with diacritics (and the ligatures) that
// fold to one base letter.
type latinVariant struct {
	base     rune
	variants string
}

// latinVariants covers the common letters of Latin-1 and Latin Extended-A. The domain has no
// Unicode decomposition (golang.org/x/text is not available here), so they are listed.
func latinVariants() []latinVariant {
	return []latinVariant{
		{'a', "àáâãäåāăąǎæ"},
		{'c', "çćĉċč"},
		{'d', "ďđð"},
		{'e', "èéêëēĕėęě"},
		{'g', "ĝğġģ"},
		{'h', "ĥħ"},
		{'i', "ìíîïĩīĭįı"},
		{'j', "ĵ"},
		{'k', "ķ"},
		{'l', "ĺļľŀł"},
		{'n', "ñńņňŉ"},
		{'o', "òóôõöøōŏőœ"},
		{'r', "ŕŗř"},
		{'s', "śŝşšß"},
		{'t', "ţťŧ"},
		{'u', "ùúûüũūŭůűų"},
		{'w', "ŵ"},
		{'y', "ýÿŷ"},
		{'z', "źżž"},
	}
}

// LetterOf returns the A–Z bar letter of a sort title: "A" to "Z" for a title that starts
// with that Latin letter, diacritics folded ("Ä" is "A"), else LetterOther.
func LetterOf(sortTitle string) string {
	first, size := utf8.DecodeRuneInString(sortTitle)
	if size == 0 || first == utf8.RuneError {
		return LetterOther
	}

	base := foldLatin(unicode.ToLower(first))
	if base < 'a' || base > 'z' {
		return LetterOther
	}

	return string(unicode.ToUpper(base))
}

func foldLatin(letter rune) rune {
	if letter < utf8.RuneSelf {
		return letter
	}

	for _, entry := range latinVariants() {
		if strings.ContainsRune(entry.variants, letter) {
			return entry.base
		}
	}

	return letter
}

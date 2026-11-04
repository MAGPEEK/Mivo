package library

import (
	"strings"
	"unicode"
)

// keySeparator replaces every run of characters that are not part of a word.
const keySeparator = ' '

// NormalizeKey turns a name into the comparable form used in match keys (ADR 0027 §3): every
// letter is case-folded like SortTitle, every run of other characters (spaces, punctuation,
// symbols) becomes one space, and the result is trimmed. "The.Matrix_(1999)" and
// "the matrix 1999" give the same key.
//
// Combining marks count as word characters: in scripts such as Devanagari they are vowel
// signs, and dropping them would merge different titles into one key.
//
// The input must be Unicode NFC. The adapters normalise file names and NFO text at the
// boundary (golang.org/x/text is not available to the standard-library-only domain), so a
// macOS NFD name over SMB and its NFC twin produce the same key.
func NormalizeKey(text string) string {
	var builder strings.Builder

	builder.Grow(len(text))

	separatorPending := false

	for _, character := range text {
		if !IsKeyCharacter(character) {
			separatorPending = true

			continue
		}

		if separatorPending && builder.Len() > 0 {
			builder.WriteRune(keySeparator)
		}

		separatorPending = false

		builder.WriteRune(foldRune(character))
	}

	return builder.String()
}

// IsKeyCharacter reports whether NormalizeKey keeps a character: letters, digits and
// combining marks. Every other character separates words. The name parser uses it to find
// word boundaries that agree with match keys.
func IsKeyCharacter(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsDigit(character) || unicode.IsMark(character)
}

package account

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/domain"
)

// Length limits of account and profile names in runes (ADR 0032 §1).
const (
	MinNameRunes = 1
	MaxNameRunes = 40
)

// Letters that render as blank space. They are in category Lo (or So), so the category checks
// below do not catch them.
const (
	hangulChoseongFiller  = '\U0000115F'
	hangulJungseongFiller = '\U00001160'
	hangulFiller          = '\U00003164'
	halfwidthHangulFiller = '\U0000FFA0'
	brailleBlank          = '\U00002800'
)

// Composer converts text to Unicode NFC. The domain is standard-library only (ADR 0019), so
// the adapter boundary passes golang.org/x/text/unicode/norm.NFC.String. NFKC is not used on
// purpose: it would silently rewrite characters the user chose (ligatures, superscripts,
// fullwidth forms), and confusable names are not a security boundary here.
type Composer func(text string) string

// NormalizeName turns user input into the stored form of an account or profile name: valid
// UTF-8, composed to NFC, trimmed, every run of inner space separators (Zs, for example a
// no-break space) collapsed to one U+0020, 1–40 runes and free of control characters, line
// and paragraph separators, format characters and blank fillers, with at least one letter or
// number. Composition runs first so a macOS NFD name and its NFC twin end up identical.
//
// A nil compose is a wiring mistake and fails with an error wrapping domain.ErrInvalid; every
// rule violation is an *InvalidFieldError for FieldName.
func NormalizeName(raw string, compose Composer) (string, error) {
	if compose == nil {
		return "", fmt.Errorf("normalize name: a composer is required: %w", domain.ErrInvalid)
	}

	if !utf8.ValidString(raw) {
		return "", invalidField(FieldName, "must be valid UTF-8")
	}

	name := collapseSpaces(strings.TrimSpace(compose(raw)))
	if err := validateName(name); err != nil {
		return "", err
	}

	return name, nil
}

// NameKey returns the case-folded key that makes names unique case-insensitively. Each rune
// is mapped through upper case to lower case, which folds pairs that plain lower-casing
// misses (final sigma, the Kelvin sign, capital sharp s U+1E9E to U+00DF, dotted and dotless
// I to i). It is simple folding: U+00DF stays distinct from "ss". The input must be a
// normalised name.
func NameKey(name string) string {
	return strings.Map(func(character rune) rune {
		return unicode.ToLower(unicode.ToUpper(character))
	}, name)
}

// validateName checks a name that should already be in its stored form.
func validateName(name string) error {
	if !utf8.ValidString(name) {
		return invalidField(FieldName, "must be valid UTF-8")
	}

	if name != strings.TrimSpace(name) || name != collapseSpaces(name) {
		return invalidField(FieldName, "must be trimmed with single spaces between words")
	}

	runes := utf8.RuneCountInString(name)
	if runes < MinNameRunes || runes > MaxNameRunes {
		return invalidField(FieldName, fmt.Sprintf("must have %d to %d characters", MinNameRunes, MaxNameRunes))
	}

	if strings.IndexFunc(name, isForbiddenNameRune) >= 0 {
		return invalidField(FieldName, "must not contain control, format, filler or line break characters")
	}

	if strings.IndexFunc(name, isLetterOrNumber) < 0 {
		return invalidField(FieldName, "must contain a letter or a number")
	}

	return nil
}

// collapseSpaces replaces every run of space separators (Zs) with one U+0020.
func collapseSpaces(text string) string {
	var builder strings.Builder

	builder.Grow(len(text))

	previousWasSpace := false

	for _, character := range text {
		isSpace := unicode.Is(unicode.Zs, character)
		if !isSpace {
			builder.WriteRune(character)
		} else if !previousWasSpace {
			builder.WriteRune(' ')
		}

		previousWasSpace = isSpace
	}

	return builder.String()
}

// isForbiddenNameRune rejects runes that break single-line display or hide text: C0/C1
// controls (Cc), line and paragraph separators (Zl, Zp), every format character (Cf, such as
// bidi overrides, zero-width space and joiners, word joiner, byte order mark, soft hyphen)
// and blank fillers. ZWNJ and ZWJ are refused too, so invisible joiners cannot make two
// names look alike; emoji sequences and Persian ZWNJ spellings are not needed for 0.1.0
// (possible later relaxation, docs/tasks/BACKLOG.md).
func isForbiddenNameRune(character rune) bool {
	switch character {
	case hangulChoseongFiller, hangulJungseongFiller, hangulFiller, halfwidthHangulFiller, brailleBlank:
		return true
	default:
		return unicode.IsControl(character) ||
			unicode.In(character, unicode.Zl, unicode.Zp, unicode.Cf)
	}
}

// isLetterOrNumber accepts runes of the categories L and N; a name of only marks, symbols or
// punctuation has nothing to recognise it by.
func isLetterOrNumber(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsNumber(character)
}

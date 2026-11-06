// Package access holds the pure rules of who may see what (ADR 0032 §7): stored rating texts
// become age buckets, content limits decide which buckets a profile may see and library access
// decides which libraries it may browse. SQL filters and use cases share these rules, so the
// server enforces the same restriction everywhere.
package access

import (
	"strconv"
	"strings"
	"unicode"
)

// AgeBucket is the minimum viewer age of a title, normalised to the buckets of ADR 0032 §7.
type AgeBucket int

// The age buckets every rating system maps to.
const (
	AgeAll AgeBucket = 0
	Age6   AgeBucket = 6
	Age12  AgeBucket = 12
	Age16  AgeBucket = 16
	Age18  AgeBucket = 18
)

// ageBuckets lists all age buckets in ascending order; bare ages round up to the first bucket
// that is not lower.
func ageBuckets() []AgeBucket {
	return []AgeBucket{AgeAll, Age6, Age12, Age16, Age18}
}

// maxAgeDigits bounds a bare age to two digits, so a long number cannot overflow and a
// year like "2019" is not taken for an age.
const maxAgeDigits = 2

// ratingPrefixes returns the prefixes stripped from the compacted rating text before the
// lookup: the US "Rated" wording, the German "FSK" and "ab" ("FSK ab 12") and country codes as
// used by NFO files and TMDB ("de/12", "DE-12", "US:PG-13"). No rating code starts with one of
// them.
func ratingPrefixes() []string {
	return []string{"RATED", "FSK", "AB", "DE", "AT", "CH", "NL", "US", "GB", "UK"}
}

// ratingEntry maps one compacted rating code (upper case, letters and digits only) to its
// bucket. Further systems are added as rows, not code.
type ratingEntry struct {
	code   string
	bucket AgeBucket
}

// ratingTable returns the named codes of ADR 0032 §7. Bare numbers (FSK, BBFC 12/15/18 and
// other age systems) are handled as ages and need no row.
func ratingTable() []ratingEntry {
	return []ratingEntry{
		// US MPAA.
		{code: "G", bucket: AgeAll},
		{code: "PG", bucket: Age6}, // also UK BBFC PG
		{code: "PG13", bucket: Age12},
		{code: "R", bucket: Age16},
		{code: "NC17", bucket: Age18},
		// US TV parental guidelines.
		{code: "TVY", bucket: AgeAll},
		{code: "TVG", bucket: AgeAll},
		{code: "TVY7", bucket: Age6},
		{code: "TVY7FV", bucket: Age6},
		{code: "TVPG", bucket: Age6},
		{code: "TV14", bucket: Age12},
		{code: "TVMA", bucket: Age16},
		// UK BBFC.
		{code: "U", bucket: AgeAll},
		{code: "12A", bucket: Age12},
		{code: "R18", bucket: Age18},
		// Adult markers without a current system (orchestrator decision 2026-11-06).
		{code: "X", bucket: Age18},
		{code: "XXX", bucket: Age18},
	}
}

// ParseRating maps a stored content rating text (NFO, TMDB certification) to its age bucket.
// It accepts German FSK and bare ages ("FSK 12", "de/12", "DE-12", "12", "18+"), US MPAA
// ("Rated PG-13"), US TV ("TV-MA") and UK BBFC ("12A", "R18"), ignoring case, spaces and
// punctuation. Ages between buckets round up (13 → 16). Unknown or empty texts report false,
// so callers can apply the hide-unrated rule.
func ParseRating(text string) (AgeBucket, bool) {
	compacted, ok := compact(text)
	if !ok {
		return AgeAll, false
	}

	code := stripPrefixes(compacted)
	if code == "" {
		return AgeAll, false
	}

	if bucket, ok := bucketForAge(code); ok {
		return bucket, true
	}

	for _, entry := range ratingTable() {
		if entry.code == code {
			return entry.bucket, true
		}
	}

	return AgeAll, false
}

// compact upper-cases text and keeps only ASCII letters and digits, so "pg-13", "PG 13" and
// "Rated: PG-13" share one form. Non-ASCII spaces and punctuation (NBSP, en dash) are dropped
// like their ASCII forms; any other non-ASCII rune (full-width "1８", superscript "1⁸", circled
// "1⑧", roman "Ⅻ", combining marks) reports false. Dropping such a rune would turn "1⁸" into
// a harmless "1", so the rating fails closed as unknown.
func compact(text string) (string, bool) {
	var builder strings.Builder

	for _, r := range text {
		switch {
		case r > unicode.MaxASCII && !unicode.IsSpace(r) && !unicode.IsPunct(r):
			return "", false
		case r <= unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			builder.WriteRune(unicode.ToUpper(r))
		}
	}

	return builder.String(), true
}

// stripPrefixes removes leading rating prefixes until none matches ("DE/FSK 12" → "12").
func stripPrefixes(code string) string {
	for stripped := true; stripped; {
		stripped = false

		for _, prefix := range ratingPrefixes() {
			if rest, ok := strings.CutPrefix(code, prefix); ok {
				code, stripped = rest, true
			}
		}
	}

	return code
}

// bucketForAge interprets an all-digit code as a minimum age and rounds it up to its bucket;
// ages above the highest bucket stay adult-only.
func bucketForAge(code string) (AgeBucket, bool) {
	if len(code) > maxAgeDigits {
		return AgeAll, false
	}

	// compact keeps no sign, so a parsed age is never negative.
	age, err := strconv.Atoi(code)
	if err != nil {
		return AgeAll, false
	}

	for _, bucket := range ageBuckets() {
		if AgeBucket(age) <= bucket {
			return bucket, true
		}
	}

	return Age18, true
}

// Valid reports whether b is one of the age buckets of ADR 0032 §7.
func (b AgeBucket) Valid() bool {
	for _, bucket := range ageBuckets() {
		if b == bucket {
			return true
		}
	}

	return false
}

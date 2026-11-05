package sqlite

import (
	"strings"
	"unicode"
)

// Bounds of a search query. A query is typed by a person, so these limits are far above any
// real search, yet keep a hostile 10 kB query from turning into thousands of FTS5 terms.
const (
	// maxSearchTerms is the number of distinct words of a query that are searched; later
	// words are ignored.
	maxSearchTerms = 8
	// maxSearchTermRunes cuts a longer word, which then still matches as a prefix.
	maxSearchTermRunes = 64
)

// searchMatchExpression turns what a person typed into an FTS5 MATCH expression in which every
// word must match as a prefix, for example `matr caf` -> `"matr"* AND "caf"*`. It returns ""
// when the query has no word.
//
// No character of the query reaches FTS5 syntax: words are runs of letters, digits and
// combining marks, everything else separates them, so quotes, `*`, `:`, `^`, parentheses and
// braces disappear. Each word is quoted as an FTS5 string, which makes operator words such as
// NEAR, AND, OR and NOT plain text. The quotes need no escaping because a word cannot contain
// a double quote.
func searchMatchExpression(query string) string {
	words := searchWords(query)
	if len(words) == 0 {
		return ""
	}

	terms := make([]string, 0, len(words))
	for _, word := range words {
		terms = append(terms, `"`+word+`"*`)
	}

	return strings.Join(terms, " AND ")
}

// searchWords splits query into at most maxSearchTerms distinct words of at most
// maxSearchTermRunes runes each. Invalid UTF-8 decodes to U+FFFD, which is a separator.
func searchWords(query string) []string {
	fields := strings.FieldsFunc(query, func(character rune) bool { return !isSearchWordRune(character) })

	words := make([]string, 0, min(len(fields), maxSearchTerms))
	for _, field := range fields {
		if len(words) == maxSearchTerms {
			break
		}

		word := truncateRunes(field, maxSearchTermRunes)
		if hasLetterOrNumber(word) && !containsWord(words, word) {
			words = append(words, word)
		}
	}

	return words
}

func isSearchWordRune(character rune) bool {
	return unicode.IsLetter(character) || unicode.IsNumber(character) || unicode.IsMark(character)
}

// hasLetterOrNumber leaves out a word of combining marks only: the tokenizer finds no token in
// it, and an empty FTS5 string is not a useful term.
func hasLetterOrNumber(word string) bool {
	return strings.IndexFunc(word, func(character rune) bool {
		return unicode.IsLetter(character) || unicode.IsNumber(character)
	}) >= 0
}

// containsWord compares case-insensitively, so "Matrix matrix" is searched once.
func containsWord(words []string, word string) bool {
	for _, existing := range words {
		if strings.EqualFold(existing, word) {
			return true
		}
	}

	return false
}

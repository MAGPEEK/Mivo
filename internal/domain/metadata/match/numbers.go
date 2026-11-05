package match

import "strings"

// numberTokens maps number words and roman numerals (2 to 20, plus "v" and "x") to the digits
// they stand for, so "Part Two", "II" and "2" count as the same sequel number. A lone "i" is
// not listed: it is a pronoun far more often than a one.
//
//nolint:gochecknoglobals // read-only lookup table
var numberTokens = map[string]string{
	"one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
	"six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10",
	"ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6", "vii": "7", "viii": "8",
	"ix": "9", "x": "10", "xi": "11", "xii": "12", "xiii": "13", "xiv": "14",
	"xv": "15", "xvi": "16", "xvii": "17", "xviii": "18", "xix": "19", "xx": "20",
}

// canonicalNumbers rewrites every number token of a normalised title as plain digits, so
// "toy story ii" and "toy story 2" compare as equal text.
func canonicalNumbers(normalized string) string {
	tokens := strings.Fields(normalized)
	for index, token := range tokens {
		if value, ok := numberValue(token); ok {
			tokens[index] = value
		}
	}

	return strings.Join(tokens, " ")
}

// numberSignature lists the numbers in a normalised title in order, digits as written. Two
// titles with different signatures are different works (sequels, remakes by year) however
// close their letters are.
func numberSignature(normalized string) string {
	var numbers []string

	for token := range strings.FieldsSeq(normalized) {
		if value, ok := numberValue(token); ok {
			numbers = append(numbers, value)
		}
	}

	return strings.Join(numbers, " ")
}

func numberValue(token string) (string, bool) {
	if value, ok := numberTokens[token]; ok {
		return value, true
	}

	for _, character := range token {
		if character < '0' || character > '9' {
			return "", false
		}
	}

	return strings.TrimLeft(token, "0"), true
}

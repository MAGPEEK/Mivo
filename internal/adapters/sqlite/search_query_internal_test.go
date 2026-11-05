package sqlite

import (
	"strings"
	"testing"
)

func TestSearchMatchExpression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  string
	}{
		{name: "empty", query: "", want: ""},
		{name: "only syntax", query: `"*():^{}-+`, want: ""},
		{name: "one word", query: "matr", want: `"matr"*`},
		{name: "every word is a prefix", query: "the matr", want: `"the"* AND "matr"*`},
		{name: "operators are words", query: "a NEAR b", want: `"a"* AND "NEAR"* AND "b"*`},
		{name: "punctuation separates", query: `title:"x"y*`, want: `"title"* AND "x"* AND "y"*`},
		{name: "duplicates ignoring case", query: "Matrix matrix MATRIX", want: `"Matrix"*`},
		{name: "accents kept", query: "Café", want: `"Café"*`},
		{name: "combining mark kept", query: "café", want: "\"café\"*"},
		{name: "marks alone dropped", query: "́́ x", want: `"x"*`},
		{name: "invalid utf-8 separates", query: "a\xffb", want: `"a"* AND "b"*`},
		{name: "digits", query: "2001", want: `"2001"*`},
		{name: "term count capped", query: "a b c d e f g h i j", want: `"a"* AND "b"* AND "c"* AND "d"* AND "e"* AND "f"* AND "g"* AND "h"*`},
		{name: "long word cut", query: strings.Repeat("é", 100), want: `"` + strings.Repeat("é", maxSearchTermRunes) + `"*`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := searchMatchExpression(test.query); got != test.want {
				t.Errorf("searchMatchExpression(%q) = %q, want %q", test.query, got, test.want)
			}
		})
	}
}

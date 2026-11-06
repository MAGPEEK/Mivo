package access_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/access"
)

func TestParseRating(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text   string
		bucket access.AgeBucket
		known  bool
	}{
		// German FSK and bare ages.
		{text: "FSK 0", bucket: access.AgeAll, known: true},
		{text: "FSK 6", bucket: access.Age6, known: true},
		{text: "FSK 12", bucket: access.Age12, known: true},
		{text: "fsk16", bucket: access.Age16, known: true},
		{text: "FSK-18", bucket: access.Age18, known: true},
		{text: "FSK ab 12", bucket: access.Age12, known: true},
		{text: "de/12", bucket: access.Age12, known: true},
		{text: "DE-12", bucket: access.Age12, known: true},
		{text: "DE:16", bucket: access.Age16, known: true},
		{text: "de/FSK 6", bucket: access.Age6, known: true},
		{text: "AT 14", bucket: access.Age16, known: true},
		{text: "CH-7", bucket: access.Age12, known: true},
		{text: "NL/9", bucket: access.Age12, known: true},
		{text: "0", bucket: access.AgeAll, known: true},
		{text: "6", bucket: access.Age6, known: true},
		{text: "12", bucket: access.Age12, known: true},
		{text: " 16 ", bucket: access.Age16, known: true},
		{text: "18", bucket: access.Age18, known: true},
		{text: "18+", bucket: access.Age18, known: true},
		{text: "3", bucket: access.Age6, known: true},
		{text: "13", bucket: access.Age16, known: true},
		{text: "15", bucket: access.Age16, known: true},
		{text: "17", bucket: access.Age18, known: true},
		{text: "21", bucket: access.Age18, known: true},
		// US MPAA.
		{text: "G", bucket: access.AgeAll, known: true},
		{text: "PG", bucket: access.Age6, known: true},
		{text: "pg-13", bucket: access.Age12, known: true},
		{text: "PG 13", bucket: access.Age12, known: true},
		{text: "Rated PG-13", bucket: access.Age12, known: true},
		{text: "Rated R", bucket: access.Age16, known: true},
		{text: "US:R", bucket: access.Age16, known: true},
		{text: "us/NC-17", bucket: access.Age18, known: true},
		{text: "NC-17", bucket: access.Age18, known: true},
		// US TV.
		{text: "TV-Y", bucket: access.AgeAll, known: true},
		{text: "TV-G", bucket: access.AgeAll, known: true},
		{text: "TV-Y7", bucket: access.Age6, known: true},
		{text: "TV-Y7-FV", bucket: access.Age6, known: true},
		{text: "tv-pg", bucket: access.Age6, known: true},
		{text: "TV-14", bucket: access.Age12, known: true},
		{text: "TV-MA", bucket: access.Age16, known: true},
		{text: "TV MA", bucket: access.Age16, known: true},
		// UK BBFC.
		{text: "U", bucket: access.AgeAll, known: true},
		{text: "GB/PG", bucket: access.Age6, known: true},
		{text: "12A", bucket: access.Age12, known: true},
		{text: "UK-12a", bucket: access.Age12, known: true},
		{text: "R18", bucket: access.Age18, known: true},
		// Adult markers.
		{text: "X", bucket: access.Age18, known: true},
		{text: "xxx", bucket: access.Age18, known: true},
		{text: "Rated X", bucket: access.Age18, known: true},
		// Unknown.
		{text: "", known: false},
		{text: "   ", known: false},
		{text: "NR", known: false},
		{text: "Not Rated", known: false},
		{text: "Unrated", known: false},
		{text: "Rated", known: false},
		{text: "FSK", known: false},
		{text: "M", known: false},
		{text: "XX", known: false},
		// Non-ASCII letters or digits fail closed instead of being dropped.
		{text: "1８", known: false},
		{text: "ＰＧ", known: false},
		{text: "FSK 1６", known: false},
		{text: "FSK ١٢", known: false},
		{text: "1⁸", known: false},
		{text: "1⑧", known: false},
		{text: "Ⅻ", known: false},
		{text: "1̈", known: false},
		{text: "PG™", known: false},
		// Non-ASCII spaces and punctuation are separators like their ASCII forms.
		{text: "PG–13", bucket: access.Age12, known: true},
		{text: "FSK 12", bucket: access.Age12, known: true},
		{text: "„FSK 16“", bucket: access.Age16, known: true},
		{text: "TV-99", known: false},
		{text: "2019", known: false},
		{text: "999", known: false},
		{text: "-", known: false},
		{text: "ab", known: false},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			bucket, known := access.ParseRating(test.text)
			if known != test.known || bucket != test.bucket {
				t.Errorf("ParseRating(%q) = %d, %t; want %d, %t", test.text, bucket, known, test.bucket, test.known)
			}
		})
	}
}

func TestAgeBucketValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		bucket access.AgeBucket
		want   bool
	}{
		{bucket: access.AgeAll, want: true},
		{bucket: access.Age6, want: true},
		{bucket: access.Age12, want: true},
		{bucket: access.Age16, want: true},
		{bucket: access.Age18, want: true},
		{bucket: -1, want: false},
		{bucket: 13, want: false},
		{bucket: 21, want: false},
	}

	for _, test := range tests {
		if got := test.bucket.Valid(); got != test.want {
			t.Errorf("AgeBucket(%d).Valid() = %t, want %t", test.bucket, got, test.want)
		}
	}
}

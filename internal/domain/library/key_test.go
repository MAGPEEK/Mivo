package library_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestNormalizeKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "case folded", text: "The Matrix", want: "the matrix"},
		{name: "separators collapse", text: "The.Matrix_(1999) - [1080p]", want: "the matrix 1999 1080p"},
		{name: "trimmed", text: "  ..Movie!!  ", want: "movie"},
		{name: "folder path", text: "Movies/Inception (2010)", want: "movies inception 2010"},
		{name: "umlaut", text: "ÜBER Film", want: "über film"},
		{name: "greek final sigma", text: "ΟΔΥΣΣΕΥΣ", want: "οδυσσευσ"},
		{name: "japanese", text: "千と千尋の神隠し", want: "千と千尋の神隠し"},
		{name: "unicode digits", text: "Show ٣", want: "show ٣"},
		{name: "combining marks stay in the word", text: "हिन्दी फ़िल्म", want: "हिन्दी फ़िल्म"},
		{name: "apostrophe splits", text: "Director's Cut", want: "director s cut"},
		{name: "unicode whitespace", text: "A  B", want: "a b"},
		{name: "only separators", text: " -_. ", want: ""},
		{name: "empty", text: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.NormalizeKey(test.text); got != test.want {
				t.Errorf("NormalizeKey(%q) = %q, want %q", test.text, got, test.want)
			}
		})
	}
}

func TestNormalizeKeyMatchesSpellingVariants(t *testing.T) {
	t.Parallel()

	variants := []string{"Dragon Ball Super", "dragon.ball.super", "DRAGON_BALL_SUPER", " Dragon - Ball  Super "}
	want := library.NormalizeKey(variants[0])

	for _, variant := range variants[1:] {
		if got := library.NormalizeKey(variant); got != want {
			t.Errorf("NormalizeKey(%q) = %q, want %q", variant, got, want)
		}
	}
}

package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestParseRelativePathAcceptsSlashSeparatedRelativePaths(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"movie.mkv",
		"Movies/Über Film (2019)/Über_Film.2019.mkv",
		"a/b/c/d.mp4",
		"..movie.mkv",
		"dir.../file..mkv",
		"Movie: Part 2.mkv",
		"Movies/..x/movie.mkv",
		"1:/movie.mkv",
		"Season 1/S01E01.mkv",
		"千と千尋/神隠し.mkv",
	} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			got, err := library.ParseRelativePath(text)
			if err != nil {
				t.Fatalf("ParseRelativePath(%q) error = %v, want nil", text, err)
			}

			if got != text {
				t.Errorf("ParseRelativePath(%q) = %q, want the input unchanged", text, got)
			}
		})
	}
}

func TestParseRelativePathRejectsUnsafePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
	}{
		{name: "empty", text: ""},
		{name: "absolute slash", text: "/x"},
		{name: "absolute root", text: "/"},
		{name: "drive letter slash", text: "C:/x"},
		{name: "lowercase drive letter", text: "c:/x"},
		{name: "drive relative", text: "C:x"},
		{name: "bare drive", text: "C:"},
		{name: "unc path", text: `\\server\x`},
		{name: "backslash separator", text: `Movies\x.mkv`},
		{name: "parent segment", text: "../x"},
		{name: "inner parent segment", text: "a/../x"},
		{name: "trailing parent segment", text: "a/.."},
		{name: "only parent", text: ".."},
		{name: "current segment", text: "./x"},
		{name: "inner current segment", text: "a/./x"},
		{name: "only current", text: "."},
		{name: "double slash", text: "a//x"},
		{name: "trailing slash", text: "a/"},
		{name: "nul byte", text: "a\x00b.mkv"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := library.ParseRelativePath(test.text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ParseRelativePath(%q) error = %v, want ErrInvalid", test.text, err)
			}

			if got != "" {
				t.Errorf("ParseRelativePath(%q) = %q on error, want empty", test.text, got)
			}
		})
	}
}

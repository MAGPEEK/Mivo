package library_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestTitleFromFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileName string
		want     string
	}{
		{name: "unicode with underscore and dot", fileName: "Über_Film.2019.mkv", want: "Über Film 2019"},
		{name: "plain", fileName: "Movie.mkv", want: "Movie"},
		{name: "dots", fileName: "The.Big.Movie.mp4", want: "The Big Movie"},
		{name: "underscores", fileName: "the_big_movie.avi", want: "the big movie"},
		{name: "collapses whitespace", fileName: "A  _ .  B.mkv", want: "A B"},
		{name: "trims", fileName: "  _Padded_ .mkv", want: "Padded"},
		{name: "unicode whitespace", fileName: "A  B.mkv", want: "A B"},
		{name: "keeps other punctuation", fileName: "Mr. Robot - S01E01 (2015).mkv", want: "Mr Robot - S01E01 (2015)"},
		{name: "no extension", fileName: "noextension", want: "noextension"},
		{name: "japanese", fileName: "千と千尋の神隠し.mkv", want: "千と千尋の神隠し"},
		{name: "only separators falls back", fileName: "___.mkv", want: "___.mkv"},
		{name: "only extension falls back", fileName: ".mkv", want: ".mkv"},
		{name: "blank falls back to placeholder", fileName: "   ", want: "Untitled"},
		{name: "empty falls back to placeholder", fileName: "", want: "Untitled"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.TitleFromFileName(test.fileName); got != test.want {
				t.Errorf("TitleFromFileName(%q) = %q, want %q", test.fileName, got, test.want)
			}
		})
	}
}

func TestSortTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "ascii", title: "Big Movie", want: "big movie"},
		{name: "umlaut", title: "Über Film 2019", want: "über film 2019"},
		{name: "greek final sigma", title: "ΟΔΥΣΣΕΥΣ ς", want: "οδυσσευσ σ"},
		{name: "long s", title: "ſeason", want: "season"},
		{name: "already folded", title: "abc", want: "abc"},
		{name: "empty", title: "", want: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.SortTitle(test.title); got != test.want {
				t.Errorf("SortTitle(%q) = %q, want %q", test.title, got, test.want)
			}
		})
	}
}

func TestSortTitleDropsOneLeadingArticle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		title string
		want  string
	}{
		{name: "the", title: "The Matrix", want: "matrix"},
		{name: "a", title: "A Beautiful Mind", want: "beautiful mind"},
		{name: "an", title: "An American Werewolf", want: "american werewolf"},
		{name: "der", title: "Der Untergang", want: "untergang"},
		{name: "die", title: "Die Welle", want: "welle"},
		{name: "das", title: "Das Boot", want: "boot"},
		{name: "ein", title: "Ein Fisch namens Wanda", want: "fisch namens wanda"},
		{name: "eine", title: "Eine Frage der Ehre", want: "frage der ehre"},
		{name: "case-insensitive", title: "THE OFFICE", want: "office"},
		{name: "only one article", title: "The A Team", want: "a team"},
		{name: "several spaces", title: "The \t Wire", want: "wire"},
		{name: "english die is still an article", title: "Die Hard", want: "hard"},
		{name: "article alone stays", title: "The", want: "the"},
		{name: "article with trailing space stays", title: "Das ", want: "das "},
		{name: "not a whole word", title: "Theory of Everything", want: "theory of everything"},
		{name: "prefix of eine", title: "Einer flog", want: "einer flog"},
		{name: "article not first", title: "Into The Wild", want: "into the wild"},
		{name: "leading space is kept", title: " The Matrix", want: " the matrix"},
		{name: "punctuation is not a separator", title: "The-Matrix", want: "the-matrix"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.SortTitle(test.title); got != test.want {
				t.Errorf("SortTitle(%q) = %q, want %q", test.title, got, test.want)
			}
		})
	}
}

func TestSortTitleFoldsCaseVariantsTogether(t *testing.T) {
	t.Parallel()

	variants := []string{"ÜBER FILM", "über film", "Über Film", "üBeR fIlM"}
	want := library.SortTitle(variants[0])

	for _, variant := range variants[1:] {
		if got := library.SortTitle(variant); got != want {
			t.Errorf("SortTitle(%q) = %q, want %q like %q", variant, got, want, variants[0])
		}
	}
}

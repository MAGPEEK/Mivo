package library_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestParseKindAcceptsKnownKinds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want library.Kind
	}{
		{text: "movies", want: library.KindMovies},
		{text: "shows", want: library.KindShows},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			got, err := library.ParseKind(test.text)
			if err != nil {
				t.Fatalf("ParseKind(%q) error = %v, want nil", test.text, err)
			}

			if got != test.want {
				t.Errorf("ParseKind(%q) = %q, want %q", test.text, got, test.want)
			}

			if got.String() != test.text {
				t.Errorf("String() = %q, want %q", got.String(), test.text)
			}

			if !got.IsValid() {
				t.Errorf("IsValid() = false for %q, want true", got)
			}
		})
	}
}

func TestParseKindRejectsUnknownKinds(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "movie", "Movies", "SHOWS", " shows", "music", "mixed"} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			got, err := library.ParseKind(text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ParseKind(%q) error = %v, want ErrInvalid", text, err)
			}

			if got != "" {
				t.Errorf("ParseKind(%q) = %q on error, want empty kind", text, got)
			}

			if library.Kind(text).IsValid() {
				t.Errorf("Kind(%q).IsValid() = true, want false", text)
			}
		})
	}
}

func TestDefinitionValidateAcceptsWellFormedDefinitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		definition library.Definition
	}{
		{
			name:       "linux movies",
			definition: library.Definition{Name: "Movies", Root: "/media/movies", Kind: library.KindMovies},
		},
		{
			name:       "windows shows",
			definition: library.Definition{Name: "Serien", Root: `D:\Serien`, Kind: library.KindShows},
		},
		{
			name:       "unicode name at the rune limit",
			definition: library.Definition{Name: strings.Repeat("ü", 128), Root: "/media", Kind: library.KindMovies},
		},
		{
			name:       "relative root is not checked here",
			definition: library.Definition{Name: "Videos", Root: "videos", Kind: library.KindMovies},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.definition.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestDefinitionValidateRejectsMalformedDefinitions(t *testing.T) {
	t.Parallel()

	valid := library.Definition{Name: "Movies", Root: "/media/movies", Kind: library.KindMovies}

	tests := []struct {
		name   string
		mutate func(*library.Definition)
	}{
		{name: "empty name", mutate: func(d *library.Definition) { d.Name = "" }},
		{name: "blank name", mutate: func(d *library.Definition) { d.Name = " \t " }},
		{name: "name too long", mutate: func(d *library.Definition) { d.Name = strings.Repeat("a", 129) }},
		{name: "name too many runes", mutate: func(d *library.Definition) { d.Name = strings.Repeat("ü", 129) }},
		{name: "name not utf-8", mutate: func(d *library.Definition) { d.Name = "bad\xff" }},
		{name: "empty root", mutate: func(d *library.Definition) { d.Root = "" }},
		{name: "root with nul", mutate: func(d *library.Definition) { d.Root = "/media\x00/movies" }},
		{name: "empty kind", mutate: func(d *library.Definition) { d.Kind = "" }},
		{name: "unknown kind", mutate: func(d *library.Definition) { d.Kind = "music" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			definition := valid
			test.mutate(&definition)

			if err := definition.Validate(); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}
}

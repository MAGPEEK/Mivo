package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestParseExtraTypeAcceptsTheEightTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want library.ExtraType
	}{
		{text: "trailer", want: library.ExtraTypeTrailer},
		{text: "featurette", want: library.ExtraTypeFeaturette},
		{text: "behind_the_scenes", want: library.ExtraTypeBehindTheScenes},
		{text: "deleted_scene", want: library.ExtraTypeDeletedScene},
		{text: "interview", want: library.ExtraTypeInterview},
		{text: "scene", want: library.ExtraTypeScene},
		{text: "short", want: library.ExtraTypeShort},
		{text: "other", want: library.ExtraTypeOther},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			got, err := library.ParseExtraType(test.text)
			if err != nil || got != test.want {
				t.Fatalf("ParseExtraType(%q) = %q, %v, want %q, nil", test.text, got, err, test.want)
			}

			if !got.IsValid() || got.String() != test.text {
				t.Errorf("IsValid() = %v, String() = %q, want true, %q", got.IsValid(), got.String(), test.text)
			}
		})
	}
}

func TestParseExtraTypeRejectsUnknownTypes(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "Trailer", "trailers", "behind-the-scenes", "deleted", "extra", " short"} {
		got, err := library.ParseExtraType(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseExtraType(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}

		if library.ExtraType(text).IsValid() {
			t.Errorf("ExtraType(%q).IsValid() = true, want false", text)
		}
	}
}

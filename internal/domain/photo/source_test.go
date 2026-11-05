package photo_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/photo"
)

func TestSourceOf(t *testing.T) {
	t.Parallel()

	if got := photo.SourceOf(photo.Info{TakenAt: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)}); got != photo.TakenAtExif {
		t.Errorf("with EXIF time = %q, want exif", got)
	}

	if got := photo.SourceOf(photo.Info{}); got != photo.TakenAtFile {
		t.Errorf("without EXIF time = %q, want file", got)
	}
}

func TestParseTakenAtSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text string
		want photo.TakenAtSource
		ok   bool
	}{
		{"exif", photo.TakenAtExif, true},
		{"file", photo.TakenAtFile, true},
		{"", "", false},
		{"EXIF", "", false},
		{"xmp", "", false},
	}

	for _, test := range cases {
		got, err := photo.ParseTakenAtSource(test.text)

		switch {
		case test.ok && (err != nil || got != test.want || got.String() != test.text):
			t.Errorf("ParseTakenAtSource(%q) = %q, %v, want %q", test.text, got, err, test.want)
		case !test.ok && !errors.Is(err, domain.ErrInvalid):
			t.Errorf("ParseTakenAtSource(%q) error = %v, want ErrInvalid", test.text, err)
		}
	}
}

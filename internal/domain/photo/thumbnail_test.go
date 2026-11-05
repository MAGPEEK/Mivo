package photo_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/photo"
)

func TestParseThumbnailState(t *testing.T) {
	t.Parallel()

	cases := []struct {
		text string
		want photo.ThumbnailState
		ok   bool
	}{
		{"ready", photo.ThumbnailReady, true},
		{"unsupported", photo.ThumbnailUnsupported, true},
		{"", "", false},
		{"Ready", "", false},
		{"failed", "", false},
	}

	for _, test := range cases {
		got, err := photo.ParseThumbnailState(test.text)

		switch {
		case test.ok && (err != nil || got != test.want):
			t.Errorf("ParseThumbnailState(%q) = %q, %v, want %q", test.text, got, err, test.want)
		case !test.ok && !errors.Is(err, domain.ErrInvalid):
			t.Errorf("ParseThumbnailState(%q) error = %v, want ErrInvalid", test.text, err)
		case test.ok && got.String() != test.text:
			t.Errorf("String() = %q, want %q", got.String(), test.text)
		}
	}
}

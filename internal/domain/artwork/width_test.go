package artwork_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestWidthsAreTheFixedSet(t *testing.T) {
	t.Parallel()

	want := []int{160, 320, 480, 640, 960, 1280, 1920}

	widths := artwork.Widths()
	got := make([]int, 0, len(widths))

	for _, width := range widths {
		got = append(got, width.Pixels())
	}

	if !slices.Equal(got, want) {
		t.Errorf("Widths() = %v, want %v", got, want)
	}
}

func TestParseWidth(t *testing.T) {
	t.Parallel()

	for _, want := range artwork.Widths() {
		got, err := artwork.ParseWidth(want.String())
		if err != nil || got != want {
			t.Errorf("ParseWidth(%q) = %d, %v, want %d, nil", want, got, err, want)
		}
	}

	invalid := []string{"", "0", "100", "161", "2560", "-160", "+160", "0160", "160px", " 160", "1e3"}

	for _, text := range invalid {
		got, err := artwork.ParseWidth(text)
		if !errors.Is(err, domain.ErrInvalid) || got != 0 {
			t.Errorf("ParseWidth(%q) = %d, %v, want 0, ErrInvalid", text, got, err)
		}
	}
}

func TestOutputFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind        artwork.Kind
		format      artwork.Format
		contentType string
		extension   string
	}{
		{kind: artwork.KindPoster, format: artwork.FormatJPEG, contentType: "image/jpeg", extension: "jpg"},
		{kind: artwork.KindBackdrop, format: artwork.FormatJPEG, contentType: "image/jpeg", extension: "jpg"},
		{kind: artwork.KindThumb, format: artwork.FormatJPEG, contentType: "image/jpeg", extension: "jpg"},
		{kind: artwork.KindBanner, format: artwork.FormatJPEG, contentType: "image/jpeg", extension: "jpg"},
		{kind: artwork.KindLogo, format: artwork.FormatPNG, contentType: "image/png", extension: "png"},
	}

	for _, test := range tests {
		format := artwork.OutputFormat(test.kind)
		if format != test.format || format.ContentType() != test.contentType || format.Extension() != test.extension {
			t.Errorf("OutputFormat(%q) = %q (%s, %s), want %q (%s, %s)", test.kind, format,
				format.ContentType(), format.Extension(), test.format, test.contentType, test.extension)
		}

		if format.String() != string(test.format) {
			t.Errorf("Format.String() = %q, want %q", format.String(), test.format)
		}
	}
}

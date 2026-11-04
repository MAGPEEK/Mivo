package metadata_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestParseDateRoundTrips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text  string
		year  int
		month time.Month
		day   int
	}{
		{text: "2024-01-05", year: 2024, month: time.January, day: 5},
		{text: "2024-02-29", year: 2024, month: time.February, day: 29},
		{text: "2000-12-31", year: 2000, month: time.December, day: 31},
		{text: "0001-01-01", year: 1, month: time.January, day: 1},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			t.Parallel()

			date, err := metadata.ParseDate(test.text)
			if err != nil {
				t.Fatalf("ParseDate(%q) error = %v, want nil", test.text, err)
			}

			if date.Year() != test.year || date.Month() != test.month || date.Day() != test.day {
				t.Errorf("ParseDate(%q) = %d-%d-%d, want %d-%d-%d", test.text,
					date.Year(), date.Month(), date.Day(), test.year, test.month, test.day)
			}

			if date.IsZero() {
				t.Errorf("ParseDate(%q).IsZero() = true, want false", test.text)
			}

			if got := date.String(); got != test.text {
				t.Errorf("ParseDate(%q).String() = %q, want the input", test.text, got)
			}
		})
	}
}

func TestParseDateRejectsMalformedAndImpossibleDates(t *testing.T) {
	t.Parallel()

	tests := []string{
		"", "2023-02-29", "2024-04-31", "2024-13-01", "2024-00-10", "2024-01-00",
		"2024-1-5", "24-01-05", "2024/01/05", "2024-01-05T00:00:00Z", " 2024-01-05",
		"0000-01-01", "05.01.2024",
	}

	for _, text := range tests {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			date, err := metadata.ParseDate(text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ParseDate(%q) error = %v, want ErrInvalid", text, err)
			}

			if !date.IsZero() {
				t.Errorf("ParseDate(%q) = %v on error, want the zero date", text, date)
			}
		})
	}
}

func TestZeroDateIsUnknown(t *testing.T) {
	t.Parallel()

	var date metadata.Date

	if !date.IsZero() || date.String() != "" || date.Year() != 0 || date.Month() != 0 || date.Day() != 0 {
		t.Errorf("zero Date = (zero %t, %q, %d, %d, %d), want unknown and empty",
			date.IsZero(), date.String(), date.Year(), date.Month(), date.Day())
	}
}

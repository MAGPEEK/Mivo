package metadata

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// minimumDateYear is the earliest year a Date accepts. Year 0 does not exist in the
// Gregorian calendar and only appears in broken NFO files.
const minimumDateYear = 1

// Date is a calendar date without time of day or time zone, such as a release or air date.
// The zero value means "unknown"; every other value is a real Gregorian date.
type Date struct {
	year  int
	month time.Month
	day   int
}

// ParseDate accepts the ISO 8601 calendar form "YYYY-MM-DD" with zero-padded month and day,
// for example "2024-01-05". Impossible dates such as "2023-02-29" and year 0 are rejected.
// The error wraps domain.ErrInvalid.
func ParseDate(text string) (Date, error) {
	parsed, err := time.Parse(time.DateOnly, text)
	if err != nil {
		return Date{}, fmt.Errorf("parse date %q: %w", text, domain.ErrInvalid)
	}

	if parsed.Year() < minimumDateYear {
		return Date{}, fmt.Errorf("parse date %q: year before %d: %w", text, minimumDateYear, domain.ErrInvalid)
	}

	return Date{year: parsed.Year(), month: parsed.Month(), day: parsed.Day()}, nil
}

// IsZero reports whether the date is unknown.
func (date Date) IsZero() bool {
	return date == Date{}
}

// Year returns the year, or 0 for an unknown date.
func (date Date) Year() int {
	return date.year
}

// Month returns the month, or 0 for an unknown date.
func (date Date) Month() time.Month {
	return date.month
}

// Day returns the day of the month, or 0 for an unknown date.
func (date Date) Day() int {
	return date.day
}

// String returns the "YYYY-MM-DD" form that ParseDate accepts, or "" for an unknown date.
func (date Date) String() string {
	if date.IsZero() {
		return ""
	}

	return fmt.Sprintf("%04d-%02d-%02d", date.year, int(date.month), date.day)
}

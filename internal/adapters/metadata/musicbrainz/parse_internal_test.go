package musicbrainz

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestParsePartialDate(t *testing.T) {
	tests := []struct {
		text     string
		wantYear int
		wantDate string
	}{
		{"", 0, ""},
		{"199", 0, ""},
		{"abcd", 0, ""},
		{"0000-01-01", 0, ""},
		{"1994", 1994, ""},
		{"2003-10", 2003, ""},
		{"1997-05-21", 1997, "1997-05-21"},
		{"1997-02-30", 1997, ""},
	}

	for _, test := range tests {
		t.Run(test.text, func(t *testing.T) {
			year, date := parsePartialDate(test.text)

			gotDate := ""
			if !date.IsZero() {
				gotDate = date.String()
			}

			if year != test.wantYear || gotDate != test.wantDate {
				t.Errorf("parsePartialDate(%q) = %d, %q; want %d, %q", test.text, year, gotDate, test.wantYear, test.wantDate)
			}
		})
	}
}

func TestNormalizeMBID(t *testing.T) {
	tests := []struct {
		id   string
		want string
		ok   bool
	}{
		{" B1392450-E666-3926-A536-22C65F834433 ", "b1392450-e666-3926-a536-22c65f834433", true},
		{"b1392450-e666-3926-a536-22c65f83443", "", false},
		{"b1392450xe666-3926-a536-22c65f834433", "", false},
		{"g1392450-e666-3926-a536-22c65f834433", "", false},
		{"b1392450-e666-3926-a536-22c65f83443/", "", false},
	}

	for _, test := range tests {
		t.Run(test.id, func(t *testing.T) {
			got, ok := normalizeMBID(test.id)
			if got != test.want || ok != test.ok {
				t.Errorf("normalizeMBID(%q) = %q, %v; want %q, %v", test.id, got, ok, test.want, test.ok)
			}
		})
	}
}

func TestGenresAreBounded(t *testing.T) {
	votes := []vote{{"a", 1}, {"b", 2}, {" ", 9}, {"c", 3}, {"d", 4}, {"e", 5}, {"f", 6}}

	if got, want := genres(votes, nil), []string{"f", "e", "d", "c", "b"}; !slices.Equal(got, want) {
		t.Errorf("genres = %v, want %v", got, want)
	}

	if got := genres(nil, nil); got != nil {
		t.Errorf("genres without votes = %v, want nil", got)
	}
}

func TestRealSleep(t *testing.T) {
	if err := realSleep(t.Context(), time.Millisecond); err != nil {
		t.Errorf("realSleep = %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := realSleep(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Errorf("realSleep after cancel = %v, want context.Canceled", err)
	}
}

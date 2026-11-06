package access_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

func mustLimit(t *testing.T, maxAge access.AgeBucket, hideUnrated bool) access.ContentLimit {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d): %v", maxAge, err)
	}

	return limit
}

func TestMaxAgeLimitRejectsInvalidBucket(t *testing.T) {
	t.Parallel()

	for _, age := range []access.AgeBucket{-1, 3, 13, 21} {
		if _, err := access.MaxAgeLimit(age, true); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("MaxAgeLimit(%d) error = %v, want ErrInvalid", age, err)
		}
	}
}

func TestContentLimitAllows(t *testing.T) {
	t.Parallel()

	teensHiding := mustLimit(t, access.Age12, true)
	teensShowing := mustLimit(t, access.Age12, false)

	tests := []struct {
		name   string
		limit  access.ContentLimit
		bucket access.AgeBucket
		known  bool
		want   bool
	}{
		{name: "no limit adult", limit: access.NoLimit(), bucket: access.Age18, known: true, want: true},
		{name: "no limit unrated", limit: access.NoLimit(), known: false, want: true},
		{name: "no age but hide unrated", limit: access.NoLimit().HidingUnrated(true), known: false, want: false},
		{name: "no age but hide unrated rated", limit: access.NoLimit().HidingUnrated(true), bucket: access.Age18, known: true, want: true},
		{name: "below max", limit: teensHiding, bucket: access.Age6, known: true, want: true},
		{name: "at max", limit: teensHiding, bucket: access.Age12, known: true, want: true},
		{name: "above max", limit: teensHiding, bucket: access.Age16, known: true, want: false},
		{name: "unrated hidden", limit: teensHiding, known: false, want: false},
		{name: "unrated shown", limit: teensShowing, known: false, want: true},
		{name: "young children all", limit: mustLimit(t, access.AgeAll, true), bucket: access.AgeAll, known: true, want: true},
		{name: "young children six", limit: mustLimit(t, access.AgeAll, true), bucket: access.Age6, known: true, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.limit.Allows(test.bucket, test.known); got != test.want {
				t.Errorf("Allows(%d, %t) = %t, want %t", test.bucket, test.known, got, test.want)
			}
		})
	}
}

func TestContentLimitAccessors(t *testing.T) {
	t.Parallel()

	limit := mustLimit(t, access.Age16, true)
	if age, ok := limit.MaxAge(); !ok || age != access.Age16 || !limit.HideUnrated() {
		t.Errorf("limit = %d, %t, hide %t; want 16, true, true", age, ok, limit.HideUnrated())
	}

	shown := limit.HidingUnrated(false)
	if shown.HideUnrated() || !limit.HideUnrated() {
		t.Error("HidingUnrated must return a changed copy and keep the receiver")
	}

	if _, ok := access.NoLimit().MaxAge(); ok {
		t.Error("NoLimit has a max age")
	}
}

func TestStricter(t *testing.T) {
	t.Parallel()

	none := access.NoLimit()
	hideOnly := access.NoLimit().HidingUnrated(true)
	six := mustLimit(t, access.Age6, true)
	sixShowing := mustLimit(t, access.Age6, false)
	sixteenShowing := mustLimit(t, access.Age16, false)

	tests := []struct {
		name          string
		first, second access.ContentLimit
		want          access.ContentLimit
	}{
		{name: "none and none", first: none, second: none, want: none},
		{name: "none and six", first: none, second: six, want: six},
		{name: "six and none", first: six, second: none, want: six},
		{name: "lower age wins", first: sixteenShowing, second: sixShowing, want: sixShowing},
		{name: "lower age wins reversed", first: sixShowing, second: sixteenShowing, want: sixShowing},
		{name: "hide unrated from either", first: sixteenShowing, second: hideOnly, want: mustLimit(t, access.Age16, true)},
		{name: "hide and age combined", first: sixteenShowing, second: six, want: six},
		{name: "hide only and none", first: hideOnly, second: none, want: hideOnly},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := access.Stricter(test.first, test.second); got != test.want {
				t.Errorf("Stricter = %+v, want %+v", got, test.want)
			}
		})
	}
}

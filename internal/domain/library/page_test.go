package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestNormalizeLimitAcceptsValidLimits(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "unset uses default", limit: 0, want: library.DefaultPageLimit},
		{name: "minimum", limit: 1, want: 1},
		{name: "default", limit: 100, want: 100},
		{name: "maximum", limit: library.MaxPageLimit, want: library.MaxPageLimit},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := library.NormalizeLimit(test.limit)
			if err != nil {
				t.Fatalf("NormalizeLimit(%d) error = %v, want nil", test.limit, err)
			}

			if got != test.want {
				t.Errorf("NormalizeLimit(%d) = %d, want %d", test.limit, got, test.want)
			}
		})
	}
}

func TestNormalizeLimitRejectsOutOfRangeLimits(t *testing.T) {
	t.Parallel()

	for _, limit := range []int{-1, -100, library.MaxPageLimit + 1, 10_000} {
		got, err := library.NormalizeLimit(limit)
		if !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("NormalizeLimit(%d) error = %v, want ErrInvalid", limit, err)
		}

		if got != 0 {
			t.Errorf("NormalizeLimit(%d) = %d on error, want 0", limit, got)
		}
	}
}

func TestPageLimitConstantsMatchTheAPIContract(t *testing.T) {
	t.Parallel()

	if library.DefaultPageLimit != 100 || library.MaxPageLimit != 500 {
		t.Errorf("DefaultPageLimit, MaxPageLimit = %d, %d, want 100, 500",
			library.DefaultPageLimit, library.MaxPageLimit)
	}
}

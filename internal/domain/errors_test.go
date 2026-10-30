package domain_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

func TestSentinelErrorsAreDistinctAndSurviveWrapping(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		domain.ErrNotFound, domain.ErrConflict, domain.ErrInvalid, domain.ErrForbidden,
		domain.ErrUnsupported, domain.ErrUnavailable,
	}

	for index, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			t.Parallel()

			wrapped := fmt.Errorf("operation context: %w", sentinel)
			if !errors.Is(wrapped, sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", wrapped, sentinel)
			}

			for otherIndex, other := range sentinels {
				if otherIndex != index && errors.Is(wrapped, other) {
					t.Errorf("errors.Is(%v, %v) = true, want false", wrapped, other)
				}
			}
		})
	}
}

package account_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/account"
)

// TestConflictSentinelsWrapErrConflict: the HTTP adapter tells last_admin, profile_limit and
// name_taken apart, while every other caller still sees a conflict.
func TestConflictSentinelsWrapErrConflict(t *testing.T) {
	t.Parallel()

	sentinels := []error{account.ErrLastAdmin, account.ErrProfileLimit, account.ErrNameTaken}

	for index, sentinel := range sentinels {
		if !errors.Is(sentinel, domain.ErrConflict) {
			t.Errorf("%v does not wrap ErrConflict", sentinel)
		}

		for other, different := range sentinels {
			if index != other && errors.Is(sentinel, different) {
				t.Errorf("%v matches %v, want distinct sentinels", sentinel, different)
			}
		}
	}
}

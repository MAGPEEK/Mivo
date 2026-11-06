package access_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

func TestProfileViewerFrom(t *testing.T) {
	t.Parallel()

	withProfile := domainaccess.Viewer{AccountID: "account-1", ProfileID: "profile-1"}

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr error
	}{
		{"no viewer", context.Background(), domain.ErrUnauthenticated},
		{"no account", access.WithViewer(context.Background(), domainaccess.Viewer{ProfileID: "profile-1"}), domain.ErrUnauthenticated},
		{"no profile", access.WithViewer(context.Background(), domainaccess.Viewer{AccountID: "account-1"}), access.ErrProfileRequired},
		{"with profile", access.WithViewer(context.Background(), withProfile), nil},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := access.ProfileViewerFrom(test.ctx)
			if !errors.Is(err, test.wantErr) || (test.wantErr == nil) != (err == nil) {
				t.Fatalf("ProfileViewerFrom error = %v, want %v", err, test.wantErr)
			}

			if test.wantErr == nil && got.ProfileID != withProfile.ProfileID {
				t.Fatalf("ProfileViewerFrom = %+v, want %+v", got, withProfile)
			}
		})
	}
}

func TestErrProfileRequiredIsForbidden(t *testing.T) {
	t.Parallel()

	if !errors.Is(access.ErrProfileRequired, domain.ErrForbidden) {
		t.Fatal("ErrProfileRequired does not wrap domain.ErrForbidden")
	}
}

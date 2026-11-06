package access_test

import (
	"context"
	"testing"

	"github.com/magpeek/mivo/internal/app/access"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

func TestViewerFromReturnsTheStoredViewer(t *testing.T) {
	t.Parallel()

	want := domainaccess.Viewer{AuthSessionID: "session-1", AccountID: "account-1", ProfileID: "profile-1", Admin: true}

	got, ok := access.ViewerFrom(access.WithViewer(context.Background(), want))
	if !ok {
		t.Fatal("ViewerFrom reported no viewer after WithViewer")
	}

	if got.AuthSessionID != want.AuthSessionID || got.AccountID != want.AccountID ||
		got.ProfileID != want.ProfileID || got.Admin != want.Admin {
		t.Fatalf("ViewerFrom = %+v, want %+v", got, want)
	}
}

func TestViewerFromWithoutViewerReportsNone(t *testing.T) {
	t.Parallel()

	got, ok := access.ViewerFrom(context.Background())
	if ok {
		t.Fatalf("ViewerFrom on a bare context = %+v, true; want none", got)
	}

	if got.Admin || got.AccountID != "" {
		t.Fatalf("ViewerFrom returned a non-zero viewer %+v", got)
	}
}

func TestWithViewerReplacesAnEarlierViewer(t *testing.T) {
	t.Parallel()

	ctx := access.WithViewer(context.Background(), domainaccess.Viewer{AccountID: "first"})
	ctx = access.WithViewer(ctx, domainaccess.Viewer{AccountID: "second"})

	got, _ := access.ViewerFrom(ctx)
	if got.AccountID != "second" {
		t.Fatalf("AccountID = %q, want second", got.AccountID)
	}
}

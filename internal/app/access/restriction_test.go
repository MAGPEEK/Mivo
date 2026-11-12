package access_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
)

func TestRestrictionFromWithoutViewerFailsClosed(t *testing.T) {
	t.Parallel()

	if _, err := access.RestrictionFrom(context.Background()); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("RestrictionFrom error = %v, want ErrUnauthenticated", err)
	}

	if _, err := access.LibraryRestriction(context.Background(), "lib"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("LibraryRestriction error = %v, want ErrUnauthenticated", err)
	}
}

func TestLibraryRestrictionChecksTheLibrary(t *testing.T) {
	t.Parallel()

	viewer := domainaccess.Viewer{AccountID: "a", Libraries: domainaccess.OnlyLibraries("allowed")}
	ctx := access.WithViewer(context.Background(), viewer)

	if _, err := access.LibraryRestriction(ctx, "allowed"); err != nil {
		t.Fatalf("allowed library: %v", err)
	}

	if _, err := access.LibraryRestriction(ctx, "other"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("other library error = %v, want ErrNotFound", err)
	}

	// A viewer without a profile still reads the catalog (the resolver decides the access).
	restriction, err := access.RestrictionFrom(ctx)
	if err != nil || restriction.AllowsLibrary("other") {
		t.Fatalf("RestrictionFrom = %+v, %v", restriction, err)
	}
}

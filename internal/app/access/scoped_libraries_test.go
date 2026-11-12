package access_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	domainaccess "github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

type fakeLibraryReader struct {
	libraries []library.ManagedLibrary
	calls     int
}

func (reader *fakeLibraryReader) ListManaged(context.Context) ([]library.ManagedLibrary, error) {
	reader.calls++

	return reader.libraries, nil
}

func (reader *fakeLibraryReader) GetManaged(_ context.Context, id string) (library.ManagedLibrary, error) {
	reader.calls++

	for _, managed := range reader.libraries {
		if managed.ID == id {
			return managed, nil
		}
	}

	return library.ManagedLibrary{}, fmt.Errorf("library %q: %w", id, domain.ErrNotFound)
}

func newFakeLibraryReader() *fakeLibraryReader {
	return &fakeLibraryReader{libraries: []library.ManagedLibrary{{ID: "movies"}, {ID: "shows"}, {ID: "music"}}}
}

func TestScopedLibrariesListTheViewersAccessSet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		viewer domainaccess.Viewer
		want   []string
	}{
		{name: "all", viewer: domainaccess.Viewer{Libraries: domainaccess.AllLibraries()}, want: []string{"movies", "shows", "music"}},
		{name: "two", viewer: domainaccess.Viewer{Libraries: domainaccess.OnlyLibraries("music", "movies", "gone")}, want: []string{"movies", "music"}},
		{name: "none", viewer: domainaccess.Viewer{}, want: nil},
	}
	for _, test := range tests {
		reader := newFakeLibraryReader()
		source := access.NewScopedLibraries(reader)

		got, err := source.VisibleLibraries(access.WithViewer(t.Context(), test.viewer))
		if err != nil {
			t.Fatalf("%s: error = %v", test.name, err)
		}

		var ids []string
		for _, managed := range got {
			ids = append(ids, managed.ID)
		}

		if fmt.Sprint(ids) != fmt.Sprint(test.want) {
			t.Errorf("%s: libraries = %v, want %v", test.name, ids, test.want)
		}
	}
}

func TestScopedLibrariesFailClosed(t *testing.T) {
	t.Parallel()

	reader := newFakeLibraryReader()
	source := access.NewScopedLibraries(reader)

	if _, err := source.VisibleLibraries(t.Context()); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("VisibleLibraries without viewer: %v, want ErrUnauthenticated", err)
	}

	if _, err := source.VisibleLibrary(t.Context(), "movies"); !errors.Is(err, domain.ErrUnauthenticated) {
		t.Errorf("VisibleLibrary without viewer: %v, want ErrUnauthenticated", err)
	}

	ctx := access.WithViewer(t.Context(), domainaccess.Viewer{Libraries: domainaccess.OnlyLibraries("movies")})

	if _, err := source.VisibleLibrary(ctx, "shows"); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("VisibleLibrary outside the set: %v, want ErrNotFound", err)
	}

	if _, err := source.VisibleLibrary(ctx, "movies"); err != nil {
		t.Errorf("VisibleLibrary inside the set: %v", err)
	}

	if reader.calls != 1 {
		t.Errorf("store calls = %d, want 1 (only the allowed library is read)", reader.calls)
	}
}

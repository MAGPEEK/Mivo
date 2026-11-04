package library_test

import (
	"errors"
	"strings"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// IDs of stored entities.
const (
	moviesID  = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	showsID   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	unknownID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
)

// Folders of the stored libraries; the service treats them as opaque.
const (
	moviesRoot = "/media/movies"
	showsRoot  = "/media/shows"
)

var errBoom = errors.New("boom")

// testEnvironment bundles the fakes behind one Service.
type testEnvironment struct {
	store    *memoryLibraryReader
	items    *memoryItemReader
	children *memoryChildReader // nil leaves Dependencies.Children unset
}

func newTestEnvironment() *testEnvironment {
	return &testEnvironment{
		store:    &memoryLibraryReader{},
		items:    newMemoryItemReader(),
		children: &memoryChildReader{},
	}
}

func (environment *testEnvironment) service(t *testing.T) *applibrary.Service {
	t.Helper()

	dependencies := applibrary.Dependencies{Libraries: environment.store, Items: environment.items}
	// A nil *memoryChildReader in the interface would not be a missing port.
	if environment.children != nil {
		dependencies.Children = environment.children
	}

	service, err := applibrary.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	return service
}

// storedMoviesAndShows puts two libraries into the store.
func (environment *testEnvironment) storedMoviesAndShows() (movies, shows library.ManagedLibrary) {
	movies = library.ManagedLibrary{
		ID: moviesID, Name: "Movies", Kind: library.KindMovies,
		Folders: []library.Folder{{ID: "movies-folder", Path: moviesRoot}},
	}
	shows = library.ManagedLibrary{
		ID: showsID, Name: "Shows", Kind: library.KindShows,
		Folders: []library.Folder{{ID: "shows-folder", Path: showsRoot}},
	}
	environment.store.put(movies, shows)

	return movies, shows
}

func TestNewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mutate   func(*applibrary.Dependencies)
		wantText string
	}{
		{name: "library reader", mutate: func(d *applibrary.Dependencies) { d.Libraries = nil }, wantText: "library reader"},
		{name: "item reader", mutate: func(d *applibrary.Dependencies) { d.Items = nil }, wantText: "item reader"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			dependencies := applibrary.Dependencies{Libraries: &memoryLibraryReader{}, Items: newMemoryItemReader()}
			test.mutate(&dependencies)

			service, err := applibrary.NewService(dependencies)
			assertWrappedError(t, err, domain.ErrInvalid, test.wantText)

			if service != nil {
				t.Error("NewService returned a service together with an error")
			}
		})
	}
}

func TestNewServiceReportsEveryMissingDependency(t *testing.T) {
	t.Parallel()

	_, err := applibrary.NewService(applibrary.Dependencies{})
	if err == nil {
		t.Fatal("NewService(empty) error = nil, want error")
	}

	for _, name := range []string{"library reader", "item reader"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("NewService(empty) error = %q, want it to name %q", err, name)
		}
	}
}

func assertWrappedError(t *testing.T, err, wantErr error, wantText string) {
	t.Helper()

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want it to wrap %v", err, wantErr)
	}

	if !strings.Contains(err.Error(), wantText) {
		t.Errorf("error = %q, want context %q", err, wantText)
	}
}

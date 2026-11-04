package library_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	domainlibrary "github.com/magpeek/mivo/internal/domain/library"
)

type managerFixture struct {
	manager *library.Manager
	store   *memoryManaged
	browser *fakeBrowser
	scans   *fakeScans
	log     *callLog
}

func newManagerFixture(t *testing.T, existing ...domainlibrary.ManagedLibrary) *managerFixture {
	t.Helper()

	log := &callLog{}
	fixture := &managerFixture{
		store: newMemoryManaged(log, existing...),
		browser: &fakeBrowser{
			roots:  []library.BrowseRoot{{ID: "r1", Path: "/media", Available: true}},
			checks: map[string]error{},
		},
		scans: &fakeScans{log: log},
		log:   log,
	}

	manager, err := library.NewManager(library.ManagerDependencies{
		Catalog: fixture.store, Writer: fixture.store, Ledger: fixture.store,
		Browser: fixture.browser, Scans: fixture.scans, Events: fakeEvents{log: log}, Suspender: fakeSuspender{log: log},
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		AllowedKinds: []domainlibrary.Kind{domainlibrary.KindMovies, domainlibrary.KindShows},
		Reserved:     []string{"/data"},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	fixture.manager = manager

	return fixture
}

func existingLibrary(id, name, path string) domainlibrary.ManagedLibrary {
	return domainlibrary.ManagedLibrary{
		ID: id, Name: name, Kind: domainlibrary.KindMovies,
		Folders: []domainlibrary.Folder{{ID: id + "-f0", Path: path}}, Options: domainlibrary.DefaultOptions(),
	}
}

func createMovies(name string, folders ...library.FolderRef) library.CreateRequest {
	return library.CreateRequest{Name: name, Kind: domainlibrary.KindMovies, Folders: folders}
}

func rootRef(path string) library.FolderRef { return library.FolderRef{RootID: "r1", Path: path} }

func TestNewManagerRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	_, err := library.NewManager(library.ManagerDependencies{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestCreateLibrary(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)

	created, err := fixture.manager.CreateLibrary(context.Background(), createMovies("Films", rootRef("films")))
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	if created.Options != domainlibrary.DefaultOptions() {
		t.Errorf("options = %+v, want defaults", created.Options)
	}

	if created.Folders[0].Path != "/media/films" {
		t.Errorf("folder path = %q", created.Folders[0].Path)
	}

	want := []string{"store.create", "event.added new-1", "scan.start new-1"}
	if got := fixture.log.all(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestCreateLibraryKeepsGivenOptionsAndLogsScanStartFailure(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)
	fixture.scans.startErr = errors.New("busy")
	request := createMovies("Films", rootRef("films"))
	request.Options = &domainlibrary.Options{ReadNFO: true}

	created, err := fixture.manager.CreateLibrary(context.Background(), request)
	if err != nil {
		t.Fatalf("CreateLibrary: %v", err)
	}

	if created.Options != (domainlibrary.Options{ReadNFO: true}) {
		t.Errorf("options = %+v", created.Options)
	}
}

func TestCreateLibraryFolderErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		checkAt string
		checkIs error
		want    library.FolderProblem
	}{
		{"missing", "b", domain.ErrNotFound, library.ProblemNotFound},
		{"forbidden", "b", domain.ErrForbidden, library.ProblemUnreadable},
		{"invalid", "b", domain.ErrInvalid, library.ProblemOutsideRoots},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newManagerFixture(t)
			fixture.browser.checks["r1:"+test.checkAt] = test.checkIs

			_, err := fixture.manager.CreateLibrary(context.Background(),
				createMovies("Films", rootRef("a"), rootRef(test.checkAt)))

			var folderErr *library.FolderCheckError
			if !errors.As(err, &folderErr) || folderErr.Index != 1 || folderErr.Problem != test.want {
				t.Fatalf("err = %v, want index 1 %s", err, test.want)
			}

			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}

			if len(fixture.log.all()) != 0 {
				t.Errorf("side effects: %v", fixture.log.all())
			}
		})
	}
}

func TestCreateLibraryUnavailableRootPassesOn(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)
	fixture.browser.checks["r1:a"] = domain.ErrUnavailable

	_, err := fixture.manager.CreateLibrary(context.Background(), createMovies("Films", rootRef("a")))
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestCreateLibraryRejectsBadReferences(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)

	_, err := fixture.manager.CreateLibrary(context.Background(),
		createMovies("Films", library.FolderRef{Path: "a"}))
	assertFolderProblem(t, err, library.ProblemReferenceShape)

	_, err = fixture.manager.CreateLibrary(context.Background(),
		createMovies("Films", library.FolderRef{FolderID: "nope"}))
	assertFolderProblem(t, err, library.ProblemUnknownFolder)
}

func assertFolderProblem(t *testing.T, err error, want library.FolderProblem) {
	t.Helper()

	var folderErr *library.FolderCheckError
	if !errors.As(err, &folderErr) || folderErr.Problem != want {
		t.Fatalf("err = %v, want problem %s", err, want)
	}
}

func TestCreateLibraryValidation(t *testing.T) {
	t.Parallel()

	other := existingLibrary("lib-a", "Movies", "/media/films")

	t.Run("overlap names the other library", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, other)

		_, err := fixture.manager.CreateLibrary(context.Background(), createMovies("New", rootRef("films/sub")))

		var folderErr *domainlibrary.FolderError
		if !errors.As(err, &folderErr) || folderErr.Reason != domainlibrary.ReasonOverlap || folderErr.OtherLibraryID != "lib-a" {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("reserved", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t)
		fixture.browser.roots[0].Path = "/"

		_, err := fixture.manager.CreateLibrary(context.Background(), createMovies("New", rootRef("data")))

		var folderErr *domainlibrary.FolderError
		if !errors.As(err, &folderErr) || folderErr.Reason != domainlibrary.ReasonReserved {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unsupported kind", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t)
		request := createMovies("New", rootRef("x"))
		request.Kind = "photos"

		_, err := fixture.manager.CreateLibrary(context.Background(), request)

		var fieldErr *domainlibrary.FieldError
		if !errors.As(err, &fieldErr) || fieldErr.Field != domainlibrary.FieldKind {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("duplicate name is a conflict", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, other)

		_, err := fixture.manager.CreateLibrary(context.Background(), createMovies("movies", rootRef("other")))
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("err = %v, want ErrConflict", err)
		}
	})

	t.Run("store conflict becomes folder overlap", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t)
		fixture.store.createErr = domain.ErrConflict

		_, err := fixture.manager.CreateLibrary(context.Background(), createMovies("New", rootRef("x")))

		var folderErr *domainlibrary.FolderError
		if !errors.As(err, &folderErr) || folderErr.Reason != domainlibrary.ReasonOverlap {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestUpdateLibrary(t *testing.T) {
	t.Parallel()

	newName := "Renamed"
	newOptions := domainlibrary.Options{ReadNFO: true}
	keepAndAdd := []library.FolderRef{{FolderID: "lib-a-f0"}, rootRef("more")}
	replace := []library.FolderRef{rootRef("elsewhere")}

	tests := []struct {
		name       string
		patch      library.UpdatePatch
		wantCalls  []string
		wantFolder int
	}{
		{
			"rename only",
			library.UpdatePatch{Name: &newName},
			[]string{"store.update", "event.changed lib-a"},
			1,
		},
		{
			"options rescan",
			library.UpdatePatch{Options: &newOptions},
			[]string{"trigger.suspend lib-a", "scan.cancel lib-a", "scan.wait lib-a", "store.update", "event.changed lib-a", "scan.start lib-a"},
			1,
		},
		{
			"keep and add folder",
			library.UpdatePatch{Folders: &keepAndAdd},
			[]string{"trigger.suspend lib-a", "scan.cancel lib-a", "scan.wait lib-a", "store.update", "event.changed lib-a", "scan.start lib-a"},
			2,
		},
		{
			"replace folder",
			library.UpdatePatch{Folders: &replace},
			[]string{"trigger.suspend lib-a", "scan.cancel lib-a", "scan.wait lib-a", "store.update", "event.changed lib-a", "scan.start lib-a"},
			1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newManagerFixture(t, existingLibrary("lib-a", "Movies", "/media/films"))

			updated, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", test.patch)
			if err != nil {
				t.Fatalf("UpdateLibrary: %v", err)
			}

			if got := fixture.log.all(); !slices.Equal(got, test.wantCalls) {
				t.Errorf("calls = %v, want %v", got, test.wantCalls)
			}

			if len(updated.Folders) != test.wantFolder || updated.Kind != domainlibrary.KindMovies {
				t.Errorf("updated = %+v", updated)
			}
		})
	}
}

func TestUpdateLibraryOwnFolderIsNotAnOverlap(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t, existingLibrary("lib-a", "Movies", "/media/films"))
	folders := []library.FolderRef{rootRef("films")}

	if _, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a",
		library.UpdatePatch{Folders: &folders}); err != nil {
		t.Fatalf("UpdateLibrary: %v", err)
	}

	if slices.Contains(fixture.log.all(), "scan.start lib-a") != true {
		t.Errorf("new folder entry should rescan: %v", fixture.log.all())
	}
}

func TestUpdateLibraryErrors(t *testing.T) {
	t.Parallel()

	a := existingLibrary("lib-a", "Movies", "/media/films")
	b := existingLibrary("lib-b", "Shows", "/media/shows")

	t.Run("unknown library", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t)

		_, err := fixture.manager.UpdateLibrary(context.Background(), "nope", library.UpdatePatch{})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("unknown folder id", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, a)
		folders := []library.FolderRef{{FolderID: "other-f0"}}

		_, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", library.UpdatePatch{Folders: &folders})
		assertFolderProblem(t, err, library.ProblemUnknownFolder)
	})

	t.Run("overlap with another library", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, a, b)
		folders := []library.FolderRef{rootRef("shows")}

		_, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", library.UpdatePatch{Folders: &folders})

		var folderErr *domainlibrary.FolderError
		if !errors.As(err, &folderErr) || folderErr.OtherLibraryID != "lib-b" {
			t.Fatalf("err = %v", err)
		}

		if len(fixture.log.all()) != 0 {
			t.Errorf("side effects: %v", fixture.log.all())
		}
	})

	t.Run("name taken by another", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, a, b)
		name := "Shows"

		_, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", library.UpdatePatch{Name: &name})
		if !errors.Is(err, domain.ErrConflict) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("scan does not stop", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, a)
		fixture.scans.waitErr = context.DeadlineExceeded
		options := domainlibrary.Options{}

		_, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", library.UpdatePatch{Options: &options})
		if !errors.Is(err, domain.ErrUnavailable) || slices.Contains(fixture.log.all(), "store.update") {
			t.Fatalf("err = %v, calls %v", err, fixture.log.all())
		}
	})

	t.Run("store conflict", func(t *testing.T) {
		t.Parallel()

		fixture := newManagerFixture(t, a)
		fixture.store.updateErr = domain.ErrConflict
		name := "X"

		_, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", library.UpdatePatch{Name: &name})

		var folderErr *domainlibrary.FolderError
		if !errors.As(err, &folderErr) {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestRemoveLibrary(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t, existingLibrary("lib-a", "Movies", "/media/films"))

	if err := fixture.manager.RemoveLibrary(context.Background(), "lib-a"); err != nil {
		t.Fatalf("RemoveLibrary: %v", err)
	}

	want := []string{"trigger.suspend lib-a", "scan.cancel lib-a", "scan.wait lib-a", "store.delete", "event.removed lib-a", "scan.forget lib-a"}
	if got := fixture.log.all(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}

	err := fixture.manager.RemoveLibrary(context.Background(), "lib-a")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("second remove err = %v, want ErrNotFound", err)
	}
}

func TestUpdateLibraryRestartsScanWhenStoreWriteFails(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t, existingLibrary("lib-a", "Movies", "/media/films"))
	fixture.store.updateErr = errors.New("disk full")
	options := domainlibrary.Options{}

	_, err := fixture.manager.UpdateLibrary(context.Background(), "lib-a", library.UpdatePatch{Options: &options})
	if err == nil {
		t.Fatal("want error")
	}

	want := []string{"trigger.suspend lib-a", "scan.cancel lib-a", "scan.wait lib-a", "store.update", "event.changed lib-a", "scan.start lib-a"}
	if got := fixture.log.all(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestRemoveLibraryKeepsLibraryWhenScanDoesNotStop(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t, existingLibrary("lib-a", "Movies", "/media/films"))
	fixture.scans.waitErr = context.DeadlineExceeded

	err := fixture.manager.RemoveLibrary(context.Background(), "lib-a")
	if !errors.Is(err, domain.ErrUnavailable) || slices.Contains(fixture.log.all(), "store.delete") {
		t.Fatalf("err = %v, calls %v", err, fixture.log.all())
	}
}

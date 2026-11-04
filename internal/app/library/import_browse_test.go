package library_test

import (
	"context"
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	domainlibrary "github.com/magpeek/mivo/internal/domain/library"
)

func definition(name, root string) domainlibrary.Definition {
	return domainlibrary.Definition{Name: name, Kind: domainlibrary.KindMovies, Root: root}
}

func TestImportConfigured(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t, existingLibrary("lib-a", "Taken", "/media/used"))
	definitions := []domainlibrary.Definition{
		definition("Films", "/media/films"),
		definition("Used", "/media/used"),
		definition("Taken", "/media/other"),
		definition("Done", "/media/done"),
	}
	fixture.store.imported["/media/done"] = true

	result, err := fixture.manager.ImportConfigured(context.Background(), definitions)
	if err != nil {
		t.Fatalf("ImportConfigured: %v", err)
	}

	if len(result.Created) != 1 || !result.Created[0].ImportedFromConfig || result.Created[0].Name != "Films" {
		t.Errorf("created = %+v", result.Created)
	}

	if len(result.Skipped) != 2 {
		t.Errorf("skipped = %v", result.Skipped)
	}

	for _, root := range []string{"/media/films", "/media/used", "/media/other", "/media/done"} {
		if !fixture.store.imported[root] {
			t.Errorf("root %s not recorded", root)
		}
	}

	again, err := fixture.manager.ImportConfigured(context.Background(), definitions)
	if err != nil || len(again.Created) != 0 || len(again.Skipped) != 0 {
		t.Errorf("second import = %+v, %v", again, err)
	}

	if len(fixture.store.libraries) != 2 {
		t.Errorf("libraries = %d, want 2 (existing + Films)", len(fixture.store.libraries))
	}
}

func TestImportConfiguredInvalidDefinitionWritesNothing(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)
	definitions := []domainlibrary.Definition{definition("Films", "/media/films"), definition("", "/media/x")}

	_, err := fixture.manager.ImportConfigured(context.Background(), definitions)
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}

	if len(fixture.log.all()) != 0 || len(fixture.store.imported) != 0 {
		t.Errorf("wrote something: %v %v", fixture.log.all(), fixture.store.imported)
	}
}

func TestImportConfiguredReservedRootFails(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)

	_, err := fixture.manager.ImportConfigured(context.Background(),
		[]domainlibrary.Definition{definition("Data", "/data/media")})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestBrowseRoots(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)

	roots, err := fixture.manager.BrowseRoots(context.Background())
	if err != nil || len(roots) != 1 || roots[0].ID != "r1" {
		t.Fatalf("roots = %+v, %v", roots, err)
	}
}

func TestBrowseFoldersMarksUnselectable(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t,
		existingLibrary("lib-a", "Movies", "/media/films"),
		existingLibrary("lib-b", "Deep", "/tv/shows"))
	fixture.browser.roots[0].Path = "/"
	fixture.browser.listing = library.FolderListing{
		Names:           []string{"data", "media", "free", "tv"},
		UnreadableCount: 2,
		Truncated:       true,
	}

	page, err := fixture.manager.BrowseFolders(context.Background(), "r1", "")
	if err != nil {
		t.Fatalf("BrowseFolders: %v", err)
	}

	want := []library.BrowsedFolder{
		{Name: "data", Reason: library.BrowseReserved},
		{Name: "media", Reason: library.BrowseInUse},
		{Name: "free", Selectable: true},
		{Name: "tv", Reason: library.BrowseInUse},
	}

	for index, folder := range want {
		if page.Folders[index] != folder {
			t.Errorf("folder %d = %+v, want %+v", index, page.Folders[index], folder)
		}
	}

	if page.UnreadableCount != 2 || !page.Truncated {
		t.Errorf("page = %+v", page)
	}
}

func TestBrowseFoldersUnknownRoot(t *testing.T) {
	t.Parallel()

	fixture := newManagerFixture(t)

	_, err := fixture.manager.BrowseFolders(context.Background(), "nope", "")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound (the API answers 404 for an unknown root)", err)
	}
}

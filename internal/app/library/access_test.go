package library_test

import (
	"errors"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

var errMustNotBeCalled = errors.New("the store must not be called for a denied library")

// poisonedEnvironment fails every store, reader and file-system call with
// errMustNotBeCalled, so a denied library must answer ErrNotFound before reaching any.
func poisonedEnvironment() *browseEnvironment {
	environment := newBrowseEnvironment()
	environment.store.getErr = errMustNotBeCalled
	environment.items.listErr = errMustNotBeCalled
	environment.listings.err = errMustNotBeCalled
	environment.folderItems.err = errMustNotBeCalled
	environment.directories.err = errMustNotBeCalled

	return environment
}

func TestEveryUseCaseWithoutViewerFailsClosed(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	environment.putShow()

	service := environment.service(t)
	ctx := t.Context()
	folder := mustParseFolderPath(t, "movies-folder")

	calls := map[string]func() error{
		"ListItems": func() error {
			_, err := service.ListItems(ctx, moviesID, library.PageRequest{}, library.ListingQuery{})

			return err
		},
		"ListFacets": func() error {
			_, err := service.ListFacets(ctx, moviesID, applibrary.FacetRequest{})

			return err
		},
		"ListLetters": func() error {
			_, err := service.ListLetters(ctx, moviesID, applibrary.LetterRequest{})

			return err
		},
		"ListFolder": func() error {
			_, err := service.ListFolder(ctx, moviesID, folder, library.PageRequest{})

			return err
		},
		"GetItem": func() error {
			_, err := service.GetItem(ctx, showID)

			return err
		},
		"ListChildren": func() error {
			_, err := service.ListChildren(ctx, showID, applibrary.ChildPageRequest{})

			return err
		},
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, domain.ErrUnauthenticated) {
			t.Errorf("%s without a viewer: error = %v, want ErrUnauthenticated", name, err)
		}
	}

	calledReaders := len(environment.items.recordedRequests()) + len(environment.children.requests) + len(environment.listings.facetRequests)
	if calledReaders != 0 {
		t.Errorf("readers were called %d times without a viewer", calledReaders)
	}
}

func TestADeniedLibraryAnswersNotFoundBeforeAnyValidationOrStoreCall(t *testing.T) {
	t.Parallel()

	// The viewer may read the shows library only; moviesID is denied.
	ctx := limitedContext(t, access.Age18, false, showsID)
	mismatch := library.ListingFilter{Kind: library.ItemKindShow}
	folder := mustParseFolderPath(t, "movies-folder/Anything")

	tests := []struct {
		name string
		call func(service *applibrary.Service) error
	}{
		{"items with an invalid cursor and limit", func(service *applibrary.Service) error {
			_, err := service.ListItems(ctx, moviesID, library.PageRequest{Limit: -5, Cursor: "garbage"}, library.ListingQuery{Filter: mismatch})

			return err
		}},
		{"facets with a mismatching kind", func(service *applibrary.Service) error {
			_, err := service.ListFacets(ctx, moviesID, applibrary.FacetRequest{Filter: mismatch})

			return err
		}},
		{"letters with a mismatching kind", func(service *applibrary.Service) error {
			_, err := service.ListLetters(ctx, moviesID, applibrary.LetterRequest{Filter: mismatch})

			return err
		}},
		{"folder view with an invalid cursor", func(service *applibrary.Service) error {
			_, err := service.ListFolder(ctx, moviesID, folder, library.PageRequest{Limit: 9999, Cursor: "garbage"})

			return err
		}},
		{"folder view at the top", func(service *applibrary.Service) error {
			_, err := service.ListFolder(ctx, moviesID, library.FolderPath{}, library.PageRequest{})

			return err
		}},
	}
	for _, test := range tests {
		environment := poisonedEnvironment()

		err := test.call(environment.service(t))
		if !errors.Is(err, domain.ErrNotFound) || errors.Is(err, errMustNotBeCalled) || errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrNotFound only", test.name, err)
		}

		if environment.folderItems.directoryCalls != 0 {
			t.Errorf("%s: the catalog was asked for directories", test.name)
		}
	}
}

func TestReadsPassTheViewersRestriction(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	environment.putShow()
	environment.items.details[showID] = applibrary.ItemDetail{Item: library.Item{ID: showID}}

	ctx := limitedContext(t, access.Age12, true, moviesID, showsID)
	service := environment.service(t)

	steps := []func() error{
		func() error {
			_, err := service.ListItems(ctx, moviesID, library.PageRequest{}, library.ListingQuery{})

			return err
		},
		func() error { _, err := service.ListFacets(ctx, moviesID, applibrary.FacetRequest{}); return err },
		func() error { _, err := service.ListLetters(ctx, moviesID, applibrary.LetterRequest{}); return err },
		func() error { _, err := service.GetItem(ctx, showID); return err },
		func() error { _, err := service.ListChildren(ctx, showID, applibrary.ChildPageRequest{}); return err },
	}
	for index, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d error = %v", index, err)
		}
	}

	want := viewerRestriction(ctx, t)
	got := []access.Restriction{
		environment.items.recordedQueries()[0].Restriction,
		environment.listings.facetRequests[0].Restriction,
		environment.listings.letterRequests[0].Restriction,
		environment.items.detailRestrictions[0],
		environment.children.restrictions[0],
	}

	for index, restriction := range got {
		assertDeepEqual(t, "restriction of step "+string(rune('0'+index)), restriction, want)
	}
}

func TestAnAdminReadsUnrestricted(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()

	if _, err := environment.service(t).ListFacets(adminContext(t), moviesID, applibrary.FacetRequest{}); err != nil {
		t.Fatalf("ListFacets error = %v", err)
	}

	if !environment.listings.facetRequests[0].Restriction.IsUnrestricted() {
		t.Error("an admin without a content limit must read unrestricted")
	}
}

// folderCatalog stores a hidden-only directory (Adult), a visible one (Kids), a mixed one
// and, inside it, a show folder whose episode is rated higher than its show.
func folderCatalog(environment *browseEnvironment) {
	rated := func(bucket access.AgeBucket) []folderRating { return []folderRating{{bucket: bucket, known: true}} }

	environment.directories.directories[moviesRoot+"/"] = []string{"Kids", "Adult", "Mixed", "Empty"}
	environment.directories.directories[moviesRoot+"/Mixed"] = []string{"Sub"}
	environment.folderItems.files["movies-folder"] = []folderFile{
		{itemID: "kid", directory: "Kids", chain: rated(access.Age6)},
		{itemID: "adult", directory: "Adult", chain: rated(access.Age18)},
		{itemID: "mixed-kid", directory: "Mixed", chain: rated(access.Age6)},
		{itemID: "mixed-adult", directory: "Mixed", chain: rated(access.Age18)},
		// The show is for children, its episode is not: the ancestor rule hides the episode
		// and the folder holds nothing else.
		{itemID: "episode", directory: "Mixed/Sub", chain: []folderRating{{bucket: access.Age6, known: true}, {bucket: access.Age18, known: true}}},
	}
	environment.folderItems.items["movies-folder/Mixed"] = []applibrary.FolderItem{
		{ItemID: "mixed-kid", Name: "kid.mkv"}, {ItemID: "mixed-adult", Name: "adult.mkv"},
	}

	for _, id := range []string{"mixed-kid", "mixed-adult"} {
		environment.folderItems.summaries[id] = applibrary.ItemSummary{Item: library.Item{ID: id}}
	}
}

func TestFolderViewIntersectsDirectoriesWithTheVisibleOnes(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	folderCatalog(environment)

	service := environment.service(t)
	top := mustParseFolderPath(t, "movies-folder")

	limited, err := service.ListFolder(limitedContext(t, access.Age6, true, moviesID), moviesID, top, library.PageRequest{})
	if err != nil {
		t.Fatalf("limited ListFolder error = %v", err)
	}

	assertDeepEqual(t, "limited entries", entryTexts(limited.Entries), []string{
		"folder Kids movies-folder/Kids", "folder Mixed movies-folder/Mixed",
	})

	admin, err := service.ListFolder(adminContext(t), moviesID, top, library.PageRequest{})
	if err != nil {
		t.Fatalf("admin ListFolder error = %v", err)
	}

	// An unrestricted viewer sees every file-system directory, also empty ones.
	assertDeepEqual(t, "admin entries", entryTexts(admin.Entries), []string{
		"folder Adult movies-folder/Adult", "folder Empty movies-folder/Empty",
		"folder Kids movies-folder/Kids", "folder Mixed movies-folder/Mixed",
	})
}

func TestFolderViewOfAMixedDirectoryListsOnlyVisibleEntries(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	folderCatalog(environment)

	ctx := limitedContext(t, access.Age6, true, moviesID)

	limited, err := environment.service(t).ListFolder(ctx, moviesID, mustParseFolderPath(t, "movies-folder/Mixed"), library.PageRequest{})
	if err != nil {
		t.Fatalf("ListFolder error = %v", err)
	}

	// "Sub" holds only the episode hidden through its 18-rated chain, so it is absent.
	assertDeepEqual(t, "entries", entryTexts(limited.Entries), []string{"item kid.mkv movies-folder/Mixed/kid.mkv mixed-kid"})
}

func TestOpeningAHiddenDirectoryByPathAnswersNotFound(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	folderCatalog(environment)

	service := environment.service(t)
	ctx := limitedContext(t, access.Age6, true, moviesID)

	for _, path := range []string{"movies-folder/Adult", "movies-folder/Mixed/Sub", "movies-folder/Nowhere"} {
		_, err := service.ListFolder(ctx, moviesID, mustParseFolderPath(t, path), library.PageRequest{})
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("%s: error = %v, want ErrNotFound", path, err)
		}
	}

	// The same directory is open to an unrestricted viewer.
	if _, err := service.ListFolder(adminContext(t), moviesID, mustParseFolderPath(t, "movies-folder/Adult"), library.PageRequest{}); err != nil {
		t.Errorf("admin opening Adult: %v", err)
	}
}

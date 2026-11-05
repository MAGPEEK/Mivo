package library_test

import (
	"encoding/base64"
	"math"
	"reflect"
	"strconv"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// entryText describes an entry as "kind name path [item id]" for compact comparisons.
func entryText(entry applibrary.FolderViewEntry) string {
	text := string(entry.Kind) + " " + entry.Name + " " + entry.Path.String()
	if entry.Item != nil {
		text += " " + entry.Item.Item.ID
	}

	return text
}

func entryTexts(entries []applibrary.FolderViewEntry) []string {
	texts := make([]string, 0, len(entries))
	for _, entry := range entries {
		texts = append(texts, entryText(entry))
	}

	return texts
}

func rawCursor(text string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(text))
}

func assertDeepEqual(t *testing.T, what string, got, want any) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s = %#v, want %#v", what, got, want)
	}
}

func mustParseFolderPath(t *testing.T, text string) library.FolderPath {
	t.Helper()

	path, err := library.ParseFolderPath(text)
	if err != nil {
		t.Fatalf("ParseFolderPath(%q) error = %v", text, err)
	}

	return path
}

func TestListFolderListsTheLibraryFoldersAtTheTop(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	environment.store.put(library.ManagedLibrary{
		ID: unknownID, Name: "Mixed", Kind: library.KindMovies,
		Folders: []library.Folder{
			{ID: "f2", Path: `D:\Media\Zeta`}, {ID: "f1", Path: "/srv/alpha/"}, {ID: "f3", Path: `E:\`}, {ID: "f4", Path: "/"},
		},
	})

	listing, err := environment.service(t).ListFolder(t.Context(), unknownID, library.FolderPath{}, library.PageRequest{})
	if err != nil {
		t.Fatalf("ListFolder error = %v", err)
	}

	want := []string{"folder / f4", `folder alpha f1`, `folder E:\ f3`, `folder Zeta f2`}
	assertDeepEqual(t, "entries", entryTexts(listing.Entries), want)

	if listing.NextCursor != "" || !listing.Path.IsTop() {
		t.Errorf("listing = %+v, want the top without next cursor", listing)
	}
}

func TestListFolderListsDirectoriesThenItemsAndPages(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	environment.directories.directories[moviesRoot+"/Animation"] = []string{"zoo", "Beta", "alpha"}
	environment.folderItems.items["movies-folder/Animation"] = []applibrary.FolderItem{
		{ItemID: "item-2", Name: "b.mkv"}, {ItemID: "item-1", Name: "A.mkv"}, {ItemID: "gone", Name: "c.mkv"},
	}
	environment.folderItems.summaries["item-1"] = applibrary.ItemSummary{Item: library.Item{ID: "item-1", Title: "One"}}
	environment.folderItems.summaries["item-2"] = applibrary.ItemSummary{Item: library.Item{ID: "item-2", Title: "Two"}}

	service := environment.service(t)
	path := mustParseFolderPath(t, "movies-folder/Animation")

	first, err := service.ListFolder(t.Context(), moviesID, path, library.PageRequest{Limit: 4})
	if err != nil {
		t.Fatalf("ListFolder error = %v", err)
	}

	assertDeepEqual(t, "first page", entryTexts(first.Entries), []string{
		"folder alpha movies-folder/Animation/alpha", "folder Beta movies-folder/Animation/Beta",
		"folder zoo movies-folder/Animation/zoo", "item A.mkv movies-folder/Animation/A.mkv item-1",
	})

	if first.Entries[3].Item.Item.Title != "One" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want the full summary and a next cursor", first)
	}

	second, err := service.ListFolder(t.Context(), moviesID, path, library.PageRequest{Limit: 4, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("ListFolder second page error = %v", err)
	}

	// "gone" has no summary any more: the catalog removed it since the listing was read.
	assertDeepEqual(t, "second page", entryTexts(second.Entries), []string{"item b.mkv movies-folder/Animation/b.mkv item-2"})

	if second.NextCursor != "" {
		t.Errorf("second page cursor = %q, want none", second.NextCursor)
	}

	_, err = service.ListFolder(t.Context(), moviesID, mustParseFolderPath(t, "movies-folder"),
		library.PageRequest{Cursor: first.NextCursor})
	assertWrappedError(t, err, domain.ErrInvalid, "not issued for this folder")
}

func TestListFolderReportsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		libraryID string
		path      string
		page      library.PageRequest
		configure func(*browseEnvironment)
		wantErr   error
		wantText  string
	}{
		{name: "unknown library", libraryID: unknownID, wantErr: domain.ErrNotFound, wantText: "list folder of library"},
		{name: "unknown folder", libraryID: moviesID, path: "shows-folder", wantErr: domain.ErrNotFound, wantText: "library folder"},
		{
			name: "directory outside the folder", libraryID: moviesID, path: "movies-folder/link",
			configure: func(environment *browseEnvironment) { environment.directories.err = domain.ErrForbidden },
			wantErr:   domain.ErrNotFound, wantText: "not inside the library folder",
		},
		{
			name: "missing directory", libraryID: moviesID, path: "movies-folder/missing",
			configure: func(environment *browseEnvironment) { environment.directories.err = domain.ErrNotFound },
			wantErr:   domain.ErrNotFound, wantText: "list directories",
		},
		{
			name: "catalog failure", libraryID: moviesID, path: "movies-folder",
			configure: func(environment *browseEnvironment) { environment.folderItems.err = errBoom },
			wantErr:   errBoom, wantText: "list folder items",
		},
		{name: "bad limit", libraryID: moviesID, page: library.PageRequest{Limit: -1}, wantErr: domain.ErrInvalid, wantText: "list folder"},
		{name: "garbage cursor", libraryID: moviesID, page: library.PageRequest{Cursor: "!!"}, wantErr: domain.ErrInvalid, wantText: "base64url"},
		{name: "cursor without position", libraryID: moviesID, page: library.PageRequest{Cursor: rawCursor("folders\x00\x00x")}, wantErr: domain.ErrInvalid, wantText: "no position"},
		{name: "cursor at zero", libraryID: moviesID, page: library.PageRequest{Cursor: rawCursor("folders\x00\x000")}, wantErr: domain.ErrInvalid, wantText: "no position"},
		{name: "negative cursor", libraryID: moviesID, page: library.PageRequest{Cursor: rawCursor("folders\x00\x00-5")}, wantErr: domain.ErrInvalid, wantText: "no position"},
		{name: "overflowing cursor", libraryID: moviesID, page: library.PageRequest{Cursor: rawCursor("folders\x00\x0099999999999999999999")}, wantErr: domain.ErrInvalid, wantText: "no position"},
		{name: "cursor of another kind", libraryID: moviesID, page: library.PageRequest{Cursor: rawCursor("items\x00\x001")}, wantErr: domain.ErrInvalid, wantText: "not issued"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := newBrowseEnvironment()
			if test.configure != nil {
				test.configure(environment)
			}

			_, err := environment.service(t).ListFolder(t.Context(), test.libraryID, mustParseFolderPath(t, test.path), test.page)
			assertWrappedError(t, err, test.wantErr, test.wantText)
		})
	}
}

// A forged cursor at the largest offset must give an empty last page, not overflow the slice
// bounds.
func TestListFolderAnswersAHugeCursorOffsetWithAnEmptyPage(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	cursor := rawCursor("folders\x00\x00" + strconv.Itoa(math.MaxInt))

	listing, err := environment.service(t).ListFolder(t.Context(), moviesID, library.FolderPath{}, library.PageRequest{Limit: library.MaxPageLimit, Cursor: cursor})
	if err != nil || len(listing.Entries) != 0 || listing.NextCursor != "" {
		t.Errorf("ListFolder = %+v, %v; want an empty last page", listing, err)
	}
}

func TestListFolderReportsSummaryFailuresAndMissingReaders(t *testing.T) {
	t.Parallel()

	environment := newBrowseEnvironment()
	environment.folderItems.items["movies-folder/"] = []applibrary.FolderItem{{ItemID: "item-1", Name: "a.mkv"}}
	environment.folderItems.summaryErr = errBoom

	_, err := environment.service(t).ListFolder(t.Context(), moviesID, mustParseFolderPath(t, "movies-folder"), library.PageRequest{})
	assertWrappedError(t, err, errBoom, "read item summaries")

	bare := newTestEnvironment()
	bare.storedMoviesAndShows()

	_, err = bare.service(t).ListFolder(t.Context(), moviesID, library.FolderPath{}, library.PageRequest{})
	assertWrappedError(t, err, domain.ErrUnavailable, "no folder readers")
}

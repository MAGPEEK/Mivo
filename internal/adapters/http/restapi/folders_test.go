package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
)

func TestListFolderRoots(t *testing.T) {
	t.Parallel()

	browsing := &fakeBrowsing{roots: []applibrary.BrowseRoot{
		{ID: testRootID, Path: "/media", Available: true},
		{ID: "1d2e3f4a-5b6c-4d7e-8f90-a1b2c3d4e5f6", Path: "/mnt/nas"},
	}}
	handler, _ := newManagementHandler(t, restapi.Options{
		Folders: browsing, Install: restapi.InstallInfo{Kind: "synology", ServiceUser: "mivo"},
	})

	got := serve(t, handler, http.MethodGet, rootsPath)
	assertJSONResponse(t, got, http.StatusOK)

	list := decodeStrictly[apigen.FolderRootList](t, got.body)
	if list.InstallKind != apigen.InstallKindSynology || list.ServiceUser != "mivo" || len(list.Roots) != 2 {
		t.Fatalf("list = %+v, want synology, mivo and 2 roots", list)
	}

	if list.Roots[0].ID.String() != testRootID || list.Roots[0].Path != "/media" || !list.Roots[0].Available || list.Roots[1].Available {
		t.Errorf("roots = %+v", list.Roots)
	}
}

func TestListFolderRootsDefaultsAndEmpty(t *testing.T) {
	t.Parallel()

	handler, _ := newManagementHandler(t, restapi.Options{Folders: &fakeBrowsing{}, Install: restapi.InstallInfo{Kind: "weird"}})

	got := serve(t, handler, http.MethodGet, rootsPath)
	assertJSONResponse(t, got, http.StatusOK)
	assertBodyContains(t, got.body, `"roots":[]`, `"installKind":"standard"`, `"serviceUser":""`)
}

func TestListFolderRootsFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		browsing   *fakeBrowsing
		wantStatus int
		wantCode   string
	}{
		{"unavailable", &fakeBrowsing{rootsErr: fmt.Errorf("roots: %w", domain.ErrUnavailable)}, 503, restapi.CodeUnavailable},
		{"unexpected", &fakeBrowsing{rootsErr: errors.New("boom")}, 500, restapi.CodeInternal},
		{"not a uuid", &fakeBrowsing{roots: []applibrary.BrowseRoot{{ID: "0123456789abcdef", Path: "/media"}}}, 500, restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newManagementHandler(t, restapi.Options{Folders: test.browsing})
			assertProblem(t, serve(t, handler, http.MethodGet, rootsPath), test.wantStatus, test.wantCode)
		})
	}
}

func TestListFoldersAnswersRootRelativePaths(t *testing.T) {
	t.Parallel()

	browsing := &fakeBrowsing{page: applibrary.FolderPage{
		Folders: []applibrary.BrowsedFolder{
			{Name: "Animation", Selectable: true},
			{Name: "data", Reason: applibrary.BrowseReserved},
			{Name: "Shows", Reason: applibrary.BrowseInUse},
			{Name: "odd", Reason: "future"},
		},
		UnreadableCount: 2, Truncated: true,
	}}
	handler, _ := newManagementHandler(t, restapi.Options{Folders: browsing})

	got := serve(t, handler, http.MethodGet, foldersPath+"?path=/Movies/Kids/")
	assertJSONResponse(t, got, http.StatusOK)

	listing := decodeStrictly[apigen.FolderListing](t, got.body)
	if listing.Path != "Movies/Kids" || listing.ParentPath == nil || *listing.ParentPath != "Movies" ||
		listing.UnreadableCount != 2 || !listing.Truncated || listing.RootID.String() != testRootID {
		t.Fatalf("listing = %+v", listing)
	}

	paths := []string{}
	for _, folder := range listing.Folders {
		paths = append(paths, folder.Path)
	}

	if want := []string{"Movies/Kids/Animation", "Movies/Kids/data", "Movies/Kids/Shows", "Movies/Kids/odd"}; !reflect.DeepEqual(paths, want) {
		t.Errorf("paths = %v, want %v", paths, want)
	}

	reasons := []string{"", string(apigen.FolderReasonReserved), string(apigen.FolderReasonInUse), ""}
	for index, folder := range listing.Folders {
		if (folder.Reason == nil) != (reasons[index] == "") || (folder.Reason != nil && string(*folder.Reason) != reasons[index]) {
			t.Errorf("folder %d reason = %v, want %q", index, folder.Reason, reasons[index])
		}
	}

	if want := [][2]string{{testRootID, "Movies/Kids"}}; !reflect.DeepEqual(browsing.requests, want) {
		t.Errorf("browser got %v, want %v", browsing.requests, want)
	}
}

func TestListFoldersOfTheRootAndOfADirectChild(t *testing.T) {
	t.Parallel()

	browsing := &fakeBrowsing{page: applibrary.FolderPage{Folders: []applibrary.BrowsedFolder{{Name: "A", Selectable: true}}}}
	handler, _ := newManagementHandler(t, restapi.Options{Folders: browsing})

	root := serve(t, handler, http.MethodGet, foldersPath)
	assertJSONResponse(t, root, http.StatusOK)
	assertBodyLacks(t, root.body, "parentPath")
	assertBodyContains(t, root.body, `"path":"A"`, `"path":""`)

	child := decodeStrictly[apigen.FolderListing](t, serve(t, handler, http.MethodGet, foldersPath+"?path=A").body)
	if child.ParentPath == nil || *child.ParentPath != "" {
		t.Errorf("parentPath of a direct child = %v, want the empty string", child.ParentPath)
	}
}

func TestListFoldersEmptyListingHasAnArray(t *testing.T) {
	t.Parallel()

	handler, _ := newManagementHandler(t, restapi.Options{Folders: &fakeBrowsing{}})

	got := serve(t, handler, http.MethodGet, foldersPath)
	assertBodyContains(t, got.body, `"folders":[]`)
}

func TestListFoldersFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"bad path", fmt.Errorf("path: %w", domain.ErrInvalid), 400, restapi.CodeInvalid},
		{"unknown root or missing folder", fmt.Errorf("root: %w", domain.ErrNotFound), 404, restapi.CodeNotFound},
		{"unreadable", fmt.Errorf("open: %w", domain.ErrForbidden), 400, restapi.CodeFolderUnreadable},
		{"timeout", fmt.Errorf("list: %w", domain.ErrUnavailable), 503, restapi.CodeUnavailable},
		{"conflict is no browse answer", fmt.Errorf("x: %w", domain.ErrConflict), 500, restapi.CodeInternal},
		{"unexpected", errors.New("boom"), 500, restapi.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newManagementHandler(t, restapi.Options{Folders: &fakeBrowsing{err: test.err}})
			assertProblem(t, serve(t, handler, http.MethodGet, foldersPath+"?path=x"), test.wantStatus, test.wantCode)
		})
	}
}

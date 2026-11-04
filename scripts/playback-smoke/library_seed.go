package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	folderRootsPath = "/api/v1/folder-roots"
	// seedLibraryName names the library the smoke creates on a server that has none.
	seedLibraryName = "media"
	// preferredRootPath is the browse root of the Docker image (OD-M3-7 A), where the
	// release smoke mounts the corpus.
	preferredRootPath = "/media"
)

// seedLibrary creates a movies library over the whole browse root, preferring the Docker
// image's /media, through the same API the web app uses, so the release smoke needs neither
// the UI nor MIVO_LIBRARIES. It reports errNoLibraries when the server offers no available
// root.
func seedLibrary(ctx context.Context, client *apiClient) error {
	var roots apigen.FolderRootList

	if err := client.call(ctx, apiCall{method: http.MethodGet, reference: folderRootsPath, want: http.StatusOK, out: &roots}); err != nil {
		return fmt.Errorf("list the folder roots: %w", err)
	}

	root, found := pickRoot(roots.Roots)
	if !found {
		return errNoLibraries
	}

	wholeRoot := ""
	body := apigen.LibraryCreate{
		Name: seedLibraryName, Kind: apigen.LibraryKindMovies,
		Folders: []apigen.FolderRef{{RootID: &root.ID, Path: &wholeRoot}},
	}

	err := client.call(ctx, apiCall{method: http.MethodPost, reference: librariesPath, body: body, want: http.StatusCreated})
	if err != nil {
		return fmt.Errorf("create the library over %s: %w", root.Path, err)
	}

	return nil
}

// pickRoot returns the available root at preferredRootPath, else the first available one.
func pickRoot(roots []apigen.FolderRoot) (apigen.FolderRoot, bool) {
	var first *apigen.FolderRoot

	for index := range roots {
		if !roots[index].Available {
			continue
		}

		if roots[index].Path == preferredRootPath {
			return roots[index], true
		}

		if first == nil {
			first = &roots[index]
		}
	}

	if first == nil {
		return apigen.FolderRoot{}, false
	}

	return *first, true
}

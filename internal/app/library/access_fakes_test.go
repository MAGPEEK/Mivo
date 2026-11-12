package library_test

import (
	"context"
	"strings"
	"testing"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain/access"
)

// adminContext carries an unrestricted administrator, the viewer of most use case tests.
func adminContext(t *testing.T) context.Context {
	t.Helper()

	return viewerContext(t, access.Viewer{
		AuthSessionID: "session-admin", AccountID: "account-admin", ProfileID: "profile-admin", Admin: true,
		Libraries: access.AllLibraries(), Limit: access.NoLimit(),
	})
}

// limitedContext carries a viewer who may read only the given libraries, up to maxAge.
func limitedContext(t *testing.T, maxAge access.AgeBucket, hideUnrated bool, libraryIDs ...string) context.Context {
	t.Helper()

	limit, err := access.MaxAgeLimit(maxAge, hideUnrated)
	if err != nil {
		t.Fatalf("MaxAgeLimit(%d) error = %v", maxAge, err)
	}

	return viewerContext(t, access.Viewer{
		AuthSessionID: "session-limited", AccountID: "account-limited", ProfileID: "profile-limited",
		Libraries: access.OnlyLibraries(libraryIDs...), Limit: limit,
	})
}

func viewerContext(t *testing.T, viewer access.Viewer) context.Context {
	t.Helper()

	return appaccess.WithViewer(t.Context(), viewer)
}

// folderRating is the rating of one item of an ancestor chain in the fake catalog.
type folderRating struct {
	bucket access.AgeBucket
	known  bool
}

// folderFile is a file of the fake catalog: its item, the directory below the library
// folder and the ratings of the item and its ancestors, which all must allow it (the
// ancestor rule of ADR 0032 section 7, implemented here so the tests prove the use case
// relies on it).
type folderFile struct {
	itemID    string
	directory string
	chain     []folderRating
}

func (file folderFile) visibleTo(libraryID string, restriction access.Restriction) bool {
	for _, rating := range file.chain {
		if !restriction.Allows(libraryID, rating.bucket, rating.known) {
			return false
		}
	}

	return len(file.chain) > 0 || restriction.AllowsLibrary(libraryID)
}

// directoriesBelow returns the first path segments below directory among the visible files.
func directoriesBelow(files []folderFile, libraryID, directory string, restriction access.Restriction) []string {
	prefix := ""
	if directory != "" {
		prefix = directory + "/"
	}

	var names []string

	for _, file := range files {
		rest, below := strings.CutPrefix(file.directory, prefix)
		if !below || rest == "" || !file.visibleTo(libraryID, restriction) {
			continue
		}

		name, _, _ := strings.Cut(rest, "/")
		if !containsName(names, name) {
			names = append(names, name)
		}
	}

	return names
}

func containsName(names []string, name string) bool {
	for _, existing := range names {
		if existing == name {
			return true
		}
	}

	return false
}

// viewerRestriction is the restriction of the viewer in ctx.
func viewerRestriction(ctx context.Context, t *testing.T) access.Restriction {
	t.Helper()

	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok {
		t.Fatal("no viewer in the context")
	}

	return viewer.Restriction()
}

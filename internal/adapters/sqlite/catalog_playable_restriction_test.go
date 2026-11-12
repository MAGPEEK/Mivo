package sqlite_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
)

// TestPlayableFileFollowsTheViewersRestriction: the file of an item hidden through an
// ancestor, by the content limit, or by the library answers like an unknown file.
func TestPlayableFileFollowsTheViewersRestriction(t *testing.T) {
	t.Parallel()

	fixture := newAncestorFixture(t)

	upToSix, err := access.MaxAgeLimit(access.Age6, false)
	if err != nil {
		t.Fatal(err)
	}

	limited := access.Viewer{Libraries: access.AllLibraries(), Limit: upToSix}.Restriction()
	otherLibrary := access.Viewer{Libraries: access.OnlyLibraries("another-library"), Limit: access.NoLimit()}.Restriction()

	tests := []struct {
		name        string
		fileID      string
		restriction access.Restriction
		wantErr     error
	}{
		{name: "visible file", fileID: "h" + chainOpen, restriction: limited},
		{name: "admin sees the hidden one", fileID: "h" + chainHidden, restriction: access.Unrestricted()},
		{name: "hidden by an ancestor", fileID: "h" + chainHidden, restriction: limited, wantErr: domain.ErrNotFound},
		{name: "hidden by the show", fileID: "h" + sixteenChild, restriction: limited, wantErr: domain.ErrNotFound},
		{name: "library outside the set", fileID: "h" + chainOpen, restriction: otherLibrary, wantErr: domain.ErrNotFound},
		{name: "zero restriction", fileID: "h" + chainOpen, wantErr: domain.ErrNotFound},
	}
	for _, test := range tests {
		_, err := fixture.reader.PlayableFile(t.Context(), test.fileID, test.restriction)
		if !errors.Is(err, test.wantErr) {
			t.Errorf("%s: error = %v, want %v", test.name, err, test.wantErr)
		}
	}
}

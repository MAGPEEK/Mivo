package access_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/access"
)

func TestLibraryAccessAllows(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		access    access.LibraryAccess
		libraryID string
		want      bool
	}{
		{name: "all", access: access.AllLibraries(), libraryID: "lib-9", want: true},
		{name: "listed", access: access.OnlyLibraries("lib-1", "lib-2"), libraryID: "lib-2", want: true},
		{name: "not listed", access: access.OnlyLibraries("lib-1"), libraryID: "lib-2", want: false},
		{name: "empty list", access: access.OnlyLibraries(), libraryID: "lib-1", want: false},
		{name: "empty id ignored", access: access.OnlyLibraries(""), libraryID: "", want: false},
		{name: "zero value", access: access.LibraryAccess{}, libraryID: "lib-1", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.access.Allows(test.libraryID); got != test.want {
				t.Errorf("Allows(%q) = %t, want %t", test.libraryID, got, test.want)
			}
		})
	}
}

func TestLibraryAccessIntersect(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		first, second access.LibraryAccess
		wantAll       bool
		wantIDs       []string
	}{
		{name: "all and all", first: access.AllLibraries(), second: access.AllLibraries(), wantAll: true, wantIDs: []string{}},
		{name: "all and list", first: access.AllLibraries(), second: access.OnlyLibraries("b", "a"), wantIDs: []string{"a", "b"}},
		{name: "list and all", first: access.OnlyLibraries("c"), second: access.AllLibraries(), wantIDs: []string{"c"}},
		{name: "overlap", first: access.OnlyLibraries("a", "b", "c"), second: access.OnlyLibraries("b", "c", "d"), wantIDs: []string{"b", "c"}},
		{name: "disjoint", first: access.OnlyLibraries("a"), second: access.OnlyLibraries("b"), wantIDs: []string{}},
		{name: "list and empty", first: access.OnlyLibraries("a"), second: access.OnlyLibraries(), wantIDs: []string{}},
		{name: "all and zero", first: access.AllLibraries(), second: access.LibraryAccess{}, wantIDs: []string{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := test.first.Intersect(test.second)
			if got.IsAll() != test.wantAll || !slices.Equal(got.IDs(), test.wantIDs) {
				t.Errorf("Intersect = all %t ids %v, want all %t ids %v", got.IsAll(), got.IDs(), test.wantAll, test.wantIDs)
			}
		})
	}
}

func TestLibraryAccessIntersectDoesNotShareState(t *testing.T) {
	t.Parallel()

	profile := access.OnlyLibraries("a")
	effective := access.AllLibraries().Intersect(profile)

	if !effective.Allows("a") || effective.Allows("b") {
		t.Fatalf("effective access = %v, want [a]", effective.IDs())
	}
}

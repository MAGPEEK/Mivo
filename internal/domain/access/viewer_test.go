package access_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/access"
)

func TestZeroRestrictionDeniesEverything(t *testing.T) {
	t.Parallel()

	for name, restriction := range map[string]access.Restriction{
		"zero restriction": {},
		"zero viewer":      access.Viewer{}.Restriction(),
	} {
		if !restriction.DeniesAll() || restriction.AllLibraries() || len(restriction.LibraryIDs()) != 0 {
			t.Errorf("%s does not deny all libraries", name)
		}

		if restriction.Allows("lib-1", access.AgeAll, true) {
			t.Errorf("%s allows a title", name)
		}
	}
}

func TestUnrestricted(t *testing.T) {
	t.Parallel()

	restriction := access.Unrestricted()
	if restriction.DeniesAll() || !restriction.AllLibraries() || restriction.HideUnrated() {
		t.Error("Unrestricted must allow all libraries and unrated titles")
	}

	if _, ok := restriction.MaxAge(); ok {
		t.Error("Unrestricted has a max age")
	}

	if !restriction.Allows("any", access.Age18, true) || !restriction.Allows("any", access.AgeAll, false) {
		t.Error("Unrestricted denies a title")
	}
}

func TestViewerRestriction(t *testing.T) {
	t.Parallel()

	viewer := access.Viewer{
		AuthSessionID: "session-1",
		AccountID:     "account-1",
		ProfileID:     "profile-2",
		Libraries:     access.OnlyLibraries("movies", "kids").Intersect(access.OnlyLibraries("kids")),
		Limit:         access.Stricter(mustLimit(t, access.Age12, false), mustLimit(t, access.Age6, true)),
	}

	restriction := viewer.Restriction()
	if restriction.AllLibraries() || restriction.DeniesAll() || !slices.Equal(restriction.LibraryIDs(), []string{"kids"}) {
		t.Errorf("libraries = all %t ids %v, want [kids]", restriction.AllLibraries(), restriction.LibraryIDs())
	}

	if age, ok := restriction.MaxAge(); !ok || age != access.Age6 || !restriction.HideUnrated() {
		t.Errorf("limit = %d, %t, hide %t; want 6, true, true", age, ok, restriction.HideUnrated())
	}

	tests := []struct {
		name      string
		libraryID string
		bucket    access.AgeBucket
		known     bool
		want      bool
	}{
		{name: "allowed", libraryID: "kids", bucket: access.Age6, known: true, want: true},
		{name: "too old", libraryID: "kids", bucket: access.Age12, known: true, want: false},
		{name: "unrated", libraryID: "kids", known: false, want: false},
		{name: "other library", libraryID: "movies", bucket: access.AgeAll, known: true, want: false},
	}

	for _, test := range tests {
		if got := restriction.Allows(test.libraryID, test.bucket, test.known); got != test.want {
			t.Errorf("%s: Allows = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestRestrictionLibraryChecks(t *testing.T) {
	t.Parallel()

	limit, err := access.MaxAgeLimit(access.Age6, false)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		restriction   access.Restriction
		allowsLibrary bool
		unrestricted  bool
	}{
		{name: "zero", restriction: access.Restriction{}},
		{name: "unrestricted", restriction: access.Unrestricted(), allowsLibrary: true, unrestricted: true},
		{name: "one library", restriction: access.Viewer{Libraries: access.OnlyLibraries("lib")}.Restriction(), allowsLibrary: true},
		{name: "age limit", restriction: access.Viewer{Libraries: access.AllLibraries(), Limit: limit}.Restriction(), allowsLibrary: true},
		{name: "hide unrated", restriction: access.Viewer{Libraries: access.AllLibraries(), Limit: access.NoLimit().HidingUnrated(true)}.Restriction(), allowsLibrary: true},
	}
	for _, test := range tests {
		if got := test.restriction.AllowsLibrary("lib"); got != test.allowsLibrary {
			t.Errorf("%s: AllowsLibrary = %v, want %v", test.name, got, test.allowsLibrary)
		}

		if got := test.restriction.IsUnrestricted(); got != test.unrestricted {
			t.Errorf("%s: IsUnrestricted = %v, want %v", test.name, got, test.unrestricted)
		}
	}
}

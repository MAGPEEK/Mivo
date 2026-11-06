package access

import (
	"maps"
	"slices"
)

// LibraryAccess is the set of libraries an account or profile may browse (ADR 0032 §7): either
// all libraries, including ones added later, or a fixed set of library ids. The zero value
// allows no library, so a forgotten assignment fails closed.
type LibraryAccess struct {
	all bool
	ids map[string]struct{}
}

// AllLibraries returns access to every library, now and in future.
func AllLibraries() LibraryAccess {
	return LibraryAccess{all: true}
}

// OnlyLibraries returns access to exactly the given library ids; empty ids are ignored and no
// ids allow nothing.
func OnlyLibraries(ids ...string) LibraryAccess {
	set := make(map[string]struct{}, len(ids))

	for _, id := range ids {
		if id != "" {
			set[id] = struct{}{}
		}
	}

	return LibraryAccess{ids: set}
}

// IsAll reports whether the access covers every library.
func (a LibraryAccess) IsAll() bool {
	return a.all
}

// IDs returns the allowed library ids in sorted order; it is empty for All, which callers
// check with IsAll first.
func (a LibraryAccess) IDs() []string {
	return slices.Sorted(maps.Keys(a.ids))
}

// Allows reports whether the library with the given id may be browsed.
func (a LibraryAccess) Allows(libraryID string) bool {
	if a.all {
		return true
	}

	_, ok := a.ids[libraryID]

	return ok
}

// Intersect returns the libraries both a and other allow; the effective access of a managed
// profile is account.Intersect(profile).
func (a LibraryAccess) Intersect(other LibraryAccess) LibraryAccess {
	switch {
	case a.all && other.all:
		return AllLibraries()
	case a.all:
		return other.clone()
	case other.all:
		return a.clone()
	}

	shared := make(map[string]struct{})

	for id := range a.ids {
		if other.Allows(id) {
			shared[id] = struct{}{}
		}
	}

	return LibraryAccess{ids: shared}
}

// clone copies the id set, so values returned by Intersect never share a map with an input.
func (a LibraryAccess) clone() LibraryAccess {
	return LibraryAccess{all: a.all, ids: maps.Clone(a.ids)}
}

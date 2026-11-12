package access

// Viewer is the signed-in profile a request acts for (ADR 0032 §7), resolved by the HTTP
// adapter. Libraries and Limit are the effective values: account ∩ profile access and the
// stricter of the profile's and its primary profile's limit. The zero value sees nothing
// because its zero Libraries allow no library. A zero Limit, however, means no content limit,
// so the resolver must always set Limit (for managed profiles via Stricter) and never rely on
// the zero value.
type Viewer struct {
	// AuthSessionID identifies the sign-in session (device) of the request.
	AuthSessionID string
	// AccountID identifies the signed-in account.
	AccountID string
	// ProfileID identifies the selected profile.
	ProfileID string
	// Admin is true only for the primary profile of an admin account.
	Admin bool
	// Libraries is the effective library access.
	Libraries LibraryAccess
	// Limit is the effective content limit.
	Limit ContentLimit
}

// Restriction returns the part of the viewer that catalog queries filter by.
func (v Viewer) Restriction() Restriction {
	return Restriction{libraries: v.Libraries, limit: v.Limit}
}

// Restriction is what a catalog read must filter by: the allowed libraries, the maximum age
// bucket and whether unrated titles are hidden. The zero value allows no library and so
// denies everything (fail closed); Unrestricted is the explicit opposite. LibraryIDs is empty
// both for all libraries and for none, so callers check AllLibraries and DeniesAll first.
type Restriction struct {
	libraries LibraryAccess
	limit     ContentLimit
}

// Unrestricted returns the restriction that allows every library and title, for admin
// viewers without a content limit.
func Unrestricted() Restriction {
	return Restriction{libraries: AllLibraries(), limit: NoLimit()}
}

// AllLibraries reports whether every library is allowed; otherwise only LibraryIDs are.
func (r Restriction) AllLibraries() bool {
	return r.libraries.IsAll()
}

// LibraryIDs returns the allowed library ids in sorted order. It is empty when AllLibraries
// is true and when DeniesAll is true; check those first and never read an empty list as
// "no filter".
func (r Restriction) LibraryIDs() []string {
	return r.libraries.IDs()
}

// DeniesAll reports whether no library is allowed, so a query can skip the database.
func (r Restriction) DeniesAll() bool {
	return !r.libraries.IsAll() && len(r.libraries.ids) == 0
}

// MaxAge returns the highest allowed age bucket; false means no age limit.
func (r Restriction) MaxAge() (AgeBucket, bool) {
	return r.limit.MaxAge()
}

// HideUnrated reports whether titles without a known rating are hidden.
func (r Restriction) HideUnrated() bool {
	return r.limit.HideUnrated()
}

// Allows reports whether a title in the given library with the given rating may be read, for
// checks outside SQL such as starting a playback session.
func (r Restriction) Allows(libraryID string, bucket AgeBucket, known bool) bool {
	return r.libraries.Allows(libraryID) && r.limit.Allows(bucket, known)
}

// AllowsLibrary reports whether the library may be browsed at all, whatever the content limit.
func (r Restriction) AllowsLibrary(libraryID string) bool {
	return r.libraries.Allows(libraryID)
}

// IsUnrestricted reports whether nothing is filtered: every library, no age limit and unrated
// titles shown. Only then may a use case show what the catalog does not know, such as
// file-system directories without items.
func (r Restriction) IsUnrestricted() bool {
	_, limited := r.limit.MaxAge()

	return r.libraries.IsAll() && !limited && !r.limit.HideUnrated()
}

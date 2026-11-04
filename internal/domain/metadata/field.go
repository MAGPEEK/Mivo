package metadata

import (
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// Field is a lockable part of an item (ADR 0027 §6, UX ED-02): a record field or the
// selection of one artwork kind. A locked field keeps its stored value whatever a scan or a
// provider says.
type Field string

// Lockable fields. The text values are stored in the item_field_locks table and used in the
// API, so they never change.
const (
	FieldTitle           Field = "title"
	FieldSortTitle       Field = "sort_title"
	FieldOriginalTitle   Field = "original_title"
	FieldYear            Field = "year"
	FieldReleaseDate     Field = "release_date"
	FieldSummary         Field = "summary"
	FieldTagline         Field = "tagline"
	FieldContentRating   Field = "content_rating"
	FieldGenres          Field = "genres"
	FieldStudios         Field = "studios"
	FieldCountries       Field = "countries"
	FieldCollections     Field = "collections"
	FieldPeople          Field = "people"
	FieldProviderIDs     Field = "provider_ids"
	FieldArtworkPoster   Field = "artwork.poster"
	FieldArtworkBackdrop Field = "artwork.backdrop"
	FieldArtworkThumb    Field = "artwork.thumb"
	FieldArtworkLogo     Field = "artwork.logo"
	FieldArtworkBanner   Field = "artwork.banner"
)

// Fields returns every lockable field in a stable order: record fields as in Record, then
// the artwork kinds.
func Fields() []Field {
	return []Field{
		FieldTitle, FieldSortTitle, FieldOriginalTitle, FieldYear, FieldReleaseDate,
		FieldSummary, FieldTagline, FieldContentRating, FieldGenres, FieldStudios,
		FieldCountries, FieldCollections, FieldPeople, FieldProviderIDs,
		FieldArtworkPoster, FieldArtworkBackdrop, FieldArtworkThumb, FieldArtworkLogo,
		FieldArtworkBanner,
	}
}

// ParseField accepts the exact text of a lockable field, for example when an adapter reads
// a stored lock back. The error wraps domain.ErrInvalid.
func ParseField(text string) (Field, error) {
	field := Field(text)
	if !slices.Contains(Fields(), field) {
		return "", fmt.Errorf("parse field %q: unknown field: %w", text, domain.ErrInvalid)
	}

	return field, nil
}

// String returns the text value of the field.
func (field Field) String() string {
	return string(field)
}

// ArtworkField returns the field that locks the selection of an artwork kind. The error
// wraps domain.ErrInvalid for an unknown kind.
func ArtworkField(kind artwork.Kind) (Field, error) {
	switch kind {
	case artwork.KindPoster:
		return FieldArtworkPoster, nil
	case artwork.KindBackdrop:
		return FieldArtworkBackdrop, nil
	case artwork.KindThumb:
		return FieldArtworkThumb, nil
	case artwork.KindLogo:
		return FieldArtworkLogo, nil
	case artwork.KindBanner:
		return FieldArtworkBanner, nil
	default:
		return "", fmt.Errorf("artwork field for kind %q: unknown kind: %w", kind, domain.ErrInvalid)
	}
}

// LockSet is the set of locked fields of one item. The zero value locks nothing.
type LockSet struct {
	fields map[Field]struct{}
}

// NewLockSet returns the set of the given fields; duplicates are ignored.
func NewLockSet(fields []Field) LockSet {
	set := LockSet{fields: make(map[Field]struct{}, len(fields))}
	for _, field := range fields {
		set.fields[field] = struct{}{}
	}

	return set
}

// Has reports whether the field is locked.
func (set LockSet) Has(field Field) bool {
	_, locked := set.fields[field]

	return locked
}

// Fields returns the locked fields in the order of the package-level Fields, so writers and
// tests see a deterministic list. Unknown fields given to NewLockSet are not listed.
func (set LockSet) Fields() []Field {
	locked := make([]Field, 0, len(set.fields))
	for _, field := range Fields() {
		if set.Has(field) {
			locked = append(locked, field)
		}
	}

	return locked
}

package metadata

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Record is what one source, or the merge of all sources, says about an item (ADR 0027 §6,
// UX DT-06, DT-08). Empty strings, zero numbers, zero dates and empty lists mean "no value".
// List order is meaningful: genres as listed, people in credit order.
type Record struct {
	Title         string
	SortTitle     string
	OriginalTitle string
	// Year is the release or first-air year, 0 when unknown.
	Year          int
	ReleaseDate   Date
	Summary       string
	Tagline       string
	ContentRating string
	Genres        []string
	Studios       []string
	Countries     []string
	Collections   []string
	People        []Credit
	// ProviderIDs holds at most one ID per provider.
	ProviderIDs []ProviderID
}

// ProviderID returns the item's ID in the given provider's database, if the record has one.
func (record Record) ProviderID(provider Provider) (string, bool) {
	for _, id := range record.ProviderIDs {
		if id.Provider == provider {
			return id.Value, true
		}
	}

	return "", false
}

// Credit is one person's part in an item. Character is set for actors and guest stars only;
// Order is the billing position (lower first) as the source lists it.
type Credit struct {
	Name      string
	Role      Role
	Character string
	Order     int
}

// Role is the part a person plays in the making of an item.
type Role string

// Known roles. The text values are stored in the database and used in the API.
const (
	RoleActor     Role = "actor"
	RoleDirector  Role = "director"
	RoleWriter    Role = "writer"
	RoleProducer  Role = "producer"
	RoleGuestStar Role = "guest_star"
)

// ParseRole accepts the exact text of a known role. The error wraps domain.ErrInvalid.
func ParseRole(text string) (Role, error) {
	role := Role(text)
	if !role.IsValid() {
		return "", fmt.Errorf("parse role %q: unknown role: %w", text, domain.ErrInvalid)
	}

	return role, nil
}

// IsValid reports whether the role is one of the known roles.
func (role Role) IsValid() bool {
	switch role {
	case RoleActor, RoleDirector, RoleWriter, RoleProducer, RoleGuestStar:
		return true
	case RoleAuthor, RoleNarrator:
		return true
	default:
		return false
	}
}

// String returns the text value of the role.
func (role Role) String() string {
	return string(role)
}

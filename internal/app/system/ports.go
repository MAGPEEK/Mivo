// Package system contains the use cases around the server itself: its persistent identity,
// its health and its self-description. It declares the ports it needs; adapters implement
// them and cmd/mivo wires them (ADR 0003).
package system

import (
	"context"

	"github.com/magpeek/mivo/internal/domain/system"
)

// IdentityStore persists the server identity. Implemented by adapters/sqlite (M1-08).
type IdentityStore interface {
	// LoadIdentity returns the stored identity or an error wrapping domain.ErrNotFound when
	// none has been saved yet.
	LoadIdentity(ctx context.Context) (system.Identity, error)
	// SaveIdentity stores the identity. It is called at most once per installation.
	SaveIdentity(ctx context.Context, identity system.Identity) error
}

// DatabaseChecker verifies that the database is reachable. Implemented by adapters/sqlite.
type DatabaseChecker interface {
	// Ping returns nil when the database answers. It must return when ctx is done.
	Ping(ctx context.Context) error
}

// IDGenerator creates new random server IDs in canonical lowercase UUID text form.
// Implemented by adapters/uuidgen.
type IDGenerator interface {
	// NewID returns a new random ID.
	NewID() (string, error)
}

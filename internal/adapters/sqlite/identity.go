package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

// IdentityRepository stores the server identity in the single-row server_identity table. It
// implements the app/system IdentityStore port: reads go to the reader pool, the insert to
// the writer pool.
type IdentityRepository struct {
	reader *sqlcgen.Queries
	writer *sqlcgen.Queries
}

// NewIdentityRepository returns a repository on db. db must be migrated before the first call.
func NewIdentityRepository(db *DB) *IdentityRepository {
	return &IdentityRepository{reader: sqlcgen.New(db.reader), writer: sqlcgen.New(db.writer)}
}

// LoadIdentity returns the stored identity. Without one it returns an error wrapping
// domain.ErrNotFound. A stored ID that is not a canonical server ID is reported as an error
// wrapping domain.ErrInvalid rather than handed to clients.
func (repository *IdentityRepository) LoadIdentity(ctx context.Context) (system.Identity, error) {
	row, err := repository.reader.GetServerIdentity(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return system.Identity{}, fmt.Errorf("load server identity: %w", domain.ErrNotFound)
	}

	if err != nil {
		return system.Identity{}, fmt.Errorf("load server identity: %w", err)
	}

	id, err := system.ParseServerID(row.ServerID)
	if err != nil {
		return system.Identity{}, fmt.Errorf("load server identity: stored id: %w", err)
	}

	return system.Identity{ID: id, CreatedAt: time.UnixMilli(row.CreatedAt).UTC()}, nil
}

// SaveIdentity inserts the identity. It never overwrites: when an identity is already stored
// it returns an error wrapping domain.ErrConflict. CreatedAt is stored with millisecond
// precision (ADR 0004), so a later load returns it truncated to milliseconds, in UTC.
// A zero ID or a zero CreatedAt is rejected with an error wrapping domain.ErrInvalid.
func (repository *IdentityRepository) SaveIdentity(ctx context.Context, identity system.Identity) error {
	if identity.ID.IsZero() {
		return fmt.Errorf("save server identity: zero server id: %w", domain.ErrInvalid)
	}

	// A zero time would be stored as a large negative number of milliseconds (year 1) and
	// could only come from a caller that forgot to set it.
	if identity.CreatedAt.IsZero() {
		return fmt.Errorf("save server identity: zero creation time: %w", domain.ErrInvalid)
	}

	inserted, err := repository.writer.InsertServerIdentity(ctx, sqlcgen.InsertServerIdentityParams{
		ServerID:  identity.ID.String(),
		CreatedAt: identity.CreatedAt.UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("save server identity: %w", err)
	}

	if inserted == 0 {
		return fmt.Errorf("save server identity: an identity is already stored: %w", domain.ErrConflict)
	}

	return nil
}

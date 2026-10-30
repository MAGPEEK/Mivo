package system

import (
	"context"
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

// EnsureIdentity returns the persisted server identity and creates it on first use. It is
// idempotent: later calls return the stored identity without saving again.
//
// The common case, an existing identity, is a plain load without any lock. Only the create
// path is serialised; a caller still waiting for it when ctx is done returns ctx.Err()
// (wrapped).
func (service *Service) EnsureIdentity(ctx context.Context) (system.Identity, error) {
	identity, found, err := service.loadIdentity(ctx)
	if err != nil || found {
		return identity, err
	}

	return service.createIdentityExclusively(ctx)
}

func (service *Service) createIdentityExclusively(ctx context.Context) (system.Identity, error) {
	select {
	case service.createSlot <- struct{}{}:
	case <-ctx.Done():
		return system.Identity{}, fmt.Errorf("ensure identity: wait for create: %w", ctx.Err())
	}

	defer func() { <-service.createSlot }()

	// Another caller may have created the identity while this one waited for the slot.
	identity, found, err := service.loadIdentity(ctx)
	if err != nil || found {
		return identity, err
	}

	return service.createIdentity(ctx)
}

// loadIdentity reports found=false without an error when no identity is stored yet.
func (service *Service) loadIdentity(ctx context.Context) (system.Identity, bool, error) {
	identity, err := service.identities.LoadIdentity(ctx)
	if err == nil {
		return identity, true, nil
	}

	if errors.Is(err, domain.ErrNotFound) {
		return system.Identity{}, false, nil
	}

	return system.Identity{}, false, fmt.Errorf("ensure identity: load: %w", err)
}

func (service *Service) createIdentity(ctx context.Context) (system.Identity, error) {
	rawID, err := service.ids.NewID()
	if err != nil {
		return system.Identity{}, fmt.Errorf("ensure identity: generate id: %w", err)
	}

	// The generator is an adapter; validating its output keeps a malformed ID from ever
	// being persisted as the permanent server identity.
	id, err := system.ParseServerID(rawID)
	if err != nil {
		return system.Identity{}, fmt.Errorf("ensure identity: validate generated id: %w", err)
	}

	identity := system.Identity{ID: id, CreatedAt: service.clock().UTC()}

	err = service.identities.SaveIdentity(ctx, identity)
	if errors.Is(err, domain.ErrConflict) {
		return service.loadAfterConflict(ctx, err)
	}

	if err != nil {
		return system.Identity{}, fmt.Errorf("ensure identity: save: %w", err)
	}

	return identity, nil
}

// loadAfterConflict handles a save that lost against another writer of the same store, for
// example a second process on the same data directory. The identity that was stored first is
// the permanent one, so it is loaded and returned instead of failing.
func (service *Service) loadAfterConflict(ctx context.Context, conflict error) (system.Identity, error) {
	identity, err := service.identities.LoadIdentity(ctx)
	if err != nil {
		return system.Identity{}, fmt.Errorf("ensure identity: save: %w; load stored identity: %w", conflict, err)
	}

	return identity, nil
}

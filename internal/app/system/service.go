package system

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

// DatabasePingTimeout bounds the database check of Health, so a hung database makes the
// health endpoint report "unavailable" quickly instead of hanging the caller.
const DatabasePingTimeout = 2 * time.Second

// BuildInfo carries the build metadata of the binary. cmd/mivo maps platform/buildinfo into
// it, because app must not import platform (ADR 0003).
type BuildInfo struct {
	// Version is the release version of the binary.
	Version string
	// Commit is the VCS revision; it may be empty.
	Commit string
	// BuildTime is the commit time in RFC 3339; it may be empty.
	BuildTime string
}

// Dependencies lists everything Service needs. All ports and Clock are required.
type Dependencies struct {
	// Identities persists the server identity.
	Identities IdentityStore
	// Database is pinged by Health.
	Database DatabaseChecker
	// IDs creates the server ID on first start.
	IDs IDGenerator
	// Clock returns the current time; injected so tests are deterministic.
	Clock func() time.Time
	// Build describes the running binary.
	Build BuildInfo
	// ProductName is the configurable product name (platform/brand in cmd/mivo).
	ProductName string
}

// Service implements the system use cases. It is safe for concurrent use.
type Service struct {
	identities  IdentityStore
	database    DatabaseChecker
	ids         IDGenerator
	clock       func() time.Time
	build       BuildInfo
	productName string

	// identityMutex serialises EnsureIdentity so concurrent first calls create one identity.
	identityMutex sync.Mutex
}

// NewService validates the dependencies and returns a Service. A missing port or clock is a
// wiring bug and is reported as an error wrapping domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new system service: %w", err)
	}

	return &Service{
		identities:  dependencies.Identities,
		database:    dependencies.Database,
		ids:         dependencies.IDs,
		clock:       dependencies.Clock,
		build:       dependencies.Build,
		productName: dependencies.ProductName,
	}, nil
}

func (dependencies Dependencies) validate() error {
	var missing []error

	if dependencies.Identities == nil {
		missing = append(missing, missingDependency("identity store"))
	}

	if dependencies.Database == nil {
		missing = append(missing, missingDependency("database checker"))
	}

	if dependencies.IDs == nil {
		missing = append(missing, missingDependency("id generator"))
	}

	if dependencies.Clock == nil {
		missing = append(missing, missingDependency("clock"))
	}

	return errors.Join(missing...)
}

func missingDependency(name string) error {
	return fmt.Errorf("missing dependency %s: %w", name, domain.ErrInvalid)
}

// EnsureIdentity returns the persisted server identity and creates it on first use. It is
// idempotent: later calls return the stored identity without saving again.
func (service *Service) EnsureIdentity(ctx context.Context) (system.Identity, error) {
	service.identityMutex.Lock()
	defer service.identityMutex.Unlock()

	identity, err := service.identities.LoadIdentity(ctx)
	if err == nil {
		return identity, nil
	}

	if !errors.Is(err, domain.ErrNotFound) {
		return system.Identity{}, fmt.Errorf("ensure identity: load: %w", err)
	}

	return service.createIdentity(ctx)
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
	if err := service.identities.SaveIdentity(ctx, identity); err != nil {
		return system.Identity{}, fmt.Errorf("ensure identity: save: %w", err)
	}

	return identity, nil
}

// Health pings the database within DatabasePingTimeout and reports the aggregated state.
// An unreachable database is a result, not an error: the server is then "unavailable".
func (service *Service) Health(ctx context.Context) system.Health {
	return system.NewHealth(service.checkDatabase(ctx))
}

func (service *Service) checkDatabase(ctx context.Context) system.HealthCheck {
	pingContext, cancel := context.WithTimeout(ctx, DatabasePingTimeout)
	defer cancel()

	err := service.database.Ping(pingContext)
	if err == nil {
		// A checker that ignores ctx may answer late; past the deadline it still counts as
		// unavailable so the timeout means the same for every implementation.
		err = pingContext.Err()
	}

	if err != nil {
		return system.FailedCheck(system.HealthCheckDatabase, pingFailureDetail(err))
	}

	return system.PassedCheck(system.HealthCheckDatabase)
}

func pingFailureDetail(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("database ping exceeded %s: %v", DatabasePingTimeout, err)
	}

	return fmt.Sprintf("database ping failed: %v", err)
}

// Info describes the server: product name, build values, API version and the persisted
// server ID. The identity is ensured, so Info works even before EnsureIdentity ran.
func (service *Service) Info(ctx context.Context) (system.Info, error) {
	identity, err := service.EnsureIdentity(ctx)
	if err != nil {
		return system.Info{}, fmt.Errorf("system info: %w", err)
	}

	return system.Info{
		ProductName: service.productName,
		Version:     service.build.Version,
		Commit:      service.build.Commit,
		BuildTime:   service.build.BuildTime,
		ServerID:    identity.ID,
		APIVersion:  system.APIVersion,
	}, nil
}

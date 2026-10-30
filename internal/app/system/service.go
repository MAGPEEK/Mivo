package system

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// identityCreators is the number of callers that may run the identity create path at once.
// One, so concurrent first calls never generate and save two identities.
const identityCreators = 1

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

// Dependencies lists everything Service needs. All ports, Clock and ProductName are required.
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

	// createSlot is a one-slot semaphore around the identity create path. A channel instead
	// of a mutex lets a waiting caller give up when its context is done.
	createSlot chan struct{}
}

// NewService validates the dependencies and returns a Service. A missing port, clock or
// product name is a wiring bug and is reported as an error wrapping domain.ErrInvalid.
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
		createSlot:  make(chan struct{}, identityCreators),
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

	// A blank name would show up as an empty product name in every client, so it is a
	// wiring bug just like a missing port.
	if strings.TrimSpace(dependencies.ProductName) == "" {
		missing = append(missing, missingDependency("product name"))
	}

	return errors.Join(missing...)
}

func missingDependency(name string) error {
	return fmt.Errorf("missing dependency %s: %w", name, domain.ErrInvalid)
}

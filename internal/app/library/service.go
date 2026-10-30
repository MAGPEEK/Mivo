package library

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// DefaultRootCheckTimeout is how long listing libraries waits for a root check. It is short
// because the library list is the first request of every client and a hung network mount
// must not block it; a root that does not answer in time is reported as unavailable.
const DefaultRootCheckTimeout = time.Second

// Dependencies lists everything Service needs. All ports and the logger are required.
type Dependencies struct {
	// Libraries persists the configured libraries.
	Libraries Store
	// Items reads the media catalog.
	Items ItemReader
	// IDs creates the IDs of new libraries.
	IDs IDGenerator
	// Roots tells whether a library root is reachable.
	Roots RootChecker
	// Logger records libraries removed by a sync.
	Logger *slog.Logger
	// RootCheckTimeout bounds every root check; zero means DefaultRootCheckTimeout.
	RootCheckTimeout time.Duration
}

// Service implements the library catalog use cases. It is safe for concurrent use.
type Service struct {
	libraries Store
	items     ItemReader
	ids       IDGenerator
	logger    *slog.Logger
	roots     *rootChecks
}

// NewService validates the dependencies and returns a Service. A missing port or logger and
// a negative timeout are wiring bugs and are reported as an error wrapping
// domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new library service: %w", err)
	}

	timeout := dependencies.RootCheckTimeout
	if timeout == 0 {
		timeout = DefaultRootCheckTimeout
	}

	return &Service{
		libraries: dependencies.Libraries,
		items:     dependencies.Items,
		ids:       dependencies.IDs,
		logger:    dependencies.Logger,
		roots:     newRootChecks(dependencies.Roots, timeout),
	}, nil
}

func (dependencies Dependencies) validate() error {
	var problems []error

	if dependencies.Libraries == nil {
		problems = append(problems, missingDependency("library store"))
	}

	if dependencies.Items == nil {
		problems = append(problems, missingDependency("item reader"))
	}

	if dependencies.IDs == nil {
		problems = append(problems, missingDependency("id generator"))
	}

	if dependencies.Roots == nil {
		problems = append(problems, missingDependency("root checker"))
	}

	if dependencies.Logger == nil {
		problems = append(problems, missingDependency("logger"))
	}

	if dependencies.RootCheckTimeout < 0 {
		problems = append(problems, fmt.Errorf("negative root check timeout %v: %w",
			dependencies.RootCheckTimeout, domain.ErrInvalid))
	}

	return errors.Join(problems...)
}

func missingDependency(name string) error {
	return fmt.Errorf("missing dependency %s: %w", name, domain.ErrInvalid)
}

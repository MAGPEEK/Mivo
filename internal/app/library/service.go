package library

import (
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// Dependencies lists everything Service needs. All ports are required except Children.
type Dependencies struct {
	// Libraries tells whether a library exists.
	Libraries ManagedReader
	// Items reads the media catalog.
	Items ItemReader
	// Children pages through the children of an item. It is optional while the milestone-2
	// catalog is wired (M3-00 dual paths); without it ListChildren reports
	// domain.ErrUnavailable.
	Children ChildReader
}

// Service implements the item use cases of the catalog. Libraries are read and changed
// through ManagedCatalog and Manager. It is safe for concurrent use.
type Service struct {
	libraries ManagedReader
	items     ItemReader
	children  ChildReader
}

// NewService validates the dependencies and returns a Service. A missing port is a wiring bug
// and is reported as an error wrapping domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new library service: %w", err)
	}

	return &Service{
		libraries: dependencies.Libraries,
		items:     dependencies.Items,
		children:  dependencies.Children,
	}, nil
}

func (dependencies Dependencies) validate() error {
	var problems []error

	if dependencies.Libraries == nil {
		problems = append(problems, missingDependency("library reader"))
	}

	if dependencies.Items == nil {
		problems = append(problems, missingDependency("item reader"))
	}

	return errors.Join(problems...)
}

func missingDependency(name string) error {
	return fmt.Errorf("missing dependency %s: %w", name, domain.ErrInvalid)
}

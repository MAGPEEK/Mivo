package library

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"

	"github.com/magpeek/mivo/internal/domain"
)

// maxRandomSeed bounds the seeds Service picks for sort "random": the API carries a seed as
// a non-negative int32.
const maxRandomSeed = math.MaxInt32 + 1

// Dependencies lists everything Service needs. Libraries and Items are required; without one
// of the optional readers the operations that need it report domain.ErrUnavailable.
type Dependencies struct {
	// Libraries tells whether a library exists.
	Libraries ManagedReader
	// Items reads the media catalog.
	Items ItemReader
	// Children pages through the children of an item. It is optional while the milestone-2
	// catalog is wired (M3-00 dual paths); without it ListChildren reports
	// domain.ErrUnavailable.
	Children ChildReader
	// Listings counts the facets and first letters of a library listing (ListFacets,
	// ListLetters).
	Listings ListingReader
	// FolderItems and Directories serve the folder view (ListFolder).
	FolderItems FolderItemReader
	Directories DirectoryLister
	// Roots tells the quick filter "unavailable" which library folders are offline; without
	// it only items whose primary file could not be probed are unavailable.
	Roots RootChecker
	// RandomSeed picks the seed of sort "random" when the client sends none; nil uses a
	// pseudo-random value in [0, 2^31).
	RandomSeed func() int64
}

// Service implements the item use cases of the catalog. Libraries are read and changed
// through ManagedCatalog and Manager. It is safe for concurrent use.
type Service struct {
	libraries   ManagedReader
	items       ItemReader
	children    ChildReader
	listings    ListingReader
	folderItems FolderItemReader
	directories DirectoryLister
	roots       *rootChecks
	randomSeed  func() int64
}

// NewService validates the dependencies and returns a Service. A missing port is a wiring bug
// and is reported as an error wrapping domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new library service: %w", err)
	}

	service := &Service{
		libraries:   dependencies.Libraries,
		items:       dependencies.Items,
		children:    dependencies.Children,
		listings:    dependencies.Listings,
		folderItems: dependencies.FolderItems,
		directories: dependencies.Directories,
		randomSeed:  dependencies.RandomSeed,
	}

	if dependencies.Roots != nil {
		service.roots = newRootChecks(dependencies.Roots, DefaultRootCheckTimeout)
	}

	if service.randomSeed == nil {
		service.randomSeed = func() int64 { return rand.Int64N(maxRandomSeed) } //nolint:gosec // a shuffle order, not a secret
	}

	return service, nil
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

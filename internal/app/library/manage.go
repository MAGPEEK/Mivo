package library

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// scanStopTimeout bounds how long a change waits for the cancelled scan of its library to
// stop. A scan that ignores the cancellation must not block the management call forever.
const scanStopTimeout = 30 * time.Second

// FolderProblem names why a folder reference could not be resolved. These are the
// resolution failures that library.FolderError cannot express.
type FolderProblem string

// Folder resolution problems; the HTTP adapter maps them to problem codes (not_found to
// folder_not_found, unreadable to folder_unreadable, outside_roots to folder_outside_roots;
// the others are plain bad requests).
const (
	ProblemNotFound       FolderProblem = "not_found"
	ProblemUnreadable     FolderProblem = "unreadable"
	ProblemOutsideRoots   FolderProblem = "outside_roots"
	ProblemUnknownFolder  FolderProblem = "unknown_folder"  // a folderID the library does not hold
	ProblemReferenceShape FolderProblem = "reference_shape" // neither folderID nor rootID
)

// FolderCheckError reports an unresolvable folder at Index. It wraps domain.ErrInvalid.
type FolderCheckError struct {
	Index   int
	Problem FolderProblem
}

// Error implements error.
func (e *FolderCheckError) Error() string {
	return fmt.Sprintf("folder %d: %s", e.Index, e.Problem)
}

// Unwrap lets errors.Is match domain.ErrInvalid.
func (e *FolderCheckError) Unwrap() error { return domain.ErrInvalid }

// FolderRef names a folder in a request: an existing folder by ID (update only) or a new
// one by browse root and path relative to it.
type FolderRef struct {
	FolderID string
	RootID   string
	Path     string
}

// CreateRequest describes a new library. Nil Options mean library.DefaultOptions.
type CreateRequest struct {
	Name             string
	Kind             library.Kind
	MetadataLanguage string
	Folders          []FolderRef
	Options          *library.Options
}

// UpdatePatch lists the fields to change; nil fields stay as they are. The kind cannot
// change.
type UpdatePatch struct {
	Name             *string
	MetadataLanguage *string
	Folders          *[]FolderRef
	Options          *library.Options
}

// ManagerDependencies lists everything Manager needs; all fields except Events are required
// (AllowedKinds, Reserved and CaseInsensitive may be zero).
type ManagerDependencies struct {
	Catalog ManagedReader
	Writer  ManagedWriter
	Ledger  ImportLedger
	Browser FolderBrowser
	Scans   ScanControl
	// Events is optional.
	Events ChangeEvents
	Logger *slog.Logger
	// AllowedKinds are the library kinds this install can create.
	AllowedKinds []library.Kind
	// Reserved are the server directories no folder may touch.
	Reserved []string
	// CaseInsensitive selects the Windows path comparison.
	CaseInsensitive bool
}

// Manager implements creating, editing, removing and importing libraries and browsing
// folders. It is safe for concurrent use; management writes are serialised so validation
// and write of two calls never interleave. The mutex is held across the wait for a
// cancelled scan (at most scanStopTimeout), so a slow-to-stop scan delays other management
// calls. Imported libraries are not scanned here: the startup scan covers them.
type Manager struct {
	deps  ManagerDependencies
	mutex sync.Mutex
}

// NewManager validates the dependencies; a missing one wraps domain.ErrInvalid.
func NewManager(deps ManagerDependencies) (*Manager, error) {
	var problems []error

	if deps.Catalog == nil {
		problems = append(problems, missingDependency("library catalog reader"))
	}

	if deps.Writer == nil {
		problems = append(problems, missingDependency("library writer"))
	}

	if deps.Ledger == nil {
		problems = append(problems, missingDependency("import ledger"))
	}

	if deps.Browser == nil {
		problems = append(problems, missingDependency("folder browser"))
	}

	if deps.Scans == nil {
		problems = append(problems, missingDependency("scan control"))
	}

	if deps.Logger == nil {
		problems = append(problems, missingDependency("logger"))
	}

	if err := errors.Join(problems...); err != nil {
		return nil, fmt.Errorf("new library manager: %w", err)
	}

	return &Manager{deps: deps}, nil
}

// CreateLibrary validates and stores a new library, announces it and starts its scan. A
// failure to start the scan is logged, not returned.
func (manager *Manager) CreateLibrary(ctx context.Context, request CreateRequest) (library.ManagedLibrary, error) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	folders, err := manager.resolveFolders(ctx, request.Folders, nil)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("create library: %w", err)
	}

	candidate := library.ManagedLibrary{
		Name: request.Name, Kind: request.Kind, Folders: folders,
		MetadataLanguage: request.MetadataLanguage, Options: library.DefaultOptions(),
	}
	if request.Options != nil {
		candidate.Options = *request.Options
	}

	if err := manager.validate(ctx, candidate); err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("create library: %w", err)
	}

	stored, err := manager.deps.Writer.CreateLibrary(ctx, candidate)
	if err != nil {
		return library.ManagedLibrary{}, fmt.Errorf("create library: %w", storeWriteError(err))
	}

	if manager.deps.Events != nil {
		manager.deps.Events.Added(stored.ID)
	}

	manager.startScan(ctx, stored.ID)

	return stored, nil
}

// RemoveLibrary stops the scan of the library, deletes it with its catalog and announces
// the removal. An unknown ID wraps domain.ErrNotFound.
func (manager *Manager) RemoveLibrary(ctx context.Context, id string) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()

	if _, err := manager.deps.Catalog.GetManaged(ctx, id); err != nil {
		return fmt.Errorf("remove library %q: %w", id, err)
	}

	if err := manager.stopScan(ctx, id); err != nil {
		return fmt.Errorf("remove library %q: %w", id, err)
	}

	if err := manager.deps.Writer.DeleteLibrary(ctx, id); err != nil {
		return fmt.Errorf("remove library %q: %w", id, err)
	}

	if manager.deps.Events != nil {
		manager.deps.Events.Removed(id)
	}

	return nil
}

// validate runs the domain validation against all other libraries and the reserved
// directories, then rejects a name another library uses with domain.ErrConflict.
func (manager *Manager) validate(ctx context.Context, candidate library.ManagedLibrary) error {
	others, err := manager.deps.Catalog.ListManaged(ctx)
	if err != nil {
		return fmt.Errorf("list libraries: %w", err)
	}

	return manager.validateAgainst(candidate, others)
}

// validateAgainst is validate with the other libraries already loaded.
func (manager *Manager) validateAgainst(candidate library.ManagedLibrary, others []library.ManagedLibrary) error {
	rules := library.FolderRules{
		CaseInsensitive: manager.deps.CaseInsensitive,
		Reserved:        manager.deps.Reserved,
		OtherLibraries:  others,
	}

	if err := library.ValidateLibrary(candidate, manager.deps.AllowedKinds, rules); err != nil {
		return err //nolint:wrapcheck // already wrapped by the domain; the HTTP adapter needs the typed error
	}

	for _, other := range others {
		if other.ID != candidate.ID && strings.EqualFold(other.Name, candidate.Name) {
			return fmt.Errorf("name %q is used by library %s: %w", candidate.Name, other.ID, domain.ErrConflict)
		}
	}

	return nil
}

// resolveFolders turns references into folders: IDs are looked up in existing, root+path
// pairs are checked through the browser.
func (manager *Manager) resolveFolders(ctx context.Context, refs []FolderRef,
	existing []library.Folder,
) ([]library.Folder, error) {
	folders := make([]library.Folder, 0, len(refs))

	for index, ref := range refs {
		folder, err := manager.resolveFolder(ctx, index, ref, existing)
		if err != nil {
			return nil, err
		}

		folders = append(folders, folder)
	}

	return folders, nil
}

func (manager *Manager) resolveFolder(ctx context.Context, index int, ref FolderRef,
	existing []library.Folder,
) (library.Folder, error) {
	if ref.FolderID != "" {
		for _, folder := range existing {
			if folder.ID == ref.FolderID {
				return folder, nil
			}
		}

		return library.Folder{}, &FolderCheckError{Index: index, Problem: ProblemUnknownFolder}
	}

	if ref.RootID == "" {
		return library.Folder{}, &FolderCheckError{Index: index, Problem: ProblemReferenceShape}
	}

	path, err := manager.deps.Browser.Check(ctx, ref.RootID, ref.Path)
	if err != nil {
		return library.Folder{}, folderCheckError(index, err)
	}

	return library.Folder{Path: path}, nil
}

// folderCheckError maps a FolderBrowser.Check failure; unavailable and anything unexpected
// (including cancellation) pass on.
func folderCheckError(index int, err error) error {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return &FolderCheckError{Index: index, Problem: ProblemNotFound}
	case errors.Is(err, domain.ErrForbidden):
		return &FolderCheckError{Index: index, Problem: ProblemUnreadable}
	case errors.Is(err, domain.ErrInvalid):
		return &FolderCheckError{Index: index, Problem: ProblemOutsideRoots}
	default:
		return fmt.Errorf("check folder %d: %w", index, err)
	}
}

// storeWriteError maps the store's unique-index conflict to a folder overlap; the store
// cannot say which folder, so the index is 0.
func storeWriteError(err error) error {
	if errors.Is(err, domain.ErrConflict) {
		return fmt.Errorf("%w: %w", &library.FolderError{Reason: library.ReasonOverlap}, err)
	}

	return err
}

func (manager *Manager) startScan(ctx context.Context, id string) {
	if err := manager.deps.Scans.Start(ctx, id); err != nil {
		manager.deps.Logger.WarnContext(ctx, "could not start the scan of a changed library",
			slog.String("library_id", id), slog.Any("error", err))
	}
}

// stopScan cancels the library's scan and waits, bounded, until it has stopped.
func (manager *Manager) stopScan(ctx context.Context, id string) error {
	manager.deps.Scans.Cancel(id)

	waitCtx, cancel := context.WithTimeout(ctx, scanStopTimeout)
	defer cancel()

	if err := manager.deps.Scans.WaitIdle(waitCtx, id); err != nil {
		return fmt.Errorf("wait for the scan to stop: %w: %w", domain.ErrUnavailable, err)
	}

	return nil
}

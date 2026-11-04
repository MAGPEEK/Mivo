package library

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// BrowseReason says why a listed folder cannot be chosen.
type BrowseReason string

// Reasons a listed folder is not selectable.
const (
	BrowseReserved BrowseReason = "reserved" // it holds a server directory
	BrowseInUse    BrowseReason = "in_use"   // it is, contains or lies inside a library folder
)

// BrowsedFolder is one sub-directory in a folder listing.
type BrowsedFolder struct {
	Name       string
	Selectable bool
	// Reason is set when Selectable is false.
	Reason BrowseReason
}

// FolderPage is a folder listing with the selectability of every entry marked.
type FolderPage struct {
	Folders         []BrowsedFolder
	UnreadableCount int
	Truncated       bool
}

// BrowseRoots returns the browse roots.
func (manager *Manager) BrowseRoots(ctx context.Context) ([]BrowseRoot, error) {
	roots, err := manager.deps.Browser.Roots(ctx)
	if err != nil {
		return nil, fmt.Errorf("browse roots: %w", err)
	}

	return roots, nil
}

// BrowseFolders lists the sub-directories of path inside the root and marks those that
// cannot be chosen. Errors of the browser pass on (domain.ErrInvalid for a bad path or an
// unknown root, domain.ErrNotFound, domain.ErrUnavailable).
func (manager *Manager) BrowseFolders(ctx context.Context, rootID, path string) (FolderPage, error) {
	rootPath, err := manager.rootPath(ctx, rootID)
	if err != nil {
		return FolderPage{}, err
	}

	listing, err := manager.deps.Browser.List(ctx, rootID, path)
	if err != nil {
		return FolderPage{}, fmt.Errorf("browse folders: %w", err)
	}

	libraries, err := manager.deps.Catalog.ListManaged(ctx)
	if err != nil {
		return FolderPage{}, fmt.Errorf("browse folders: list libraries: %w", err)
	}

	parent := joinFolder(rootPath, path)
	page := FolderPage{
		Folders:         make([]BrowsedFolder, 0, len(listing.Names)),
		UnreadableCount: listing.UnreadableCount,
		Truncated:       listing.Truncated,
	}

	for _, name := range listing.Names {
		page.Folders = append(page.Folders, manager.markFolder(name, joinFolder(parent, name), libraries))
	}

	return page, nil
}

func (manager *Manager) rootPath(ctx context.Context, rootID string) (string, error) {
	roots, err := manager.deps.Browser.Roots(ctx)
	if err != nil {
		return "", fmt.Errorf("browse folders: %w", err)
	}

	for _, root := range roots {
		if root.ID == rootID {
			return root.Path, nil
		}
	}

	return "", fmt.Errorf("browse folders: unknown root %q: %w", rootID, domain.ErrInvalid)
}

// markFolder applies the domain folder rules to a one-folder probe: reserved wins over in
// use.
func (manager *Manager) markFolder(name, path string, libraries []library.ManagedLibrary) BrowsedFolder {
	probe := library.ManagedLibrary{Folders: []library.Folder{{Path: path}}}

	reserved := library.FolderRules{CaseInsensitive: manager.deps.CaseInsensitive, Reserved: manager.deps.Reserved}
	if hasFolderReason(reserved.Check(probe), library.ReasonReserved) {
		return BrowsedFolder{Name: name, Reason: BrowseReserved}
	}

	inUse := library.FolderRules{CaseInsensitive: manager.deps.CaseInsensitive, OtherLibraries: libraries}
	if hasFolderReason(inUse.Check(probe), library.ReasonOverlap) {
		return BrowsedFolder{Name: name, Reason: BrowseInUse}
	}

	return BrowsedFolder{Name: name, Selectable: true}
}

// joinFolder joins with "/" because the folder rules accept it on every platform; the
// listed names come from the browser and never contain separators.
func joinFolder(parent, child string) string {
	return strings.TrimRight(parent, `/\`) + "/" + strings.Trim(child, "/")
}

func hasFolderReason(err error, reason library.FolderReason) bool {
	var folderErr *library.FolderError

	return errors.As(err, &folderErr) && folderErr.Reason == reason
}

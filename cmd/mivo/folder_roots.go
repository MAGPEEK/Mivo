package main

import (
	"context"
	"crypto/sha1" //nolint:gosec // UUIDv5 (RFC 9562) is defined with SHA-1; it names a path, nothing is protected by it
	"fmt"
	"path/filepath"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/fs/folderbrowse"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
)

// folderRootNamespace is the UUIDv5 namespace of the browse root IDs. It never changes: the
// IDs it produces are what the web app keeps in its URLs.
const folderRootNamespace = "5d0b8f3e-2f6a-4c53-9a0e-6f4f2b1c7d21"

// Bits of a UUIDv5: the version nibble of byte 6 and the variant bits of byte 8.
const (
	uuidVersionByte    = 6
	uuidVariantByte    = 8
	uuidVersion5       = 0x50
	uuidVersionMask    = 0x0f
	uuidVariantRFC9562 = 0x80
	uuidVariantMask    = 0x3f
)

// folderRootID returns the API ID of a browse root: a UUIDv5 of its cleaned path, so the ID
// is canonical (the API contract says uuid) and stays the same across restarts.
func folderRootID(rootPath string) string {
	namespace := uuid.MustParse(folderRootNamespace)

	hash := sha1.New() //nolint:gosec // see the import
	hash.Write(namespace[:])
	hash.Write([]byte(filepath.Clean(rootPath)))

	var id uuid.UUID

	copy(id[:], hash.Sum(nil))
	id[uuidVersionByte] = id[uuidVersionByte]&uuidVersionMask | uuidVersion5
	id[uuidVariantByte] = id[uuidVariantByte]&uuidVariantMask | uuidVariantRFC9562

	return id.String()
}

// folderBrowserAdapter exposes the folder browser as the library manager's FolderBrowser port.
// The browser identifies roots by an opaque hex string; the API needs canonical UUIDs, so the
// adapter translates in both directions.
type folderBrowserAdapter struct {
	browser *folderbrowse.Browser
	// browserIDs maps an API root ID to the browser's own ID.
	browserIDs map[string]string
	// apiIDs maps the browser's ID to the API root ID.
	apiIDs map[string]string
}

var _ applibrary.FolderBrowser = (*folderBrowserAdapter)(nil)

// newFolderBrowserAdapter wraps browser, which was created for rootPaths.
func newFolderBrowserAdapter(browser *folderbrowse.Browser, rootPaths []string) *folderBrowserAdapter {
	adapter := &folderBrowserAdapter{
		browser:    browser,
		browserIDs: make(map[string]string, len(rootPaths)),
		apiIDs:     make(map[string]string, len(rootPaths)),
	}

	for _, rootPath := range rootPaths {
		apiID, browserID := folderRootID(rootPath), folderbrowse.RootID(rootPath)
		adapter.browserIDs[apiID] = browserID
		adapter.apiIDs[browserID] = apiID
	}

	return adapter
}

// Roots lists the browse roots with their API IDs.
func (adapter *folderBrowserAdapter) Roots(ctx context.Context) ([]applibrary.BrowseRoot, error) {
	found := adapter.browser.Roots(ctx)
	roots := make([]applibrary.BrowseRoot, 0, len(found))

	for _, root := range found {
		roots = append(roots, applibrary.BrowseRoot{
			ID: adapter.apiIDs[root.ID], Path: root.Path, Available: root.Available,
		})
	}

	return roots, nil
}

// List returns the names of the readable sub-folders of relativePath.
func (adapter *folderBrowserAdapter) List(ctx context.Context, rootID, relativePath string,
) (applibrary.FolderListing, error) {
	browserID, err := adapter.browserID(rootID)
	if err != nil {
		return applibrary.FolderListing{}, err
	}

	listing, err := adapter.browser.List(ctx, browserID, relativePath)
	if err != nil {
		return applibrary.FolderListing{}, fmt.Errorf("list folders: %w", err)
	}

	names := make([]string, 0, len(listing.Folders))
	for _, folder := range listing.Folders {
		names = append(names, folder.Name)
	}

	return applibrary.FolderListing{
		Names: names, UnreadableCount: listing.UnreadableCount, Truncated: listing.Truncated,
	}, nil
}

// Check verifies the folder and returns its absolute path.
func (adapter *folderBrowserAdapter) Check(ctx context.Context, rootID, relativePath string) (string, error) {
	browserID, err := adapter.browserID(rootID)
	if err != nil {
		return "", err
	}

	absolute, err := adapter.browser.Check(ctx, browserID, relativePath)
	if err != nil {
		return "", fmt.Errorf("check folder: %w", err)
	}

	return absolute, nil
}

// browserID translates an API root ID; an unknown one is a bad reference.
func (adapter *folderBrowserAdapter) browserID(rootID string) (string, error) {
	browserID, found := adapter.browserIDs[rootID]
	if !found {
		return "", fmt.Errorf("browse root %q: %w", rootID, domain.ErrInvalid)
	}

	return browserID, nil
}

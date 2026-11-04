package restapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
)

// pathSeparator separates the elements of the root-relative folder paths of the API.
const pathSeparator = "/"

// FolderBrowsing is the port of the folder browser endpoints, implemented by
// app/library.Manager.
type FolderBrowsing interface {
	// BrowseRoots returns the folder roots the user may browse.
	BrowseRoots(ctx context.Context) ([]applibrary.BrowseRoot, error)
	// BrowseFolders lists the sub-directories of path inside the root with their
	// selectability. An unknown root or a missing folder wraps domain.ErrNotFound, a bad path
	// domain.ErrInvalid, an unreadable folder domain.ErrForbidden and a root that does not
	// answer domain.ErrUnavailable.
	BrowseFolders(ctx context.Context, rootID, path string) (applibrary.FolderPage, error)
}

// InstallInfo describes how this server is installed, so the UI can explain permission
// problems of the folder browser.
type InstallInfo struct {
	// Kind is "standard", "docker" or "synology"; empty means standard.
	Kind string
	// ServiceUser is the account the server runs as; empty when unknown.
	ServiceUser string
}

// ListFolderRoots answers the folder roots with the installation facts.
func (api *API) ListFolderRoots(ctx context.Context, _ apigen.ListFolderRootsRequestObject) (apigen.ListFolderRootsResponseObject, error) {
	roots, err := api.folders.BrowseRoots(ctx)
	if err != nil {
		return nil, operationFailure(operationListFolderRoots, err, http.StatusServiceUnavailable)
	}

	response := apigen.FolderRootList{
		Roots:       make([]apigen.FolderRoot, 0, len(roots)),
		InstallKind: installKindResponse(api.install.Kind),
		ServiceUser: api.install.ServiceUser,
	}

	for _, root := range roots {
		id, err := parseStoredID("folder root", root.ID)
		if err != nil {
			return nil, operationFailure(operationListFolderRoots, err)
		}

		response.Roots = append(response.Roots, apigen.FolderRoot{ID: id, Path: root.Path, Available: root.Available})
	}

	return apigen.ListFolderRoots200JSONResponse(response), nil
}

// ListFolders answers the sub-folders of one folder of a root. The paths in the answer are
// relative to the root: the API never shows where on the disk the root lies in a listing.
func (api *API) ListFolders(ctx context.Context, request apigen.ListFoldersRequestObject) (apigen.ListFoldersResponseObject, error) {
	folderPath := ""
	if request.Params.Path != nil {
		folderPath = strings.Trim(*request.Params.Path, pathSeparator)
	}

	page, err := api.folders.BrowseFolders(ctx, request.RootID.String(), folderPath)
	if errors.Is(err, domain.ErrForbidden) {
		problem := NewProblem(http.StatusBadRequest, CodeFolderUnreadable)

		return apigen.ListFoldersdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	if err != nil {
		return nil, operationFailure(operationListFolders, err,
			http.StatusBadRequest, http.StatusNotFound, http.StatusServiceUnavailable)
	}

	return apigen.ListFolders200JSONResponse(listingResponse(request.RootID, folderPath, page)), nil
}

func listingResponse(rootID apigen.RootID, folderPath string, page applibrary.FolderPage) apigen.FolderListing {
	listing := apigen.FolderListing{
		RootID:          rootID,
		Path:            folderPath,
		ParentPath:      parentPath(folderPath),
		Folders:         make([]apigen.FolderEntry, 0, len(page.Folders)),
		UnreadableCount: int32(min(page.UnreadableCount, maxInt32)), //nolint:gosec // bounded by the min
		Truncated:       page.Truncated,
	}

	for _, folder := range page.Folders {
		entry := apigen.FolderEntry{
			Name: folder.Name, Path: joinPath(folderPath, folder.Name), Selectable: folder.Selectable,
			Reason: folderReasonResponse(folder.Reason),
		}
		listing.Folders = append(listing.Folders, entry)
	}

	return listing
}

// maxInt32 bounds counts that the contract types as int32.
const maxInt32 = 1<<31 - 1

func joinPath(parent, name string) string {
	if parent == "" {
		return name
	}

	return parent + pathSeparator + name
}

// parentPath is nil for the root itself; the root is the parent of its direct sub-folders,
// which the contract writes as the empty string.
func parentPath(folderPath string) *string {
	if folderPath == "" {
		return nil
	}

	parent := ""
	if index := strings.LastIndex(folderPath, pathSeparator); index >= 0 {
		parent = folderPath[:index]
	}

	return &parent
}

// folderReasonResponse omits a reason the contract does not list rather than sending it.
func folderReasonResponse(reason applibrary.BrowseReason) *apigen.FolderReason {
	var response apigen.FolderReason

	switch reason {
	case applibrary.BrowseReserved:
		response = apigen.FolderReasonReserved
	case applibrary.BrowseInUse:
		response = apigen.FolderReasonInUse
	default:
		return nil
	}

	return &response
}

// installKindResponse falls back to the standard install for a value the contract does not
// list; the configuration validates the value before it gets here.
func installKindResponse(kind string) apigen.InstallKind {
	switch kind {
	case string(apigen.InstallKindDocker):
		return apigen.InstallKindDocker
	case string(apigen.InstallKindSynology):
		return apigen.InstallKindSynology
	default:
		return apigen.InstallKindStandard
	}
}

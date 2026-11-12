package library

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// FolderEntryKind says what a folder view entry is.
type FolderEntryKind string

// The kinds of folder view entries.
const (
	FolderEntryFolder FolderEntryKind = "folder"
	FolderEntryItem   FolderEntryKind = "item"
)

// FolderItem is a catalog item found directly in a directory: Name is the file name of its
// first file there.
type FolderItem struct {
	ItemID string
	Name   string
}

// FolderViewEntry is one child of a folder in the folder view. Item is set for kind "item".
type FolderViewEntry struct {
	Kind FolderEntryKind
	Name string
	Path library.FolderPath
	Item *ItemSummary
}

// FolderView is one page of the children of a library folder: sub-folders first, then
// items, each group ordered by name. NextCursor is empty on the last page.
type FolderView struct {
	Path       library.FolderPath
	Entries    []FolderViewEntry
	NextCursor string
}

// ListFolder returns one page of the children of a folder of the library. The top lists the
// library folders; below it the sub-directories come from the file system, confined to the
// library folder (a symlink leaving it is neither listed nor opened), and the items from the
// catalog. A limit outside 1 to library.MaxPageLimit and a cursor not issued for this path
// wrap domain.ErrInvalid; an unknown library, an unknown library folder and a directory that
// is missing or lies outside the library folder wrap domain.ErrNotFound. Without the folder
// readers the error wraps domain.ErrUnavailable.
func (service *Service) ListFolder(ctx context.Context, libraryID string, path library.FolderPath, page library.PageRequest) (FolderView, error) {
	if service.folderItems == nil || service.directories == nil {
		return FolderView{}, fmt.Errorf("list folder of library %q: no folder readers: %w", libraryID, domain.ErrUnavailable)
	}

	listing, err := service.listFolder(ctx, libraryID, path, page)
	if err != nil {
		return FolderView{}, fmt.Errorf("list folder of library %q: %w", libraryID, err)
	}

	return listing, nil
}

func (service *Service) listFolder(ctx context.Context, libraryID string, path library.FolderPath, page library.PageRequest) (FolderView, error) {
	// The access check comes first: a denied library must answer not found whatever else is wrong.
	restriction, err := appaccess.LibraryRestriction(ctx, libraryID)
	if err != nil {
		return FolderView{}, err
	}

	limit, err := library.NormalizeLimit(page.Limit)
	if err != nil {
		return FolderView{}, err
	}

	offset, err := decodeFolderCursor(page.Cursor, path)
	if err != nil {
		return FolderView{}, err
	}

	managed, err := service.libraries.GetManaged(ctx, libraryID)
	if err != nil {
		return FolderView{}, storedDataError(err)
	}

	entries, err := service.folderEntries(ctx, managed, path, restriction)
	if err != nil {
		return FolderView{}, err
	}

	// The offset comes from a client cursor: clamp it before adding, so a forged huge offset
	// cannot overflow into a negative slice bound.
	start := min(offset, len(entries))
	end := start + min(limit, len(entries)-start)
	listing := FolderView{Path: path, Entries: entries[start:end]}

	if end < len(entries) {
		listing.NextCursor = encodeFolderCursor(path, end)
	}

	listing.Entries, err = service.withSummaries(ctx, listing.Entries, restriction)

	return listing, err
}

// folderEntries returns every child of the folder in view order, items still without their
// summary.
func (service *Service) folderEntries(ctx context.Context, managed library.ManagedLibrary, path library.FolderPath, restriction access.Restriction) ([]FolderViewEntry, error) {
	if path.IsTop() {
		return libraryFolderEntries(managed.Folders), nil
	}

	index := slices.IndexFunc(managed.Folders, func(folder library.Folder) bool { return folder.ID == path.FolderID })
	if index < 0 {
		return nil, fmt.Errorf("library folder %q: %w", path.FolderID, domain.ErrNotFound)
	}

	folder := managed.Folders[index]

	directories, err := service.visibleDirectories(ctx, folder, path, restriction)
	if err != nil {
		return nil, err
	}

	items, err := service.folderItems.ListFolderItems(ctx, folder.ID, path.Relative(), restriction)
	if err != nil {
		return nil, fmt.Errorf("list folder items: %w", err)
	}

	return childEntries(path, directories, items), nil
}

func libraryFolderEntries(folders []library.Folder) []FolderViewEntry {
	entries := make([]FolderViewEntry, 0, len(folders))
	for _, folder := range folders {
		entries = append(entries, FolderViewEntry{
			Kind: FolderEntryFolder, Name: folderDisplayName(folder.Path), Path: library.FolderPath{FolderID: folder.ID},
		})
	}

	slices.SortStableFunc(entries, compareEntryNames)

	return entries
}

// folderDisplayName is the last segment of a library folder path ("Movies" for
// "D:\Media\Movies"); a drive or file system root keeps its whole path.
func folderDisplayName(folderPath string) string {
	trimmed := strings.TrimRight(folderPath, `/\`)

	name := trimmed[strings.LastIndexAny(trimmed, `/\`)+1:]
	if name == "" || strings.HasSuffix(name, ":") {
		return folderPath
	}

	return name
}

func childEntries(path library.FolderPath, directories []string, items []FolderItem) []FolderViewEntry {
	folders := make([]FolderViewEntry, 0, len(directories))
	for _, name := range directories {
		folders = append(folders, FolderViewEntry{Kind: FolderEntryFolder, Name: name, Path: path.Child(name)})
	}

	found := make([]FolderViewEntry, 0, len(items))
	for _, item := range items {
		found = append(found, FolderViewEntry{
			Kind: FolderEntryItem, Name: item.Name, Path: path.Child(item.Name), Item: &ItemSummary{Item: library.Item{ID: item.ItemID}},
		})
	}

	slices.SortStableFunc(folders, compareEntryNames)
	slices.SortStableFunc(found, compareEntryNames)

	return append(folders, found...)
}

// compareEntryNames orders by name case-insensitively, then exactly, so the order is total.
func compareEntryNames(left, right FolderViewEntry) int {
	return cmp.Or(
		cmp.Compare(strings.ToLower(left.Name), strings.ToLower(right.Name)),
		cmp.Compare(left.Name, right.Name),
	)
}

// withSummaries replaces the placeholder summaries of the item entries with the catalog's.
// An item removed since the listing was read is dropped.
func (service *Service) withSummaries(ctx context.Context, entries []FolderViewEntry, restriction access.Restriction) ([]FolderViewEntry, error) {
	var itemIDs []string

	for _, entry := range entries {
		if entry.Item != nil {
			itemIDs = append(itemIDs, entry.Item.Item.ID)
		}
	}

	if len(itemIDs) == 0 {
		return entries, nil
	}

	summaries, err := service.folderItems.ItemSummaries(ctx, itemIDs, restriction)
	if err != nil {
		return nil, fmt.Errorf("read item summaries: %w", err)
	}

	byID := make(map[string]ItemSummary, len(summaries))
	for _, summary := range summaries {
		byID[summary.Item.ID] = summary
	}

	completed := make([]FolderViewEntry, 0, len(entries))

	for _, entry := range entries {
		if entry.Item != nil {
			summary, found := byID[entry.Item.Item.ID]
			if !found {
				continue
			}

			entry.Item = &summary
		}

		completed = append(completed, entry)
	}

	return completed, nil
}

// visibleDirectories returns the sub-directories of the directory the viewer may see: the
// file-system directories, intersected with the catalog's directories that hold a visible
// item unless the viewer is unrestricted (an administrator sees empty directories too). A
// directory that is hidden by the restriction answers like a missing one, so opening it by
// path reveals nothing.
func (service *Service) visibleDirectories(
	ctx context.Context, folder library.Folder, path library.FolderPath, restriction access.Restriction,
) ([]string, error) {
	directories, err := service.directories.ListDirectories(ctx, folder.Path, path.Relative())
	if errors.Is(err, domain.ErrForbidden) {
		// A directory outside the library folder (a symlink leaving it) must look missing.
		return nil, fmt.Errorf("folder not inside the library folder: %w", domain.ErrNotFound)
	}

	if err != nil {
		return nil, fmt.Errorf("list directories: %w", err)
	}

	if restriction.IsUnrestricted() {
		return directories, nil
	}

	if err := service.checkDirectoryVisible(ctx, folder, path, restriction); err != nil {
		return nil, err
	}

	visible, err := service.folderItems.ListFolderDirectories(ctx, folder.ID, path.Relative(), restriction)
	if err != nil {
		return nil, fmt.Errorf("list visible directories: %w", err)
	}

	return slices.DeleteFunc(directories, func(name string) bool { return !slices.Contains(visible, name) }), nil
}

// checkDirectoryVisible refuses a directory below the library folder that holds no visible
// item. Checking the last segment against its parent suffices: a visible directory has a
// visible item beneath it, which makes every ancestor visible too.
func (service *Service) checkDirectoryVisible(
	ctx context.Context, folder library.Folder, path library.FolderPath, restriction access.Restriction,
) error {
	if len(path.Segments) == 0 {
		return nil
	}

	last := len(path.Segments) - 1

	siblings, err := service.folderItems.ListFolderDirectories(ctx, folder.ID, strings.Join(path.Segments[:last], "/"), restriction)
	if err != nil {
		return fmt.Errorf("list visible directories: %w", err)
	}

	if !slices.Contains(siblings, path.Segments[last]) {
		return fmt.Errorf("directory %q: %w", path.Segments[last], domain.ErrNotFound)
	}

	return nil
}

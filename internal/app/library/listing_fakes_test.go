package library_test

import (
	"context"
	"slices"
	"sync"
	"testing"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/library"
)

// memoryListingReader is an in-memory ListingReader answering fixed results and recording
// the requests.
type memoryListingReader struct {
	mutex      sync.Mutex
	facets     []applibrary.Facet
	characters []applibrary.CharacterCount
	err        error

	facetRequests  []applibrary.FacetRequest
	letterRequests []applibrary.LetterRequest
}

func (reader *memoryListingReader) CountFacets(_ context.Context, _ string, request applibrary.FacetRequest) ([]applibrary.Facet, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.facetRequests = append(reader.facetRequests, request)

	return reader.facets, reader.err
}

func (reader *memoryListingReader) CountFirstCharacters(_ context.Context, _ string, request applibrary.LetterRequest) ([]applibrary.CharacterCount, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.letterRequests = append(reader.letterRequests, request)

	return reader.characters, reader.err
}

// memoryFolderItems is an in-memory FolderItemReader: items per "rootID/directory" and the
// summaries it knows.
type memoryFolderItems struct {
	items map[string][]applibrary.FolderItem
	// files is the catalog behind ListFolderDirectories (per root ID); libraryID is the library
	// the roots belong to.
	files     map[string][]folderFile
	libraryID string
	// directoryCalls counts ListFolderDirectories calls.
	directoryCalls int
	summaries      map[string]applibrary.ItemSummary
	// err fails ListFolderItems, summaryErr ItemSummaries.
	err        error
	summaryErr error
}

func (reader *memoryFolderItems) ListFolderItems(_ context.Context, rootID, directory string, restriction access.Restriction) ([]applibrary.FolderItem, error) {
	var visible []applibrary.FolderItem

	for _, item := range reader.items[rootID+"/"+directory] {
		if reader.itemVisible(rootID, item.ItemID, restriction) {
			visible = append(visible, item)
		}
	}

	return visible, reader.err
}

// itemVisible applies the restriction to the catalog files of the item; an item without
// catalog files (older fixtures) is visible to everyone who may read the library.
func (reader *memoryFolderItems) itemVisible(rootID, itemID string, restriction access.Restriction) bool {
	known := false

	for _, file := range reader.files[rootID] {
		if file.itemID != itemID {
			continue
		}

		known = true

		if file.visibleTo(reader.libraryID, restriction) {
			return true
		}
	}

	return !known && restriction.AllowsLibrary(reader.libraryID)
}

func (reader *memoryFolderItems) ListFolderDirectories(_ context.Context, rootID, directory string, restriction access.Restriction) ([]string, error) {
	reader.directoryCalls++

	return directoriesBelow(reader.files[rootID], reader.libraryID, directory, restriction), reader.err
}

func (reader *memoryFolderItems) ItemSummaries(_ context.Context, itemIDs []string, _ access.Restriction) ([]applibrary.ItemSummary, error) {
	var summaries []applibrary.ItemSummary

	for _, id := range itemIDs {
		if summary, found := reader.summaries[id]; found {
			summaries = append(summaries, summary)
		}
	}

	return summaries, reader.summaryErr
}

// memoryDirectories is an in-memory DirectoryLister: names per "root/directory", or err.
type memoryDirectories struct {
	directories map[string][]string
	err         error
}

func (lister *memoryDirectories) ListDirectories(_ context.Context, root, directory string) ([]string, error) {
	if lister.err != nil {
		return nil, lister.err
	}

	return slices.Clone(lister.directories[root+"/"+directory]), nil
}

// browseEnvironment is a testEnvironment with the browse readers wired.
type browseEnvironment struct {
	*testEnvironment

	listings    *memoryListingReader
	folderItems *memoryFolderItems
	directories *memoryDirectories
	roots       applibrary.RootChecker
	seed        int64
}

func newBrowseEnvironment() *browseEnvironment {
	environment := &browseEnvironment{
		testEnvironment: newTestEnvironment(),
		listings:        &memoryListingReader{},
		folderItems:     &memoryFolderItems{libraryID: moviesID, files: map[string][]folderFile{}, items: map[string][]applibrary.FolderItem{}, summaries: map[string]applibrary.ItemSummary{}},
		directories:     &memoryDirectories{directories: map[string][]string{}},
		seed:            4711,
	}
	environment.storedMoviesAndShows()

	return environment
}

func (environment *browseEnvironment) service(t *testing.T) *applibrary.Service {
	t.Helper()

	dependencies := applibrary.Dependencies{
		Libraries: environment.store, Items: environment.items, Listings: environment.listings,
		FolderItems: environment.folderItems, Directories: environment.directories, Roots: environment.roots,
		RandomSeed: func() int64 { return environment.seed },
	}
	if environment.children != nil {
		dependencies.Children = environment.children
	}

	service, err := applibrary.NewService(dependencies)
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	return service
}

func (reader *memoryItemReader) recordedQueries() []library.ListingQuery {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	return slices.Clone(reader.queries)
}

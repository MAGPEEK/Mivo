package scan_test

import (
	"context"
	"fmt"
	"sync"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// metadataCatalog is an in-memory MetadataCatalog: saving updates the state it returns next.
type metadataCatalog struct {
	mutex          sync.Mutex
	states         map[string]scan.MetadataState
	metadataSaves  int
	artworkSaves   int
	savedCandidate map[string][]artwork.Candidate
}

func newMetadataCatalog(itemIDs ...string) *metadataCatalog {
	catalog := &metadataCatalog{states: map[string]scan.MetadataState{}, savedCandidate: map[string][]artwork.Candidate{}}
	for _, id := range itemIDs {
		catalog.states[id] = scan.MetadataState{}
	}

	return catalog
}

func (catalog *metadataCatalog) ItemMetadataState(_ context.Context, itemID string) (scan.MetadataState, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	state, found := catalog.states[itemID]
	if !found {
		return scan.MetadataState{}, fmt.Errorf("item %s: %w", itemID, domain.ErrNotFound)
	}

	return state, nil
}

func (catalog *metadataCatalog) SaveMetadata(_ context.Context, itemID string, record metadata.Record) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	state := catalog.states[itemID]
	state.Record = record
	catalog.states[itemID] = state
	catalog.metadataSaves++

	return nil
}

func (catalog *metadataCatalog) SaveArtwork(_ context.Context, itemID string, candidates []artwork.Candidate,
	selection map[artwork.Kind]artwork.Candidate,
) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	state := catalog.states[itemID]
	state.CurrentArtwork = selection
	catalog.states[itemID] = state
	catalog.savedCandidate[itemID] = candidates
	catalog.artworkSaves++

	return nil
}

func (catalog *metadataCatalog) state(itemID string) scan.MetadataState {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.states[itemID]
}

func (catalog *metadataCatalog) set(itemID string, state scan.MetadataState) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	catalog.states[itemID] = state
}

// memorySidecars serves sidecar content by path; a missing path is not found, errs injects
// failures, onRead runs before each read.
type memorySidecars struct {
	mutex   sync.Mutex
	content map[string]string
	errs    map[string]error
	onRead  func(ctx context.Context) error
	reads   []string
	// roots holds the library root of each read, in order.
	roots []string
}

func newMemorySidecars() *memorySidecars {
	return &memorySidecars{content: map[string]string{}, errs: map[string]error{}}
}

func (sidecars *memorySidecars) ReadSidecar(ctx context.Context, root, relativePath string, _ int64) ([]byte, error) {
	sidecars.mutex.Lock()
	sidecars.reads = append(sidecars.reads, relativePath)
	sidecars.roots = append(sidecars.roots, root)
	onRead, injected := sidecars.onRead, sidecars.errs[relativePath]
	text, found := sidecars.content[relativePath]
	sidecars.mutex.Unlock()

	if onRead != nil {
		if err := onRead(ctx); err != nil {
			return nil, err
		}
	}

	if injected != nil {
		return nil, injected
	}

	if !found {
		return nil, fmt.Errorf("sidecar %s: %w", relativePath, domain.ErrNotFound)
	}

	return []byte(text), nil
}

// scriptedParser answers by NFO content; unknown content is unsupported.
type scriptedParser struct {
	results map[string]scan.NFOResult
	kinds   []scan.NFOKind
}

func (parser *scriptedParser) Parse(data []byte, kind scan.NFOKind) (scan.NFOResult, error) {
	parser.kinds = append(parser.kinds, kind)

	result, found := parser.results[string(data)]
	if !found {
		return scan.NFOResult{}, fmt.Errorf("parse: %w", domain.ErrUnsupported)
	}

	return result, nil
}

// nfoWith returns an NFOResult with one record.
func nfoWith(record metadata.Record, warnings ...string) scan.NFOResult {
	return scan.NFOResult{Records: []metadata.Record{record}, Warnings: warnings}
}

// inMoviesFolder returns a file state in the only folder of the movies library of the refresh
// rig; the refresh finds a file's folder by RootID.
func inMoviesFolder(path string, size int64) library.FileState {
	state := fileAt(path, size)
	state.RootID = moviesID

	return state
}

func nfoFile(path string, size int64) library.FileState {
	state := inMoviesFolder(path, size)
	state.Class = library.FileClassNFO

	return state
}

func imageFile(path string, size int64) library.FileState {
	state := inMoviesFolder(path, size)
	state.Class = library.FileClassImage

	return state
}

func refreshFile(path, itemID string, level library.Level) scan.RefreshFile {
	return scan.RefreshFile{
		State:     inMoviesFolder(path, 1),
		Placement: library.Placement{Levels: []library.Level{level}},
		ItemIDs:   []string{itemID},
	}
}

func movieLevel(title string, year int) library.Level {
	return library.Level{Kind: library.ItemKindMovie, MatchKey: title, Title: title, Year: library.NumberOf(year)}
}

// episodeFile is an episode of show "Show" in season 1 at path.
func episodeFile(path, episodeNumberID string, number int) scan.RefreshFile {
	return scan.RefreshFile{
		State: inMoviesFolder(path, 1),
		Placement: library.Placement{Levels: []library.Level{
			{Kind: library.ItemKindShow, MatchKey: "show", Title: "Show"},
			{Kind: library.ItemKindSeason, MatchKey: "1", Title: "Season 1", Number: library.NumberOf(1)},
			{Kind: library.ItemKindEpisode, MatchKey: episodeNumberID, Title: "Episode", Number: library.NumberOf(number)},
		}},
		ItemIDs: []string{"show", "season", episodeNumberID},
	}
}

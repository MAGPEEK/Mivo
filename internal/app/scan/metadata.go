package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// maxNFOBytes is the size limit for reading an NFO file (ADR 0027 §7).
const maxNFOBytes int64 = 1 << 20

// Fixed NFO file names (ADR 0027 §7).
const (
	movieNFOName  = "movie.nfo"
	showNFOName   = "tvshow.nfo"
	seasonNFOName = "season.nfo"
	nfoExtension  = ".nfo"
)

// Warning codes logged for an NFO that could not be used.
const (
	nfoUnreadable = "nfo_unreadable"
	nfoOversized  = "nfo_oversized"
	nfoMalformed  = "nfo_malformed"
)

// MetadataRefresherDeps lists what NewMetadataRefresher needs; every field is required.
type MetadataRefresherDeps struct {
	Catalog MetadataCatalog
	Reader  SidecarReader
	Parser  NFOParser
	Logger  *slog.Logger
}

// MetadataRefresher is the ItemRefresher of the v2 scan: it reads NFO files, merges them with
// the file-name data under the item's locks and selects local and embedded artwork. It writes
// no file and fetches no URL (ADR 0010, ADR 0022).
type MetadataRefresher struct {
	deps MetadataRefresherDeps
}

// NewMetadataRefresher validates the dependencies; a missing one is a wiring bug and wraps
// domain.ErrInvalid.
func NewMetadataRefresher(deps MetadataRefresherDeps) (*MetadataRefresher, error) {
	if deps.Catalog == nil || deps.Reader == nil || deps.Parser == nil || deps.Logger == nil {
		return nil, fmt.Errorf("new metadata refresher: missing dependency: %w", domain.ErrInvalid)
	}

	return &MetadataRefresher{deps: deps}, nil
}

// refreshRun is one RefreshItems call.
type refreshRun struct {
	refresher *MetadataRefresher
	request   RefreshRequest
	sidecars  sidecarIndex
	index     *itemIndex
	warnings  int
}

// RefreshItems recomputes metadata and artwork of the requested items and of every other item
// of the request's files that has no stored metadata yet (an early-ended scan may have saved
// items whose refresh failed). Items are processed one after the other, as NFO files lie on
// NAS disks. Only cancellation and catalog errors fail the refresh.
func (refresher *MetadataRefresher) RefreshItems(ctx context.Context, request RefreshRequest) error {
	run := &refreshRun{
		refresher: refresher, request: request,
		sidecars: newSidecarIndex(request.Sidecars), index: newItemIndex(request.Files, request.Library.Folders),
	}

	defer func() {
		if request.CountMetadataWarnings != nil && run.warnings > 0 {
			request.CountMetadataWarnings(run.warnings)
		}
	}()

	ids, requested := run.index.order(request.ItemIDs)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("refresh items: %w", err)
		}

		if err := run.refreshItem(ctx, run.index.items[id], requested[id]); err != nil {
			return fmt.Errorf("refresh item %s: %w", id, err)
		}
	}

	return nil
}

func (run *refreshRun) refreshItem(ctx context.Context, item *refreshItem, requested bool) error {
	catalog := run.refresher.deps.Catalog

	state, err := catalog.ItemMetadataState(ctx, item.id)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("load metadata state: %w", err)
	}

	if !requested && state.Record.Title != "" {
		return nil
	}

	if err := run.refreshMetadata(ctx, item, state); err != nil {
		return err
	}

	return run.refreshArtwork(ctx, item, state)
}

func (run *refreshRun) refreshMetadata(ctx context.Context, item *refreshItem, state MetadataState) error {
	nfo, outcome, err := run.readNFO(ctx, item)
	if err != nil {
		return err
	}

	var sources []metadata.SourcedRecord

	switch outcome {
	case nfoUsed:
		sources = append(sources, metadata.SourcedRecord{Source: metadata.SourceNFO, Record: nfo})
	case nfoUnusable:
		// A broken NFO must not wipe what an earlier good one gave: the stored record leads,
		// the file name only fills gaps.
		sources = append(sources, metadata.SourcedRecord{Source: metadata.SourceNFO, Record: state.Record})
	case nfoAbsent:
	}

	sources = append(sources, metadata.SourcedRecord{Source: metadata.SourceFilename, Record: filenameRecord(item.level)})
	merged := metadata.Merge(state.Record, state.Locks, sources)

	if sameRecord(state.Record, merged) {
		return nil
	}

	err = run.refresher.deps.Catalog.SaveMetadata(ctx, item.id, merged)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("save metadata: %w", err)
	}

	return nil
}

// filenameRecord is what the placement knows: the parsed title and year.
func filenameRecord(level library.Level) metadata.Record {
	year, _ := level.Year.Get()

	return metadata.Record{Title: level.Title, Year: year}
}

// nfoKindOf maps an item kind to the element its NFO describes.
func nfoKindOf(kind library.ItemKind) (NFOKind, bool) {
	switch kind {
	case library.ItemKindMovie:
		return NFOKindMovie, true
	case library.ItemKindShow:
		return NFOKindShow, true
	case library.ItemKindSeason:
		return NFOKindSeason, true
	case library.ItemKindEpisode:
		return NFOKindEpisode, true
	case library.ItemKindExtra, library.ItemKindArtist, library.ItemKindAlbum, library.ItemKindTrack,
		library.ItemKindPhotoAlbum, library.ItemKindPhoto, library.ItemKindAudiobook:
		return "", false
	default:
		return "", false
	}
}

// nfoOutcome says what the NFO of an item contributed.
type nfoOutcome int

const (
	// nfoAbsent: the item has no NFO.
	nfoAbsent nfoOutcome = iota
	// nfoUsed: the NFO was parsed.
	nfoUsed
	// nfoUnusable: the NFO exists but cannot be used (forbidden, oversized, malformed).
	nfoUnusable
)

// readNFO finds, reads and parses the item's NFO. An NFO that cannot be used is nfoUnusable
// and never fails the scan. A source that is unavailable or vanished mid-scan is an error, so
// the refresh is retried (the sidecar state is saved only after a successful refresh) instead
// of replacing good metadata with file-name data.
func (run *refreshRun) readNFO(ctx context.Context, item *refreshItem) (metadata.Record, nfoOutcome, error) {
	kind, hasNFO := nfoKindOf(item.level.Kind)
	if !hasNFO || !run.request.Library.Options.ReadNFO {
		return metadata.Record{}, nfoAbsent, nil
	}

	nfo, found := run.sidecars.first(run.nfoCandidates(item))
	if !found {
		return metadata.Record{}, nfoAbsent, nil
	}

	root, found := folderRoot(run.request.Library, nfo.rootID)
	if !found {
		return metadata.Record{}, nfoAbsent, nil
	}

	nfoPath := nfo.path

	data, err := run.refresher.deps.Reader.ReadSidecar(ctx, root, nfoPath, maxNFOBytes)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return metadata.Record{}, nfoAbsent, fmt.Errorf("read %s: %w", nfoPath, ctxErr)
	}

	if outcome, readErr := run.classifyRead(ctx, nfoPath, data, err); outcome != nfoUsed || readErr != nil {
		return metadata.Record{}, outcome, readErr
	}

	record, usable := run.parseNFO(ctx, nfoPath, data, kind)
	if !usable {
		return metadata.Record{}, nfoUnusable, nil
	}

	return record, nfoUsed, nil
}

// classifyRead judges the result of reading an NFO: nfoUsed means the data may be parsed; an
// error means the refresh must be retried.
func (run *refreshRun) classifyRead(ctx context.Context, nfoPath string, data []byte, err error) (nfoOutcome, error) {
	switch {
	case errors.Is(err, domain.ErrUnavailable) || errors.Is(err, domain.ErrNotFound):
		return nfoAbsent, fmt.Errorf("read %s: %w", nfoPath, err)
	case errors.Is(err, domain.ErrUnsupported):
		run.warn(ctx, nfoPath, nfoOversized)

		return nfoUnusable, nil
	case err != nil:
		run.warn(ctx, nfoPath, nfoUnreadable)

		return nfoUnusable, nil
	case int64(len(data)) > maxNFOBytes:
		run.warn(ctx, nfoPath, nfoOversized)

		return nfoUnusable, nil
	}

	return nfoUsed, nil
}

// parseNFO returns the record that describes the item. Records carry no episode number, so a
// multi-episode file (several records) gives the item its first record; this is a known
// limitation (ADR 0027 §7).
func (run *refreshRun) parseNFO(ctx context.Context, nfoPath string, data []byte, kind NFOKind) (metadata.Record, bool) {
	result, err := run.refresher.deps.Parser.Parse(data, kind)
	if err != nil {
		run.warn(ctx, nfoPath, nfoMalformed)

		return metadata.Record{}, false
	}

	for _, code := range result.Warnings {
		run.warn(ctx, nfoPath, code)
	}

	if len(result.Records) == 0 {
		return metadata.Record{}, false
	}

	return result.Records[0], true
}

// warn logs a problem with an NFO at debug level (relative path and code only) and counts it.
func (run *refreshRun) warn(ctx context.Context, relativePath, code string) {
	run.warnings++
	run.refresher.deps.Logger.LogAttrs(ctx, slog.LevelDebug, "scan could not use nfo",
		slog.String("library", run.request.Library.ID), slog.String("path", relativePath), slog.String("code", code))
}

// nfoCandidates lists the NFO files of the item in the order of ADR 0027 §7; when the item
// has files in several folders, those of the first folder come first.
func (run *refreshRun) nfoCandidates(item *refreshItem) []fileKey {
	switch item.level.Kind {
	case library.ItemKindMovie:
		return run.movieNFOCandidates(item)
	case library.ItemKindShow:
		return perFolderFile(item, item.showDirectory, showNFOName)
	case library.ItemKindSeason:
		return perFolderFile(item, item.seasonDirectory, seasonNFOName)
	case library.ItemKindEpisode:
		return siblingNFOs(item.own)
	case library.ItemKindExtra, library.ItemKindArtist, library.ItemKindAlbum, library.ItemKindTrack,
		library.ItemKindPhotoAlbum, library.ItemKindPhoto, library.ItemKindAudiobook:
		return nil
	default:
		return nil
	}
}

func (run *refreshRun) movieNFOCandidates(item *refreshItem) []fileKey {
	candidates := siblingNFOs(item.own)

	for _, file := range item.own {
		directory := keyOf(file.State).directory()
		if run.index.isFolderMovie(directory) {
			candidates = append(candidates, fileKey{rootID: directory.rootID, path: path.Join(directory.path, movieNFOName)})
		}
	}

	return candidates
}

// siblingNFOs lists "<base>.nfo" next to each file.
func siblingNFOs(files []RefreshFile) []fileKey {
	candidates := make([]fileKey, 0, len(files))

	for _, file := range files {
		candidates = append(candidates, fileKey{
			rootID: file.State.RootID,
			path:   strings.TrimSuffix(file.State.RelativePath, path.Ext(file.State.RelativePath)) + nfoExtension,
		})
	}

	return candidates
}

// perFolderFile lists the file called name in the directory directoryOf finds in each folder
// of the item.
func perFolderFile(item *refreshItem, directoryOf func(rootID string) string, name string) []fileKey {
	var candidates []fileKey

	for _, rootID := range item.rootIDs() {
		if directory := directoryOf(rootID); directory != "" {
			candidates = append(candidates, fileKey{rootID: rootID, path: path.Join(directory, name)})
		}
	}

	return candidates
}

// sidecarIndex finds sidecars case-insensitively, as SMB shares and exFAT do not keep case.
// Folders are kept apart: a candidate only matches a sidecar of the same folder.
type sidecarIndex struct {
	byLowerPath map[fileKey]library.FileState
	byPath      map[fileKey]library.FileState
	byDirectory map[fileKey][]library.FileState
}

func newSidecarIndex(sidecars []library.FileState) sidecarIndex {
	index := sidecarIndex{
		byLowerPath: map[fileKey]library.FileState{}, byPath: map[fileKey]library.FileState{},
		byDirectory: map[fileKey][]library.FileState{},
	}

	for _, state := range sidecars {
		key := keyOf(state)
		index.byPath[key] = state
		index.byLowerPath[fileKey{rootID: key.rootID, path: strings.ToLower(key.path)}] = state
		index.byDirectory[key.directory()] = append(index.byDirectory[key.directory()], state)
	}

	for _, states := range index.byDirectory {
		slices.SortFunc(states, func(first, second library.FileState) int {
			return strings.Compare(first.RelativePath, second.RelativePath)
		})
	}

	return index
}

// first returns the actual key of the first NFO candidate that exists.
func (index sidecarIndex) first(candidates []fileKey) (fileKey, bool) {
	for _, candidate := range candidates {
		state, found := index.byPath[candidate]
		if !found {
			state, found = index.byLowerPath[fileKey{rootID: candidate.rootID, path: strings.ToLower(candidate.path)}]
		}

		if found && state.Class == library.FileClassNFO {
			return keyOf(state), true
		}
	}

	return fileKey{}, false
}

// images lists the image sidecars directly in directory.
func (index sidecarIndex) images(directory fileKey) []library.FileState {
	var images []library.FileState

	for _, state := range index.byDirectory[directory] {
		if state.Class == library.FileClassImage {
			images = append(images, state)
		}
	}

	return images
}

// sameRecord compares two records by value. A merged record without a sort title stands for
// the sort title the catalog derives from the title.
func sameRecord(stored, merged metadata.Record) bool {
	return stored.Title == merged.Title && metadata.SortTitleFor(stored) == metadata.SortTitleFor(merged) &&
		stored.OriginalTitle == merged.OriginalTitle && stored.Year == merged.Year &&
		stored.ReleaseDate == merged.ReleaseDate && stored.Summary == merged.Summary &&
		stored.Tagline == merged.Tagline && stored.ContentRating == merged.ContentRating &&
		slices.Equal(stored.Genres, merged.Genres) && slices.Equal(stored.Studios, merged.Studios) &&
		slices.Equal(stored.Countries, merged.Countries) && slices.Equal(stored.Collections, merged.Collections) &&
		slices.Equal(stored.People, merged.People) && slices.Equal(stored.ProviderIDs, merged.ProviderIDs)
}

package scan_test

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// fakeRootPrefix is the absolute path prefix the fake resolver puts before a relative path.
const fakeRootPrefix = "/abs/"

// memoryLibraries is an in-memory library source.
type memoryLibraries struct {
	mutex     sync.Mutex
	libraries []library.Library
	listErr   error
	getErr    error
	// beforeGet, when set, runs at the start of every GetLibrary call.
	beforeGet func()
}

func (source *memoryLibraries) GetLibrary(_ context.Context, libraryID string) (library.Summary, error) {
	if source.beforeGet != nil {
		source.beforeGet()
	}

	source.mutex.Lock()
	defer source.mutex.Unlock()

	if source.getErr != nil {
		return library.Summary{}, source.getErr
	}

	for _, candidate := range source.libraries {
		if candidate.ID == libraryID {
			return library.Summary{Library: candidate}, nil
		}
	}

	return library.Summary{}, fmt.Errorf("library %s: %w", libraryID, domain.ErrNotFound)
}

func (source *memoryLibraries) ListLibraries(context.Context) ([]library.Summary, error) {
	source.mutex.Lock()
	defer source.mutex.Unlock()

	if source.listErr != nil {
		return nil, source.listErr
	}

	summaries := make([]library.Summary, 0, len(source.libraries))
	for _, candidate := range source.libraries {
		summaries = append(summaries, library.Summary{Library: candidate})
	}

	return summaries, nil
}

// walkResult is what the fake walker reports for one root.
type walkResult struct {
	files   []library.FileState
	skipped []string
	// failAfter, when set, is returned after every file was visited.
	failAfter error
}

// scriptedWalker visits the configured files of a root. Tests replace the script between
// scans with set; started and release, when set, pause each walk until the test lets it go.
// A stubborn walker ignores cancellation while paused, like a walk stuck on a hung NAS mount.
type scriptedWalker struct {
	mutex    sync.Mutex
	results  map[string]walkResult
	started  chan struct{}
	release  chan struct{}
	stubborn bool
	calls    int
}

func newScriptedWalker() *scriptedWalker {
	return &scriptedWalker{results: map[string]walkResult{}}
}

func (walker *scriptedWalker) set(root string, result walkResult) {
	walker.mutex.Lock()
	defer walker.mutex.Unlock()

	walker.results[root] = result
}

// forget removes the script of root, so the next walk reports the root as unavailable.
func (walker *scriptedWalker) forget(root string) {
	walker.mutex.Lock()
	defer walker.mutex.Unlock()

	delete(walker.results, root)
}

func (walker *scriptedWalker) Walk(
	ctx context.Context, root string, visit func(library.FileState) error,
) ([]string, error) {
	walker.mutex.Lock()
	result, found := walker.results[root]
	walker.calls++
	started, release, stubborn := walker.started, walker.release, walker.stubborn
	walker.mutex.Unlock()

	pauseContext := ctx
	if stubborn {
		pauseContext = context.WithoutCancel(ctx)
	}

	if err := pause(pauseContext, started, release); err != nil {
		return nil, err
	}

	if !found {
		return nil, fmt.Errorf("walk %s: %w", root, domain.ErrUnavailable)
	}

	for _, file := range result.files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		if err := visit(file); err != nil {
			return nil, err
		}
	}

	return slices.Clone(result.skipped), result.failAfter
}

func (walker *scriptedWalker) callCount() int {
	walker.mutex.Lock()
	defer walker.mutex.Unlock()

	return walker.calls
}

// pause signals started and waits for release or the end of ctx; nil channels skip it.
func pause(ctx context.Context, started, release chan struct{}) error {
	if started != nil {
		select {
		case started <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if release == nil {
		return nil
	}

	select {
	case <-release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// prefixResolver joins fakeRootPrefix, the root and the relative path, or fails for the
// relative paths listed in errs.
type prefixResolver struct {
	errs map[string]error
}

func (resolver prefixResolver) Resolve(root, relativePath string) (string, error) {
	if err := resolver.errs[relativePath]; err != nil {
		return "", err
	}

	return absolutePath(root, relativePath), nil
}

func absolutePath(root, relativePath string) string {
	return fakeRootPrefix + root + "/" + relativePath
}

// probeOutcome is the scripted result of probing one absolute path.
type probeOutcome struct {
	description media.Description
	err         error
}

// recordingProber returns scripted outcomes (default: one H.264 video stream) and records
// calls and the largest number of concurrent probes. When gate is set, every probe waits
// for a token from gate or for the end of its context.
type recordingProber struct {
	mutex          sync.Mutex
	outcomes       map[string]probeOutcome
	gate           chan struct{}
	entered        chan struct{}
	probed         []string
	active         int
	maxActive      int
	canceledProbes int
}

func newRecordingProber() *recordingProber {
	return &recordingProber{outcomes: map[string]probeOutcome{}}
}

func (prober *recordingProber) script(path string, outcome probeOutcome) {
	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	prober.outcomes[path] = outcome
}

func (prober *recordingProber) Probe(ctx context.Context, path string) (media.Description, error) {
	outcome := prober.enter(path)
	defer prober.leave()

	if err := prober.wait(ctx); err != nil {
		return media.Description{}, err
	}

	return outcome.description, outcome.err
}

func (prober *recordingProber) enter(path string) probeOutcome {
	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	prober.probed = append(prober.probed, path)
	prober.active++
	prober.maxActive = max(prober.maxActive, prober.active)

	if outcome, found := prober.outcomes[path]; found {
		return outcome
	}

	return probeOutcome{description: videoDescription()}
}

func (prober *recordingProber) wait(ctx context.Context) error {
	if prober.entered != nil {
		select {
		case prober.entered <- struct{}{}:
		case <-ctx.Done():
		}
	}

	if prober.gate == nil {
		return nil
	}

	select {
	case <-prober.gate:
		return nil
	case <-ctx.Done():
		prober.mutex.Lock()
		prober.canceledProbes++
		prober.mutex.Unlock()

		return fmt.Errorf("probe: %w", ctx.Err())
	}
}

func (prober *recordingProber) leave() {
	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	prober.active--
}

func (prober *recordingProber) probedPaths() []string {
	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	return slices.Sorted(slices.Values(prober.probed))
}

func (prober *recordingProber) stats() (activeNow, maxActive, canceled int) {
	prober.mutex.Lock()
	defer prober.mutex.Unlock()

	return prober.active, prober.maxActive, prober.canceledProbes
}

// memoryCatalog is an in-memory catalog writer keyed by file ID. It enforces the record
// rules of the sqlite adapter (see scan.ProbedFile) and, like the adapter, ignores unknown
// or foreign IDs in RemoveFiles. Both kinds of misuse are also recorded as violations,
// which the harness asserts never happen: the scan must not depend on them.
type memoryCatalog struct {
	mutex sync.Mutex
	files map[string]scan.ProbedFile
	// libraryKinds maps library IDs to their kinds for the item kind rule.
	libraryKinds map[string]library.Kind
	violations   []string
	removed      []string
	statesErr    error
	saveErr      error
	removeErr    error
	saves        int
}

func newMemoryCatalog() *memoryCatalog {
	return &memoryCatalog{files: map[string]scan.ProbedFile{}}
}

func (catalog *memoryCatalog) FileStates(_ context.Context, libraryID string) ([]scan.CatalogFile, error) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.statesErr != nil {
		return nil, catalog.statesErr
	}

	var states []scan.CatalogFile

	for _, stored := range catalog.files {
		if stored.File.LibraryID != libraryID {
			continue
		}

		states = append(states, scan.CatalogFile{
			ID:     stored.File.ID,
			ItemID: stored.File.ItemID,
			State: library.FileState{
				RelativePath: stored.File.RelativePath,
				Size:         stored.File.Size,
				ModifiedAt:   stored.File.ModifiedAt,
			},
		})
	}

	return states, nil
}

func (catalog *memoryCatalog) SaveProbedFile(_ context.Context, record scan.ProbedFile) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.saveErr != nil {
		return catalog.saveErr
	}

	if violation := catalog.checkRecord(record); violation != "" {
		catalog.violations = append(catalog.violations, violation)

		return fmt.Errorf("save %s: %s: %w", record.File.RelativePath, violation, domain.ErrInvalid)
	}

	catalog.saves++
	catalog.files[record.File.ID] = record

	return nil
}

// checkRecord applies the rules of scan.ProbedFile; the empty string means none is broken.
func (catalog *memoryCatalog) checkRecord(record scan.ProbedFile) string {
	item, file := record.Item, record.File

	switch {
	case record.ProbedAt.IsZero():
		return "zero probed time"
	case file.Validate() != nil:
		return "invalid file: " + file.Validate().Error()
	case item.LibraryID != file.LibraryID:
		return "item and file belong to different libraries"
	case item.ID != file.ItemID:
		return "file does not point at the item"
	case catalog.hasItem(item.ID):
		return ""
	}

	return checkNewItem(item, catalog.libraryKinds[item.LibraryID])
}

func checkNewItem(item library.Item, libraryKind library.Kind) string {
	wantKind, err := library.ItemKindForLibrary(libraryKind)

	switch {
	case item.CreatedAt.IsZero():
		return "new item without creation time"
	case strings.TrimSpace(item.Title) == "":
		return "new item with a blank title"
	case err != nil || item.Kind != wantKind:
		return fmt.Sprintf("new item kind %q in a %q library", item.Kind, libraryKind)
	default:
		return ""
	}
}

func (catalog *memoryCatalog) hasItem(itemID string) bool {
	for _, stored := range catalog.files {
		if stored.Item.ID == itemID {
			return true
		}
	}

	return false
}

func (catalog *memoryCatalog) RemoveFiles(_ context.Context, libraryID string, fileIDs []string) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.removeErr != nil {
		return catalog.removeErr
	}

	for _, fileID := range fileIDs {
		stored, found := catalog.files[fileID]
		if !found || stored.File.LibraryID != libraryID {
			catalog.violations = append(catalog.violations,
				fmt.Sprintf("remove unknown or foreign file %s from library %s", fileID, libraryID))

			continue
		}

		delete(catalog.files, fileID)
		catalog.removed = append(catalog.removed, fileID)
	}

	return nil
}

func (catalog *memoryCatalog) violationList() []string {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return slices.Clone(catalog.violations)
}

// byPath returns the stored records keyed by relative path.
func (catalog *memoryCatalog) byPath() map[string]scan.ProbedFile {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	records := make(map[string]scan.ProbedFile, len(catalog.files))
	for _, record := range catalog.files {
		records[record.File.RelativePath] = record
	}

	return records
}

func (catalog *memoryCatalog) paths() []string {
	return slices.Sorted(maps.Keys(catalog.byPath()))
}

func (catalog *memoryCatalog) saveCount() int {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.saves
}

// counterIDs returns "id-1", "id-2", … or err when set.
type counterIDs struct {
	mutex sync.Mutex
	next  int
	err   error
}

func (generator *counterIDs) NewID() (string, error) {
	generator.mutex.Lock()
	defer generator.mutex.Unlock()

	if generator.err != nil {
		return "", generator.err
	}

	generator.next++

	return fmt.Sprintf("id-%d", generator.next), nil
}

// tickingClock advances by one second per call, so start and finish times differ.
type tickingClock struct {
	mutex sync.Mutex
	now   time.Time
}

func (clock *tickingClock) Now() time.Time {
	clock.mutex.Lock()
	defer clock.mutex.Unlock()

	clock.now = clock.now.Add(time.Second)

	return clock.now
}

// syncBuffer is a goroutine-safe log sink.
type syncBuffer struct {
	mutex   sync.Mutex
	builder strings.Builder
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	written, err := buffer.builder.Write(data)
	if err != nil {
		return written, fmt.Errorf("write log: %w", err)
	}

	return written, nil
}

func (buffer *syncBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.builder.String()
}

func videoDescription() media.Description {
	return media.Description{
		Container:  media.ContainerMKV,
		FormatName: "matroska,webm",
		Duration:   time.Minute,
		Streams: []media.Stream{
			{Index: 0, Kind: media.StreamKindVideo, Codec: "h264", Width: 1920, Height: 1080},
			{Index: 1, Kind: media.StreamKindAudio, Codec: "aac", Channels: 2},
		},
	}
}

func audioOnlyDescription() media.Description {
	return media.Description{
		Container: media.ContainerMKV,
		Streams:   []media.Stream{{Index: 0, Kind: media.StreamKindAudio, Codec: "flac"}},
	}
}

// blockingHandler pauses every record with the given message: it signals entered and waits
// for release before the record is written. entered must be buffered for every such record.
type blockingHandler struct {
	slog.Handler

	message string
	entered chan struct{}
	release chan struct{}
}

func (handler blockingHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Message == handler.message {
		handler.entered <- struct{}{}
		<-handler.release
	}

	if err := handler.Handler.Handle(ctx, record); err != nil {
		return fmt.Errorf("handle log record: %w", err)
	}

	return nil
}

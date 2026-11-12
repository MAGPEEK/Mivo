package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Dependencies lists everything Service needs. Every field is required.
type Dependencies struct {
	// Libraries looks up the libraries to scan.
	Libraries LibrarySource
	// Walker lists the video files of a root.
	Walker Walker
	// Resolver turns root-relative paths into absolute paths for the prober.
	Resolver PathResolver
	// Prober describes media files.
	Prober Prober
	// CatalogV2 persists the hierarchy, files, sidecars and metadata of a scan.
	CatalogV2 CatalogV2
	// Refresher optionally recomputes metadata of the items a scan touched; nil does nothing.
	Refresher ItemRefresher
	// Music holds the ports of the music pipeline; a music library fails to scan without them.
	Music MusicPorts
	// Audiobooks holds the ports of the audiobook pipeline; an audiobooks library fails to scan
	// without them.
	Audiobooks AudiobookPorts
	// Photos holds the ports of the photo pipeline; a photos library fails to scan without
	// them.
	Photos PhotoPorts
	// Discs holds the ports that resolve and probe disc titles; without them discs are
	// skipped and stored entries at or below them kept (DiscPorts).
	Discs DiscPorts
	// Subtitles holds the optional ports that record external subtitle files; without them the
	// scan leaves subtitles alone (SubtitlePorts).
	Subtitles SubtitlePorts
	// Finished, when set, is called after every scan ended, whatever its outcome.
	Finished func(ctx context.Context, libraryID string)
	// Enrichment, when set, queues online enrichment of the items a scan refreshed.
	Enrichment EnrichmentRequester
	// IDs creates item and file IDs.
	IDs IDGenerator
	// Clock returns the current time; injected so tests are deterministic.
	Clock func() time.Time
	// Logger receives one line per finished scan and per-file problems at debug level.
	Logger *slog.Logger
}

// Service runs library scans in the background, at most one per library. It owns the scan
// goroutines: they run on a context created by NewService, never on a caller's context, and
// Close cancels and waits for them. It is safe for concurrent use.
type Service struct {
	libraries LibrarySource
	walker    Walker
	resolver  PathResolver
	prober    Prober
	catalogV2 CatalogV2
	refresher ItemRefresher
	music     MusicPorts
	audiobook AudiobookPorts
	photo     PhotoPorts
	discs     DiscPorts
	subtitles SubtitlePorts
	finished  func(ctx context.Context, libraryID string)
	enrich    EnrichmentRequester
	ids       IDGenerator
	clock     func() time.Time
	logger    *slog.Logger

	// scanContext is the parent of every scan; cancelScans ends them on Close. It is stored
	// because the scans outlive the request that started them (ADR 0022).
	scanContext context.Context
	cancelScans context.CancelFunc
	// logContext carries the values of scanContext but is never cancelled, so the finish
	// line of a scan stopped by Close is still written.
	logContext context.Context

	// probeSlots is a semaphore with MaxConcurrentProbes slots shared by every scan, so
	// StartAll over several libraries on one NAS still runs at most that many probes.
	probeSlots chan struct{}
	// photoReadSlots bounds the photo metadata reads of every scan the same way
	// (MaxConcurrentPhotoReads).
	photoReadSlots chan struct{}

	mutex    sync.Mutex
	trackers map[string]*tracker
	// withoutVideo holds, per library ID and relative path, the files whose probe found no
	// video stream, so an unchanged one is not probed again on every scan.
	withoutVideo map[string]map[fileKey]library.FileState
	// lastOptions holds the options of the last scan that ended successfully, per library ID.
	lastOptions map[string]library.Options
	// pendingRefresh holds, per library ID, the items whose refresh waits for an unavailable
	// folder to return (memory only, like the options).
	pendingRefresh map[string]map[string]struct{}
	// detailRetries holds, per library ID, the files that were saved but whose details
	// (audiobook chapters, photo details) could not be written; the next scan reads them again
	// although they are unchanged.
	detailRetries map[string]map[fileKey]struct{}
	// unsupportedDiscs holds, per library ID, the discs this server start probed as
	// disc_unsupported; an unchanged one is probed again only after a restart (needsProbe).
	unsupportedDiscs map[string]map[fileKey]struct{}
	// reanalyses holds, per item ID, the channel closed when its running re-analysis ends.
	reanalyses map[string]chan struct{}
	closed     bool
}

// NewService validates the dependencies and returns a Service. A missing dependency is a
// wiring bug and is reported as an error wrapping domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new scan service: %w", err)
	}

	scanContext, cancelScans := context.WithCancel(context.Background())

	refresher := dependencies.Refresher
	if refresher == nil {
		refresher = noRefresh{}
	}

	return &Service{
		catalogV2:   dependencies.CatalogV2,
		refresher:   refresher,
		music:       dependencies.Music,
		audiobook:   dependencies.Audiobooks,
		photo:       dependencies.Photos,
		discs:       dependencies.Discs,
		subtitles:   dependencies.Subtitles,
		finished:    dependencies.Finished,
		enrich:      dependencies.Enrichment,
		libraries:   dependencies.Libraries,
		walker:      dependencies.Walker,
		resolver:    dependencies.Resolver,
		prober:      dependencies.Prober,
		ids:         dependencies.IDs,
		clock:       dependencies.Clock,
		logger:      dependencies.Logger,
		scanContext: scanContext,
		cancelScans: cancelScans,
		logContext:  context.WithoutCancel(scanContext),
		probeSlots:  make(chan struct{}, MaxConcurrentProbes),

		photoReadSlots: make(chan struct{}, MaxConcurrentPhotoReads),
		trackers:       map[string]*tracker{},
		lastOptions:    map[string]library.Options{},

		pendingRefresh: map[string]map[string]struct{}{},
		detailRetries:  map[string]map[fileKey]struct{}{},
		withoutVideo:   map[string]map[fileKey]library.FileState{},

		unsupportedDiscs: map[string]map[fileKey]struct{}{},
		reanalyses:       map[string]chan struct{}{},
	}, nil
}

func (dependencies Dependencies) validate() error {
	present := []struct {
		name string
		set  bool
	}{
		{name: "libraries", set: dependencies.Libraries != nil},
		{name: "walker", set: dependencies.Walker != nil},
		{name: "resolver", set: dependencies.Resolver != nil},
		{name: "prober", set: dependencies.Prober != nil},
		{name: "catalog", set: dependencies.CatalogV2 != nil},
		{name: "id generator", set: dependencies.IDs != nil},
		{name: "clock", set: dependencies.Clock != nil},
		{name: "logger", set: dependencies.Logger != nil},
	}

	var missing []error

	for _, dependency := range present {
		if !dependency.set {
			missing = append(missing, fmt.Errorf("missing dependency %s: %w", dependency.name, domain.ErrInvalid))
		}
	}

	return errors.Join(missing...)
}

// Start starts a background scan of the library and returns its status. When a scan of the
// library already runs, no second one starts and the running scan's status is returned.
// ctx only bounds the library lookup; the scan itself runs until it finishes or Close is
// called. Errors wrap domain.ErrNotFound for an unknown library and domain.ErrUnavailable
// after Close.
func (service *Service) Start(ctx context.Context, libraryID string) (Status, error) {
	if err := service.ensureOpen(); err != nil {
		return Status{}, fmt.Errorf("start scan of library %s: %w", libraryID, err)
	}

	summary, err := service.libraries.GetLibrary(ctx, libraryID)
	if err != nil {
		return Status{}, fmt.Errorf("start scan of library %s: %w", libraryID, err)
	}

	status, err := service.launch(summary)
	if err != nil {
		return Status{}, fmt.Errorf("start scan of library %s: %w", libraryID, err)
	}

	return status, nil
}

// StartAll starts a scan of every library, for example at server start (OD-3). Libraries
// whose scan already runs keep it. Errors wrap domain.ErrUnavailable after Close.
func (service *Service) StartAll(ctx context.Context) error {
	if err := service.ensureOpen(); err != nil {
		return fmt.Errorf("start all scans: %w", err)
	}

	summaries, err := service.libraries.ListLibraries(ctx)
	if err != nil {
		return fmt.Errorf("start all scans: %w", err)
	}

	for _, summary := range summaries {
		if _, err := service.launch(summary); err != nil {
			return fmt.Errorf("start all scans: library %s: %w", summary.ID, err)
		}
	}

	return nil
}

// Status returns the status of the current or last scan of the library; an idle zero status
// when it was never scanned since the server started (statuses are kept in memory only).
func (service *Service) Status(libraryID string) Status {
	service.mutex.Lock()
	scanTracker := service.trackers[libraryID]
	service.mutex.Unlock()

	if scanTracker == nil {
		return Status{State: StateIdle}
	}

	return scanTracker.snapshot()
}

// Close stops accepting scans, cancels the running ones and waits until they have ended or
// ctx is done. It may be called again, for example after a timeout, to keep waiting.
func (service *Service) Close(ctx context.Context) error {
	service.mutex.Lock()
	service.closed = true
	service.cancelScans()

	pending := make([]<-chan struct{}, 0, len(service.trackers)+len(service.reanalyses))
	for _, scanTracker := range service.trackers {
		pending = append(pending, scanTracker.done)
	}

	for _, done := range service.reanalyses {
		pending = append(pending, done)
	}
	service.mutex.Unlock()

	for _, done := range pending {
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("close scan service: %w", ctx.Err())
		}
	}

	return nil
}

func (service *Service) ensureOpen() error {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	if service.closed {
		return fmt.Errorf("scan service is closed: %w", domain.ErrUnavailable)
	}

	return nil
}

// launch starts the scan goroutine unless the service is closed or a scan of the library
// already runs. Close is checked again here because it may have run during the lookup.
func (service *Service) launch(scanned library.ManagedLibrary) (Status, error) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	if service.closed {
		return Status{}, fmt.Errorf("scan service is closed: %w", domain.ErrUnavailable)
	}

	if previous := service.trackers[scanned.ID]; previous != nil {
		if status, running := previous.current(); running {
			return status, nil
		}
	}

	scanTracker := newTracker(service.scanContext, scanned.Name, service.clock())
	service.trackers[scanned.ID] = scanTracker

	go service.runScan(scanned, scanTracker)

	return scanTracker.snapshot(), nil
}

// runScan executes one scan, logs its end and only then publishes the final status: until
// publish closes done the scan counts as running, so neither a new Start nor Close can miss
// this goroutine. The options of a scan that succeeded are remembered for the next one.
func (service *Service) runScan(scanned library.ManagedLibrary, scanTracker *tracker) {
	// Cancel ends this scan only, Close all of them (through the parent context).
	scanContext := scanTracker.scanContext
	defer scanTracker.cancel()

	err := service.pipelineFor(scanned, scanTracker).execute(scanContext)
	code, cause := describeEnd(err, scanContext.Err() != nil)
	status := scanTracker.final(code, service.clock())

	// A scan that could not see every folder did not apply the options everywhere: the full
	// refresh stays due.
	if err == nil && status.UnavailableFolders == 0 {
		service.rememberOptions(scanned.ID, scanned.Options)
	}

	service.logFinished(scanned, status, cause)

	if service.finished != nil {
		service.finished(service.logContext, scanned.ID)
	}

	scanTracker.publish(status)
}

// Cancel stops the running scan of the library, if any, and returns when it has ended or ctx
// is done. The scan ends with ErrorCodeCanceled; it removes nothing it had not already
// justified, because removals only follow a completed walk (ADR 0022). It does nothing for a
// library without a running scan. Library management uses it before changing or removing a
// library. Cancel does not stop new scans: the library manager suspends the scan triggers before it cancels
// and until the library change, or a trigger can start a scan on the old definition.
func (service *Service) Cancel(ctx context.Context, libraryID string) error {
	scanTracker := service.trackerOf(libraryID)
	if scanTracker == nil {
		return nil
	}

	scanTracker.cancel()

	return awaitScan(ctx, scanTracker, "cancel scan of library "+libraryID)
}

// WaitIdle returns when no scan of the library runs or ctx is done.
func (service *Service) WaitIdle(ctx context.Context, libraryID string) error {
	scanTracker := service.trackerOf(libraryID)
	if scanTracker == nil {
		return nil
	}

	return awaitScan(ctx, scanTracker, "wait for scan of library "+libraryID)
}

// Forget drops what the service remembers in memory about a library that was deleted: the
// options of its last scan, the refresh waiting for an unavailable folder, the files known
// to have no video and the discs probed as unsupported. Call it after Cancel/WaitIdle, or a scan that is still ending stores them
// again. It never touches the catalog.
func (service *Service) Forget(libraryID string) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	delete(service.lastOptions, libraryID)
	delete(service.pendingRefresh, libraryID)
	delete(service.withoutVideo, libraryID)
	delete(service.detailRetries, libraryID)
	delete(service.unsupportedDiscs, libraryID)
}

func (service *Service) trackerOf(libraryID string) *tracker {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return service.trackers[libraryID]
}

func awaitScan(ctx context.Context, scanTracker *tracker, what string) error {
	select {
	case <-scanTracker.done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s: %w", what, ctx.Err())
	}
}

// optionsChanged reports whether the library was scanned to the end with other options than
// options. A library never scanned since the server started reports false: its stored
// metadata already reflects the options in force, so the first scan refreshes changed items
// only.
func (service *Service) optionsChanged(libraryID string, options library.Options) bool {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	last, known := service.lastOptions[libraryID]

	return known && last != options
}

// takePendingRefresh returns and clears the items waiting for a refresh.
func (service *Service) takePendingRefresh(libraryID string) []string {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	ids := slices.Collect(maps.Keys(service.pendingRefresh[libraryID]))
	delete(service.pendingRefresh, libraryID)

	return ids
}

// peekPendingRefresh lists the items waiting for a refresh without taking them; the music
// pipeline needs them to know which unchanged tracks to read before the refresh takes them.
func (service *Service) peekPendingRefresh(libraryID string) []string {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return slices.Collect(maps.Keys(service.pendingRefresh[libraryID]))
}

// addPendingRefresh marks items as waiting for a refresh.
func (service *Service) addPendingRefresh(libraryID string, ids []string) {
	if len(ids) == 0 {
		return
	}

	service.mutex.Lock()
	defer service.mutex.Unlock()

	if service.pendingRefresh[libraryID] == nil {
		service.pendingRefresh[libraryID] = map[string]struct{}{}
	}

	for _, id := range ids {
		service.pendingRefresh[libraryID][id] = struct{}{}
	}
}

func (service *Service) rememberOptions(libraryID string, options library.Options) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.lastOptions[libraryID] = options
}

// logFinished writes the one line per scan. A failure is logged with its code and a cause
// that contains no absolute path; the wrapped error is not logged because walker, resolver
// and prober errors may name the library root.
func (service *Service) logFinished(scanned library.ManagedLibrary, status Status, cause string) {
	level := slog.LevelInfo
	attributes := []slog.Attr{
		slog.String("library", scanned.ID),
		slog.String("name", scanned.Name),
		slog.String("state", string(status.State)),
		slog.Duration("duration", status.FinishedAt.Sub(status.StartedAt)),
		slog.Int64("added", status.Counts.Added),
		slog.Int64("updated", status.Counts.Updated),
		slog.Int64("removed", status.Counts.Removed),
		slog.Int64("unchanged", status.Counts.Unchanged),
		slog.Int64("failed", status.Counts.Failed),
		slog.Int64("skipped", status.Counts.Skipped),
	}

	if status.ErrorCode != ErrorCodeNone {
		attributes = append(attributes, slog.String("code", string(status.ErrorCode)))
	}

	if status.ErrorCode != ErrorCodeNone && status.ErrorCode != ErrorCodeCanceled {
		level = slog.LevelWarn
		attributes = append(attributes, slog.String("cause", cause))
	}

	service.logger.LogAttrs(service.logContext, level, "library scan finished", attributes...)
}

// filesWithoutVideo returns a copy of the remembered files of the library whose probe found
// no video stream.
func (service *Service) filesWithoutVideo(libraryID string) map[fileKey]library.FileState {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return maps.Clone(service.withoutVideo[libraryID])
}

// rememberFilesWithoutVideo replaces the remembered files without a video stream of the
// library. They are kept in memory only, like the scan status (ADR 0022 notes): after a
// restart they are probed once more.
func (service *Service) rememberFilesWithoutVideo(libraryID string, files map[fileKey]library.FileState) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.withoutVideo[libraryID] = files
}

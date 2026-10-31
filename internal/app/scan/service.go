package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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
	// Catalog persists the scan results.
	Catalog CatalogWriter
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
	catalog   CatalogWriter
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

	mutex    sync.Mutex
	trackers map[string]*tracker
	// withoutVideo holds, per library ID and relative path, the files whose probe found no
	// video stream, so an unchanged one is not probed again on every scan.
	withoutVideo map[string]map[string]library.FileState
	closed       bool
}

// NewService validates the dependencies and returns a Service. A missing dependency is a
// wiring bug and is reported as an error wrapping domain.ErrInvalid.
func NewService(dependencies Dependencies) (*Service, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new scan service: %w", err)
	}

	scanContext, cancelScans := context.WithCancel(context.Background())

	return &Service{
		libraries:    dependencies.Libraries,
		walker:       dependencies.Walker,
		resolver:     dependencies.Resolver,
		prober:       dependencies.Prober,
		catalog:      dependencies.Catalog,
		ids:          dependencies.IDs,
		clock:        dependencies.Clock,
		logger:       dependencies.Logger,
		scanContext:  scanContext,
		cancelScans:  cancelScans,
		logContext:   context.WithoutCancel(scanContext),
		probeSlots:   make(chan struct{}, MaxConcurrentProbes),
		trackers:     map[string]*tracker{},
		withoutVideo: map[string]map[string]library.FileState{},
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
		{name: "catalog", set: dependencies.Catalog != nil},
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

	status, err := service.launch(summary.Library)
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
		if _, err := service.launch(summary.Library); err != nil {
			return fmt.Errorf("start all scans: library %s: %w", summary.Library.ID, err)
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

	pending := make([]*tracker, 0, len(service.trackers))
	for _, scanTracker := range service.trackers {
		pending = append(pending, scanTracker)
	}
	service.mutex.Unlock()

	for _, scanTracker := range pending {
		select {
		case <-scanTracker.done:
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
func (service *Service) launch(scanned library.Library) (Status, error) {
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

	scanTracker := newTracker(service.clock())
	service.trackers[scanned.ID] = scanTracker

	go service.runScan(scanned, scanTracker)

	return scanTracker.snapshot(), nil
}

// runScan executes one scan, logs its end and only then publishes the final status: until
// publish closes done the scan counts as running, so neither a new Start nor Close can miss
// this goroutine.
func (service *Service) runScan(scanned library.Library, scanTracker *tracker) {
	err := service.newRun(scanned, scanTracker).execute(service.scanContext)
	code, cause := describeEnd(err, service.scanContext.Err() != nil)
	status := scanTracker.final(code, service.clock())

	service.logFinished(scanned, status, cause)
	scanTracker.publish(status)
}

// logFinished writes the one line per scan. A failure is logged with its code and a cause
// that contains no absolute path; the wrapped error is not logged because walker, resolver
// and prober errors may name the library root.
func (service *Service) logFinished(scanned library.Library, status Status, cause string) {
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
func (service *Service) filesWithoutVideo(libraryID string) map[string]library.FileState {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return maps.Clone(service.withoutVideo[libraryID])
}

// rememberFilesWithoutVideo replaces the remembered files without a video stream of the
// library. They are kept in memory only, like the scan status (ADR 0022 notes): after a
// restart they are probed once more.
func (service *Service) rememberFilesWithoutVideo(libraryID string, files map[string]library.FileState) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	service.withoutVideo[libraryID] = files
}

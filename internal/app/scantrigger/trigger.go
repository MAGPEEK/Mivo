package scantrigger

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Timing of the automatic scans (ADR 0027 §9).
const (
	// QuietPeriod is how long a library must be free of changes before its scan starts, so
	// copying a season starts one scan instead of one per file.
	QuietPeriod = 30 * time.Second
	// MaxDelay bounds the wait after the first change, so changes that never pause (a long
	// copy, a busy download folder) still show up.
	MaxDelay = 5 * time.Minute
	// CheckInterval is how often the library list is refreshed, failed watches are retried
	// and the periodic scans are checked.
	CheckInterval = time.Minute
	// EvaluationInterval is the tick of the debounce; the quiet period ends at most this
	// late, which is negligible against 30 s.
	EvaluationInterval = 5 * time.Second
)

// Log messages; the fallback depends on whether the periodic scan is on.
const (
	watchFailedMessage    = "cannot watch library folder"
	watchPartialMessage   = "library folder is only partly watched"
	periodicFallback      = "relying on the periodic scan"
	manualFallback        = "changes appear after the next manual scan"
	reasonWatch           = "watch"
	reasonPeriodic        = "periodic"
	scanStartedMessage    = "automatic scan started"
	scanNotStartedMessage = "automatic scan could not start"
)

// errWatchEnded reports a watch that ended without an error while it should still run.
var errWatchEnded = errors.New("watch ended unexpectedly")

// Settings switch the two parts of the trigger; the zero value turns both off.
type Settings struct {
	// Watch watches every library root and scans after changes.
	Watch bool
	// Interval rescans a library this long after its last scan finished; 0 turns the
	// periodic scan off.
	Interval time.Duration
}

// Dependencies lists everything Trigger needs. Every field except Settings is required.
type Dependencies struct {
	Libraries LibraryLister
	Scanner   Scanner
	Watcher   Watcher
	Clock     Clock
	Logger    *slog.Logger
	Settings  Settings
}

// Trigger starts scans after file system changes and periodically. Run drives it; the
// change callbacks of the watches and ReportWatchError may be called concurrently.
type Trigger struct {
	libraries LibraryLister
	scanner   Scanner
	watcher   Watcher
	clock     Clock
	logger    *slog.Logger
	settings  Settings

	// mutex guards states; the watch goroutines record changes while Run evaluates them.
	mutex  sync.Mutex
	states map[string]*libraryState
	// lastCheck and the watch group are used by Run's goroutine only.
	lastCheck time.Time
	watches   sync.WaitGroup
	// afterTick, when set by a test, is called after each handled tick.
	afterTick func()
}

// New validates the dependencies and returns a Trigger. A missing dependency or a negative
// interval is a wiring bug and is reported as an error wrapping domain.ErrInvalid.
func New(dependencies Dependencies) (*Trigger, error) {
	if err := dependencies.validate(); err != nil {
		return nil, fmt.Errorf("new scan trigger: %w", err)
	}

	return &Trigger{
		libraries: dependencies.Libraries,
		scanner:   dependencies.Scanner,
		watcher:   dependencies.Watcher,
		clock:     dependencies.Clock,
		logger:    dependencies.Logger,
		settings:  dependencies.Settings,
		states:    map[string]*libraryState{},
	}, nil
}

func (dependencies Dependencies) validate() error {
	present := []struct {
		name string
		set  bool
	}{
		{name: "library lister", set: dependencies.Libraries != nil},
		{name: "scanner", set: dependencies.Scanner != nil},
		{name: "watcher", set: dependencies.Watcher != nil},
		{name: "clock", set: dependencies.Clock != nil},
		{name: "logger", set: dependencies.Logger != nil},
	}

	var problems []error

	for _, dependency := range present {
		if !dependency.set {
			problems = append(problems, fmt.Errorf("missing dependency %s: %w", dependency.name, domain.ErrInvalid))
		}
	}

	if dependencies.Settings.Interval < 0 {
		problems = append(problems, fmt.Errorf("negative scan interval %s: %w", dependencies.Settings.Interval, domain.ErrInvalid))
	}

	return errors.Join(problems...)
}

// Run watches the libraries and starts scans until ctx is done. It returns after every
// watch it started has ended. Run must not be called twice at the same time.
func (trigger *Trigger) Run(ctx context.Context) {
	if !trigger.settings.Watch && trigger.settings.Interval == 0 {
		<-ctx.Done()
		return
	}

	ticker := trigger.clock.NewTicker(EvaluationInterval)

	defer func() {
		ticker.Stop()
		// Watches run on contexts derived from ctx, so they are ending already.
		trigger.watches.Wait()
		trigger.forgetLibraries()
	}()

	trigger.check(ctx, trigger.clock.Now())

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.Ticks():
			trigger.tick(ctx, now)
		}
	}
}

func (trigger *Trigger) tick(ctx context.Context, now time.Time) {
	if now.Sub(trigger.lastCheck) >= CheckInterval {
		trigger.check(ctx, now)
	}

	trigger.startDebouncedScans(ctx, now)

	if trigger.afterTick != nil {
		trigger.afterTick()
	}
}

// check refreshes the library list, (re)starts watches and starts the periodic scans that
// are due. When the list cannot be read, the known libraries stay as they are.
func (trigger *Trigger) check(ctx context.Context, now time.Time) {
	trigger.lastCheck = now

	summaries, err := trigger.libraries.ListLibraries(ctx)
	if err != nil {
		if ctx.Err() == nil {
			trigger.logger.Warn("automatic scans: cannot list libraries", slog.String("error", err.Error()))
		}
	} else {
		trigger.syncLibraries(ctx, summaries, now)
	}

	if trigger.settings.Interval > 0 {
		trigger.startPeriodicScans(ctx, now)
	}
}

// start starts a scan; reason tells the log why.
func (trigger *Trigger) start(ctx context.Context, libraryID, reason string) {
	if _, err := trigger.scanner.Start(ctx, libraryID); err != nil {
		if ctx.Err() == nil {
			trigger.logger.Warn(scanNotStartedMessage, slog.String("library", libraryID),
				slog.String("reason", reason), slog.String("error", err.Error()))
		}

		return
	}

	trigger.logger.Info(scanStartedMessage, slog.String("library", libraryID), slog.String("reason", reason))
}

// fallback names what keeps the library current when watching fails.
func (trigger *Trigger) fallback() string {
	if trigger.settings.Interval > 0 {
		return periodicFallback
	}

	return manualFallback
}

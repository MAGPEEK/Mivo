package playback

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// cleanupTimeout bounds removing a session's workspace. Removal runs detached from the
// request, so a client that hangs up mid-stop does not leave the directory behind.
const cleanupTimeout = 30 * time.Second

// minReencodingSessions is the smallest re-encoding limit: the configuration has no
// "transcoding off" value, a server without encoders reports it through its capabilities.
const minReencodingSessions = 1

// Dependencies lists the ports and helpers Service needs; all are required except Discs.
type Dependencies struct {
	// Discs plays disc sources; without them a disc fails with ErrDiscPlaybackUnavailable.
	Discs DiscPorts
	// Subtitles lists external subtitle tracks; without it only embedded tracks exist.
	Subtitles SubtitleTracks
	// Hardware learns about failed hardware encoders; optional.
	Hardware  HardwareStatus
	Files     FileSource
	Paths     PathResolver
	Keyframes KeyframeLocator
	Producer  StreamProducer
	Workspace Workspace
	URLs      StreamURLBuilder
	IDs       IDGenerator
	// Clock returns the current time; injected so tests control access times.
	Clock func() time.Time
	// After waits like time.After; injected so tests drive the idle reaper without sleeping.
	After  func(wait time.Duration) <-chan time.Time
	Logger *slog.Logger
}

// Options carries the configured values (ADR 0003: cmd maps the config into them).
type Options struct {
	// Capabilities are the FFmpeg capabilities detected at startup.
	Capabilities playback.ServerCapabilities
	// Policy holds the transcode height cap (OD-5 B: 1080).
	Policy playback.Policy
	// MaxTranscodeSessions limits concurrent sessions that re-encode a stream (OD-4 A,
	// default 2); between 1 and MaxFFmpegSessions.
	MaxTranscodeSessions int
	// ThrottleAhead pauses a production that is this far ahead of the viewer (ADR 0030
	// section 2); 0 turns throttling off.
	ThrottleAhead time.Duration
}

// Service implements the playback session use cases. It is safe for concurrent use.
type Service struct {
	files        FileSource
	paths        PathResolver
	keyframes    KeyframeLocator
	discs        DiscPorts
	subtitles    SubtitleTracks
	hardware     HardwareStatus
	producer     StreamProducer
	workspace    Workspace
	urls         StreamURLBuilder
	ids          IDGenerator
	clock        func() time.Time
	after        func(wait time.Duration) <-chan time.Time
	logger       *slog.Logger
	capabilities playback.ServerCapabilities
	policy       playback.Policy
	sessions     *registry
	// throttleAhead is the allowed lead of a production over the viewer; 0 disables.
	throttleAhead time.Duration

	// processContext outlives every request: FFmpeg, the reaper and the production watchers
	// run under it, never under a request context (ADR 0024). Close cancels it.
	processContext  context.Context
	cancelProcesses context.CancelFunc
}

// NewService validates the dependencies and options and returns a Service. A missing
// dependency or an invalid option is a wiring or configuration bug and wraps
// domain.ErrInvalid, so a bad transcode height fails at startup instead of on the first
// request. The service starts its idle reaper at once; Close stops it.
func NewService(dependencies Dependencies, options Options) (*Service, error) {
	if err := errors.Join(dependencies.validate(), options.validate()); err != nil {
		return nil, fmt.Errorf("new playback service: %w", err)
	}

	processContext, cancelProcesses := context.WithCancel(context.Background())

	service := &Service{
		files:         dependencies.Files,
		paths:         dependencies.Paths,
		keyframes:     dependencies.Keyframes,
		discs:         dependencies.Discs,
		subtitles:     dependencies.Subtitles,
		hardware:      dependencies.Hardware,
		producer:      dependencies.Producer,
		workspace:     dependencies.Workspace,
		urls:          dependencies.URLs,
		ids:           dependencies.IDs,
		clock:         dependencies.Clock,
		after:         dependencies.After,
		logger:        dependencies.Logger,
		capabilities:  options.Capabilities,
		policy:        options.Policy,
		throttleAhead: options.ThrottleAhead,
		sessions: newRegistry(sessionLimits{
			maxReencoding: options.MaxTranscodeSessions,
			maxFFmpeg:     MaxFFmpegSessions,
			maxSessions:   MaxSessions,
		}),
		processContext:  processContext,
		cancelProcesses: cancelProcesses,
	}

	service.startReaper()
	service.startThrottle()

	return service, nil
}

func (dependencies Dependencies) validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{name: "file source", present: dependencies.Files != nil},
		{name: "path resolver", present: dependencies.Paths != nil},
		{name: "keyframe locator", present: dependencies.Keyframes != nil},
		{name: "stream producer", present: dependencies.Producer != nil},
		{name: "workspace", present: dependencies.Workspace != nil},
		{name: "stream url builder", present: dependencies.URLs != nil},
		{name: "id generator", present: dependencies.IDs != nil},
		{name: "clock", present: dependencies.Clock != nil},
		{name: "timer", present: dependencies.After != nil},
		{name: "logger", present: dependencies.Logger != nil},
	}

	var missing []error

	for _, dependency := range required {
		if !dependency.present {
			missing = append(missing, fmt.Errorf("missing dependency %s: %w", dependency.name, domain.ErrInvalid))
		}
	}

	return errors.Join(missing...)
}

func (options Options) validate() error {
	if options.ThrottleAhead < 0 {
		return fmt.Errorf("throttle lead %s is negative: %w", options.ThrottleAhead, domain.ErrInvalid)
	}

	if options.MaxTranscodeSessions < minReencodingSessions || options.MaxTranscodeSessions > MaxFFmpegSessions {
		return fmt.Errorf("re-encoding session limit %d outside %d..%d: %w",
			options.MaxTranscodeSessions, minReencodingSessions, MaxFFmpegSessions, domain.ErrInvalid)
	}

	return options.Policy.Validate()
}

// Get returns the session with the given ID or an error wrapping domain.ErrNotFound. A
// session of another sign-in session (non-admin) counts as unknown; without a viewer the error
// wraps domain.ErrUnauthenticated. It does
// not count as an access: only stream requests keep a session alive.
func (s *Service) Get(ctx context.Context, id string) (Session, error) {
	if err := s.authorize(ctx, id); err != nil {
		return Session{}, err
	}

	session, found := s.sessions.get(id)
	if !found {
		return Session{}, fmt.Errorf("playback session %s: %w", id, domain.ErrNotFound)
	}

	return session, nil
}

// Access records the start of a stream request for the session and returns what the stream
// handler needs. The session counts as in use until the returned access is released, so the
// caller must call StreamAccess.Release (deferred) when the request ends; Stop, Close and a
// replacement still end the session while it is in use. An unknown or stopped session wraps
// domain.ErrNotFound.
func (s *Service) Access(ctx context.Context, id string) (StreamAccess, error) {
	if err := s.authorize(ctx, id); err != nil {
		return StreamAccess{}, err
	}

	access, entry, found := s.sessions.touch(id, s.clock())
	if !found {
		return StreamAccess{}, fmt.Errorf("playback session %s: %w", id, domain.ErrNotFound)
	}

	access.lease = &accessLease{end: func() { s.sessions.endAccess(entry, s.clock()) }}
	access.noteSegment = func(index int) { s.sessions.noteSegment(entry, index) }

	return access, nil
}

// Stop ends a session: it kills and waits for its FFmpeg and removes its workspace. Stopping
// an unknown or already stopped session is not an error, and neither is a workspace that
// cannot be removed: the session is gone, the leftover directory is logged and purged at the
// next start. A session of another sign-in session (non-admin) answers domain.ErrNotFound and is left
// running (ADR 0024, Notes M7-14); without a viewer the error wraps domain.ErrUnauthenticated.
func (s *Service) Stop(ctx context.Context, id string) error {
	if err := s.authorize(ctx, id); err != nil {
		return err
	}

	if entry, found := s.sessions.remove(id); found {
		defer s.sessions.untrack()

		s.teardown(ctx, entry)
	}

	return nil
}

// teardownAll stops several sessions in parallel: each stop waits for its FFmpeg to exit.
func (s *Service) teardownAll(ctx context.Context, entries []*sessionEntry) {
	var group sync.WaitGroup

	for _, entry := range entries {
		group.Go(func() { s.teardown(ctx, entry) })
	}

	group.Wait()
}

// teardown stops a session that is no longer in the registry. Only the caller that took the
// record out of the registry calls it, so every production is stopped once.
func (s *Service) teardown(ctx context.Context, entry *sessionEntry) {
	// The delivery adapter's deferred Release is the only guarantee that a request ends
	// (ADR 0024, Notes M2-38); requests still open here make a leaked grant visible.
	if open := s.sessions.openRequests(entry); open > 0 {
		s.logger.DebugContext(ctx, "playback session torn down with stream requests open",
			"playback_id", entry.session.ID, "open_requests", open)
	}

	if entry.production != nil {
		resumeBeforeStop(entry, s.sessions.wasPaused(entry))
		entry.production.Stop()
	}

	if entry.directory == "" {
		return
	}

	if err := s.removeWorkspace(ctx, entry.session.ID); err != nil {
		s.logger.WarnContext(ctx, "playback session stopped, but its workspace is left behind",
			"playback_id", entry.session.ID, "error", err)
	}
}

// removeWorkspace deletes a session's directory detached from ctx's cancellation, bounded by
// cleanupTimeout.
func (s *Service) removeWorkspace(ctx context.Context, id string) error {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
	defer cancel()

	if err := s.workspace.Remove(cleanupContext, id); err != nil {
		return fmt.Errorf("remove workspace: %w", err)
	}

	return nil
}

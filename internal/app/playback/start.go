package playback

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// fallbackContentType is served for a direct-play container without a MIME type. Decide
// only plans direct play for containers that have one, so it is a guard, not a path.
const fallbackContentType = "application/octet-stream"

// Start decides how the client plays the file and starts the session (ADR 0023, ADR 0024).
// Direct play needs no FFmpeg; remux and transcode create the session's workspace and start
// the stream producer under the service's own context, so the production outlives the
// request.
//
// A session named in ReplacesSessionID hands its slot to the new session atomically when
// the new one is reserved, so a seek never trips a limit, and keeps running until the new
// production has started; a failed start leaves it untouched.
//
// Errors wrap domain.ErrInvalid (profile or request), domain.ErrNotFound (unknown file or a
// file missing on disk), domain.ErrForbidden (a file outside its library),
// domain.ErrUnsupported (file without a probe result, or a playback.Decide failure such as
// playback.ErrNoCompatibleFormat), ErrSessionLimitReached, domain.ErrUnavailable (the service
// is closed, ErrServiceClosed, or the startup purge runs), or the error of a port.
func (s *Service) Start(ctx context.Context, request StartRequest) (Session, error) {
	session, err := s.start(ctx, request)
	if err != nil {
		return Session{}, fmt.Errorf("start playback of file %s: %w", request.FileID, err)
	}

	return session, nil
}

func (s *Service) start(ctx context.Context, request StartRequest) (Session, error) {
	prepared, err := s.prepare(ctx, request)
	if err != nil {
		return Session{}, err
	}

	entry, err := s.newEntry(prepared, request.ReplacesSessionID)
	if err != nil {
		return Session{}, err
	}

	id := entry.session.ID

	if err := s.sessions.reserve(entry); err != nil {
		return Session{}, err
	}

	// Close waits until this start has activated or torn down what it started.
	defer s.sessions.untrack()

	var started activation

	if prepared.plan.Delivery == playback.DeliveryHLS {
		started, err = s.produce(ctx, productionRequest{sessionID: id, prepared: prepared, requested: request.StartPosition})
		if err != nil {
			s.sessions.release(id)

			return Session{}, err
		}
	}

	started.activatedAt = s.clock()

	session, err := s.activate(ctx, id, started)
	if err != nil {
		return Session{}, err
	}

	if started.production != nil {
		s.follow(id, started.production)
	}

	return session, nil
}

// preparedStart is a decided plan for a file whose path was resolved.
type preparedStart struct {
	playable  PlayableFile
	plan      playback.Plan
	inputPath string
}

// prepare decides the plan and resolves the file's path. Every mode resolves it, so a
// deleted file or one that escapes its library fails here for direct play too, before a
// slot is taken.
func (s *Service) prepare(ctx context.Context, request StartRequest) (preparedStart, error) {
	playable, plan, err := s.decide(ctx, request)
	if err != nil {
		return preparedStart{}, err
	}

	inputPath, err := s.paths.Resolve(playable.LibraryRoot, playable.File.RelativePath)
	if err != nil {
		return preparedStart{}, fmt.Errorf("resolve %s: %w", playable.File.RelativePath, err)
	}

	return preparedStart{playable: playable, plan: plan, inputPath: inputPath}, nil
}

// decide validates the profile, loads the file and runs the decision engine.
func (s *Service) decide(ctx context.Context, request StartRequest) (PlayableFile, playback.Plan, error) {
	if err := request.Profile.Validate(); err != nil {
		return PlayableFile{}, playback.Plan{}, err
	}

	playable, err := s.files.PlayableFile(ctx, request.FileID)
	if err != nil {
		return PlayableFile{}, playback.Plan{}, fmt.Errorf("load file: %w", err)
	}

	if !playable.File.Probed() {
		return PlayableFile{}, playback.Plan{}, fmt.Errorf("file has no probe result (%s): %w",
			playable.File.ProbeError, domain.ErrUnsupported)
	}

	plan, err := playback.Decide(playback.Input{
		Description:  playable.File.Description,
		Profile:      request.Profile,
		Capabilities: s.capabilities,
		Request: playback.Request{
			AudioStreamIndex: request.AudioStreamIndex,
			StartPosition:    request.StartPosition,
		},
		Policy: s.policy,
	})
	if err != nil {
		return PlayableFile{}, playback.Plan{}, err
	}

	return playable, plan, nil
}

// newEntry builds the registry record of a new session.
func (s *Service) newEntry(prepared preparedStart, replaces string) (*sessionEntry, error) {
	id, err := s.ids.NewID()
	if err != nil {
		return nil, fmt.Errorf("generate session id: %w", err)
	}

	now := s.clock()
	plan, file := prepared.plan, prepared.playable.File
	hls := plan.Delivery == playback.DeliveryHLS

	entry := &sessionEntry{
		session: Session{
			ID: id, FileID: file.ID, State: StateReady, Plan: plan,
			StreamURL: s.urls.StreamURL(id, plan.Delivery),
			Duration:  file.Description.Duration,
			CreatedAt: now, LastAccessedAt: now,
		},
		replaces:     replaces,
		reencodes:    plan.Mode == playback.ModeTranscode,
		runsFFmpeg:   hls,
		libraryRoot:  prepared.playable.LibraryRoot,
		relativePath: file.RelativePath,
	}

	if hls {
		entry.session.State = StateStarting
	} else {
		entry.contentType = contentType(plan.SourceContainer)
	}

	return entry, nil
}

// activate makes the started session visible and tears down the session it replaced. A
// session that vanished while it started (Close) must not leave its FFmpeg behind.
func (s *Service) activate(ctx context.Context, id string, started activation) (Session, error) {
	session, replaced, found := s.sessions.activate(id, started)
	if !found {
		s.teardown(ctx, &sessionEntry{
			session: Session{ID: id}, production: started.production, directory: started.directory,
		})

		// Only Close removes a pending session that its start did not release.
		return Session{}, fmt.Errorf("session %s ended while it started: %w", id, ErrServiceClosed)
	}

	if replaced != nil {
		s.teardown(ctx, replaced)
	}

	return session, nil
}

// productionRequest is what produce needs to start one session's FFmpeg.
type productionRequest struct {
	sessionID string
	prepared  preparedStart
	// requested is the start position the client asked for.
	requested time.Duration
}

// produce finds the actual start, creates the workspace and starts FFmpeg. A producer
// failure removes the workspace again.
func (s *Service) produce(ctx context.Context, request productionRequest) (activation, error) {
	id, prepared := request.sessionID, request.prepared

	start, err := s.actualStart(ctx, request)
	if err != nil {
		return activation{}, err
	}

	directory, err := s.workspace.Create(ctx, id)
	if err != nil {
		return activation{}, fmt.Errorf("create workspace: %w", err)
	}

	job := Job{
		SessionID: id, Plan: prepared.plan,
		LibraryRoot: prepared.playable.LibraryRoot, RelativePath: prepared.playable.File.RelativePath,
		InputPath: prepared.inputPath, StartPosition: start, OutputDir: directory,
	}

	production, err := s.producer.Start(s.processContext, job) //nolint:contextcheck // FFmpeg must outlive the request (ADR 0024)
	if err != nil {
		if cleanupErr := s.removeWorkspace(ctx, id); cleanupErr != nil {
			s.logger.WarnContext(ctx, "clean up after a failed stream start", "session", id, "error", cleanupErr)
		}

		// A start racing Close sees the cancelled process context; that is the shutdown
		// (503), not a server error, so the cancellation is not wrapped.
		if s.processContext.Err() != nil {
			return activation{}, fmt.Errorf("start stream producer: %w", ErrServiceClosed)
		}

		return activation{}, fmt.Errorf("start stream producer: %w", err)
	}

	return activation{production: production, directory: directory, startPosition: start}, nil
}

// actualStart is where the stream really starts. Copied video can only start at a keyframe,
// so the locator finds the last one at or before the requested position; re-encoded video
// and audio-only streams start exactly. When the lookup fails (for example a file without a
// format start time) the stream starts at the beginning instead of failing the session.
func (s *Service) actualStart(ctx context.Context, request productionRequest) (time.Duration, error) {
	requested := request.requested

	video, hasVideo := request.prepared.plan.Video()
	if requested == 0 || !hasVideo || video.Action != playback.ActionCopy {
		return requested, nil
	}

	keyframe, err := s.keyframes.KeyframeAtOrBefore(ctx, request.prepared.inputPath, requested)
	if err == nil {
		return keyframe, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return 0, fmt.Errorf("find keyframe before %s: %w", requested, ctxErr)
	}

	s.logger.WarnContext(ctx, "keyframe lookup failed, starting at the beginning of the file",
		"session", request.sessionID, "position", requested, "error", err)

	return 0, nil
}

// contentType is the Content-Type of a directly played file (ADR 0023 table).
func contentType(container media.Container) string {
	key, _ := media.DirectPlayKey(container)

	if mimeType, found := media.MIMEType(key); found {
		return mimeType
	}

	return fallbackContentType
}

package playback

import (
	"context"
	"errors"
	"fmt"
	"time"

	appaccess "github.com/magpeek/mivo/internal/app/access"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// fallbackContentType is served for a direct-play container without a MIME type. Decide
// only plans direct play for containers that have one, so it is a guard, not a path.
const fallbackContentType = "application/octet-stream"

// ErrFileOutsideLibrary reports a catalogued file whose path now leaves its library folder,
// for example through a link swapped in after the scan. It wraps domain.ErrUnsupported, not
// domain.ErrForbidden: the client can do nothing about this file, while a forbidden answer
// (HTTP 403) means the caller lacks a permission, such as a client outside the LAN.
var ErrFileOutsideLibrary = fmt.Errorf("media file is outside its library folder: %w", domain.ErrUnsupported)

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
// file missing on disk), ErrFileOutsideLibrary (a file outside its library),
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
	// The viewer comes first: without one nothing is read, and a file the viewer may not see
	// is refused as not found while the plan is made, before a slot is taken.
	viewer, ok := appaccess.ViewerFrom(ctx)
	if !ok {
		return Session{}, fmt.Errorf("viewer: %w", domain.ErrUnauthenticated)
	}

	prepared, err := s.prepare(ctx, request, viewer.Restriction())
	if err != nil {
		return Session{}, err
	}

	prepared.owner = viewer.AuthSessionID
	prepared.clientName = request.Profile.Name
	request.ReplacesSessionID = s.replaceableBy(viewer, request.ReplacesSessionID)

	prepared, entry, err := s.reserveSlot(prepared, request)
	if err != nil {
		return Session{}, err
	}

	id := entry.session.ID

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
		s.follow(id, started.production, s.encoderFor(prepared.plan))
	}

	return session, nil
}

// reserveSlot creates the session record and takes its slot. A burn-in that the limit
// refuses falls back to no subtitle when the request allows it; the replaced session, whose
// slot counts as freed by the reservation, keeps running whenever this fails.
func (s *Service) reserveSlot(prepared preparedStart, request StartRequest) (preparedStart, *sessionEntry, error) {
	entry, err := s.newEntry(prepared, request.ReplacesSessionID)
	if err != nil {
		return preparedStart{}, nil, err
	}

	reserveErr := s.sessions.reserve(entry)
	if reserveErr == nil {
		return prepared, entry, nil
	}

	if !errors.Is(reserveErr, ErrSessionLimitReached) || !request.AllowDegradeSubtitles ||
		prepared.plan.Subtitle.Mode != playback.SubtitleBurnIn {
		return preparedStart{}, nil, reserveErr
	}

	plan, err := withoutSubtitle(prepared.input)
	if err != nil {
		return preparedStart{}, nil, reserveErr
	}

	prepared.plan = plan

	if entry, err = s.newEntry(prepared, request.ReplacesSessionID); err != nil {
		return preparedStart{}, nil, err
	}

	if err := s.sessions.reserve(entry); err != nil {
		return preparedStart{}, nil, err
	}

	return prepared, entry, nil
}

// preparedStart is a decided plan for a file whose path was resolved.
type preparedStart struct {
	playable PlayableFile
	plan     playback.Plan
	// input is what the plan was decided from, kept to decide again without a subtitle.
	input playback.Input
	// owner is the auth session that starts the session (Viewer.AuthSessionID).
	owner      string
	clientName string
	inputPath  string
	// disc is the title of a disc source; nil for a single file. inputPath is then the
	// absolute disc root.
	disc *media.DiscSource
}

// prepare decides the plan and resolves the file's path. Every mode resolves it, so a
// deleted file or one that escapes its library fails here for direct play too, before a
// slot is taken. A disc also resolves its main title.
func (s *Service) prepare(ctx context.Context, request StartRequest, restriction access.Restriction) (preparedStart, error) {
	prepared, err := s.decide(ctx, request, restriction)
	if err != nil {
		return preparedStart{}, err
	}

	playable := prepared.playable

	if playable.File.Layout.IsDisc() {
		prepared.inputPath, prepared.disc, err = s.prepareDisc(ctx, playable)
		if err != nil {
			return preparedStart{}, err
		}

		return prepared, nil
	}

	prepared.inputPath, err = s.paths.Resolve(playable.LibraryRoot, playable.File.RelativePath)
	if err != nil {
		return preparedStart{}, s.resolveFailure(ctx, playable.File, err)
	}

	return prepared, nil
}

// encoderFor is the video encoder of the plan's transcode; the zero value when no video is
// re-encoded.
func (s *Service) encoderFor(plan playback.Plan) playback.Encoder {
	if video, found := plan.Video(); !found || video.Action != playback.ActionTranscode {
		return playback.Encoder{}
	}

	return playback.ChooseVideoEncoder(s.capabilities, s.policy.HardwareAcceleration)
}

// resolveFailure reports a file that cannot be resolved. A file that leaves its library
// folder becomes ErrFileOutsideLibrary, keeping only the resolver's message: domain.ErrForbidden
// would tell the client it lacks a permission it can never get. The escape is logged at warn
// here because a client error never reaches the info log of the HTTP adapter, and a catalogued
// file that suddenly points outside its library (a link swapped in after the scan) is worth
// an operator's attention.
func (s *Service) resolveFailure(ctx context.Context, file media.File, err error) error {
	if !errors.Is(err, domain.ErrForbidden) {
		return fmt.Errorf("resolve %s: %w", file.RelativePath, err)
	}

	s.logger.WarnContext(ctx, "refused to play a file that leaves its library folder",
		"file", file.ID, "path", file.RelativePath, "error", err)

	return fmt.Errorf("resolve %s: %w (%s)", file.RelativePath, ErrFileOutsideLibrary, err.Error())
}

// decide validates the profile, loads the file and runs the decision engine.
func (s *Service) decide(ctx context.Context, request StartRequest, restriction access.Restriction) (preparedStart, error) {
	if err := request.Profile.Validate(); err != nil {
		return preparedStart{}, err
	}

	playable, err := s.files.PlayableFile(ctx, request.FileID, restriction)
	if err != nil {
		return preparedStart{}, fmt.Errorf("load file: %w", err)
	}

	if !playable.File.Probed() {
		return preparedStart{}, fmt.Errorf("file has no probe result (%s): %w",
			playable.File.ProbeError, domain.ErrUnsupported)
	}

	builder := inputBuilder{capabilities: s.capabilities, policy: s.policy, subtitles: s.subtitles}

	input, err := builder.build(ctx, playable, request)
	if err != nil {
		return preparedStart{}, err
	}

	plan, err := playback.Decide(input)
	if err != nil {
		return preparedStart{}, err
	}

	return preparedStart{playable: playable, plan: plan, input: input}, nil
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
			SubtitleMode: plan.Subtitle.Mode, SubtitleTrackID: plan.Subtitle.TrackID,
			SourceBitRate: file.Description.BitRate,
			OutputBitRate: outputBitRate(plan, file.Description.BitRate),
		},
		replaces:     replaces,
		owner:        prepared.owner,
		clientName:   prepared.clientName,
		encoder:      s.encoderFor(plan),
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
		BurnInSubtitle: burnInTrack(prepared.input, prepared.plan),
		Encoder:        s.encoderFor(prepared.plan),
	}

	if video, found := prepared.plan.Video(); found {
		job.MaxBitrate = video.MaxBitrate
	}

	if disc := prepared.disc; disc != nil {
		job.RelativePath, job.Layout, job.DiscTitle = disc.RelativePath, disc.Layout, disc.Title
	}

	production, err := s.producer.Start(s.processContext, job) //nolint:contextcheck // FFmpeg must outlive the request (ADR 0024)
	if err != nil {
		if cleanupErr := s.removeWorkspace(ctx, id); cleanupErr != nil {
			s.logger.WarnContext(ctx, "clean up after a failed stream start", "playback_id", id, "error", cleanupErr)
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

	keyframe, err := s.keyframeAtOrBefore(ctx, request.prepared, requested)
	if err == nil {
		return keyframe, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return 0, fmt.Errorf("find keyframe before %s: %w", requested, ctxErr)
	}

	s.logger.WarnContext(ctx, "keyframe lookup failed, starting at the beginning of the file",
		"playback_id", request.sessionID, "position", requested, "error", err)

	return 0, nil
}

// keyframeAtOrBefore searches the file, or the joined clips of a disc title.
func (s *Service) keyframeAtOrBefore(ctx context.Context, prepared preparedStart, position time.Duration) (time.Duration, error) {
	if prepared.disc != nil {
		return s.discs.Keyframes.DiscKeyframeAtOrBefore(ctx, prepared.playable.LibraryRoot, *prepared.disc, position)
	}

	return s.keyframes.KeyframeAtOrBefore(ctx, prepared.inputPath, position)
}

// contentType is the Content-Type of a directly played file (ADR 0023 table).
func contentType(container media.Container) string {
	key, _ := media.DirectPlayKey(container)

	if mimeType, found := media.MIMEType(key); found {
		return mimeType
	}

	return fallbackContentType
}

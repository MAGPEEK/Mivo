package playback

import (
	"context"
	"errors"
	"fmt"
)

// FailureTranscodeFailed is the failure code of a session whose FFmpeg exited with an error
// or without producing a segment (API failureCode).
const FailureTranscodeFailed = "transcode_failed"

// follow starts a watcher that moves an HLS session's state with its production. It runs
// under the process context and ends with the production or on Close.
func (s *Service) follow(id string, production Production) {
	if !s.sessions.track() {
		return
	}

	go func() {
		defer s.sessions.untrack()

		s.followProduction(id, production)
	}()
}

// followProduction waits for the first segment (ready) and then for the exit (ended or
// failed). Production.Ready returns the exit result at once after Done (port contract).
func (s *Service) followProduction(id string, production Production) {
	if err := production.Ready(s.processContext); err != nil {
		s.finish(id, err)

		return
	}

	s.sessions.transition(id, StateReady, "")

	select {
	case <-production.Done():
		s.finish(id, production.Ready(s.processContext))
	case <-s.processContext.Done():
	}
}

// finish records how a production ended. A stopped production and a shutdown are no
// outcome: the session is gone already and its kill is no failure.
func (s *Service) finish(id string, outcome error) {
	if s.processContext.Err() != nil || errors.Is(outcome, ErrProductionStopped) {
		return
	}

	if outcome == nil {
		if _, changed := s.sessions.transition(id, StateEnded, ""); changed {
			s.logger.DebugContext(s.processContext, "playback session production finished", "session", id)
		}

		return
	}

	// Info, not warn: the producer already logs the failed process once at warn with its exit
	// code and stderr tail; this line only records the session's state change.
	if session, changed := s.sessions.transition(id, StateFailed, FailureTranscodeFailed); changed {
		s.logger.InfoContext(s.processContext, "playback session failed",
			"session", id, "file", session.FileID, "failure_code", session.FailureCode, "error", outcome)
	}
}

// Close shuts the service down before the server exits (ADR 0024): it refuses new sessions
// with ErrServiceClosed, stops every session in parallel (FFmpeg killed and waited for,
// directories removed), waits for starts still in flight to tear down what they started, and
// stops the idle reaper and waits for a running workspace purge. It returns ctx.Err() when
// ctx ends first; the processes are then killed through their cancelled context. A second
// Close is a no-op: it returns nil at once, neither waiting for the first nor returning its
// result.
func (s *Service) Close(ctx context.Context) error {
	sessions, first := s.sessions.close()
	if !first {
		return nil
	}

	finished := make(chan struct{})

	go func() {
		defer close(finished)

		// Stop first: a kill through the context would look like a crash to the producer.
		s.teardownAll(ctx, sessions)
		s.cancelProcesses()
		s.sessions.wait()
	}()

	select {
	case <-finished:
		s.logger.InfoContext(ctx, "playback sessions stopped", "sessions", len(sessions))

		return nil
	case <-ctx.Done():
		s.cancelProcesses()

		return fmt.Errorf("close playback service: %w", ctx.Err())
	}
}

// PurgeWorkspace deletes the session directories a previous run left behind (ADR 0024). It
// must run once at startup, before the first session: afterwards it would delete live
// directories and is refused with domain.ErrConflict. New sessions are refused while it
// runs. The number of removed directories is logged; a failed purge is returned.
func (s *Service) PurgeWorkspace(ctx context.Context) error {
	if err := s.sessions.beginPurge(); err != nil {
		return fmt.Errorf("purge playback workspace: %w", err)
	}

	defer s.sessions.endPurge()

	removed, err := s.workspace.Purge(ctx)

	s.logger.InfoContext(ctx, "purged leftover playback session directories", "removed", removed)

	if err != nil {
		return fmt.Errorf("purge playback workspace (%d removed): %w", removed, err)
	}

	return nil
}

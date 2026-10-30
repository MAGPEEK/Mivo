package playback

import (
	"context"
	"time"
)

// Idle reaping (ADR 0024).
const (
	// IdleTimeout stops a session whose files were not requested for this long. hls.js
	// reloads an unfinished playlist every target duration, which keeps a playing or paused
	// HLS session alive.
	IdleTimeout = 2 * time.Minute
	// IdleCheckInterval is how often the reaper looks for idle sessions, so a session is
	// stopped at most this long after its idle timeout.
	IdleCheckInterval = 15 * time.Second
)

// startReaper starts the goroutine that stops idle sessions until Close.
func (s *Service) startReaper() {
	if !s.sessions.track() {
		return
	}

	go func() {
		defer s.sessions.untrack()

		s.reap(s.processContext)
	}()
}

// reap checks for idle sessions every IdleCheckInterval until ctx ends.
func (s *Service) reap(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.after(IdleCheckInterval):
			s.stopIdle(ctx)
		}
	}
}

// stopIdle stops every session idle for IdleTimeout or longer. The sessions leave the
// registry first, so the lock is not held while their FFmpeg is waited for.
func (s *Service) stopIdle(ctx context.Context) {
	now := s.clock()
	idle := s.sessions.removeIdle(now.Add(-IdleTimeout))

	for _, entry := range idle {
		s.logger.InfoContext(ctx, "stopping idle playback session",
			"session", entry.session.ID, "idle", now.Sub(entry.session.LastAccessedAt))
	}

	s.teardownAll(ctx, idle)
}

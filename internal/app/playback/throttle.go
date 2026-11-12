package playback

import (
	"errors"
	"time"
)

// Throttling (ADR 0030 section 2).
const (
	// ThrottleInterval is how often the controller compares production and viewer.
	ThrottleInterval = 2 * time.Second
	// SegmentDuration is the HLS segment length of ADR 0024; the producer cuts segments of
	// this length, so produced media time maps to segment indexes.
	SegmentDuration = 6 * time.Second
	// startupSegments is the burst every session produces unthrottled, so playback starts
	// without waiting for the viewer's first requests.
	startupSegments = 3
)

// throttleTarget is what one controller tick needs of a session.
type throttleTarget struct {
	entry      *sessionEntry
	production Production
	highest    int
	paused     bool
}

// startThrottle starts the controller goroutine; ahead 0 disables throttling.
func (s *Service) startThrottle() {
	if s.throttleAhead <= 0 || !s.sessions.track() {
		return
	}

	go func() {
		defer s.sessions.untrack()

		for {
			select {
			case <-s.processContext.Done():
				return
			case <-s.after(ThrottleInterval):
				s.throttleTick()
			}
		}
	}()
}

// throttleTick pauses or resumes every production that supports it.
func (s *Service) throttleTick() {
	for _, target := range s.sessions.throttleTargets() {
		pausable, ok := target.production.(Pausable)
		if !ok || !pausable.Pausable() {
			continue
		}

		reporter, ok := target.production.(ProgressReporter)
		if !ok {
			continue
		}

		produced := int(reporter.Progress().OutputTime / SegmentDuration)

		want := throttleDecision(target.paused, produced, target.highest, s.aheadSegments())
		if want == target.paused {
			continue
		}

		s.applyThrottle(pausable, target.entry, want)
	}
}

// aheadSegments is the allowed lead in whole segments, rounded up.
func (s *Service) aheadSegments() int {
	return int((s.throttleAhead + SegmentDuration - 1) / SegmentDuration)
}

// throttleDecision tells whether the production should be paused. It pauses when the lead
// over the highest requested segment exceeds ahead and resumes only below half of it, so the
// production does not flap around the limit. The startup burst is never throttled.
func throttleDecision(paused bool, produced, highest, ahead int) bool {
	lead := produced - highest

	if produced <= startupSegments {
		return false
	}

	if paused {
		return lead*2 >= ahead
	}

	return lead > ahead
}

// applyThrottle pauses or resumes and records the flag, unless the session left the
// registry meanwhile, in which case a pause is undone.
func (s *Service) applyThrottle(pausable Pausable, entry *sessionEntry, pause bool) {
	var err error
	if pause {
		err = pausable.Pause()
	} else {
		err = pausable.Resume()
	}

	if err != nil {
		if !errors.Is(err, ErrProductionStopped) {
			s.logger.WarnContext(s.processContext, "playback throttle failed",
				"playback_id", entry.session.ID, "pause", pause, "error", err)
		}

		return
	}

	if !s.sessions.setPaused(entry, pause) && pause {
		_ = pausable.Resume() // the session is gone; do not leave a frozen process behind
	}
}

// resumeBeforeStop wakes a paused production so the kill takes effect at once.
func resumeBeforeStop(entry *sessionEntry, paused bool) {
	if !paused {
		return
	}

	if pausable, ok := entry.production.(Pausable); ok {
		_ = pausable.Resume() // best effort; Stop resumes a paused process itself
	}
}

// RequestSegment records that the client asked for HLS segment index of the session, the
// viewer position the throttle controller measures the lead against. It never lowers the
// recorded maximum.
func (access StreamAccess) RequestSegment(index int) {
	if access.noteSegment != nil {
		access.noteSegment(index)
	}
}

// noteSegment raises the highest requested segment of the entry.
func (sessions *registry) noteSegment(entry *sessionEntry, index int) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry.highestSegment = max(entry.highestSegment, index)
}

// throttleTargets lists the visible, running HLS sessions.
func (sessions *registry) throttleTargets() []throttleTarget {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	var targets []throttleTarget

	for _, entry := range sessions.entries {
		if entry.pending || entry.production == nil || entry.session.State != StateReady {
			continue
		}

		targets = append(targets, throttleTarget{
			entry: entry, production: entry.production, highest: entry.highestSegment, paused: entry.paused,
		})
	}

	return targets
}

// setPaused records the paused flag; false when the session is no longer registered.
func (sessions *registry) setPaused(entry *sessionEntry, paused bool) bool {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if sessions.entries[entry.session.ID] != entry {
		return false
	}

	entry.paused = paused

	return true
}

// wasPaused reads the flag of a record that left the registry.
func (sessions *registry) wasPaused(entry *sessionEntry) bool {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	return entry.paused
}

package scantrigger

import (
	"context"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
)

// periodicCandidate is a library the periodic check looks at.
type periodicCandidate struct {
	libraryID string
	since     time.Time
}

// startDebouncedScans starts the scans whose debounce has elapsed. A library whose scan
// runs keeps its debounce due and gets exactly one more scan once that scan has finished,
// however many changes arrived meanwhile.
func (trigger *Trigger) startDebouncedScans(ctx context.Context, now time.Time) {
	for _, libraryID := range trigger.collectDue(now) {
		if trigger.scanner.Status(libraryID).State == scan.StateRunning {
			continue
		}

		trigger.clearDue(libraryID)
		trigger.start(ctx, libraryID, reasonWatch)
	}
}

// collectDue turns elapsed debounces into due scans and returns every library with a due
// scan.
func (trigger *Trigger) collectDue(now time.Time) []string {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	var due []string

	for libraryID, state := range trigger.states {
		quiet := now.Sub(state.lastChange) >= QuietPeriod
		waitedLongEnough := now.Sub(state.firstChange) >= MaxDelay

		if state.pending && (quiet || waitedLongEnough) {
			state.pending = false
			state.due = true
		}

		if state.due {
			due = append(due, libraryID)
		}
	}

	return due
}

func (trigger *Trigger) clearDue(libraryID string) {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	if state := trigger.states[libraryID]; state != nil {
		state.due = false
	}
}

// startPeriodicScans starts a scan of every library whose last scan finished at least the
// interval ago (ADR 0027 §9: measured from the end). A library without a finished scan
// counts from when the trigger first saw it, so the startup scan is not doubled.
func (trigger *Trigger) startPeriodicScans(ctx context.Context, now time.Time) {
	for _, candidate := range trigger.periodicCandidates() {
		status := trigger.scanner.Status(candidate.libraryID)
		if status.State == scan.StateRunning {
			continue
		}

		reference := status.FinishedAt
		if reference.IsZero() {
			reference = candidate.since
		}

		if now.Sub(reference) >= trigger.settings.Interval {
			trigger.start(ctx, candidate.libraryID, reasonPeriodic)
		}
	}
}

func (trigger *Trigger) periodicCandidates() []periodicCandidate {
	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	candidates := make([]periodicCandidate, 0, len(trigger.states))
	for libraryID, state := range trigger.states {
		candidates = append(candidates, periodicCandidate{libraryID: libraryID, since: state.since})
	}

	return candidates
}

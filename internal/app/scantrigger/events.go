package scantrigger

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// HoldTimeout bounds Suspend: when the library change that should follow never comes (the
// update was rejected), the library is scanned again after this long.
const HoldTimeout = 2 * time.Minute

type eventKind int

const (
	eventAdded eventKind = iota
	eventChanged
	eventRemoved
)

type libraryEvent struct {
	kind      eventKind
	libraryID string
}

// eventQueue hands library events from any goroutine (HTTP handlers) to the loop of Run. A
// push never blocks and never drops an event.
type eventQueue struct {
	mutex  sync.Mutex
	events []libraryEvent
	// wake holds one token while events wait.
	wake chan struct{}
	// closed is set while no Run consumes the queue, so it does not grow unbounded.
	closed bool
}

func newEventQueue() *eventQueue {
	return &eventQueue{wake: make(chan struct{}, 1), closed: true}
}

// open starts accepting events; Run calls it before its first check.
func (queue *eventQueue) open() {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	queue.closed = false
}

// close drops the queued events and rejects new ones until the next open.
func (queue *eventQueue) close() {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	queue.closed = true
	queue.events = nil
}

func (queue *eventQueue) push(event libraryEvent) {
	queue.mutex.Lock()

	if queue.closed {
		queue.mutex.Unlock()
		return
	}

	queue.events = append(queue.events, event)
	queue.mutex.Unlock()

	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

func (queue *eventQueue) drain() []libraryEvent {
	queue.mutex.Lock()
	defer queue.mutex.Unlock()

	events := queue.events
	queue.events = nil

	return events
}

// Added implements the library manager's event port: the library is watched and joins the
// periodic check. It does not block.
func (trigger *Trigger) Added(libraryID string) { trigger.enqueue(eventAdded, libraryID) }

// Changed adopts the changed folders of the library and keeps its debounce state.
func (trigger *Trigger) Changed(libraryID string) { trigger.enqueue(eventChanged, libraryID) }

// Removed stops the watches of the library and forgets its pending scans.
func (trigger *Trigger) Removed(libraryID string) { trigger.enqueue(eventRemoved, libraryID) }

func (trigger *Trigger) enqueue(kind eventKind, libraryID string) {
	// With both parts off nothing consumes the queue.
	if !trigger.settings.Watch && trigger.settings.Interval == 0 {
		return
	}

	trigger.events.push(libraryEvent{kind: kind, libraryID: libraryID})
}

// Suspend keeps the trigger from starting scans of the library until the next library event
// for it or HoldTimeout. Library management calls it before it cancels a scan, so no trigger
// starts a scan on the old definition between Cancel and the change. It returns once a start
// that was already running has returned, so the Cancel that follows sees that scan.
//
// Limit: a library the trigger does not know yet (its Added event is not handled) cannot be
// held; it has no scan to start either, and the Added event applies the stored definition.
func (trigger *Trigger) Suspend(libraryID string) {
	trigger.startGate.Lock()
	defer trigger.startGate.Unlock()

	now := trigger.clock.Now()

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	if state := trigger.states[libraryID]; state != nil {
		state.heldUntil = now.Add(HoldTimeout)
	}
}

// handleEvents applies the queued library events in order.
func (trigger *Trigger) handleEvents(ctx context.Context) {
	for _, event := range trigger.events.drain() {
		trigger.handleEvent(ctx, event)
	}

	if trigger.afterEvents != nil {
		trigger.afterEvents()
	}
}

func (trigger *Trigger) handleEvent(ctx context.Context, event libraryEvent) {
	if event.kind == eventRemoved {
		trigger.mutex.Lock()
		known := trigger.removeLocked(event.libraryID)
		trigger.mutex.Unlock()

		if !known {
			trigger.logger.Debug(ignoredEventMessage, slog.String("library", event.libraryID))
		}

		return
	}

	// The event carries only the ID: the stored definition is what counts. A library that is
	// gone by now (ErrNotFound, also for Changed) was deleted after the event was queued; its
	// Removed event follows and the next check reconciles anyway.
	stored, err := trigger.libraries.GetLibrary(ctx, event.libraryID)
	if err != nil {
		trigger.logEventLookupFailure(ctx, event.libraryID, err)
		return
	}

	now := trigger.clock.Now()

	trigger.mutex.Lock()
	defer trigger.mutex.Unlock()

	trigger.applyLocked(ctx, stored, now)
	trigger.states[stored.ID].heldUntil = time.Time{}
}

// logEventLookupFailure ignores an event for a library the store does not know; any other
// failure is left to the next check, which reads the whole list.
func (trigger *Trigger) logEventLookupFailure(ctx context.Context, libraryID string, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		trigger.logger.Debug(ignoredEventMessage, slog.String("library", libraryID))
		return
	}

	if ctx.Err() == nil {
		trigger.logger.Warn("automatic scans: cannot read changed library; the next check retries",
			slog.String("library", libraryID), slog.String("error", err.Error()))
	}
}

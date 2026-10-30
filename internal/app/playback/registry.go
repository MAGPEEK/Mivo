package playback

import (
	"fmt"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Session limits (ADR 0024).
const (
	// MaxFFmpegSessions is the fixed safety cap of sessions running FFmpeg at all, remux
	// included (OD-4 A). The configurable re-encoding limit can never exceed it.
	MaxFFmpegSessions = 8
	// MaxSessions caps all sessions, direct play included. Sessions are cheap, but without
	// login any client could otherwise fill the memory until the idle timeout reaps them.
	MaxSessions = 256
)

// LimitReachedCode is the problem code of a request beyond a session limit (ADR 0024).
const LimitReachedCode = "session_limit_reached"

// ErrSessionLimitReached reports that a new session would exceed a session limit. It wraps
// domain.ErrUnavailable: retrying later may succeed.
var ErrSessionLimitReached = fmt.Errorf("%s: %w", LimitReachedCode, domain.ErrUnavailable)

// sessionEntry is the registry's record of one session. The registry guards every field;
// callers only ever receive copies.
type sessionEntry struct {
	session Session
	// pending marks a session that is still being set up. It holds its slot but is
	// invisible to Get, Access and Stop until it is activated.
	pending bool
	// replaces is the session this one takes over when it is activated; replacedBy is the
	// pending session that claimed this one. A claimed session keeps running (the viewer
	// still watches it) but its slot already belongs to the replacement.
	replaces   string
	replacedBy string
	// reencodes counts the session against the re-encoding limit, runsFFmpeg against the
	// FFmpeg cap.
	reencodes  bool
	runsFFmpeg bool

	libraryRoot  string
	relativePath string
	contentType  string
	directory    string
	production   Production
}

// activation is what a started production adds to a pending session.
type activation struct {
	production    Production
	directory     string
	startPosition time.Duration
}

// sessionLimits are the limits of OD-4 A plus the overall cap.
type sessionLimits struct {
	maxReencoding int
	maxFFmpeg     int
	maxSessions   int
}

// sessionCounts are the slots in use.
type sessionCounts struct {
	reencoding, ffmpeg, all int
}

// registry holds the sessions in memory and enforces the limits atomically, so concurrent
// starts never exceed them. Its methods never call ports, so the lock is never held while
// waiting for FFmpeg or the file system.
type registry struct {
	mutex   sync.Mutex
	entries map[string]*sessionEntry
	limits  sessionLimits
}

func newRegistry(limits sessionLimits) *registry {
	return &registry{entries: make(map[string]*sessionEntry), limits: limits}
}

// reserve adds a new pending session when the limits allow it. When the entry replaces a
// visible session that no other replacement claimed yet, it takes over that session's slot
// in the same step, so no other client can take it during a seek (ADR 0024: seeking never
// trips the limit); an unknown or already claimed session is ignored. A reached limit wraps
// ErrSessionLimitReached; a session ID in use is an internal error (a broken generator).
func (sessions *registry) reserve(entry *sessionEntry) error {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if _, exists := sessions.entries[entry.session.ID]; exists {
		return fmt.Errorf("session id %s is already in use", entry.session.ID)
	}

	replaced, found := sessions.visible(entry.replaces)
	if !found || replaced.replacedBy != "" {
		replaced, entry.replaces = nil, ""
	}

	if err := sessions.checkLimits(entry, replaced); err != nil {
		return err
	}

	if replaced != nil {
		replaced.replacedBy = entry.session.ID
	}

	entry.pending = true
	sessions.entries[entry.session.ID] = entry

	return nil
}

// checkLimits counts pending sessions too (their FFmpeg is about to start) but not the
// session the entry replaces.
func (sessions *registry) checkLimits(entry, replaced *sessionEntry) error {
	counts := sessions.countExcept(replaced)

	if counts.all >= sessions.limits.maxSessions {
		return fmt.Errorf("%d sessions running, at most %d allowed: %w",
			counts.all, sessions.limits.maxSessions, ErrSessionLimitReached)
	}

	if entry.reencodes && counts.reencoding >= sessions.limits.maxReencoding {
		return fmt.Errorf("%d re-encoding sessions running, at most %d allowed: %w",
			counts.reencoding, sessions.limits.maxReencoding, ErrSessionLimitReached)
	}

	if entry.runsFFmpeg && counts.ffmpeg >= sessions.limits.maxFFmpeg {
		return fmt.Errorf("%d FFmpeg sessions running, at most %d allowed: %w",
			counts.ffmpeg, sessions.limits.maxFFmpeg, ErrSessionLimitReached)
	}

	return nil
}

// countExcept counts the slots in use, leaving out excluded (which may be nil). A session
// claimed by a replacement is left out as well: its slot belongs to the replacement.
func (sessions *registry) countExcept(excluded *sessionEntry) sessionCounts {
	var counts sessionCounts

	for _, existing := range sessions.entries {
		if existing == excluded || existing.replacedBy != "" {
			continue
		}

		counts.all++

		if existing.reencodes {
			counts.reencoding++
		}

		if existing.runsFFmpeg {
			counts.ffmpeg++
		}
	}

	return counts
}

// activate makes a pending session visible with its production and returns its snapshot,
// together with the record of the session it replaced, which the caller tears down outside
// the lock. The second result is false when the pending session no longer exists.
func (sessions *registry) activate(id string, started activation) (Session, *sessionEntry, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.entries[id]
	if !found || !entry.pending {
		return Session{}, nil, false
	}

	entry.pending = false
	entry.production = started.production
	entry.directory = started.directory
	entry.session.StartPosition = started.startPosition

	return entry.session.clone(), sessions.takeReplaced(entry), true
}

// takeReplaced removes the session entry replaces, if it still exists and is still claimed
// by entry. The caller holds the lock.
func (sessions *registry) takeReplaced(entry *sessionEntry) *sessionEntry {
	replaced, found := sessions.entries[entry.replaces]
	if !found || replaced.replacedBy != entry.session.ID {
		return nil
	}

	delete(sessions.entries, entry.replaces)

	return replaced
}

// release drops a pending session whose start failed, freeing its slot and its claim on the
// session it would have replaced, which keeps running.
func (sessions *registry) release(id string) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.entries[id]
	if !found {
		return
	}

	if replaced, exists := sessions.entries[entry.replaces]; exists && replaced.replacedBy == id {
		replaced.replacedBy = ""
	}

	delete(sessions.entries, id)
}

// get returns a snapshot of a visible session.
func (sessions *registry) get(id string) (Session, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found {
		return Session{}, false
	}

	return entry.session.clone(), true
}

// touch records an access of a visible session and returns what its stream handler needs.
func (sessions *registry) touch(id string, now time.Time) (StreamAccess, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found {
		return StreamAccess{}, false
	}

	entry.session.LastAccessedAt = now

	access := StreamAccess{Delivery: entry.session.Plan.Delivery, sessionID: id, production: entry.production}
	if access.Delivery == playback.DeliveryFile {
		access.LibraryRoot, access.RelativePath, access.ContentType = entry.libraryRoot, entry.relativePath, entry.contentType
	} else {
		access.Directory = entry.directory
	}

	return access, true
}

// remove takes a visible session out of the registry and returns its record. Exactly one
// caller gets the record, so only that caller tears the session down.
func (sessions *registry) remove(id string) (*sessionEntry, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found {
		return nil, false
	}

	delete(sessions.entries, id)

	return entry, true
}

// visible returns an activated session; the caller holds the lock.
func (sessions *registry) visible(id string) (*sessionEntry, bool) {
	entry, found := sessions.entries[id]
	if !found || entry.pending {
		return nil, false
	}

	return entry, true
}

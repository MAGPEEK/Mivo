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

// ErrServiceClosed reports a request after Close: the server is shutting down. It wraps
// domain.ErrUnavailable.
var ErrServiceClosed = fmt.Errorf("playback service is shut down: %w", domain.ErrUnavailable)

// errWorkspacePurging refuses new sessions while the startup purge runs, which would delete
// their directories.
var errWorkspacePurging = fmt.Errorf("playback workspace is being purged: %w", domain.ErrUnavailable)

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
	// inFlight counts the stream requests still being served. The reaper keeps a session
	// with any: a browser may stream a whole MP4 over one long Range request.
	inFlight int
}

// activation is what a started production adds to a pending session.
type activation struct {
	production    Production
	directory     string
	startPosition time.Duration
	// activatedAt restarts the idle time: clients learn the session ID only now.
	activatedAt time.Time
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

// add counts existing, which claimant is about to replace (nil when no one claimed it). The
// slots of a claimed session belong to its claimant, except in the categories the claimant
// does not occupy: a transcode being replaced by a remux keeps re-encoding until the remux
// has started and stopped it.
func (counts *sessionCounts) add(existing, claimant *sessionEntry) {
	if claimant == nil {
		counts.all++
	}

	if existing.reencodes && (claimant == nil || !claimant.reencodes) {
		counts.reencoding++
	}

	if existing.runsFFmpeg && (claimant == nil || !claimant.runsFFmpeg) {
		counts.ffmpeg++
	}
}

// registry holds the sessions in memory and enforces the limits atomically, so concurrent
// starts never exceed them. Its methods never call ports, so the lock is never held while
// waiting for FFmpeg or the file system.
type registry struct {
	mutex   sync.Mutex
	entries map[string]*sessionEntry
	limits  sessionLimits

	// closed refuses new sessions after Close; purging refuses them while the workspace is
	// purged; used records that a session was ever reserved, after which a purge could
	// delete a live directory.
	closed, purging, used bool
	// holders counts the work that holds session records outside the registry (starts,
	// stops, the reaper, production watchers). Close waits for it, so no FFmpeg outlives
	// the service. Every Add happens under the mutex while the registry is open, so no Add
	// races with the Wait of Close.
	holders sync.WaitGroup
}

func newRegistry(limits sessionLimits) *registry {
	return &registry{entries: make(map[string]*sessionEntry), limits: limits}
}

// reserve adds a new pending session when the limits allow it. When the entry replaces a
// visible session that no other replacement claimed yet, it takes over that session's slot
// in the same step, so no other client can take it during a seek (ADR 0024: seeking never
// trips the limit); an unknown or already claimed session is ignored. A reached limit wraps
// ErrSessionLimitReached; a session ID in use is an internal error (a broken generator).
// On success the caller holds the entry and must call untrack when it is done with it.
func (sessions *registry) reserve(entry *sessionEntry) error {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if err := sessions.admission(); err != nil {
		return err
	}

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
	sessions.used = true
	sessions.holders.Add(1)

	return nil
}

// admission tells whether new sessions are accepted; the caller holds the lock.
func (sessions *registry) admission() error {
	switch {
	case sessions.closed:
		return ErrServiceClosed
	case sessions.purging:
		return errWorkspacePurging
	default:
		return nil
	}
}

// checkLimits counts pending sessions too (their FFmpeg is about to start).
func (sessions *registry) checkLimits(entry, replaced *sessionEntry) error {
	counts := sessions.count(entry, replaced)

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

// count counts the slots in use as entry sees them when it claims replaced (nil when it
// replaces none). Every claimed session, including replaced, shares its slots with its
// claimant (see sessionCounts.add). The caller holds the lock.
func (sessions *registry) count(entry, replaced *sessionEntry) sessionCounts {
	var counts sessionCounts

	for _, existing := range sessions.entries {
		var claimant *sessionEntry

		switch {
		case existing == replaced:
			claimant = entry
		case existing.replacedBy != "":
			claimant = sessions.entries[existing.replacedBy]
		}

		counts.add(existing, claimant)
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
	entry.session.LastAccessedAt = started.activatedAt

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

// touch records the start of a stream request of a visible session: it marks the session in
// use until endAccess and returns what the stream handler needs, plus the record endAccess
// takes.
func (sessions *registry) touch(id string, now time.Time) (StreamAccess, *sessionEntry, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found {
		return StreamAccess{}, nil, false
	}

	entry.session.LastAccessedAt = now
	entry.inFlight++

	access := StreamAccess{Delivery: entry.session.Plan.Delivery, sessionID: id, production: entry.production}
	if access.Delivery == playback.DeliveryFile {
		access.LibraryRoot, access.RelativePath, access.ContentType = entry.libraryRoot, entry.relativePath, entry.contentType
	} else {
		access.Directory = entry.directory
	}

	return access, entry, true
}

// endAccess records the end of a stream request: the idle time restarts at now, so a session
// read by one long request becomes idle only when that request ends. It works on the record,
// not the ID, so a request of a session that was stopped meanwhile changes nothing visible;
// the count never goes below zero.
func (sessions *registry) endAccess(entry *sessionEntry, now time.Time) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	if entry.inFlight > 0 {
		entry.inFlight--
	}

	if now.After(entry.session.LastAccessedAt) {
		entry.session.LastAccessedAt = now
	}
}

// remove takes a visible session out of the registry and returns its record. Exactly one
// caller gets the record, so only that caller tears the session down; it must call untrack
// when the teardown is done. Its Add never races with the Wait of Close: close empties the
// registry under the same lock, so once it is closed remove finds nothing and adds nothing.
func (sessions *registry) remove(id string) (*sessionEntry, bool) {
	sessions.mutex.Lock()
	defer sessions.mutex.Unlock()

	entry, found := sessions.visible(id)
	if !found {
		return nil, false
	}

	delete(sessions.entries, id)
	sessions.holders.Add(1)

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

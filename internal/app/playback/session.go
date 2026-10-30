package playback

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain/playback"
)

// State is the lifecycle state of a session (API PlaybackSessionState, ADR 0024).
type State string

// The session states. Direct-play sessions start ready; HLS sessions start starting and
// follow their production: ready with the first segment, ended after a clean exit with
// segments, failed (FailureTranscodeFailed) after an exit error or a clean exit without any
// segment. A stopped session is removed, not ended.
const (
	StateStarting State = "starting"
	StateReady    State = "ready"
	StateEnded    State = "ended"
	StateFailed   State = "failed"
)

// Session is a snapshot of one playback session. Callers own the returned value.
type Session struct {
	// ID is a random UUID; it makes the stream URL unguessable (ADR 0024).
	ID     string
	FileID string
	State  State
	// FailureCode is the machine-readable reason of StateFailed (for example
	// "transcode_failed"); empty in every other state.
	FailureCode string
	Plan        playback.Plan
	// StreamURL is the opaque URL the client loads, built by the StreamURLBuilder.
	StreamURL string
	// StartPosition is where the stream starts in the file: the keyframe at or before the
	// requested position when video is copied, the requested position when it is
	// re-encoded, and 0 for direct play, whose stream is the whole file (the player seeks in
	// it locally).
	StartPosition time.Duration
	// Duration is the probed duration of the file; 0 when unknown.
	Duration time.Duration
	// CreatedAt and LastAccessedAt come from the service clock; Access updates
	// LastAccessedAt, which drives the idle timeout.
	CreatedAt      time.Time
	LastAccessedAt time.Time
}

// clone returns a copy that shares no slices with the session, so callers cannot change
// the stored plan.
func (session Session) clone() Session {
	session.Plan.Streams = slices.Clone(session.Plan.Streams)
	session.Plan.Reasons = slices.Clone(session.Plan.Reasons)
	session.Plan.Warnings = slices.Clone(session.Plan.Warnings)

	return session
}

// StartRequest asks to play one media file for one client.
type StartRequest struct {
	FileID  string
	Profile playback.ClientProfile
	// StartPosition is the requested position in the file; 0 starts at the beginning.
	StartPosition time.Duration
	// AudioStreamIndex selects an audio stream by its ffprobe index; nil plays the default.
	AudioStreamIndex *int
	// ReplacesSessionID names the session this one replaces, for example after a seek outside
	// the produced range. The new session takes over its slot when it is reserved, so the
	// limits never refuse the seek; the old session keeps playing until the new production
	// has started and is stopped only then, so a failed start leaves it untouched. Empty,
	// unknown or already replaced IDs are ignored.
	ReplacesSessionID string
}

// StreamAccess is what the stream handler needs to serve a session's bytes. It also marks the
// session in use: the idle reaper keeps a session while any of its accesses is open, so the
// delivery adapter must call Release (deferred) once it has served the request.
type StreamAccess struct {
	Delivery playback.Delivery
	// LibraryRoot, RelativePath and ContentType locate and describe the original file of a
	// direct-play session (playback.DeliveryFile); empty for HLS.
	LibraryRoot  string
	RelativePath string
	ContentType  string
	// Directory is the workspace directory holding the playlist and segments of an HLS
	// session; empty for direct play.
	Directory string

	sessionID  string
	production Production
	// lease is shared by every copy of the access, so releasing one copy releases all.
	lease *accessLease
}

// accessLease ends one access exactly once.
type accessLease struct {
	once sync.Once
	end  func()
}

// Release ends the request this access was opened for: the session's idle time restarts
// now, and when no other request is open the reaper may stop it after IdleTimeout. Every
// successful Service.Access must be released, typically with defer. Release is idempotent
// (also across copies), safe after the session was stopped, and a no-op on the zero value.
func (access StreamAccess) Release() {
	if access.lease != nil {
		access.lease.once.Do(access.lease.end)
	}
}

// WaitReady blocks until the stream can be loaded. Direct play is ready at once; HLS waits
// for the production's first segment and reports a failed production or ctx.Err().
func (access StreamAccess) WaitReady(ctx context.Context) error {
	if access.production == nil {
		return nil
	}

	if err := access.production.Ready(ctx); err != nil {
		return fmt.Errorf("wait for stream of session %s: %w", access.sessionID, err)
	}

	return nil
}

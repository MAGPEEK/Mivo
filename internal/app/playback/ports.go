// Package playback contains the playback session use cases (ADR 0023, ADR 0024): a client
// asks to play a media file with its profile, the decision engine picks direct play, remux
// or transcode, and the service starts FFmpeg only for the HLS modes, enforces the session
// limits and stops sessions on request. Sessions live in memory; their state follows the
// production, idle sessions are reaped, and Close stops every FFmpeg on shutdown.
//
// The package declares the ports it needs; adapters implement them (sqlite: the playable
// file source, fs/libraryfs: the path resolver, ffmpeg: the keyframe locator and the stream
// producer, fs/transcodedir: the workspace, http/stream: the stream URL builder) and
// cmd/mivo wires them (ADR 0003).
package playback

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// PlayableFile is a catalogued media file together with the root of its library, which the
// service needs to resolve the file's relative path.
type PlayableFile struct {
	File media.File
	// LibraryRoot is the absolute root folder of the file's library.
	LibraryRoot string
}

// FileSource loads media files for playback. Implemented by adapters/sqlite.
type FileSource interface {
	// PlayableFile returns the file with its streams and library root, or an error wrapping
	// domain.ErrNotFound for an unknown file ID.
	PlayableFile(ctx context.Context, fileID string) (PlayableFile, error)
}

// PathResolver turns a library-relative path into the absolute path FFmpeg opens, refusing
// paths that leave the library root. Implemented by adapters/fs/libraryfs.
type PathResolver interface {
	// Resolve returns the absolute path of a regular file below root. Errors wrap
	// domain.ErrForbidden (outside the root), domain.ErrNotFound (missing file) or
	// domain.ErrUnavailable (unreachable library folder).
	Resolve(root, relativePath string) (string, error)
}

// KeyframeLocator finds where a stream-copy session really starts. Implemented by
// adapters/ffmpeg (ffprobe).
type KeyframeLocator interface {
	// KeyframeAtOrBefore returns the position of the last video keyframe at or before
	// position, measured from the format start like FFmpeg's -ss (not as an absolute
	// timestamp). It returns 0 when no keyframe is found and an error when ffprobe cannot
	// search the file; the service then starts at 0.
	KeyframeAtOrBefore(ctx context.Context, path string, position time.Duration) (time.Duration, error)
}

// Job is everything the stream producer needs to write one session's HLS output.
type Job struct {
	// SessionID names the session in logs.
	SessionID string
	// Plan is a remux or transcode plan of playback.Decide.
	Plan playback.Plan
	// LibraryRoot and RelativePath identify the input inside its library, for producers
	// that open the file themselves (os.Root); InputPath is the same file resolved to an
	// absolute path.
	LibraryRoot  string
	RelativePath string
	InputPath    string
	// StartPosition is where FFmpeg seeks to (-ss) before reading; 0 starts at the
	// beginning. For copied video it is already the keyframe position.
	StartPosition time.Duration
	// OutputDir is the session's workspace directory that receives the playlist and segments.
	OutputDir string
}

// StreamProducer starts the FFmpeg process of a session. Implemented by adapters/ffmpeg.
type StreamProducer interface {
	// Start launches the production of job. ctx is owned by the playback service and bounds
	// the process lifetime; it is never a request context.
	Start(ctx context.Context, job Job) (Production, error)
}

// ErrProductionStopped is what Production.Ready reports after Stop: the session was stopped
// on purpose, so the kill's exit error is not a failure. It wraps domain.ErrNotFound, because
// a stopped session no longer exists for its clients.
var ErrProductionStopped = fmt.Errorf("production stopped: %w", domain.ErrNotFound)

// Production is one running FFmpeg process writing HLS into a session's workspace.
// Implementations are safe for concurrent use.
type Production interface {
	// Ready blocks until the playlist lists a first segment and returns nil; ctx.Err() when
	// ctx ends first. Once the process has ended (Done is closed), Ready returns at once and
	// tells how it ended, in this order:
	//   - after Stop: an error wrapping ErrProductionStopped, never the kill's exit error;
	//   - FFmpeg exited with an error, before or after the first segment: that exit error;
	//   - a clean exit without any segment (empty output): an error saying so;
	//   - a clean exit with segments: nil.
	Ready(ctx context.Context) error
	// Stop kills the process if it still runs and waits for it to exit. It is idempotent.
	Stop()
	// Done is closed when the process has exited, for whatever reason.
	Done() <-chan struct{}
}

// Workspace manages the per-session output directories. Implemented by
// adapters/fs/transcodedir.
type Workspace interface {
	// Create makes the directory of a session and returns its absolute path.
	Create(ctx context.Context, sessionID string) (string, error)
	// Remove deletes the directory of a session; a missing directory is not an error.
	Remove(ctx context.Context, sessionID string) error
	// Purge deletes the session directories a previous run left behind and returns how many
	// it removed. It continues after a failed removal and reports the joined errors.
	Purge(ctx context.Context) (int, error)
}

// StreamURLBuilder builds the opaque stream URL clients load. Implemented by
// adapters/http/stream, the only place that knows the stream routes.
type StreamURLBuilder interface {
	// StreamURL returns the URL of a session's file (playback.DeliveryFile) or HLS playlist
	// (playback.DeliveryHLS).
	StreamURL(sessionID string, delivery playback.Delivery) string
}

// IDGenerator creates random session IDs in canonical lowercase UUID text form (ADR 0024:
// unguessable stream URLs). Implemented by adapters/uuidgen.
type IDGenerator interface {
	// NewID returns a new random ID.
	NewID() (string, error)
}

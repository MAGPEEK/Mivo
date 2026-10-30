package stream

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// readyTimeout bounds how long a stream request waits for the session's first segment
// (ADR 0024) before it answers 503 stream_not_ready.
const readyTimeout = 20 * time.Second

// sessionAttribute names the session ID in log records, as app/playback does.
const sessionAttribute = "session"

// Sessions grants access to the streams of playback sessions. Implemented by
// *playback.Service (app/playback).
type Sessions interface {
	// Access records a stream request for the session and returns what may be served. An
	// unknown or stopped session wraps domain.ErrNotFound.
	Access(ctx context.Context, sessionID string) (appplayback.StreamAccess, error)
}

// LibraryFiles opens the original file of a direct-play session. Implemented by
// *libraryfs.FileSystem, which opens it through os.Root of the library root.
type LibraryFiles interface {
	// Open opens a regular file below root for reading. A missing file wraps
	// domain.ErrNotFound; the caller closes the file.
	Open(root, relativePath string) (*os.File, error)
}

// SessionFiles opens a file of a session's HLS output. Implemented by
// *transcodedir.Workspace, which opens it through os.Root of the session directory.
type SessionFiles interface {
	// OpenFile opens name inside the directory of the session. A missing session or file
	// wraps domain.ErrNotFound; the caller closes the file.
	OpenFile(sessionID, name string) (*os.File, error)
}

// Options are the dependencies of the stream handlers; all are required.
type Options struct {
	Sessions Sessions
	Library  LibraryFiles
	Output   SessionFiles
	Logger   *slog.Logger
}

func (options Options) validate() error {
	switch {
	case options.Sessions == nil:
		return fmt.Errorf("missing sessions: %w", domain.ErrInvalid)
	case options.Library == nil:
		return fmt.Errorf("missing library files: %w", domain.ErrInvalid)
	case options.Output == nil:
		return fmt.Errorf("missing session files: %w", domain.ErrInvalid)
	case options.Logger == nil:
		return fmt.Errorf("missing logger: %w", domain.ErrInvalid)
	default:
		return nil
	}
}

// Handlers serves the stream routes. It is safe for concurrent use.
type Handlers struct {
	access  accessFunc
	library LibraryFiles
	output  SessionFiles
	logger  *slog.Logger
	// readyTimeout is the readyTimeout constant; tests shorten it.
	readyTimeout time.Duration
}

// New returns the stream handlers. A missing dependency is a wiring bug and wraps
// domain.ErrInvalid.
func New(options Options) (*Handlers, error) {
	if err := options.validate(); err != nil {
		return nil, fmt.Errorf("new stream handlers: %w", err)
	}

	return &Handlers{
		access:       grantsOf(options.Sessions),
		library:      options.Library,
		output:       options.Output,
		logger:       options.Logger,
		readyTimeout: readyTimeout,
	}, nil
}

// Mounts returns the stream routes for restapi.Options.Mounts.
func (handlers *Handlers) Mounts() []restapi.Mount {
	return []restapi.Mount{
		{Pattern: filePattern, Handler: http.HandlerFunc(handlers.serveFile)},
		{Pattern: hlsPattern, Handler: http.HandlerFunc(handlers.serveHLS)},
	}
}

// grant is a stream access the playback service granted for one request. The handler must
// call release once it is done with the stream, however the request ends: while a grant is
// held the session counts as in use and is not reaped as idle, also during a Range response
// that runs longer than the idle timeout.
type grant struct {
	sessionID string
	delivery  playback.Delivery
	// libraryRoot, relativePath and contentType describe the file of a direct-play session.
	libraryRoot  string
	relativePath string
	contentType  string
	// waitReady blocks until the stream can be loaded (StreamAccess.WaitReady).
	waitReady func(ctx context.Context) error
	// release ends the grant (StreamAccess.Release); it is idempotent.
	release func()
}

// accessFunc grants access to a session's stream. The handlers take a function rather than
// the Sessions port so tests can observe waitReady and release, which StreamAccess keeps
// behind unexported fields.
type accessFunc func(ctx context.Context, sessionID string) (grant, error)

// grantsOf adapts the playback service's StreamAccess to grants.
func grantsOf(sessions Sessions) accessFunc {
	return func(ctx context.Context, sessionID string) (grant, error) {
		access, err := sessions.Access(ctx, sessionID)
		if err != nil {
			return grant{}, fmt.Errorf("access stream: %w", err)
		}

		return grant{
			sessionID: sessionID, delivery: access.Delivery,
			libraryRoot: access.LibraryRoot, relativePath: access.RelativePath, contentType: access.ContentType,
			waitReady: access.WaitReady, release: access.Release,
		}, nil
	}
}

// sessionID returns the canonical lowercase form of the request's session ID. Clients may
// send any spelling the API's uuid format allows; sessions are keyed by the canonical one.
func sessionID(request *http.Request) (string, bool) {
	parsed, err := uuid.Parse(request.PathValue(sessionIDParameter))
	if err != nil {
		return "", false
	}

	return parsed.String(), true
}

// grantAccess asks the playback service for the session's stream, which also records the
// access and so keeps the session alive. It answers the request itself when access is
// refused and reports false then; on success the caller must release the grant.
func (handlers *Handlers) grantAccess(response *statuswriter.Recorder, request *http.Request, id string) (grant, bool) {
	granted, err := handlers.access(request.Context(), id)
	if err == nil {
		return granted, true
	}

	if errors.Is(err, domain.ErrNotFound) {
		writeProblem(response, notFoundProblem())

		return grant{}, false
	}

	handlers.logger.ErrorContext(request.Context(), "grant stream access",
		logtext.RequestAttributes(request, slog.String(sessionAttribute, id), logtext.Error(err))...)
	writeProblem(response, restapi.InternalProblem())

	return grant{}, false
}

package restapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Options are the dependencies of the REST API. Every service and the logger are required:
// the contract has no optional operations.
type Options struct {
	// System serves the health and system info endpoints.
	System SystemService
	// Libraries serves the library endpoints.
	Libraries LibraryCatalog
	// Items serves the item endpoints.
	Items ItemCatalog
	// Scan starts library scans and reports their status.
	Scan ScanService
	// Playback creates, reads and stops playback sessions.
	Playback PlaybackService
	// Logger records failures; request details are logged, never sent to clients.
	Logger *slog.Logger
	// Mounts are extra routes on the API mux for handlers the generated strict server
	// cannot express, such as the stream delivery (ADR 0024). They share the API's body
	// limit and its 404 and 405 problem answers.
	Mounts []Mount
}

// Mount is one extra route of the API mux.
type Mount struct {
	// Pattern is a net/http ServeMux pattern without a host, normally with a method, whose
	// path starts with /api/, for example "GET /api/v1/playback/sessions/{sessionId}/file".
	// It must not overlap a route of the contract or another mount.
	Pattern string
	// Handler serves the requests that match Pattern.
	Handler http.Handler
}

// validate reports every wiring bug at once as an error wrapping domain.ErrInvalid. Mount
// patterns are checked when they are registered, because only the mux knows whether they
// conflict.
func (options Options) validate() error {
	required := []struct {
		name    string
		present bool
	}{
		{name: "system service", present: options.System != nil},
		{name: "library catalog", present: options.Libraries != nil},
		{name: "item catalog", present: options.Items != nil},
		{name: "scan service", present: options.Scan != nil},
		{name: "playback service", present: options.Playback != nil},
		{name: "logger", present: options.Logger != nil},
	}

	var problems []error

	for _, dependency := range required {
		if !dependency.present {
			problems = append(problems, fmt.Errorf("missing %s: %w", dependency.name, domain.ErrInvalid))
		}
	}

	for index, mount := range options.Mounts {
		if err := mount.validate(index); err != nil {
			problems = append(problems, err)
		}
	}

	return errors.Join(problems...)
}

func (mount Mount) validate(index int) error {
	switch {
	case mount.Pattern == "":
		return fmt.Errorf("mount %d: missing pattern: %w", index, domain.ErrInvalid)
	case mount.Handler == nil:
		return fmt.Errorf("mount %d (%s): missing handler: %w", index, mount.Pattern, domain.ErrInvalid)
	case !strings.HasPrefix(patternPath(mount.Pattern), apiPathPrefix):
		return fmt.Errorf("mount %d (%s): path outside %s: %w", index, mount.Pattern, apiPathPrefix, domain.ErrInvalid)
	default:
		return nil
	}
}

// apiPathPrefix is where the HTTP server routes requests to this handler; a mount anywhere
// else would never be reached.
const apiPathPrefix = "/api/"

// patternPath returns the path of a ServeMux pattern "[METHOD ][HOST]/[PATH]". A pattern
// with a host yields a path that does not start with "/", which validate rejects as well.
func patternPath(pattern string) string {
	_, path, hasMethod := strings.Cut(pattern, " ")
	if !hasMethod {
		return pattern
	}

	return strings.TrimLeft(path, " \t")
}

// registerMount adds mount to mux. ServeMux.Handle panics on a malformed pattern or one that
// conflicts with a registered route; both are wiring bugs that must reach the caller as an
// error rather than crash the server, so the panic is turned into an error here.
func registerMount(mux *http.ServeMux, mount Mount) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("mount %s: %v: %w", mount.Pattern, recovered, domain.ErrInvalid)
		}
	}()

	mux.Handle(mount.Pattern, mount.Handler)

	return nil
}

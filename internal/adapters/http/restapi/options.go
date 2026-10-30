package restapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Options are the dependencies of the REST API. System and Logger are required. A nil
// feature service makes the operations of that feature answer 501 with the internal code
// "not_implemented", so features can be wired one at a time; the composition root makes
// them mandatory once all of them exist (M2-32).
type Options struct {
	// System serves the health and system info endpoints.
	System SystemService
	// Library serves the library and item endpoints.
	Library LibraryService
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

// validate reports a wiring bug as an error wrapping domain.ErrInvalid. Mount patterns are
// checked when they are registered, because only the mux knows whether they conflict.
func (options Options) validate() error {
	if options.System == nil {
		return fmt.Errorf("missing system service: %w", domain.ErrInvalid)
	}

	if options.Logger == nil {
		return fmt.Errorf("missing logger: %w", domain.ErrInvalid)
	}

	for index, mount := range options.Mounts {
		if mount.Pattern == "" {
			return fmt.Errorf("mount %d: missing pattern: %w", index, domain.ErrInvalid)
		}

		if mount.Handler == nil {
			return fmt.Errorf("mount %d (%s): missing handler: %w", index, mount.Pattern, domain.ErrInvalid)
		}

		if !strings.HasPrefix(patternPath(mount.Pattern), apiPathPrefix) {
			return fmt.Errorf("mount %d (%s): path outside %s: %w", index, mount.Pattern, apiPathPrefix, domain.ErrInvalid)
		}
	}

	return nil
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

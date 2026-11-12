package restapi

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Options are the dependencies of the REST API. The logger and every service except Rows, Search and
// Metadata are required.
type Options struct {
	// System serves the health and system info endpoints.
	System SystemService
	// Libraries serves the library endpoints.
	Libraries LibraryCatalog
	// Items serves the item endpoints.
	Items ItemCatalog
	// Rows generates the Home and Overview rows. It is optional: without it
	// the rows endpoints answer 501 "not_implemented", which their contract allows, so test
	// servers that never ask for rows need not wire it.
	Rows RowsService
	// Search searches the catalog. Optional like Rows: without it the search endpoint answers
	// 501 "not_implemented".
	Search SearchService
	// Scan starts library scans and reports their status.
	Scan ScanService
	// Playback creates, reads and stops playback sessions.
	Playback PlaybackService
	// Manager creates, changes and removes libraries.
	Manager LibraryManager
	// Folders serves the folder browser.
	Folders FolderBrowsing
	// Install describes the installation for the folder roots answer.
	Install InstallInfo
	// LibraryKinds are the kinds createLibrary accepts (SystemInfo.libraryKinds).
	LibraryKinds []library.Kind
	// Metadata serves the online metadata endpoints (milestone 6). Every service in it is
	// optional; see MetadataServices.
	Metadata MetadataServices
	// Logger records failures; request details are logged, never sent to clients.
	Logger *slog.Logger
	// Mounts are extra routes on the API mux for handlers the generated strict server
	// cannot express, such as the stream delivery (ADR 0024). They share the API's body
	// limit and its 404 and 405 problem answers.
	Mounts []Mount
	// Auth serves setup, sign-in, the own account, profile selection and the own devices
	// (ADR 0032). It is optional: without it those endpoints answer 501 "not_implemented".
	Auth AuthService
	// Users serves the user administration endpoints (ADR 0032). Every service in it is
	// optional; see UserServices.
	Users UserServices
	// Watch serves the watch state endpoints and adds viewerState to items (ADR 0032 section 8).
	// It is optional: without it those endpoints answer 501 "not_implemented" and items carry
	// no viewerState.
	Watch WatchService
	// Streaming serves the session list, the transcoder status and test, the trickplay manifest
	// and the external subtitle tracks of the media file resource (ADR 0030). Every service in
	// it is optional; see StreamingServices.
	Streaming StreamingServices
	// Reanalysis re-probes the files of one item (PL-35). It is optional: without it the
	// endpoint answers 501 "not_implemented".
	Reanalysis ItemReanalyzer
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
		{name: "library manager", present: options.Manager != nil},
		{name: "folder browser", present: options.Folders != nil},
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

package stream_test

import (
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/stream"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	logger := slog.New(slog.DiscardHandler)
	library := mustLibraryFS(t, logger)
	workspace := mustWorkspace(t)
	complete := stream.Options{Sessions: env.service, Library: library, Output: workspace, Logger: logger}

	tests := map[string]func(*stream.Options){
		"sessions":      func(options *stream.Options) { options.Sessions = nil },
		"library files": func(options *stream.Options) { options.Library = nil },
		"session files": func(options *stream.Options) { options.Output = nil },
		"logger":        func(options *stream.Options) { options.Logger = nil },
	}

	for name, remove := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			options := complete
			remove(&options)

			handlers, err := stream.New(options)
			if !errors.Is(err, domain.ErrInvalid) || handlers != nil {
				t.Fatalf("New() = (%v, %v), want (nil, ErrInvalid)", handlers, err)
			}

			if !strings.Contains(err.Error(), name) {
				t.Errorf("error %q does not name the missing %s", err, name)
			}
		})
	}
}

func TestStreamURLNamesTheDeliveryRoute(t *testing.T) {
	t.Parallel()

	builder := stream.NewURLBuilder()
	tests := []struct {
		delivery playback.Delivery
		want     string
	}{
		{delivery: playback.DeliveryFile, want: "/api/v1/playback/sessions/" + unknownSessionID + "/file"},
		{delivery: playback.DeliveryHLS, want: "/api/v1/playback/sessions/" + unknownSessionID + "/hls/index.m3u8"},
		{delivery: playback.Delivery("smoke-signals"), want: ""},
	}

	for _, test := range tests {
		if got := builder.StreamURL(unknownSessionID, test.delivery); got != test.want {
			t.Errorf("StreamURL(%q) = %q, want %q", test.delivery, got, test.want)
		}
	}
}

func TestStreamURLEscapesTheSessionID(t *testing.T) {
	t.Parallel()

	got := stream.NewURLBuilder().StreamURL("a/../b", playback.DeliveryFile)
	if want := "/api/v1/playback/sessions/a%2F..%2Fb/file"; got != want {
		t.Errorf("StreamURL = %q, want %q", got, want)
	}
}

// TestSessionStreamURLsReachTheirHandlers checks that the URL the service hands out is
// served by the mounted routes, for both deliveries.
func TestSessionStreamURLsReachTheirHandlers(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)

	direct := env.start(t, directFileID)
	if recorder := env.get(t, direct.StreamURL); recorder.Code != http.StatusOK {
		t.Errorf("GET %s = %d, want 200", direct.StreamURL, recorder.Code)
	}

	hls, production := env.startHLS(t)
	writeOutput(t, production, "index.m3u8", []byte("#EXTM3U\n"))
	production.markReady()

	if recorder := env.get(t, hls.StreamURL); recorder.Code != http.StatusOK {
		t.Errorf("GET %s = %d, want 200", hls.StreamURL, recorder.Code)
	}
}

func TestQuietPatternsAreTheMountedRoutes(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)

	var mounted []string
	for _, mount := range env.handlers.Mounts() {
		mounted = append(mounted, mount.Pattern)
	}

	want := []string{
		"GET /api/v1/playback/sessions/{sessionId}/file",
		"GET /api/v1/playback/sessions/{sessionId}/hls/{fileName}",
	}

	if !slices.Equal(mounted, want) || !slices.Equal(stream.QuietPatterns(), want) {
		t.Errorf("mounts = %q, quiet patterns = %q, want both %q", mounted, stream.QuietPatterns(), want)
	}
}

func TestReadyTimeoutIsTwentySeconds(t *testing.T) {
	t.Parallel()

	if stream.ReadyTimeout != 20*time.Second {
		t.Errorf("ReadyTimeout = %s, want 20s (ADR 0024)", stream.ReadyTimeout)
	}
}

func TestStreamRoutesRefuseUnknownAndMalformedSessions(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	stopped := env.start(t, directFileID)

	if err := env.service.Stop(t.Context(), stopped.ID); err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	tests := map[string]struct {
		target     string
		wantStatus int
		wantCode   string
	}{
		"unknown file session":   {target: fileURL(unknownSessionID), wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		"unknown hls session":    {target: hlsURL(unknownSessionID, "index.m3u8"), wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		"stopped session":        {target: stopped.StreamURL, wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		"malformed file session": {target: fileURL("not-a-uuid"), wantStatus: http.StatusBadRequest, wantCode: restapi.CodeBadRequest},
		"malformed hls session":  {target: hlsURL("1234", "index.m3u8"), wantStatus: http.StatusBadRequest, wantCode: restapi.CodeBadRequest},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assertProblem(t, env.get(t, test.target), test.wantStatus, test.wantCode)
		})
	}
}

func TestStreamRoutesServeOnlyTheirDelivery(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	direct := env.start(t, directFileID)
	hls, production := env.startHLS(t)
	production.markReady()

	assertProblem(t, env.get(t, hlsURL(direct.ID, "index.m3u8")), http.StatusNotFound, restapi.CodeNotFound)
	assertProblem(t, env.get(t, fileURL(hls.ID)), http.StatusNotFound, restapi.CodeNotFound)
}

func TestSessionIDsAreMatchedInCanonicalForm(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	session := env.start(t, directFileID)

	if recorder := env.get(t, fileURL(strings.ToUpper(session.ID))); recorder.Code != http.StatusOK {
		t.Errorf("GET with an uppercase session ID = %d, want 200", recorder.Code)
	}
}

func TestStreamRequestsRecordTheAccess(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	direct := env.start(t, directFileID)
	hls, production := env.startHLS(t)
	writeOutput(t, production, "index.m3u8", []byte("#EXTM3U\n"))
	production.markReady()

	for _, session := range []struct{ id, target string }{{direct.ID, direct.StreamURL}, {hls.ID, hls.StreamURL}} {
		accessed := env.clock.advance(time.Minute)
		env.get(t, session.target)

		if got := lastAccess(t, env, session.id); !got.Equal(accessed) {
			t.Errorf("LastAccessedAt after GET %s = %s, want %s", session.target, got, accessed)
		}
	}

	// A name the route never serves must not keep a session alive.
	before := lastAccess(t, env, hls.ID)
	env.clock.advance(time.Minute)
	env.get(t, hlsURL(hls.ID, "secret.txt"))

	if got := lastAccess(t, env, hls.ID); !got.Equal(before) {
		t.Errorf("LastAccessedAt after an invalid name = %s, want unchanged %s", got, before)
	}
}

func lastAccess(t *testing.T, env *environment, sessionID string) time.Time {
	t.Helper()

	session, err := env.service.Get(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("Get(%s) error = %v", sessionID, err)
	}

	return session.LastAccessedAt
}

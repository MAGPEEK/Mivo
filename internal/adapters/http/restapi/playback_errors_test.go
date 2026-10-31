package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// sessionLimitRetryAfter pins the Retry-After of session_limit_reached; players wait this
// long before they offer a retry.
const sessionLimitRetryAfter = "30"

// startFailure wraps err like app/playback.Service.Start does.
func startFailure(err error) error {
	return fmt.Errorf("start playback of file %s: %w", testFileID, err)
}

// TestCreatePlaybackSessionMapsStartErrors checks the specific playback problems before the
// generic ones they wrap: ErrNoCompatibleFormat and ErrTranscodeUnavailable wrap
// domain.ErrUnsupported, ErrSessionLimitReached wraps domain.ErrUnavailable.
func TestCreatePlaybackSessionMapsStartErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		err            error
		wantStatus     int
		wantCode       string
		wantRetryAfter string
	}{
		{
			name: "unknown file", err: startFailure(fmt.Errorf("load file: %w", domain.ErrNotFound)),
			wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound,
		},
		{
			name: "no compatible format", err: startFailure(playback.ErrNoCompatibleFormat),
			wantStatus: http.StatusUnprocessableEntity, wantCode: restapi.CodeNoCompatibleFormat,
		},
		{
			name: "transcode unavailable", err: startFailure(playback.ErrTranscodeUnavailable),
			wantStatus: http.StatusUnprocessableEntity, wantCode: restapi.CodeTranscodeUnavailable,
		},
		{
			name:       "session limit reached",
			err:        startFailure(fmt.Errorf("2 re-encoding sessions running, at most 2 allowed: %w", appplayback.ErrSessionLimitReached)),
			wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeSessionLimitReached, wantRetryAfter: sessionLimitRetryAfter,
		},
		{
			name:       "file with a probe error",
			err:        startFailure(fmt.Errorf("file has no probe result (probe_failed): %w", domain.ErrUnsupported)),
			wantStatus: http.StatusUnprocessableEntity, wantCode: restapi.CodeUnsupported,
		},
		{
			name:       "shutting down",
			err:        startFailure(fmt.Errorf("session ended while it started: %w", domain.ErrUnavailable)),
			wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeUnavailable,
		},
		{
			name:       "start position beyond the file",
			err:        startFailure(fmt.Errorf("start position beyond the duration: %w", domain.ErrInvalid)),
			wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid,
		},
		{
			// 403 means "client outside the LAN" (M2-31), so the escape is "unsupported".
			name:       "file outside its library",
			err:        startFailure(fmt.Errorf("resolve movies/x.mkv: %w", appplayback.ErrFileOutsideLibrary)),
			wantStatus: http.StatusUnprocessableEntity, wantCode: restapi.CodeUnsupported,
		},
		{
			name: "port failure", err: startFailure(errors.New("create workspace: disk full")),
			wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, logs := newPlaybackHandler(t, &fakePlaybackService{startErr: test.err})

			got := postSession(t, handler, playbackRequestBody)
			assertProblem(t, got, test.wantStatus, test.wantCode)

			if retryAfter := got.header.Get("Retry-After"); retryAfter != test.wantRetryAfter {
				t.Errorf("Retry-After = %q, want %q", retryAfter, test.wantRetryAfter)
			}

			if test.wantCode == restapi.CodeSessionLimitReached {
				assertLogged(t, logs.String(), `level=WARN msg="playback session limit reached" operation=createPlaybackSession`)
			}

			if strings.Contains(logs.String(), "playback session started") {
				t.Errorf("a failed start was logged as started: %s", logs.String())
			}
		})
	}
}

// TestProblemForPlaybackErrors covers the playback rows of the shared mapping, which the
// stream handlers use as well.
func TestProblemForPlaybackErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "no compatible format", err: playback.ErrNoCompatibleFormat, wantStatus: http.StatusUnprocessableEntity, wantCode: "no_compatible_format"},
		{name: "transcode unavailable", err: playback.ErrTranscodeUnavailable, wantStatus: http.StatusUnprocessableEntity, wantCode: "transcode_unavailable"},
		{name: "session limit reached", err: appplayback.ErrSessionLimitReached, wantStatus: http.StatusServiceUnavailable, wantCode: "session_limit_reached"},
		{name: "plain unsupported", err: domain.ErrUnsupported, wantStatus: http.StatusUnprocessableEntity, wantCode: "unsupported"},
		{name: "plain unavailable", err: domain.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: "unavailable"},
		{name: "stopped production", err: appplayback.ErrProductionStopped, wantStatus: http.StatusNotFound, wantCode: "not_found"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			problem := restapi.ProblemForError(fmt.Errorf("wrapped: %w", test.err))
			if int(problem.Status) != test.wantStatus || problem.Code != test.wantCode {
				t.Errorf("ProblemForError(%v) = %d %q, want %d %q", test.err, problem.Status, problem.Code, test.wantStatus, test.wantCode)
			}
		})
	}
}

func TestCreatePlaybackSessionLogsTheStart(t *testing.T) {
	t.Parallel()

	handler, logs := newPlaybackHandler(t, &fakePlaybackService{session: transcodeSession()})

	got := postSession(t, handler, marshalPlaybackRequest(t, fullPlaybackRequest(t)))
	if got.status != http.StatusCreated {
		t.Fatalf("status = %d, want %d (body %s)", got.status, http.StatusCreated, got.body)
	}

	assertLogged(t, logs.String(), `level=INFO msg="playback session started" session=`+testSessionID+
		" file="+testFileID+" mode=transcode reasons=container_not_supported,video_codec_not_supported")

	if strings.Contains(logs.String(), testProfileName) {
		t.Errorf("the client profile reached the log: %s", logs.String())
	}
}

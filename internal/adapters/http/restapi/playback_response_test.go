package restapi_test

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// rawSession decodes a session body into its JSON fields, so tests see null, [] and missing
// fields as they are on the wire.
func rawSession(t *testing.T, body []byte) map[string]json.RawMessage {
	t.Helper()

	return decode[map[string]json.RawMessage](t, body)
}

// TestDirectPlaySessionSerialisesEmptyLists checks the M2-09 note: direct-play plans carry
// nil reasons and warnings, which the contract requires as arrays, never null.
func TestDirectPlaySessionSerialisesEmptyLists(t *testing.T) {
	t.Parallel()

	withoutStreams := directPlaySession()
	withoutStreams.Plan.Streams = nil

	for name, session := range map[string]appplayback.Session{"direct play": directPlaySession(), "no streams": withoutStreams} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: session})

			assertEmptyLists(t, rawSession(t, postSession(t, handler, playbackRequestBody).body))
			assertEmptyLists(t, rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body))
		})
	}
}

// assertEmptyLists checks that reasons and warnings are [] and streams is an array.
func assertEmptyLists(t *testing.T, fields map[string]json.RawMessage) {
	t.Helper()

	for _, list := range []string{"reasons", "warnings"} {
		if string(fields[list]) != "[]" {
			t.Errorf("%s = %s, want []", list, fields[list])
		}
	}

	if streams := string(fields["streams"]); !strings.HasPrefix(streams, "[") {
		t.Errorf("streams = %q, want an array", streams)
	}
}

func TestPlaybackSessionOmitsUnknownOptionalFields(t *testing.T) {
	t.Parallel()

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: directPlaySession()})

	fields := rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body)

	for _, omitted := range []string{"durationSeconds", "failureCode", "sourceBitRate"} {
		if value, present := fields[omitted]; present {
			t.Errorf("%s = %s, want it omitted", omitted, value)
		}
	}

	if string(fields["startPositionSeconds"]) != "0" || string(fields["state"]) != `"ready"` {
		t.Errorf("startPositionSeconds/state = %s/%s, want 0/\"ready\"", fields["startPositionSeconds"], fields["state"])
	}

	if want := `[{"action":"copy","kind":"video","sourceCodec":"h264","sourceIndex":0,"targetCodec":"h264"}]`; string(fields["streams"]) != want {
		t.Errorf("streams = %s, want %s (sizes, channels and encoder omitted)", fields["streams"], want)
	}
}

func TestFailedPlaybackSessionCarriesItsFailureCode(t *testing.T) {
	t.Parallel()

	session := transcodeSession()
	session.State, session.FailureCode = appplayback.StateFailed, "transcode_failed"

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: session})

	fields := rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body)
	if string(fields["state"]) != `"failed"` || string(fields["failureCode"]) != `"transcode_failed"` {
		t.Errorf("state/failureCode = %s/%s, want \"failed\"/\"transcode_failed\"", fields["state"], fields["failureCode"])
	}
}

// TestPlaybackSessionPassesUnknownValuesThrough checks that values a newer app or domain
// adds reach the client unchanged: the contract tells clients to tolerate them.
func TestPlaybackSessionPassesUnknownValuesThrough(t *testing.T) {
	t.Parallel()

	session := transcodeSession()
	session.State = "paused"
	session.Plan.Mode, session.Plan.Delivery = "future_mode", "dash"
	session.Plan.Streams = []playback.StreamDecision{{Kind: media.StreamKind("data"), Action: "rewrap", SourceCodec: "x", TargetCodec: "y"}}
	session.Plan.Reasons = []playback.Reason{{Code: "future_reason", Scope: "chapter"}}
	session.Plan.Warnings = []playback.Warning{{Code: "future_warning", Scope: "attachment"}}

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: session})

	fields := rawSession(t, serve(t, handler, http.MethodGet, sessionPath).body)

	want := map[string]string{
		"state":    `"paused"`,
		"mode":     `"future_mode"`,
		"delivery": `"dash"`,
		"streams":  `[{"action":"rewrap","kind":"data","sourceCodec":"x","sourceIndex":0,"targetCodec":"y"}]`,
		"reasons":  `[{"code":"future_reason","scope":"chapter"}]`,
		"warnings": `[{"code":"future_warning","scope":"attachment"}]`,
	}

	for field, value := range want {
		if string(fields[field]) != value {
			t.Errorf("%s = %s, want %s", field, fields[field], value)
		}
	}
}

// TestCreatePlaybackSessionStopsASessionItCannotDescribe covers a broken ID generator: the
// client gets a 500 and cannot stop the session, so the handler stops it instead of leaving
// its FFmpeg to the idle timeout.
func TestCreatePlaybackSessionStopsASessionItCannotDescribe(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		stopErr error
		wantLog string
	}{
		{name: "stopped", wantLog: `msg="api operation failed" operation=createPlaybackSession`},
		{name: "stop fails", stopErr: errors.New("workspace locked"), wantLog: "stop undescribable session: workspace locked"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			session := transcodeSession()
			session.ID = "not-a-uuid"

			service := &fakePlaybackService{session: session, stopErr: test.stopErr}
			handler, logs := newPlaybackHandler(t, service)

			assertProblem(t, postSession(t, handler, playbackRequestBody), http.StatusInternalServerError, restapi.CodeInternal)

			if stopped := service.stoppedIDs(); !slices.Equal(stopped, []string{"not-a-uuid"}) {
				t.Errorf("Stop calls = %v, want [not-a-uuid]", stopped)
			}

			assertLogged(t, logs.String(), test.wantLog)

			if strings.Contains(logs.String(), "playback session started") {
				t.Errorf("an undescribable session was logged as started: %s", logs.String())
			}
		})
	}
}

// TestCreatePlaybackSessionWithoutBody covers a direct call: the generated router always
// binds a body, but the handler must not trust its callers with a nil pointer.
func TestCreatePlaybackSessionWithoutBody(t *testing.T) {
	t.Parallel()

	logger, _ := newTestLogger()
	service := &fakePlaybackService{session: transcodeSession()}

	api, err := restapi.NewAPI(completeOptions(restapi.Options{Playback: service, Logger: logger}))
	if err != nil {
		t.Fatalf("NewAPI: %v", err)
	}

	result, err := api.CreatePlaybackSession(t.Context(), apigen.CreatePlaybackSessionRequestObject{})
	if !errors.Is(err, domain.ErrInvalid) || result != nil {
		t.Errorf("CreatePlaybackSession(nil body) = %v, %v; want an error wrapping ErrInvalid", result, err)
	}

	if started := service.startRequests(); len(started) != 0 {
		t.Errorf("Start was called without a body: %+v", started)
	}
}

// TestPlaybackSessionContractValues pins the shared conversions (contract_values.go):
// positions and durations in seconds rounded to milliseconds, sizes the contract cannot carry
// dropped, not clamped.
func TestPlaybackSessionContractValues(t *testing.T) {
	t.Parallel()

	session := transcodeSession()
	session.StartPosition = 598*time.Second + 500400*time.Microsecond
	session.Duration = 5423*time.Second + 41700*time.Microsecond
	// Values beyond int32 are dropped too (TestOptionalInt32DropsValuesBeyondInt32).
	session.Plan.Streams[0].TargetWidth = -1

	handler, _ := newPlaybackHandler(t, &fakePlaybackService{session: session})
	got := serve(t, handler, http.MethodGet, sessionPath)
	assertJSONResponse(t, got, http.StatusOK)

	body := decode[apigen.PlaybackSession](t, got.body)
	if body.StartPositionSeconds != 598.5 || body.DurationSeconds == nil || *body.DurationSeconds != 5423.042 {
		t.Errorf("start/duration = %v/%v, want 598.5/5423.042", body.StartPositionSeconds, body.DurationSeconds)
	}

	if width := body.Streams[0].TargetWidth; width != nil {
		t.Errorf("targetWidth = %d, want it dropped", *width)
	}
}

// TestPlaybackSessionWithUnrepresentableSourceIndexIsAServerFault: clients select streams by
// index, so an index the contract cannot carry answers 500 instead of being clamped.
func TestPlaybackSessionWithUnrepresentableSourceIndexIsAServerFault(t *testing.T) {
	t.Parallel()

	for _, index := range append([]int{-1}, beyondInt32()...) {
		session := transcodeSession()
		session.Plan.Streams[0].SourceIndex = index

		handler, logs := newPlaybackHandler(t, &fakePlaybackService{session: session})

		assertProblem(t, serve(t, handler, http.MethodGet, sessionPath), http.StatusInternalServerError, restapi.CodeInternal)
		assertLogged(t, logs.String(), "stream index")
	}
}

func TestPlaybackSessionWithMalformedIDsIsAServerFault(t *testing.T) {
	t.Parallel()

	tests := map[string]func(session *appplayback.Session){
		"session id": func(session *appplayback.Session) { session.ID = "42" },
		"file id":    func(session *appplayback.Session) { session.FileID = "movies/a.mkv" },
	}

	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := transcodeSession()
			change(&session)

			handler, logs := newPlaybackHandler(t, &fakePlaybackService{session: session})

			assertProblem(t, serve(t, handler, http.MethodGet, sessionPath), http.StatusInternalServerError, restapi.CodeInternal)
			assertLogged(t, logs.String(), `msg="api operation failed" operation=getPlaybackSession`)
		})
	}
}

// beyondInt32 returns ints the contract's int32 cannot carry; empty where int is 32 bits
// wide (GOARCH=arm). The conversion goes through a variable so the file compiles there.
func beyondInt32() []int {
	const int64Bits = 64

	if strconv.IntSize < int64Bits {
		return nil
	}

	above := int64(math.MaxInt32) + 1

	return []int{int(above)}
}

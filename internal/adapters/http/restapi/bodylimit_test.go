package restapi_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

const (
	kibibyte = 1 << 10

	uploadPattern = "POST /api/v1/test/upload"
	uploadPath    = "/api/v1/test/upload"
)

// jsonBodyOfSize returns a JSON object of exactly size bytes that every operation accepts:
// unknown members are ignored.
func jsonBodyOfSize(size int) string {
	const prefix, suffix = `{"padding":"`, `"}`

	return prefix + strings.Repeat("x", size-len(prefix)-len(suffix)) + suffix
}

// newBodyRequest builds a POST request; announced false hides the length like a chunked
// upload does, so only reading the body can find out that it is too large.
func newBodyRequest(t *testing.T, target, body string, announced bool) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, target, strings.NewReader(body))
	if !announced {
		request.ContentLength = -1
		request.Body = io.NopCloser(bytes.NewReader([]byte(body)))
	}

	return request
}

// uploadRecorder is a mounted handler that reads the whole body and records whether the
// body limit cut it off.
type uploadRecorder struct {
	calls   atomic.Int32
	cutOffs atomic.Int32
}

func (recorder *uploadRecorder) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	recorder.calls.Add(1)

	_, err := io.ReadAll(request.Body)

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		recorder.cutOffs.Add(1)
		writer.WriteHeader(http.StatusRequestEntityTooLarge)

		return
	}

	writer.WriteHeader(http.StatusNoContent)
}

func TestRequestBodyLimitOnOperations(t *testing.T) {
	t.Parallel()

	handler, _ := newTestHandlerWithOptions(t, restapi.Options{})
	tooLarge := jsonBodyOfSize(restapi.MaxRequestBodyBytes + kibibyte)
	// The decoder rejects this text at its first byte, long before the limit.
	tooLargeText := strings.Repeat("x", restapi.MaxRequestBodyBytes+kibibyte)

	tests := []struct {
		name       string
		body       string
		announced  bool
		wantStatus int
		wantCode   string
	}{
		{name: "announced 65 KiB", body: tooLarge, announced: true, wantStatus: http.StatusRequestEntityTooLarge, wantCode: restapi.CodePayloadTooLarge},
		{name: "unannounced 65 KiB", body: tooLarge, wantStatus: http.StatusRequestEntityTooLarge, wantCode: restapi.CodePayloadTooLarge},
		{name: "announced 65 KiB of non-JSON", body: tooLargeText, announced: true, wantStatus: http.StatusRequestEntityTooLarge, wantCode: restapi.CodePayloadTooLarge},
		{name: "unannounced 65 KiB of non-JSON", body: tooLargeText, wantStatus: http.StatusRequestEntityTooLarge, wantCode: restapi.CodePayloadTooLarge},
		{name: "unannounced small non-JSON", body: "fileId=1", wantStatus: http.StatusBadRequest, wantCode: restapi.CodeBadRequest},
		{name: "unannounced 64 KiB of non-JSON", body: tooLargeText[:restapi.MaxRequestBodyBytes], wantStatus: http.StatusBadRequest, wantCode: restapi.CodeBadRequest},
		// The stub answers 501 only after the generated code decoded the body.
		{name: "exactly 64 KiB", body: jsonBodyOfSize(restapi.MaxRequestBodyBytes), announced: true, wantStatus: http.StatusNotImplemented, wantCode: notImplementedCode},
		{name: "unannounced 64 KiB", body: jsonBodyOfSize(restapi.MaxRequestBodyBytes), wantStatus: http.StatusNotImplemented, wantCode: notImplementedCode},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := serveRequest(t, handler, newBodyRequest(t, sessionsPath, test.body, test.announced))
			assertProblem(t, got, test.wantStatus, test.wantCode)
		})
	}
}

func TestRequestBodyLimitOnMounts(t *testing.T) {
	t.Parallel()

	tooLarge := strings.Repeat("x", restapi.MaxRequestBodyBytes+kibibyte)

	t.Run("announced 65 KiB never reaches the mount", func(t *testing.T) {
		t.Parallel()

		recorder := &uploadRecorder{}
		handler, _ := newTestHandlerWithOptions(t, restapi.Options{Mounts: []restapi.Mount{{Pattern: uploadPattern, Handler: recorder}}})

		got := serveRequest(t, handler, newBodyRequest(t, uploadPath, tooLarge, true))
		assertProblem(t, got, http.StatusRequestEntityTooLarge, restapi.CodePayloadTooLarge)

		if calls := recorder.calls.Load(); calls != 0 {
			t.Errorf("mount called %d times, want 0", calls)
		}
	})

	t.Run("unannounced 65 KiB is cut off while read", func(t *testing.T) {
		t.Parallel()

		recorder := &uploadRecorder{}
		handler, _ := newTestHandlerWithOptions(t, restapi.Options{Mounts: []restapi.Mount{{Pattern: uploadPattern, Handler: recorder}}})

		serveRequest(t, handler, newBodyRequest(t, uploadPath, tooLarge, false))

		if cutOffs := recorder.cutOffs.Load(); cutOffs != 1 {
			t.Errorf("mount saw %d cut-off bodies, want 1", cutOffs)
		}
	})
}

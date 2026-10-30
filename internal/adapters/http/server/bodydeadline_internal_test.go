package server

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/webui"
	"github.com/magpeek/mivo/internal/domain/system"
)

const (
	// shortBodyReadTimeout replaces BodyReadTimeout, so these tests take well under a
	// second each.
	shortBodyReadTimeout = 200 * time.Millisecond

	// outlastTheDeadline is how long the long-running handlers keep going: well past the
	// deadline, so net/http's background read would have hit it.
	outlastTheDeadline = 3 * shortBodyReadTimeout

	// streamChunkInterval spaces the chunks of the streaming handler.
	streamChunkInterval = 50 * time.Millisecond

	// hangBound fails a test whose request hangs instead of being cut off.
	hangBound = 10 * time.Second

	// silentBodyHeader announces 100 KiB, above the API's body limit, and sends nothing.
	silentBodyHeader = "Content-Length: 102400\r\n\r\n"

	// silentChunkedHeader starts a chunked body and sends no chunk.
	silentChunkedHeader = "Transfer-Encoding: chunked\r\n\r\n"
)

// stubSystemService answers the system endpoints with fixed values.
type stubSystemService struct{}

func (stubSystemService) Health(context.Context) system.Health {
	return system.NewHealth(system.PassedCheck(system.HealthCheckDatabase))
}

func (stubSystemService) Info(context.Context) (system.Info, error) {
	return system.Info{}, nil
}

// newDeadlineTestServer serves the full chain with the real API and web UI and the short
// body read timeout over a real connection: only net/http itself applies read deadlines,
// discards unread bodies and runs the background read.
func newDeadlineTestServer(t *testing.T, withFallback bool, mounts ...restapi.Mount) *httptest.Server {
	t.Helper()

	logger, _ := newTestLogger()

	api, err := restapi.NewHandler(restapi.Options{System: stubSystemService{}, Logger: logger, Mounts: mounts})
	if err != nil {
		t.Fatalf("restapi.NewHandler: %v", err)
	}

	options := Options{Logger: logger, APIHandler: api}

	if withFallback {
		ui, err := webui.NewHandler(fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}, logger)
		if err != nil {
			t.Fatalf("webui.NewHandler: %v", err)
		}

		options.Fallback = ui
	}

	settings, err := newChainSettings(options)
	if err != nil {
		t.Fatalf("newChainSettings: %v", err)
	}

	settings.bodyReadTimeout = shortBodyReadTimeout

	server := httptest.NewServer(newHandler(options, settings))
	t.Cleanup(server.Close)

	return server
}

// TestBodyReadDeadlineEndsSilentBodies sends request heads whose body never arrives (or
// stops after one chunk). Every one must be answered, or its connection closed, soon after
// the deadline, also when the handler never reads the body and net/http discards it.
func TestBodyReadDeadlineEndsSilentBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		withFallback bool
		head         string
		wantStatus   int
	}{
		{name: "api early 413", head: "POST /api/v1/playback/sessions HTTP/1.1\r\n" + silentBodyHeader, wantStatus: http.StatusRequestEntityTooLarge},
		{name: "api chunked", head: "POST /api/v1/playback/sessions HTTP/1.1\r\n" + silentChunkedHeader, wantStatus: http.StatusBadRequest},
		{
			name: "api trickled chunk", head: "POST /api/v1/playback/sessions HTTP/1.1\r\n" + silentChunkedHeader + "a\r\n{\"fileId\":\r\n",
			wantStatus: http.StatusBadRequest,
		},
		{name: "unknown api route", head: "POST /api/v1/nope HTTP/1.1\r\n" + silentChunkedHeader, wantStatus: http.StatusNotFound},
		{name: "webui post", withFallback: true, head: "POST / HTTP/1.1\r\n" + silentChunkedHeader, wantStatus: http.StatusMethodNotAllowed},
		{name: "webui announced post", withFallback: true, head: "POST /library HTTP/1.1\r\n" + silentBodyHeader, wantStatus: http.StatusMethodNotAllowed},
		{name: "get with a body", withFallback: true, head: "GET / HTTP/1.1\r\n" + silentChunkedHeader, wantStatus: http.StatusOK},
		{name: "nil fallback", head: "POST /library HTTP/1.1\r\n" + silentBodyHeader, wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := newDeadlineTestServer(t, test.withFallback)
			started := time.Now()

			if status := sendRaw(t, server, test.head); status != test.wantStatus {
				t.Errorf("status = %d, want %d", status, test.wantStatus)
			}

			// The answer waits for the body; only the deadline can have released it.
			if elapsed := time.Since(started); elapsed < shortBodyReadTimeout {
				t.Errorf("answered after %v, before the deadline of %v", elapsed, shortBodyReadTimeout)
			}
		})
	}
}

// TestBodyReadDeadlineSparesBodilessStreams: a GET has no body, so it gets no deadline.
// Otherwise net/http's background read would hit it and cancel the long response.
func TestBodyReadDeadlineSparesBodilessStreams(t *testing.T) {
	t.Parallel()

	contextErrors := make(chan error, 1)
	stream := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		contextErrors <- streamUntil(request.Context(), writer, time.Now().Add(outlastTheDeadline))
	})
	server := newDeadlineTestServer(t, false, restapi.Mount{Pattern: "GET /api/v1/test/stream", Handler: stream})

	body := get(t, server, "/api/v1/test/stream")

	if err := <-contextErrors; err != nil {
		t.Errorf("request context ended mid-stream: %v", err)
	}

	if minimum := int(outlastTheDeadline / streamChunkInterval); len(body) < minimum {
		t.Errorf("received %d chunks, want at least %d", len(body), minimum)
	}
}

// TestBodyReadDeadlineEndsWithTheBody: once the body is read to its end, a handler may take
// longer than the deadline without losing its request context. net/http clears the read
// deadline when it starts its background read; this guards that behaviour on toolchain
// updates, because limitBodyReadTime relies on it.
func TestBodyReadDeadlineEndsWithTheBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "announced", body: "Content-Length: 7\r\n\r\n{\"a\":1}"},
		{name: "chunked", body: "Transfer-Encoding: chunked\r\n\r\n7\r\n{\"a\":1}\r\n0\r\n\r\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			contextErrors := make(chan error, 1)
			slow := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, err := io.ReadAll(request.Body)
				if err == nil {
					err = waitOrCancel(request.Context(), outlastTheDeadline)
				}

				contextErrors <- err

				writer.WriteHeader(http.StatusNoContent)
			})
			server := newDeadlineTestServer(t, false, restapi.Mount{Pattern: "POST /api/v1/test/slow", Handler: slow})

			status := sendRaw(t, server, "POST /api/v1/test/slow HTTP/1.1\r\n"+test.body)

			if err := <-contextErrors; err != nil {
				t.Errorf("handler after the body: %v, want its context alive", err)
			}

			if status != http.StatusNoContent {
				t.Errorf("status = %d, want 204", status)
			}
		})
	}
}

// sendRaw writes head (request line, headers without Host, blank line and any body bytes)
// on a fresh connection and returns the response status. A raw connection is needed because
// http.Client never sends a body head without its body.
func sendRaw(t *testing.T, server *httptest.Server, head string) int {
	t.Helper()

	var dialer net.Dialer

	connection, err := dialer.DialContext(t.Context(), "tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	t.Cleanup(func() { _ = connection.Close() })

	if err := connection.SetDeadline(time.Now().Add(hangBound)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}

	requestLine, headers, _ := strings.Cut(head, "\r\n")
	if _, err := io.WriteString(connection, requestLine+"\r\nHost: test\r\n"+headers); err != nil {
		t.Fatalf("write request: %v", err)
	}

	response, err := http.ReadResponse(bufio.NewReader(connection), nil)

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		t.Fatalf("no answer within %v: the body was not cut off", hangBound)
	}

	if err != nil {
		t.Fatalf("no answer: %v", err)
	}

	_ = response.Body.Close() // only the status is read

	return response.StatusCode
}

// streamUntil writes and flushes one byte per streamChunkInterval until end. It returns the
// request context's error if the context ends first.
func streamUntil(ctx context.Context, writer http.ResponseWriter, end time.Time) error {
	controller := http.NewResponseController(writer)

	for time.Now().Before(end) {
		if _, err := writer.Write([]byte("x")); err != nil {
			return err
		}

		if err := controller.Flush(); err != nil {
			return err
		}

		if err := waitOrCancel(ctx, streamChunkInterval); err != nil {
			return err
		}
	}

	return nil
}

// waitOrCancel waits for duration and returns ctx's error if ctx ends first.
func waitOrCancel(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return errors.Join(errors.New("request context ended"), ctx.Err())
	}
}

func get(t *testing.T, server *httptest.Server, path string) []byte {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}

	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return body
}

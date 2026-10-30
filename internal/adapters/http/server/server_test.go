package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/adapters/http/server"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/system"
)

// runTimeout bounds how long a test waits for Run to return after cancellation; it is
// above server.ShutdownTimeout only to tell a hang from a slow shutdown.
const runTimeout = server.ShutdownTimeout + 5*time.Second

type fakeSystemService struct {
	info system.Info
}

func (fakeSystemService) Health(context.Context) system.Health {
	return system.NewHealth(system.PassedCheck(system.HealthCheckDatabase))
}

func (service fakeSystemService) Info(context.Context) (system.Info, error) {
	return service.info, nil
}

func discardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func newAPIHandler(t *testing.T) http.Handler {
	t.Helper()

	serverID, err := system.ParseServerID("0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e")
	if err != nil {
		t.Fatalf("ParseServerID: %v", err)
	}

	service := fakeSystemService{info: system.Info{
		ProductName: "Mivo", Version: "1.0.0", ServerID: serverID, APIVersion: system.APIVersion,
	}}

	handler, err := restapi.NewHandler(restapi.Options{System: service, Logger: discardLogger()})
	if err != nil {
		t.Fatalf("restapi.NewHandler: %v", err)
	}

	return handler
}

// startServer runs a server on listener and returns the cancel function and Run's result.
func startServer(t *testing.T, options server.Options) (context.CancelFunc, <-chan error) {
	t.Helper()

	httpServer, err := server.New(options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	result := make(chan error, 1)
	go func() { result <- httpServer.Run(ctx) }()

	return cancel, result
}

func waitForRun(t *testing.T, result <-chan error) error {
	t.Helper()

	select {
	case err := <-result:
		return err
	case <-time.After(runTimeout):
		t.Fatal("Run did not return after the context was cancelled")

		return nil
	}
}

func TestRunServesAPIAndShutsDownOnCancel(t *testing.T) {
	t.Parallel()

	listener := openListener(t)

	cancel, result := startServer(t, server.Options{
		Listener: listener, Logger: discardLogger(), APIHandler: newAPIHandler(t),
	})

	baseURL := "http://" + listener.Addr().String()
	assertGet(t, baseURL+"/api/v1/health", http.StatusOK, `"status":"ok"`)
	assertGet(t, baseURL+"/api/v1/system/info", http.StatusOK, `"serverId":"0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e"`)
	assertGet(t, baseURL+"/api/v1/nope", http.StatusNotFound, `"code":"not_found"`)

	cancel()

	if err := waitForRun(t, result); err != nil {
		t.Errorf("Run = %v, want nil after cancellation", err)
	}
}

// fallbackStatus marks answers of the counting fallback, so a test can tell them apart.
const fallbackStatus = http.StatusTeapot

func TestRunNeverHandsAPIPathsToTheFallback(t *testing.T) {
	t.Parallel()

	listener := openListener(t)

	var fallbackCalls atomic.Int32

	fallback := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fallbackCalls.Add(1)
		writer.WriteHeader(fallbackStatus)
	})

	cancel, result := startServer(t, server.Options{
		Listener: listener, Logger: discardLogger(), APIHandler: newAPIHandler(t), Fallback: fallback,
	})

	baseURL := "http://" + listener.Addr().String()

	tests := []struct {
		path       string
		wantStatus int
	}{
		{path: "/api/v1/health", wantStatus: http.StatusOK},
		{path: "/api/v1/x", wantStatus: http.StatusNotFound},
		{path: "/api/", wantStatus: http.StatusNotFound},
		// ServeMux redirects the subtree root without its slash into the API.
		{path: "/api", wantStatus: http.StatusTemporaryRedirect},
		// ServeMux matches the escaped path, so these two miss the /api/ pattern.
		{path: "/api%2Fv1%2Fx", wantStatus: http.StatusNotFound},
		{path: "/%2e%2e/api/v1/x", wantStatus: http.StatusNotFound},
	}

	for _, test := range tests {
		if status := statusOf(t, baseURL+test.path); status != test.wantStatus {
			t.Errorf("GET %s = %d, want %d", test.path, status, test.wantStatus)
		}
	}

	if calls := fallbackCalls.Load(); calls != 0 {
		t.Errorf("fallback calls for API paths = %d, want 0", calls)
	}

	// Control: the fallback is wired and counts, so the zero above is meaningful.
	if status := statusOf(t, baseURL+"/library/x"); status != fallbackStatus || fallbackCalls.Load() != 1 {
		t.Errorf("GET /library/x = %d with %d fallback calls, want %d with 1", status, fallbackCalls.Load(), fallbackStatus)
	}

	cancel()

	if err := waitForRun(t, result); err != nil {
		t.Errorf("Run = %v, want nil after cancellation", err)
	}
}

// statusOf requests url without following redirects and returns the status.
func statusOf(t *testing.T, url string) int {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	if err := response.Body.Close(); err != nil {
		t.Errorf("close body: %v", err)
	}

	return response.StatusCode
}

func openListener(t *testing.T) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	return listener
}

func assertGet(t *testing.T, url string, wantStatus int, wantBody string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	if response.StatusCode != wantStatus || !strings.Contains(string(body), wantBody) {
		t.Errorf("GET %s = %d %s, want %d containing %s", url, response.StatusCode, body, wantStatus, wantBody)
	}
}

func TestRunListensOnAddress(t *testing.T) {
	t.Parallel()

	cancel, result := startServer(t, server.Options{
		Address: "127.0.0.1:0", Logger: discardLogger(), APIHandler: newAPIHandler(t),
	})

	cancel()

	if err := waitForRun(t, result); err != nil {
		t.Errorf("Run = %v, want nil after cancellation", err)
	}
}

func TestRunReportsListenFailure(t *testing.T) {
	t.Parallel()

	httpServer, err := server.New(server.Options{
		Address: "127.0.0.1:not-a-port", Logger: discardLogger(), APIHandler: newAPIHandler(t),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = httpServer.Run(t.Context())
	if err == nil || !strings.Contains(err.Error(), "listen on 127.0.0.1:not-a-port") {
		t.Errorf("Run = %v, want a listen error", err)
	}
}

func TestRunReportsServeFailure(t *testing.T) {
	t.Parallel()

	// A closed listener makes Serve fail at once, before the context ends.
	_, result := startServer(t, server.Options{
		Listener: closedListener(t), Logger: discardLogger(), APIHandler: newAPIHandler(t),
	})

	if err := waitForRun(t, result); err == nil || !strings.Contains(err.Error(), "serve http") {
		t.Errorf("Run = %v, want a serve error", err)
	}
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	t.Parallel()

	apiHandler := http.NotFoundHandler()
	listener := closedListener(t)

	tests := []struct {
		name    string
		options server.Options
	}{
		{name: "neither address nor listener", options: server.Options{Logger: discardLogger(), APIHandler: apiHandler}},
		{
			name:    "both address and listener",
			options: server.Options{Address: ":0", Listener: listener, Logger: discardLogger(), APIHandler: apiHandler},
		},
		{name: "missing logger", options: server.Options{Address: ":0", APIHandler: apiHandler}},
		{name: "missing api handler", options: server.Options{Address: ":0", Logger: discardLogger()}},
		{
			name:    "empty quiet pattern",
			options: server.Options{Address: ":0", Logger: discardLogger(), APIHandler: apiHandler, QuietPatterns: []string{""}},
		},
		{
			name:    "conflicting quiet patterns",
			options: server.Options{Address: ":0", Logger: discardLogger(), APIHandler: apiHandler, QuietPatterns: []string{"GET /a/{x}", "GET /a/{y}"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			httpServer, err := server.New(test.options)
			if !errors.Is(err, domain.ErrInvalid) || httpServer != nil {
				t.Errorf("New = %v, %v, want nil server and ErrInvalid", httpServer, err)
			}
		})
	}
}

func closedListener(t *testing.T) net.Listener {
	t.Helper()

	listener := openListener(t)

	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	return listener
}

package healthprobe_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/healthprobe"
)

// shortTimeout keeps the timeout test fast; the handler blocks far longer.
const shortTimeout = 50 * time.Millisecond

// serverAddress starts a server that answers the health path with status and returns its
// host:port.
func serverAddress(t *testing.T, status int) string {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != healthprobe.HealthPath {
			writer.WriteHeader(http.StatusNotFound)

			return
		}

		writer.WriteHeader(status)
	}))
	t.Cleanup(server.Close)

	return server.Listener.Addr().String()
}

func TestCheckAcceptsOnlyOK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        int
		wantUnhealthy bool
	}{
		{name: "200 is healthy", status: http.StatusOK},
		{name: "503 is unhealthy", status: http.StatusServiceUnavailable, wantUnhealthy: true},
		{name: "204 is unhealthy", status: http.StatusNoContent, wantUnhealthy: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := healthprobe.Check(t.Context(), healthprobe.Options{Address: serverAddress(t, tt.status)})

			if tt.wantUnhealthy {
				if !errors.Is(err, healthprobe.ErrUnhealthy) {
					t.Fatalf("Check() = %v, want ErrUnhealthy", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("Check() = %v, want nil", err)
			}
		})
	}
}

func TestCheckNamesStatusAndURL(t *testing.T) {
	t.Parallel()

	address := serverAddress(t, http.StatusServiceUnavailable)

	err := healthprobe.Check(t.Context(), healthprobe.Options{Address: address})
	if err == nil {
		t.Fatal("Check() = nil, want an error")
	}

	for _, want := range []string{"http://" + address + healthprobe.HealthPath, "status 503"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func TestCheckDoesNotFollowRedirect(t *testing.T) {
	t.Parallel()

	const healthyPath = "/healthy"

	mux := http.NewServeMux()
	mux.HandleFunc(healthprobe.HealthPath, func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, healthyPath, http.StatusFound)
	})
	mux.HandleFunc(healthyPath, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	// The redirect target answers 200, so only a probe that stops at the redirect fails here.
	err := healthprobe.Check(t.Context(), healthprobe.Options{Address: server.Listener.Addr().String()})

	if !errors.Is(err, healthprobe.ErrUnhealthy) {
		t.Fatalf("Check() = %v, want ErrUnhealthy", err)
	}

	if want := "status 302"; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err, want)
	}
}

func TestCheckTimesOut(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// Blocks until the test ends, far beyond the probe timeout.
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	started := time.Now()
	err := healthprobe.Check(t.Context(), healthprobe.Options{
		Address: server.Listener.Addr().String(),
		Timeout: shortTimeout,
	})

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Check() = %v, want context.DeadlineExceeded", err)
	}

	if elapsed := time.Since(started); elapsed > healthprobe.DefaultTimeout {
		t.Errorf("Check() took %v, want about %v", elapsed, shortTimeout)
	}
}

func TestCheckReportsConnectionRefused(t *testing.T) {
	t.Parallel()

	err := healthprobe.Check(t.Context(), healthprobe.Options{Address: closedPortAddress(t)})

	var opError *net.OpError
	if !errors.As(err, &opError) || opError.Op != "dial" {
		t.Fatalf("Check() = %v, want a dial error", err)
	}

	// The errno differs per OS (ECONNREFUSED, WSAECONNREFUSED), the word in its text does not.
	if !strings.Contains(opError.Err.Error(), "refused") {
		t.Errorf("Check() = %v, want a refused connection", err)
	}

	if errors.Is(err, healthprobe.ErrUnhealthy) {
		t.Errorf("Check() = %v, a refused connection is no answer and must not wrap ErrUnhealthy", err)
	}
}

func TestCheckRejectsInvalidAddress(t *testing.T) {
	t.Parallel()

	err := healthprobe.Check(t.Context(), healthprobe.Options{Address: "bad host:80"})
	if err == nil {
		t.Fatal("Check() = nil, want an error")
	}
}

func TestCheckHonoursCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := healthprobe.Check(ctx, healthprobe.Options{Address: serverAddress(t, http.StatusOK)})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Check() = %v, want context.Canceled", err)
	}
}

// closedPortAddress returns a loopback address on which nothing listens.
func closedPortAddress(t *testing.T) string {
	t.Helper()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	return address
}

func TestAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "empty host probes loopback", host: "", port: 8460, want: "127.0.0.1:8460"},
		{name: "IPv4 any probes loopback", host: "0.0.0.0", port: 8460, want: "127.0.0.1:8460"},
		{name: "IPv6 any probes loopback", host: "::", port: 9000, want: "127.0.0.1:9000"},
		{name: "specific IPv4 is kept", host: "192.0.2.10", port: 8460, want: "192.0.2.10:8460"},
		{name: "specific IPv6 is bracketed", host: "::1", port: 8460, want: "[::1]:8460"},
		{name: "host name is kept", host: "localhost", port: 8460, want: "localhost:8460"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := healthprobe.Address(tt.host, tt.port); got != tt.want {
				t.Errorf("Address(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

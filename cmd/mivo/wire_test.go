package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	httpserver "github.com/magpeek/mivo/internal/adapters/http/server"
	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/instancelock"
)

// Environment variable names of the milestone-2 settings the wiring tests set.
const (
	envLibraries     = brand.EnvPrefix + "_LIBRARIES"
	envToolsFFmpeg   = brand.EnvPrefix + "_TOOLS_FFMPEG"
	envToolsFFprobe  = brand.EnvPrefix + "_TOOLS_FFPROBE"
	envServerLANOnly = brand.EnvPrefix + "_SERVER_LAN_ONLY"
)

const (
	// scanTimeout bounds waiting for the startup scan of a test library.
	scanTimeout = 60 * time.Second
	// pollInterval spaces the status requests while a test waits for a scan.
	pollInterval = 50 * time.Millisecond
	// publicPeerAddress is a documentation address (TEST-NET-3) the LAN guard must refuse.
	publicPeerAddress = "203.0.113.7"
	publicPeerPort    = 40000
)

func TestRunRefusesATranscodePathWithATemplateMarker(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	environment[envPathsTranscode] = filepath.Join(t.TempDir(), "transcode%d")

	assertStartFails(t, environment, `contains "%"`)
}

// TestRunRefusesASessionsEntryThatIsNoDirectory: a file (or a link) at <transcode>/sessions
// would make every session fail, so the start fails and names the cause.
func TestRunRefusesASessionsEntryThatIsNoDirectory(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	transcode := environment[envPathsTranscode]

	if err := os.MkdirAll(transcode, 0o750); err != nil {
		t.Fatalf("create transcode directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(transcode, "sessions"), nil, 0o600); err != nil {
		t.Fatalf("write sessions file: %v", err)
	}

	assertStartFails(t, environment, "sessions entry is not a plain directory")
}

// TestRunLocksBeforeOpeningTheDatabase proves the order "instance lock before database"
// (M2-13): the database path is a directory, so opening it would fail with "start database",
// but the held lock must refuse the start first.
func TestRunLocksBeforeOpeningTheDatabase(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	environment[envLogLevel] = "debug"
	dataDir := environment[envPathsData]

	if err := os.MkdirAll(filepath.Join(dataDir, databaseFileName), 0o750); err != nil {
		t.Fatalf("create database directory: %v", err)
	}

	lock, err := instancelock.Acquire(dataDir)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	t.Cleanup(func() { _ = lock.Release() })

	stderr := assertStartFails(t, environment, "another server is already using "+dataDir+": ")

	for _, unwanted := range []string{"start database", "starting " + brand.DisplayName} {
		if strings.Contains(stderr, unwanted) {
			t.Errorf("stderr = %q, want no %q before the lock is held", stderr, unwanted)
		}
	}

	if strings.Count(stderr, dataDir) != 1 {
		t.Errorf("stderr = %q, want the data directory named once", stderr)
	}
}

// TestRunGuardsTheLANByDefault wires the LAN-only guard (OD-10): with the default
// configuration a public peer gets 403 except for the health check; lan_only=false lets it in.
func TestRunGuardsTheLANByDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		lanOnly    string
		wantStatus int
	}{
		{name: "default", wantStatus: http.StatusForbidden},
		{name: "lan_only=false", lanOnly: "false", wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := isolatedEnvironment(t)
			if test.lanOnly != "" {
				environment[envServerLANOnly] = test.lanOnly
			}

			server := startTestServer(t, environment, runHooks{wrapListener: asPublicPeers})

			if status, _ := fetchStatus(t, server.baseURL+"/api/v1/libraries"); status != test.wantStatus {
				t.Errorf("GET /api/v1/libraries from a public peer = %d, want %d", status, test.wantStatus)
			}

			assertHealthy(t, server.baseURL)
			server.stop(t)
		})
	}
}

// TestRunLogsSessionStopsAtInfo: only the stream delivery routes are quiet, never the session
// resource, so a DELETE of a session stays in the info log (M2-08).
func TestRunLogsSessionStopsAtInfo(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	environment[envLogLevel] = "info"
	server := startTestServer(t, environment, runHooks{})

	const sessionPath = "/api/v1/playback/sessions/5d4c3b2a-1f0e-4d9c-8b7a-6f5e4d3c2b1a"

	if status := sendRequest(t, http.MethodDelete, server.baseURL+sessionPath); status != http.StatusNoContent {
		t.Errorf("DELETE %s = %d, want %d", sessionPath, status, http.StatusNoContent)
	}

	server.stop(t)
	assertLogLine(t, server.stderr.String(), "level=INFO", `msg="http request"`, "method=DELETE", "path="+sessionPath, "status=204")
}

// TestRunStartsWithoutFFmpegTools: missing tools degrade the server instead of stopping it;
// the startup scan reports prober_unavailable and the log says why.
func TestRunStartsWithoutFFmpegTools(t *testing.T) {
	t.Parallel()

	libraryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(libraryRoot, "movie.mkv"), []byte("not a video"), 0o600); err != nil {
		t.Fatalf("write library file: %v", err)
	}

	environment := isolatedEnvironment(t)
	environment[envLibraries] = libraryRoot
	environment[envToolsFFmpeg] = missingToolPath(t, "ffmpeg")
	environment[envToolsFFprobe] = missingToolPath(t, "ffprobe")

	server := startTestServer(t, environment, runHooks{})

	scanned := awaitScannedLibrary(t, server.baseURL)
	if scanned.Scan.ErrorCode == nil || *scanned.Scan.ErrorCode != "prober_unavailable" {
		t.Errorf("scan = %+v, want error code prober_unavailable", scanned.Scan)
	}

	server.stop(t)

	logs := server.stderr.String()
	assertLogLine(t, logs, "level=WARN", "ffprobe not found")
	assertLogLine(t, logs, "level=WARN", "ffmpeg not found: only direct play available")
	assertLogLine(t, logs, "level=WARN", `msg="library scan finished"`, "code=prober_unavailable")
}

// testServer is a server started by startTestServer.
type testServer struct {
	baseURL   string
	stderr    *syncBuffer
	exitCodes <-chan int
	cancel    context.CancelFunc
}

// startTestServer runs the server until stop or the end of the test.
func startTestServer(t *testing.T, environment map[string]string, hooks runHooks) testServer {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	env, _, stderr := newTestEnv(environment)
	addresses := make(chan net.Addr, 1)
	exitCodes := make(chan int, 1)
	hooks.onListening = func(address net.Addr) { addresses <- address }

	go func() { exitCodes <- run(ctx, nil, env, hooks) }()

	address := awaitAddress(t, addresses, exitCodes, stderr)

	return testServer{baseURL: "http://" + address.String(), stderr: stderr, exitCodes: exitCodes, cancel: cancel}
}

// stop shuts the server down like Ctrl+C and expects a clean exit.
func (server testServer) stop(t *testing.T) {
	t.Helper()

	server.cancel()
	assertCleanExit(t, server.exitCodes, server.stderr)
}

// asPublicPeers presents every connection to the server as coming from a public address.
func asPublicPeers(listener net.Listener) net.Listener {
	return publicPeerListener{Listener: listener}
}

type publicPeerListener struct {
	net.Listener
}

func (listener publicPeerListener) Accept() (net.Conn, error) {
	conn, err := listener.Listener.Accept()
	if err != nil {
		return nil, err //nolint:wrapcheck // net/http inspects the listener's errors
	}

	return publicPeerConn{Conn: conn}, nil
}

type publicPeerConn struct {
	net.Conn
}

func (publicPeerConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP(publicPeerAddress), Port: publicPeerPort}
}

// awaitScannedLibrary waits until the only library has finished its startup scan.
func awaitScannedLibrary(t *testing.T, baseURL string) apigen.Library {
	t.Helper()

	deadline := time.Now().Add(scanTimeout)

	for {
		var list struct {
			Libraries []apigen.Library `json:"libraries"`
		}

		getJSON(t, baseURL+"/api/v1/libraries", &list)

		if len(list.Libraries) != 1 {
			t.Fatalf("libraries = %+v, want exactly the configured one", list.Libraries)
		}

		scanned := list.Libraries[0]
		if scanned.Scan.State != apigen.ScanStateRunning && scanned.Scan.FinishedAt != nil {
			return scanned
		}

		if time.Now().After(deadline) {
			t.Fatalf("library scan did not finish within %s: %+v", scanTimeout, scanned.Scan)
		}

		time.Sleep(pollInterval)
	}
}

// sendRequest sends a request without a body and returns the status.
func sendRequest(t *testing.T, method, url string) int {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}

	_ = response.Body.Close()

	return response.StatusCode
}

// assertLogLine checks that one line of logs contains every part.
func assertLogLine(t *testing.T, logs string, parts ...string) {
	t.Helper()

	for line := range strings.SplitSeq(logs, "\n") {
		if containsAll(line, parts) {
			return
		}
	}

	t.Errorf("no log line contains all of %q:\n%s", parts, logs)
}

func containsAll(text string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(text, part) {
			return false
		}
	}

	return true
}

// TestShutdownStepsFitTheBudget keeps the bounded shutdown steps inside the 20 s that Docker
// (--stop-timeout 20) and the Synology stop script grant before SIGKILL.
func TestShutdownStepsFitTheBudget(t *testing.T) {
	t.Parallel()

	const killAfter = 20 * time.Second

	steps := httpserver.ShutdownTimeout + playbackCloseTimeout + scanCloseTimeout + databaseCloseReserve
	if steps > shutdownBudget || shutdownBudget >= killAfter {
		t.Errorf("shutdown steps take up to %s, budget %s, killed after %s; want steps <= budget < kill",
			steps, shutdownBudget, killAfter)
	}
}

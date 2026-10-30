package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/config"
	"github.com/magpeek/mivo/internal/platform/paths"
)

// startTimeout bounds how long a test waits for the server to listen or to exit. Migrating
// a new database under the race detector takes a few seconds on slow machines.
const startTimeout = 30 * time.Second

// listeningMessage is the log message of the HTTP server once its listener is bound.
const listeningMessage = "http server listening"

// Environment variable names of the ADR 0018 settings the tests set.
const (
	envServerHost     = brand.EnvPrefix + "_SERVER_HOST"
	envServerPort     = brand.EnvPrefix + "_SERVER_PORT"
	envLogLevel       = brand.EnvPrefix + "_LOG_LEVEL"
	envPathsData      = brand.EnvPrefix + "_PATHS_DATA"
	envPathsCache     = brand.EnvPrefix + "_PATHS_CACHE"
	envPathsTranscode = brand.EnvPrefix + "_PATHS_TRANSCODE"
)

func TestRunServesSystemEndpointsAndKeepsServerIDAcrossRestarts(t *testing.T) {
	t.Parallel()

	configDir := t.TempDir()
	// Relative paths resolve against the config directory. The file asks for the default
	// port; the environment must win and choose an OS-assigned one.
	writeConfigFile(t, configDir, "server:\n  host: 127.0.0.1\n  port: 8460\n"+
		"paths:\n  data: data\n  cache: cache\n  transcode: transcode\n")

	environment := map[string]string{envServerPort: "0"}

	firstID := runAndQueryServer(t, []string{"--config-dir", configDir}, environment)

	for _, dir := range []string{"data", "cache", "transcode"} {
		assertExists(t, filepath.Join(configDir, dir))
	}

	assertExists(t, filepath.Join(configDir, "data", databaseFileName))

	// The second start selects the same directory through the environment variable.
	environment[paths.ConfigDirEnv()] = configDir

	if secondID := runAndQueryServer(t, nil, environment); secondID != firstID {
		t.Errorf("server id after restart = %q, want %q", secondID, firstID)
	}

	// Windows refuses to delete open files, so this fails while the database is still open.
	if err := os.RemoveAll(filepath.Join(configDir, "data")); err != nil {
		t.Errorf("remove data directory after the run: %v", err)
	}
}

func TestRunRoutesAPIAndWebUI(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	env, _, stderr := newTestEnv(isolatedEnvironment(t))
	addresses := make(chan net.Addr, 1)
	exitCodes := make(chan int, 1)
	hooks := runHooks{onListening: func(address net.Addr) { addresses <- address }}

	go func() { exitCodes <- run(ctx, nil, env, hooks) }()

	baseURL := "http://" + awaitAddress(t, addresses, exitCodes, stderr).String()

	tests := []struct {
		name            string
		path            string
		wantStatus      int
		wantContentType string
	}{
		// Both builds answer the same way: the placeholder or the embedded app (ADR 0020).
		{name: "web root", path: "/", wantStatus: http.StatusOK, wantContentType: "text/html; charset=utf-8"},
		{name: "client route", path: "/library/x", wantStatus: http.StatusOK, wantContentType: "text/html; charset=utf-8"},
		{name: "missing web file", path: "/missing.js", wantStatus: http.StatusNotFound, wantContentType: "text/plain; charset=utf-8"},
		{name: "api route", path: "/api/v1/health", wantStatus: http.StatusOK, wantContentType: "application/json"},
		{name: "unknown api route", path: "/api/v1/does-not-exist", wantStatus: http.StatusNotFound, wantContentType: "application/problem+json"},
		{name: "unknown api route without extension", path: "/api/library/x", wantStatus: http.StatusNotFound, wantContentType: "application/problem+json"},
		{name: "api route with encoded slashes", path: "/api%2Fv1%2Fx", wantStatus: http.StatusNotFound, wantContentType: "application/problem+json"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, contentType := fetchStatus(t, baseURL+test.path)
			if status != test.wantStatus || contentType != test.wantContentType {
				t.Errorf("GET %s = %d %q, want %d %q", test.path, status, contentType, test.wantStatus, test.wantContentType)
			}
		})
	}

	cancel()
	assertCleanExit(t, exitCodes, stderr)
}

func TestWithCloseErrorKeepsCloseFailures(t *testing.T) {
	t.Parallel()

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	startupErr := errors.New("migration failed")
	closeErr := errors.New("disk gone")
	stopped := fmt.Errorf("run http server: %w", context.Canceled)

	tests := []struct {
		name        string
		ctx         context.Context
		result      error
		closeErr    error
		wantIs      []error
		wantNotIs   []error
		wantNil     bool
		wantStopped bool
	}{
		{name: "nothing failed", ctx: t.Context(), wantNil: true},
		{name: "only the result failed", ctx: t.Context(), result: startupErr, wantIs: []error{startupErr}},
		{name: "only close failed", ctx: t.Context(), closeErr: closeErr, wantIs: []error{closeErr}},
		{name: "both failed", ctx: t.Context(), result: startupErr, closeErr: closeErr, wantIs: []error{startupErr, closeErr}},
		{name: "stopped while starting is kept", ctx: cancelled, result: stopped, wantIs: []error{context.Canceled}, wantStopped: true},
		{
			name: "stopped while starting does not hide a close failure", ctx: cancelled, result: stopped, closeErr: closeErr,
			wantIs: []error{closeErr}, wantNotIs: []error{context.Canceled},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := withCloseError(test.ctx, test.result, test.closeErr)

			if (got == nil) != test.wantNil {
				t.Fatalf("withCloseError = %v, want nil: %t", got, test.wantNil)
			}

			assertErrorChain(t, got, test.wantIs, test.wantNotIs)

			if stoppedWhileStarting(test.ctx, got) != test.wantStopped {
				t.Errorf("stoppedWhileStarting(%v) = %t, want %t", got, !test.wantStopped, test.wantStopped)
			}
		})
	}
}

func assertErrorChain(t *testing.T, err error, wantIs, wantNotIs []error) {
	t.Helper()

	for _, target := range wantIs {
		if !errors.Is(err, target) {
			t.Errorf("error %v does not wrap %v", err, target)
		}
	}

	for _, target := range wantNotIs {
		if errors.Is(err, target) {
			t.Errorf("error %v wraps %v, want it dropped", err, target)
		}
	}
}

// fetchStatus requests url and returns the status and content type, without following
// redirects.
func fetchStatus(t *testing.T, url string) (int, string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultTransport.RoundTrip(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	defer func() { _ = response.Body.Close() }()

	return response.StatusCode, response.Header.Get("Content-Type")
}

func TestRunReportsStartupErrorsOnOneLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configFile string
		prepare    func(t *testing.T, environment map[string]string)
		wantPart   string
	}{
		{
			name:       "invalid yaml",
			configFile: "server: [\n",
			wantPart:   "invalid YAML",
		},
		{
			name:     "invalid environment value",
			prepare:  func(_ *testing.T, environment map[string]string) { environment[envServerPort] = "eighty" },
			wantPart: envServerPort,
		},
		{
			name:     "unwritable data directory",
			prepare:  blockDataDirectory,
			wantPart: "prepare directories",
		},
		{
			name:     "port already in use",
			prepare:  occupyPort,
			wantPart: "listen on",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			environment := isolatedEnvironment(t)
			if tt.configFile != "" {
				writeConfigFile(t, environment[paths.ConfigDirEnv()], tt.configFile)
			}

			if tt.prepare != nil {
				tt.prepare(t, environment)
			}

			env, stdout, stderr := newTestEnv(environment)

			if code := run(t.Context(), nil, env, runHooks{}); code != exitFailure {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, exitFailure, stderr.String())
			}

			assertStartupError(t, stderr.String(), tt.wantPart)
			assertNoOutput(t, stdout)
		})
	}
}

func TestRunExitsCleanlyWhenStoppedWhileStarting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	env, _, stderr := newTestEnv(isolatedEnvironment(t))

	if code := run(ctx, nil, env, runHooks{}); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %q)", code, exitOK, stderr.String())
	}

	if strings.Contains(stderr.String(), brand.ID+": ") {
		t.Errorf("stderr = %q, want no startup error", stderr.String())
	}
}

// runAndQueryServer starts the server, checks both system endpoints, stops it through the
// context and returns the reported server ID.
func runAndQueryServer(t *testing.T, args []string, environment map[string]string) string {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	env, stdout, stderr := newTestEnv(environment)
	addresses := make(chan net.Addr, 1)
	exitCodes := make(chan int, 1)
	hooks := runHooks{onListening: func(address net.Addr) { addresses <- address }}

	go func() { exitCodes <- run(ctx, args, env, hooks) }()

	address := awaitAddress(t, addresses, exitCodes, stderr)
	assertNotDefaultPort(t, address)

	baseURL := "http://" + address.String()

	assertHealthy(t, baseURL)
	serverID := fetchServerID(t, baseURL)

	cancel()
	assertCleanExit(t, exitCodes, stderr)
	assertNoOutput(t, stdout)

	if count := strings.Count(stderr.String(), listeningMessage); count != 1 {
		t.Errorf("log has %d %q lines, want 1:\n%s", count, listeningMessage, stderr.String())
	}

	return serverID
}

// assertNotDefaultPort proves the environment overrode the port of the config file: the
// file asks for the default port, the environment for an OS-assigned one.
func assertNotDefaultPort(t *testing.T, address net.Addr) {
	t.Helper()

	tcpAddress, ok := address.(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address %v is not TCP", address)
	}

	if tcpAddress.Port == config.DefaultPort {
		t.Errorf("bound port = %d, want an OS-assigned port instead of the default", tcpAddress.Port)
	}
}

func awaitAddress(t *testing.T, addresses <-chan net.Addr, exitCodes <-chan int, stderr *syncBuffer) net.Addr {
	t.Helper()

	select {
	case address := <-addresses:
		return address
	case code := <-exitCodes:
		t.Fatalf("run exited with %d before listening:\n%s", code, stderr.String())
	case <-time.After(startTimeout):
		t.Fatalf("server did not listen within %s:\n%s", startTimeout, stderr.String())
	}

	return nil
}

func assertCleanExit(t *testing.T, exitCodes <-chan int, stderr *syncBuffer) {
	t.Helper()

	select {
	case code := <-exitCodes:
		if code != exitOK {
			t.Fatalf("exit code = %d, want %d:\n%s", code, exitOK, stderr.String())
		}
	case <-time.After(startTimeout):
		t.Fatalf("server did not stop within %s:\n%s", startTimeout, stderr.String())
	}
}

func assertHealthy(t *testing.T, baseURL string) {
	t.Helper()

	var health struct {
		Status string `json:"status"`
	}

	getJSON(t, baseURL+"/api/v1/health", &health)

	if health.Status != "ok" {
		t.Errorf("health status = %q, want ok", health.Status)
	}
}

func fetchServerID(t *testing.T, baseURL string) string {
	t.Helper()

	var info struct {
		ProductName string `json:"productName"`
		Version     string `json:"version"`
		ServerID    string `json:"serverId"`
	}

	getJSON(t, baseURL+"/api/v1/system/info", &info)

	if info.ProductName != brand.DisplayName || info.Version == "" || info.ServerID == "" {
		t.Errorf("system info = %+v, want product name %q, a version and a server id", info, brand.DisplayName)
	}

	return info.ServerID
}

func getJSON(t *testing.T, url string, target any) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	defer func() { _ = response.Body.Close() }()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d, want %d", url, response.StatusCode, http.StatusOK)
	}

	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatalf("GET %s: decode: %v", url, err)
	}
}

// assertStartupError checks that the error is the last line of stderr, starts with the
// command name and comes without panic or stack trace. Lines before it are log records.
func assertStartupError(t *testing.T, stderr, wantPart string) {
	t.Helper()

	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	last := lines[len(lines)-1]

	if !strings.HasPrefix(last, brand.ID+": ") || !strings.Contains(last, wantPart) {
		t.Errorf("last stderr line = %q, want %q prefix and %q", last, brand.ID+": ", wantPart)
	}

	if strings.Count(stderr, brand.ID+": ") != 1 {
		t.Errorf("stderr = %q, want exactly one error line", stderr)
	}

	if strings.Contains(stderr, "panic") || strings.Contains(stderr, "goroutine ") {
		t.Errorf("stderr = %q, want no panic or stack trace", stderr)
	}
}

func assertNoOutput(t *testing.T, stdout *syncBuffer) {
	t.Helper()

	if stdout.String() != "" {
		t.Errorf("stdout = %q, want it empty", stdout.String())
	}
}

func assertExists(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("stat %s: %v", path, err)
	}
}

// isolatedEnvironment points every directory into a temp directory and the listener to a
// free loopback port, so a test never touches the host's directories or port 8460.
func isolatedEnvironment(t *testing.T) map[string]string {
	t.Helper()

	root := t.TempDir()

	return map[string]string{
		paths.ConfigDirEnv(): filepath.Join(root, "config"),
		envPathsData:         filepath.Join(root, "data"),
		envPathsCache:        filepath.Join(root, "cache"),
		envPathsTranscode:    filepath.Join(root, "transcode"),
		envServerHost:        "127.0.0.1",
		envServerPort:        "0",
		envLogLevel:          "warn",
	}
}

// blockDataDirectory places a regular file where a parent of the data directory must be,
// which makes creating it fail on every OS and for every user, root included.
func blockDataDirectory(t *testing.T, environment map[string]string) {
	t.Helper()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}

	environment[envPathsData] = filepath.Join(blocker, "data")
}

// occupyPort binds a loopback port for the duration of the test and configures it.
func occupyPort(t *testing.T, environment map[string]string) {
	t.Helper()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), listenNetwork, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	tcpAddress, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		t.Fatalf("listener address %v is not TCP", listener.Addr())
	}

	environment[envServerPort] = strconv.Itoa(tcpAddress.Port)
}

func writeConfigFile(t *testing.T, configDir, content string) {
	t.Helper()

	if err := os.MkdirAll(configDir, 0o750); err != nil {
		t.Fatalf("create config dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(configDir, config.FileName), []byte(content), 0o600); err != nil {
		t.Fatalf("write config file: %v", err)
	}
}

func newTestEnv(environment map[string]string) (runEnv, *syncBuffer, *syncBuffer) {
	stdout, stderr := &syncBuffer{}, &syncBuffer{}

	return runEnv{stdout: stdout, stderr: stderr, lookupEnv: lookupIn(environment)}, stdout, stderr
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes of the server's goroutines.
type syncBuffer struct {
	mutex  sync.Mutex
	buffer bytes.Buffer
}

func (buffer *syncBuffer) Write(data []byte) (int, error) {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.Write(data)
}

func (buffer *syncBuffer) String() string {
	buffer.mutex.Lock()
	defer buffer.mutex.Unlock()

	return buffer.buffer.String()
}

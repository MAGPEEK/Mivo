package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/healthprobe"
	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestHealthcheckExitCodes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		status         int
		extraArgs      []string
		environment    map[string]string
		wantCode       int
		wantStderrPart string
	}{
		{name: "healthy server", status: http.StatusOK, wantCode: exitOK},
		{
			name: "unhealthy server", status: http.StatusServiceUnavailable, wantCode: exitFailure,
			wantStderrPart: brand.ID + " " + healthcheckCommand + ": check health: probe",
		},
		{
			name: "invalid configuration", status: http.StatusOK, wantCode: exitFailure,
			environment:    map[string]string{envServerPort: "not-a-port"},
			wantStderrPart: "load configuration",
		},
		{
			name: "unknown flag never uses the exit code Docker reserves", status: http.StatusOK,
			extraArgs: []string{"--unknown-flag"}, wantCode: exitFailure,
			wantStderrPart: "flag provided but not defined: -unknown-flag",
		},
		{
			name: "unexpected argument", status: http.StatusOK,
			extraArgs: []string{"now"}, wantCode: exitFailure,
			wantStderrPart: `unexpected argument "now"`,
		},
		{
			name: "help succeeds", status: http.StatusOK,
			extraArgs: []string{"-h"}, wantCode: exitOK,
			wantStderrPart: "-config-dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			environment := healthcheckEnvironment(t, stubHealthServer(t, tt.status))
			for key, value := range tt.environment {
				environment[key] = value
			}

			env, stdout, stderr := newTestEnv(environment)

			code := run(t.Context(), append([]string{healthcheckCommand}, tt.extraArgs...), env, runHooks{})
			if code != tt.wantCode {
				t.Fatalf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}

			assertNoOutput(t, stdout)
			assertStderr(t, stderr.String(), tt.wantStderrPart == "", tt.wantStderrPart)
		})
	}
}

func TestHealthcheckReportsUnreachableServer(t *testing.T) {
	t.Parallel()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), listenNetwork, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	address := listener.Addr()
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	env, _, stderr := newTestEnv(healthcheckEnvironment(t, address))

	if code := run(t.Context(), []string{healthcheckCommand}, env, runHooks{}); code != exitFailure {
		t.Fatalf("exit code = %d, want %d", code, exitFailure)
	}

	if lines := strings.Count(stderr.String(), "\n"); lines != 1 {
		t.Errorf("stderr = %q, want exactly one line", stderr.String())
	}
}

func TestHealthcheckAcceptsRunningServer(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	environment := isolatedEnvironment(t)
	serverEnv, _, serverStderr := newTestEnv(environment)
	addresses := make(chan net.Addr, 1)
	exitCodes := make(chan int, 1)
	hooks := runHooks{onListening: func(address net.Addr) { addresses <- address }}

	go func() { exitCodes <- run(ctx, nil, serverEnv, hooks) }()

	address := awaitAddress(t, addresses, exitCodes, serverStderr)

	// The probe reads the same configuration as the server, only the chosen port differs.
	checkEnvironment := healthcheckEnvironment(t, address)
	checkEnv, _, checkStderr := newTestEnv(checkEnvironment)

	if code := run(t.Context(), []string{healthcheckCommand}, checkEnv, runHooks{}); code != exitOK {
		t.Errorf("exit code = %d, want %d (stderr: %q)", code, exitOK, checkStderr.String())
	}

	cancel()
	assertCleanExit(t, exitCodes, serverStderr)
}

// stubHealthServer answers the health path with status and returns its address.
func stubHealthServer(t *testing.T, status int) net.Addr {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != healthprobe.HealthPath {
			writer.WriteHeader(http.StatusNotFound)

			return
		}

		writer.WriteHeader(status)
	}))
	t.Cleanup(server.Close)

	return server.Listener.Addr()
}

// healthcheckEnvironment configures temp directories and the port of address. The host stays
// unset, as in the container image, so the probe must pick the loopback address itself.
func healthcheckEnvironment(t *testing.T, address net.Addr) map[string]string {
	t.Helper()

	tcpAddress, ok := address.(*net.TCPAddr)
	if !ok {
		t.Fatalf("address %v is not TCP", address)
	}

	environment := isolatedEnvironment(t)
	delete(environment, envServerHost)
	environment[envServerPort] = strconv.Itoa(tcpAddress.Port)

	return environment
}

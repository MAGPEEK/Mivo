package server

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
)

// publicPeer is a socket peer on the internet the guard refuses.
const publicPeer = "203.0.113.7:40000"

// refusedMessage is the debug log message of a refused request.
const refusedMessage = `msg="request from a public address refused"`

// wantRefusalDetail is the fixed detail of the 403 problem; clients may show it as is.
const wantRefusalDetail = "Requests from outside the local network are refused (server.lan_only)."

// onLinkPrefix is the global IPv6 prefix of the fake host interface in the guard tests.
const onLinkPrefix = "2001:db8:1:2::/64"

func newDebugLogger() (*slog.Logger, *syncBuffer) {
	logs := &syncBuffer{}

	return slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})), logs
}

func TestFixedLANRanges(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{name: "ipv4 loopback", remoteAddr: "127.0.0.1:50000", want: true},
		{name: "ipv4 loopback range end", remoteAddr: "127.255.255.254:1", want: true},
		{name: "rfc 1918 10/8 start", remoteAddr: "10.0.0.1:1", want: true},
		{name: "rfc 1918 10/8 end", remoteAddr: "10.255.255.255:1", want: true},
		{name: "rfc 1918 172.16/12 start", remoteAddr: "172.16.0.1:1", want: true},
		{name: "rfc 1918 172.16/12 end", remoteAddr: "172.31.255.255:1", want: true},
		{name: "docker bridge gateway", remoteAddr: "172.17.0.1:1", want: true},
		{name: "rfc 1918 192.168/16", remoteAddr: "192.168.178.20:1", want: true},
		{name: "rfc 6598 start (tailscale)", remoteAddr: "100.64.0.1:1", want: true},
		{name: "rfc 6598 end", remoteAddr: "100.127.255.255:1", want: true},
		{name: "ipv4 link-local", remoteAddr: "169.254.10.20:1", want: true},
		{name: "below 172.16/12", remoteAddr: "172.15.255.255:1", want: false},
		{name: "above 172.16/12", remoteAddr: "172.32.0.0:1", want: false},
		{name: "next to 192.168/16", remoteAddr: "192.169.0.1:1", want: false},
		{name: "below rfc 6598", remoteAddr: "100.63.255.255:1", want: false},
		{name: "above rfc 6598", remoteAddr: "100.128.0.0:1", want: false},
		{name: "public ipv4", remoteAddr: "8.8.8.8:443", want: false},
		{name: "ipv4 documentation range", remoteAddr: "192.0.2.1:1234", want: false},
		{name: "ipv4 unspecified", remoteAddr: "0.0.0.0:1", want: false},
		{name: "ipv4 broadcast", remoteAddr: "255.255.255.255:1", want: false},
		{name: "ipv4 multicast", remoteAddr: "224.0.0.1:1", want: false},
		{name: "ipv6 loopback", remoteAddr: "[::1]:50000", want: true},
		{name: "ipv6 ula fc00::/7 low half", remoteAddr: "[fc00::1]:1", want: true},
		{name: "ipv6 ula fd00::/8", remoteAddr: "[fd12:3456:789a::1]:1", want: true},
		{name: "ipv6 link-local", remoteAddr: "[fe80::1]:1", want: true},
		{name: "ipv6 link-local with zone", remoteAddr: "[fe80::1%eth0]:1", want: true},
		{name: "public ipv6", remoteAddr: "[2606:4700:4700::1111]:443", want: false},
		{name: "ipv6 documentation range", remoteAddr: "[2001:db8::1]:1", want: false},
		{name: "ipv6 unspecified", remoteAddr: "[::]:1", want: false},
		{name: "ipv6 multicast link-local scope", remoteAddr: "[ff02::1]:1", want: false},
		{name: "deprecated ipv6 site-local", remoteAddr: "[fec0::1]:1", want: false},
		{name: "mapped ipv4 loopback", remoteAddr: "[::ffff:127.0.0.1]:1", want: true},
		{name: "mapped rfc 1918", remoteAddr: "[::ffff:192.168.1.10]:1", want: true},
		{name: "mapped rfc 6598", remoteAddr: "[::ffff:100.64.0.1]:1", want: true},
		{name: "mapped link-local", remoteAddr: "[::ffff:169.254.1.1]:1", want: true},
		{name: "mapped public ipv4", remoteAddr: "[::ffff:8.8.8.8]:1", want: false},
		{name: "mapped hex notation public", remoteAddr: "[::ffff:808:808]:1", want: false},
		// Translation prefixes carry an IPv4 address the peer does not own; only the real
		// mapped form ::ffff:0:0/96 is unwrapped.
		{name: "ipv4-compatible private", remoteAddr: "[::10.0.0.1]:1", want: false},
		{name: "nat64 of a private address", remoteAddr: "[64:ff9b::10.0.0.1]:1", want: false},
		{name: "6to4 of a private address", remoteAddr: "[2002:a00:1::1]:1", want: false},
		{name: "empty", remoteAddr: "", want: false},
		{name: "garbage", remoteAddr: "not an address", want: false},
		{name: "ipv4 without port", remoteAddr: "192.168.1.10", want: false},
		{name: "ipv6 without brackets", remoteAddr: "::1", want: false},
		{name: "ipv6 without port", remoteAddr: "[::1]", want: false},
		{name: "host name", remoteAddr: "localhost:80", want: false},
		{name: "port out of range", remoteAddr: "192.168.1.10:70000", want: false},
		{name: "non-numeric port", remoteAddr: "192.168.1.10:http", want: false},
		{name: "address list", remoteAddr: "192.168.1.10, 8.8.8.8:1", want: false},
		{name: "unix socket peer", remoteAddr: "@", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			address, ok := peerAddress(test.remoteAddr)
			if got := ok && isLANAddress(address); got != test.want {
				t.Errorf("fixed LAN range check of %q = %v, want %v", test.remoteAddr, got, test.want)
			}
		})
	}
}

// guardCase is one request through the chain with the LAN guard on.
type guardCase struct {
	name       string
	remoteAddr string
	method     string
	target     string
	headers    map[string]string
	// wantBody is the handler that answered: "api" or "web"; empty for a refused request.
	wantBody string
}

func guardCases() []guardCase {
	spoofedLAN := map[string]string{
		"X-Forwarded-For": "192.168.1.5", "X-Real-IP": "10.0.0.5", "Forwarded": "for=192.168.1.5",
	}

	return []guardCase{
		{name: "lan api route", remoteAddr: lanPeer, target: "/api/v1/system/info", wantBody: "api"},
		{name: "lan web route", remoteAddr: lanPeer, target: "/library/7", wantBody: "web"},
		{name: "loopback ipv6", remoteAddr: "[::1]:5000", target: "/api/v1/libraries", wantBody: "api"},
		{name: "mapped lan", remoteAddr: "[::ffff:10.1.2.3]:5000", target: "/", wantBody: "web"},
		{name: "public api route", remoteAddr: publicPeer, target: "/api/v1/system/info"},
		{name: "public web root", remoteAddr: publicPeer, target: "/"},
		{name: "public web route", remoteAddr: publicPeer, target: "/library/7"},
		{name: "public stream", remoteAddr: publicPeer, target: "/api/v1/playback/sessions/x/file"},
		{name: "public post", remoteAddr: publicPeer, method: http.MethodPost, target: "/api/v1/playback/sessions"},
		{name: "public ipv6", remoteAddr: "[2606:4700:4700::1111]:443", target: "/api/v1/libraries"},
		// The fake host interface of TestLANGuard carries onLinkPrefix (dual-stack home network).
		{name: "on-link global ipv6", remoteAddr: "[2001:db8:1:2::abcd]:5000", target: "/api/v1/libraries", wantBody: "api"},
		{name: "global ipv6 next to the on-link prefix", remoteAddr: "[2001:db8:1:3::abcd]:5000", target: "/api/v1/libraries"},
		{name: "mapped public", remoteAddr: "[::ffff:8.8.8.8]:443", target: "/api/v1/libraries"},
		{name: "malformed peer", remoteAddr: "garbage", target: "/api/v1/libraries"},
		{name: "empty peer", remoteAddr: "", target: "/"},
		// X-Forwarded-For and friends are client-controlled until a trusted-proxy setting
		// exists (milestone 7b); only the socket peer counts, in both directions.
		{name: "public with spoofed lan headers", remoteAddr: publicPeer, target: "/api/v1/libraries", headers: spoofedLAN},
		{
			name: "lan with public forwarded header", remoteAddr: lanPeer, target: "/api/v1/libraries", wantBody: "api",
			headers: map[string]string{"X-Forwarded-For": "8.8.8.8", "Forwarded": "for=8.8.8.8"},
		},
		{name: "public health", remoteAddr: publicPeer, target: "/api/v1/health", wantBody: "api"},
		{name: "public health head", remoteAddr: publicPeer, method: http.MethodHead, target: "/api/v1/health", wantBody: "api"},
		{name: "public health with query", remoteAddr: publicPeer, target: "/api/v1/health?verbose=1", wantBody: "api"},
		{name: "malformed peer health", remoteAddr: "garbage", target: "/api/v1/health", wantBody: "api"},
		{name: "public health post", remoteAddr: publicPeer, method: http.MethodPost, target: "/api/v1/health"},
		{name: "public health trailing slash", remoteAddr: publicPeer, target: "/api/v1/health/"},
		{name: "public health below", remoteAddr: publicPeer, target: "/api/v1/health/x"},
		{name: "public health dot segments", remoteAddr: publicPeer, target: "/api/v1/health/../system/info"},
		{name: "public health encoded letter", remoteAddr: publicPeer, target: "/api/v1/heal%74h"},
		{name: "public health encoded slashes", remoteAddr: publicPeer, target: "/api%2Fv1%2Fhealth"},
		{name: "public health prefix lookalike", remoteAddr: publicPeer, target: "/api/v1/healthz"},
	}
}

func TestLANGuard(t *testing.T) {
	t.Parallel()

	for _, test := range guardCases() {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logger, logs := newDebugLogger()
			options := Options{
				Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), Fallback: textHandler(http.StatusOK, "web"),
				InterfacePrefixes: &fakePrefixProvider{prefixes: []netip.Prefix{netip.MustParsePrefix(onLinkPrefix)}},
			}

			recorder := serveChain(t, options, guardRequest(t, test))

			if test.wantBody == "" {
				assertRefused(t, recorder, logs.String(), test.remoteAddr)

				return
			}

			assertPassed(t, recorder, test, logs.String())
		})
	}
}

func guardRequest(t *testing.T, test guardCase) *http.Request {
	t.Helper()

	method := test.method
	if method == "" {
		method = http.MethodGet
	}

	request := newRequest(t, method, test.target)
	request.RemoteAddr = test.remoteAddr

	for name, value := range test.headers {
		request.Header.Set(name, value)
	}

	return request
}

func assertPassed(t *testing.T, recorder *httptest.ResponseRecorder, test guardCase, logs string) {
	t.Helper()

	// net/http drops the body of a HEAD response itself; the recorder keeps it.
	if recorder.Code != http.StatusOK || recorder.Body.String() != test.wantBody {
		t.Errorf("%s %s from %q = %d %q, want 200 from %s", test.method, test.target, test.remoteAddr, recorder.Code, recorder.Body, test.wantBody)
	}

	if strings.Contains(logs, refusedMessage) {
		t.Errorf("log %q reports a refusal for a passed request", logs)
	}
}

func assertRefused(t *testing.T, recorder *httptest.ResponseRecorder, logs, remoteAddr string) {
	t.Helper()

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, body %s, want 403", recorder.Code, recorder.Body)
	}

	var problem apigen.Problem
	if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil || problem.Code != restapi.CodeForbidden ||
		problem.Status != http.StatusForbidden || problem.Detail == nil || *problem.Detail != wantRefusalDetail {
		t.Errorf("body %s, decode error %v, want a 403 forbidden problem with detail %q", recorder.Body, err, wantRefusalDetail)
	}

	if contentType := recorder.Header().Get("Content-Type"); contentType != restapi.ProblemMediaType {
		t.Errorf("Content-Type = %q, want %q", contentType, restapi.ProblemMediaType)
	}

	for _, header := range []string{"X-Content-Type-Options", "Referrer-Policy", requestid.HeaderName} {
		if recorder.Header().Get(header) == "" {
			t.Errorf("%s missing on the refusal", header)
		}
	}

	wantLogs := []string{"level=DEBUG " + refusedMessage, "remote_address=", `msg="http request"`, "status=403"}
	if remoteAddr != "" {
		wantLogs = append(wantLogs, remoteAddr)
	}

	for _, want := range wantLogs {
		if !strings.Contains(logs, want) {
			t.Errorf("log %q lacks %q", logs, want)
		}
	}
}

func TestLANGuardOffServesPublicPeers(t *testing.T) {
	t.Parallel()

	for _, remoteAddr := range []string{publicPeer, "[2606:4700:4700::1111]:443", "garbage"} {
		logger, logs := newDebugLogger()
		options := Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api"), AllowPublicClients: true}

		request := newRequest(t, http.MethodGet, "/api/v1/libraries")
		request.RemoteAddr = remoteAddr

		recorder := serveChain(t, options, request)
		if recorder.Code != http.StatusOK || recorder.Body.String() != "api" {
			t.Errorf("GET from %q with the guard off = %d %q, want 200 api", remoteAddr, recorder.Code, recorder.Body)
		}

		if strings.Contains(logs.String(), refusedMessage) {
			t.Errorf("log %q reports a refusal with the guard off", logs)
		}
	}
}

// TestLANGuardTrustsOnlyTheSocketPeer runs over a real loopback connection: a forwarded
// header that names a public client changes nothing, because the guard reads RemoteAddr,
// which net/http takes from the socket.
func TestLANGuardTrustsOnlyTheSocketPeer(t *testing.T) {
	t.Parallel()

	logger, logs := newDebugLogger()
	testServer := httptest.NewServer(newTestChain(t, Options{Logger: logger, APIHandler: textHandler(http.StatusOK, "api")}))
	t.Cleanup(testServer.Close)

	// Not the health endpoint: the guard lets that through for every peer.
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, testServer.URL+"/api/v1/libraries", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("X-Forwarded-For", "8.8.8.8")
	request.Header.Set("Forwarded", "for=8.8.8.8")

	response, err := testServer.Client().Do(request)
	if err != nil {
		t.Fatalf("GET over loopback: %v", err)
	}

	if closeErr := response.Body.Close(); closeErr != nil {
		t.Logf("close body: %v", closeErr)
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 for a loopback peer", response.StatusCode)
	}

	if strings.Contains(logs.String(), refusedMessage) {
		t.Errorf("log %q refuses a loopback peer", logs)
	}
}

func TestRunLogsTheClientAccessMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		allowPublicClients bool
		want               string
	}{
		{name: "guard on by default", want: "lan_only=true"},
		{name: "guard off", allowPublicClients: true, want: "lan_only=false"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			logger, logs := newTestLogger()

			httpServer, err := New(Options{
				Listener: openTestListener(t), Logger: logger, APIHandler: textHandler(http.StatusOK, ""),
				AllowPublicClients: test.allowPublicClients,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			// A cancelled context makes Run log, serve and shut down at once.
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			if err := httpServer.Run(ctx); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if line := logs.String(); !strings.Contains(line, `msg="http server listening"`) || !strings.Contains(line, test.want) {
				t.Errorf("log %q, want the listening line with %s", line, test.want)
			}
		})
	}
}

func openTestListener(t *testing.T) net.Listener {
	t.Helper()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	return listener
}

// TestGeneralOptionsRequestGoesThroughTheChain: net/http answers "OPTIONS *" itself unless
// DisableGeneralOptionsHandler is set, which would skip the guard, the security headers and
// the access log.
func TestGeneralOptionsRequestGoesThroughTheChain(t *testing.T) {
	t.Parallel()

	logger, logs := newTestLogger()
	listener := openTestListener(t)

	httpServer, err := New(Options{Listener: listener, Logger: logger, APIHandler: textHandler(http.StatusOK, "api")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)

	go func() { result <- httpServer.Run(ctx) }()

	status, header := sendGeneralOptions(t, listener.Addr().String())

	cancel()

	if err := <-result; err != nil {
		t.Errorf("Run: %v", err)
	}

	// ServeMux answers "*" with 400; net/http's own handler would answer 200 without headers.
	if status != http.StatusBadRequest || header.Get(headerContentTypeOptions) != valueNoSniff {
		t.Errorf("OPTIONS * = %d with headers %v, want 400 from the chain", status, header)
	}

	if !strings.Contains(logs.String(), "method=OPTIONS") {
		t.Errorf("access log %q lacks the OPTIONS * request", logs)
	}
}

// sendGeneralOptions writes a raw "OPTIONS *" request, which net/http's client cannot send,
// and returns the status and headers of the answer.
func sendGeneralOptions(t *testing.T, address string) (int, http.Header) {
	t.Helper()

	var dialer net.Dialer

	connection, err := dialer.DialContext(t.Context(), "tcp", address)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	defer func() { _ = connection.Close() }()

	if _, err := connection.Write([]byte("OPTIONS * HTTP/1.1\r\nHost: mivo\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write request: %v", err)
	}

	response, err := http.ReadResponse(bufio.NewReader(connection), nil)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if err := response.Body.Close(); err != nil {
		t.Logf("close body: %v", err)
	}

	return response.StatusCode, response.Header
}

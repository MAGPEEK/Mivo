package server

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/clientorigin"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/auth"
	"github.com/magpeek/mivo/internal/domain"
)

// Socket peers and clients of the trusted-proxy tests.
const (
	proxyAddress      = "10.0.0.2"
	proxyPeer         = proxyAddress + ":41000"
	secondProxy       = "10.0.0.3"
	publicClient      = "203.0.113.7"
	lanClient         = "192.168.1.10"
	forwardedFor      = "X-Forwarded-For"
	forwardedProto    = "X-Forwarded-Proto"
	libraryCollection = "/api/v1/libraries"
)

// trustedProxies trusts the two test proxies and an IPv6 one.
func trustedProxies() []netip.Prefix {
	return []netip.Prefix{
		netip.MustParsePrefix(proxyAddress + "/32"), netip.MustParsePrefix(secondProxy + "/32"),
		netip.MustParsePrefix("2001:db8:ffff::/48"),
	}
}

func newTestResolver(t *testing.T, proxies []netip.Prefix) originResolver {
	t.Helper()

	resolver, err := newOriginResolver(proxies)
	if err != nil {
		t.Fatalf("newOriginResolver: %v", err)
	}

	return resolver
}

// proxiedRequest builds a request from remoteAddr with the given header lines.
func proxiedRequest(t *testing.T, remoteAddr string, header http.Header) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, libraryCollection, nil)
	request.RemoteAddr = remoteAddr

	for name, values := range header {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	return request
}

// classOf classifies a resolved origin the way the guards do.
func classOf(t *testing.T, origin clientorigin.Origin) auth.AddressClass {
	t.Helper()

	logger, _ := newDebugLogger()
	guard := newLANGuard(chainSettings{
		logger: logger, now: time.Now,
		interfacePrefixes: &fakePrefixProvider{prefixes: []netip.Prefix{netip.MustParsePrefix(onLinkPrefix)}},
	})

	request := httptest.NewRequestWithContext(clientorigin.With(t.Context(), origin), http.MethodGet, "/", nil)
	if guard.admitsPeer(request) {
		return auth.AddressLAN
	}

	return auth.AddressRemote
}

// TestProxyClientAddressFromATrustedPeer covers the X-Forwarded-For walk of ADR 0032 §3.
func TestProxyClientAddressFromATrustedPeer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		lines     []string
		want      string
		wantClass auth.AddressClass
	}{
		{name: "single hop", lines: []string{publicClient}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "single lan hop", lines: []string{lanClient}, want: lanClient, wantClass: auth.AddressLAN},
		{name: "several hops in one header", lines: []string{publicClient + ", " + secondProxy}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "several header lines", lines: []string{"198.51.100.1", publicClient, secondProxy}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "client-supplied left prefix", lines: []string{"10.0.0.1, " + publicClient}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "client-supplied lan prefix on its own line", lines: []string{lanClient, publicClient}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "all entries trusted", lines: []string{secondProxy + ", " + proxyAddress}, want: secondProxy, wantClass: auth.AddressLAN},
		{name: "entry with port", lines: []string{publicClient + ":4711"}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "bracketed ipv6 with port", lines: []string{"[2001:db8::1]:4711"}, want: "2001:db8::1", wantClass: auth.AddressRemote},
		{name: "bracketed ipv6", lines: []string{"[2001:db8::1]"}, want: "2001:db8::1", wantClass: auth.AddressRemote},
		{name: "bare ipv6", lines: []string{"2001:db8::1"}, want: "2001:db8::1", wantClass: auth.AddressRemote},
		{name: "on-link ipv6", lines: []string{"2001:db8:1:2::abcd"}, want: "2001:db8:1:2::abcd", wantClass: auth.AddressLAN},
		{name: "mapped ipv4", lines: []string{"::ffff:" + lanClient}, want: lanClient, wantClass: auth.AddressLAN},
		{name: "mapped trusted proxy is skipped", lines: []string{publicClient + ", ::ffff:" + secondProxy}, want: publicClient, wantClass: auth.AddressRemote},
		{name: "trusted ipv6 proxy is skipped", lines: []string{lanClient + ", 2001:db8:ffff::5"}, want: lanClient, wantClass: auth.AddressLAN},
		{name: "surrounding spaces", lines: []string{"  " + lanClient + "  ,\t" + secondProxy + " "}, want: lanClient, wantClass: auth.AddressLAN},
		{name: "garbage left of the client is never read", lines: []string{"garbage, " + publicClient}, want: publicClient, wantClass: auth.AddressRemote},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			request := proxiedRequest(t, proxyPeer, http.Header{forwardedFor: test.lines})
			got := newTestResolver(t, trustedProxies()).resolve(request)

			if got.Address != netip.MustParseAddr(test.want) || got.ForcedRemote || got.Proxy != netip.MustParseAddr(proxyAddress) {
				t.Errorf("origin = %+v, want client %s behind proxy %s", got, test.want, proxyAddress)
			}

			if class := classOf(t, got); class != test.wantClass {
				t.Errorf("class = %s, want %s", class, test.wantClass)
			}
		})
	}
}

// TestProxyFailsClosedWithoutAUsableClient: a trusted proxy that names no client must not
// make its clients look local.
func TestProxyFailsClosedWithoutAUsableClient(t *testing.T) {
	t.Parallel()

	tooManyHops := strings.Repeat(secondProxy+",", maxForwardedEntries) + lanClient

	tests := []struct {
		name  string
		lines []string
	}{
		{name: "no header"},
		{name: "empty value", lines: []string{""}},
		{name: "only spaces", lines: []string{"  "}},
		{name: "unknown", lines: []string{"unknown"}},
		{name: "obfuscated identifier", lines: []string{"_hidden"}},
		{name: "zone id", lines: []string{"fe80::1%eth0"}},
		{name: "bracketed zone id with port", lines: []string{"[fe80::1%eth0]:80"}},
		{name: "host name", lines: []string{"client.example"}},
		{name: "garbage before the client is found", lines: []string{lanClient + ", garbage, " + secondProxy}},
		{name: "empty entry before the client is found", lines: []string{lanClient + ",, " + secondProxy}},
		{name: "trailing comma", lines: []string{lanClient + ","}},
		{name: "unclosed bracket", lines: []string{"[2001:db8::1"}},
		{name: "more hops than the bound", lines: []string{tooManyHops}},
		{name: "more lines than the bound", lines: slices.Repeat([]string{lanClient}, maxForwardedEntries+1)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			if test.lines != nil {
				header[forwardedFor] = test.lines
			}

			got := newTestResolver(t, trustedProxies()).resolve(proxiedRequest(t, proxyPeer, header))

			proxy := netip.MustParseAddr(proxyAddress)
			if got.Address != proxy || got.Proxy != proxy || !got.ForcedRemote {
				t.Errorf("origin = %+v, want the proxy's address forced remote", got)
			}

			if class := classOf(t, got); class != auth.AddressRemote {
				t.Errorf("class = %s, want remote", class)
			}
		})
	}
}

// TestProxyAcceptsTheBoundItself: exactly maxForwardedEntries hops are still read.
func TestProxyAcceptsTheBoundItself(t *testing.T) {
	t.Parallel()

	lines := []string{lanClient + strings.Repeat(","+secondProxy, maxForwardedEntries-1)}
	got := newTestResolver(t, trustedProxies()).resolve(proxiedRequest(t, proxyPeer, http.Header{forwardedFor: lines}))

	if got.Address != netip.MustParseAddr(lanClient) || got.ForcedRemote {
		t.Errorf("origin = %+v, want client %s", got, lanClient)
	}
}

// TestProxyScheme: the last X-Forwarded-Proto value of a trusted peer decides; an untrusted
// peer is https only on a TLS socket.
func TestProxyScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		lines      []string
		tls        bool
		want       bool
	}{
		{name: "trusted https", remoteAddr: proxyPeer, lines: []string{"https"}, want: true},
		{name: "trusted upper case", remoteAddr: proxyPeer, lines: []string{"HTTPS"}, want: true},
		{name: "trusted with spaces", remoteAddr: proxyPeer, lines: []string{" https "}, want: true},
		{name: "trusted http", remoteAddr: proxyPeer, lines: []string{"http"}},
		{name: "trusted without header", remoteAddr: proxyPeer},
		{name: "trusted two values, last https", remoteAddr: proxyPeer, lines: []string{"http, https"}, want: true},
		{name: "trusted two values, last http", remoteAddr: proxyPeer, lines: []string{"https, http"}},
		{name: "trusted two lines, last https", remoteAddr: proxyPeer, lines: []string{"http", "https"}, want: true},
		{name: "trusted two lines, last http", remoteAddr: proxyPeer, lines: []string{"https", "http"}},
		{name: "trusted unknown value", remoteAddr: proxyPeer, lines: []string{"wss"}},
		{name: "trusted empty value", remoteAddr: proxyPeer, lines: []string{""}},
		{name: "trusted empty last value", remoteAddr: proxyPeer, lines: []string{"https,"}},
		{name: "trusted tls socket without header", remoteAddr: proxyPeer, tls: true},
		{name: "untrusted https on a plain socket", remoteAddr: publicPeer, lines: []string{"https"}},
		{name: "untrusted lan https on a plain socket", remoteAddr: lanPeer, lines: []string{"https"}},
		{name: "untrusted tls socket", remoteAddr: publicPeer, tls: true, want: true},
		{name: "untrusted tls socket claiming http", remoteAddr: publicPeer, lines: []string{"http"}, tls: true, want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{forwardedFor: {lanClient}}
			if test.lines != nil {
				header[forwardedProto] = test.lines
			}

			request := proxiedRequest(t, test.remoteAddr, header)
			if test.tls {
				request.TLS = &tls.ConnectionState{}
			}

			if got := newTestResolver(t, trustedProxies()).resolve(request); got.Secure != test.want {
				t.Errorf("Secure = %t, want %t", got.Secure, test.want)
			}
		})
	}
}

// spoofedHeaders are every forwarding header a client may send to look local and secure.
func spoofedHeaders() http.Header {
	return http.Header{
		forwardedFor: {lanClient}, forwardedProto: {"https"}, "X-Forwarded-Host": {"evil.example"},
		"Forwarded": {"for=" + lanClient + ";proto=https"}, "X-Real-Ip": {lanClient},
	}
}

// TestProxyHeadersAreIgnoredFromUntrustedPeers: with the empty list, and with a list the
// socket peer is not in, only the socket counts.
func TestProxyHeadersAreIgnoredFromUntrustedPeers(t *testing.T) {
	t.Parallel()

	lists := map[string][]netip.Prefix{"empty list": nil, "peer outside the list": trustedProxies()}
	peers := []string{publicPeer, lanPeer, "127.0.0.1:5000", "[::1]:5000", "[::ffff:203.0.113.7]:40000"}

	for name, proxies := range lists {
		for _, peer := range peers {
			t.Run(name+" "+peer, func(t *testing.T) {
				t.Parallel()

				got := newTestResolver(t, proxies).resolve(proxiedRequest(t, peer, spoofedHeaders()))
				socket := netip.MustParseAddrPort(peer).Addr().Unmap()

				want := clientorigin.Origin{Address: socket, Socket: peer}
				if got != want {
					t.Errorf("origin = %+v, want %+v", got, want)
				}
			})
		}
	}
}

func TestProxyResolverKeepsAMalformedSocketAddressUntrusted(t *testing.T) {
	t.Parallel()

	got := newTestResolver(t, trustedProxies()).resolve(proxiedRequest(t, "garbage", spoofedHeaders()))
	if want := (clientorigin.Origin{Socket: "garbage"}); got != want {
		t.Errorf("origin = %+v, want %+v", got, want)
	}

	if class := classOf(t, got); class != auth.AddressRemote {
		t.Errorf("class = %s, want remote", class)
	}
}

// TestProxyMappedSocketPeerIsTrusted: a dual-stack listener reports an IPv4 proxy as
// ::ffff:a.b.c.d.
func TestProxyMappedSocketPeerIsTrusted(t *testing.T) {
	t.Parallel()

	request := proxiedRequest(t, "[::ffff:"+proxyAddress+"]:41000", http.Header{forwardedFor: {publicClient}})

	got := newTestResolver(t, trustedProxies()).resolve(request)
	if got.Address != netip.MustParseAddr(publicClient) || got.Proxy != netip.MustParseAddr(proxyAddress) {
		t.Errorf("origin = %+v, want client %s behind %s", got, publicClient, proxyAddress)
	}
}

func TestProxyOptionsRefuseUnsafePrefixes(t *testing.T) {
	t.Parallel()

	tests := map[string]netip.Prefix{
		"everything ipv4": netip.MustParsePrefix("0.0.0.0/0"),
		"everything ipv6": netip.MustParsePrefix("::/0"),
		"zero value":      {},
		"mapped":          netip.MustParsePrefix("::ffff:10.0.0.0/104"),
	}

	for name, prefix := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			logger, _ := newDebugLogger()

			_, err := New(Options{
				Address: "127.0.0.1:0", Logger: logger, APIHandler: textHandler(http.StatusOK, "api"),
				TrustedProxies: []netip.Prefix{prefix},
			})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("New with trusted proxy %s = %v, want an error wrapping ErrInvalid", prefix, err)
			}
		})
	}
}

// TestProxyOptionsMaskPrefixes: a prefix with host bits still matches its network.
func TestProxyOptionsMaskPrefixes(t *testing.T) {
	t.Parallel()

	resolver := newTestResolver(t, []netip.Prefix{netip.PrefixFrom(netip.MustParseAddr("10.0.0.77"), 24)})
	if !resolver.trusts(netip.MustParseAddr(proxyAddress)) {
		t.Errorf("10.0.0.77/24 does not trust %s", proxyAddress)
	}
}

// proxyChainCase is one request through the whole chain behind (or beside) a trusted proxy.
type proxyChainCase struct {
	name       string
	remoteAddr string
	method     string
	target     string
	headers    map[string]string
	bearer     string
	setup      bool
	lanOnly    bool
	wantStatus int
}

func proxyChainOptions(test proxyChainCase, handler http.Handler) (Options, *syncBuffer) {
	logger, logs := newDebugLogger()

	return Options{
		Logger: logger, APIHandler: handler, Fallback: textHandler(http.StatusOK, "web"),
		AllowPublicClients: !test.lanOnly, TrustedProxies: trustedProxies(),
		InterfacePrefixes: &fakePrefixProvider{prefixes: []netip.Prefix{netip.MustParsePrefix(onLinkPrefix)}},
		Authentication:    Authentication{Sessions: &fakeResolver{}, Setup: fakeSetup{required: test.setup}},
	}, logs
}

func newProxyChainRequest(t *testing.T, test proxyChainCase) *http.Request {
	t.Helper()

	body := ""
	if isUnsafeMethod(test.method) {
		body = "{}"
	}

	return newAuthRequest(t, authCase{
		method: test.method, target: test.target, remoteAddr: test.remoteAddr, headers: test.headers,
		body: body, bearer: test.bearer,
	})
}

// TestProxyGuardsDecideByTheResolvedClient runs the spoofing, fail-closed and happy cases
// through the LAN guard, the authentication, the management guard and the setup gate.
func TestProxyGuardsDecideByTheResolvedClient(t *testing.T) {
	t.Parallel()

	spoof := map[string]string{forwardedFor: lanClient, forwardedProto: "https"}
	post, get := http.MethodPost, http.MethodGet

	tests := []proxyChainCase{
		{name: "spoofing public peer, setup", remoteAddr: publicPeer, method: post, target: apiV1Setup, headers: spoof, setup: true, wantStatus: http.StatusForbidden},
		{name: "spoofing public peer, manage", remoteAddr: publicPeer, method: post, target: apiV1Libraries, headers: spoof, bearer: adminToken, wantStatus: http.StatusForbidden},
		{name: "spoofing public peer, lan guard", remoteAddr: publicPeer, method: get, target: apiV1Libraries, headers: spoof, bearer: userToken, lanOnly: true, wantStatus: http.StatusForbidden},
		{name: "proxied public client, setup", remoteAddr: proxyPeer, method: post, target: apiV1Setup, headers: map[string]string{forwardedFor: publicClient}, setup: true, wantStatus: http.StatusForbidden},
		{name: "proxied public client, manage", remoteAddr: proxyPeer, method: post, target: apiV1Libraries, headers: map[string]string{forwardedFor: publicClient}, bearer: adminToken, wantStatus: http.StatusForbidden},
		{name: "proxied public client, lan guard", remoteAddr: proxyPeer, method: get, target: apiV1Libraries, headers: map[string]string{forwardedFor: publicClient}, bearer: userToken, lanOnly: true, wantStatus: http.StatusForbidden},
		{name: "proxied public client, media without lan_only", remoteAddr: proxyPeer, method: get, target: apiV1Libraries, headers: map[string]string{forwardedFor: publicClient}, bearer: userToken, wantStatus: http.StatusOK},
		{name: "proxied lan client, setup", remoteAddr: proxyPeer, method: post, target: apiV1Setup, headers: map[string]string{forwardedFor: lanClient}, setup: true, wantStatus: http.StatusOK},
		{name: "proxied lan client, manage", remoteAddr: proxyPeer, method: post, target: apiV1Libraries, headers: map[string]string{forwardedFor: lanClient}, bearer: adminToken, wantStatus: http.StatusOK},
		{name: "proxied lan client, lan guard", remoteAddr: proxyPeer, method: get, target: apiV1Libraries, headers: map[string]string{forwardedFor: lanClient}, bearer: userToken, lanOnly: true, wantStatus: http.StatusOK},
		{name: "client-supplied lan prefix, manage", remoteAddr: proxyPeer, method: post, target: apiV1Libraries, headers: map[string]string{forwardedFor: lanClient + ", " + publicClient}, bearer: adminToken, wantStatus: http.StatusForbidden},
		{name: "proxy without the header, setup", remoteAddr: proxyPeer, method: post, target: apiV1Setup, setup: true, wantStatus: http.StatusForbidden},
		{name: "proxy without the header, manage", remoteAddr: proxyPeer, method: post, target: apiV1Libraries, bearer: adminToken, wantStatus: http.StatusForbidden},
		{name: "proxy without the header, lan guard", remoteAddr: proxyPeer, method: get, target: apiV1Libraries, bearer: userToken, lanOnly: true, wantStatus: http.StatusForbidden},
		{name: "proxy with unknown, manage", remoteAddr: proxyPeer, method: post, target: apiV1Libraries, headers: map[string]string{forwardedFor: "unknown"}, bearer: adminToken, wantStatus: http.StatusForbidden},
		{name: "proxy with real-ip only, setup", remoteAddr: proxyPeer, method: post, target: apiV1Setup, headers: map[string]string{"X-Real-IP": lanClient, "Forwarded": "for=" + lanClient}, setup: true, wantStatus: http.StatusForbidden},
		{name: "health through a proxy without the header", remoteAddr: proxyPeer, method: get, target: "/api/v1/health", lanOnly: true, wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			options, _ := proxyChainOptions(test, viewerEcho())

			recorder := serveChain(t, options, newProxyChainRequest(t, test))
			if recorder.Code != test.wantStatus {
				t.Errorf("%s %s = %d %s, want %d", test.method, test.target, recorder.Code, recorder.Body, test.wantStatus)
			}
		})
	}
}

// TestProxyHandlersGetTheResolvedPeerAndScheme: the sign-in throttle address, the device's
// address class and the Secure attribute of the session cookie come from the resolved origin.
func TestProxyHandlersGetTheResolvedPeerAndScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		remoteAddr string
		headers    map[string]string
		wantPeer   auth.Peer
		wantSecure bool
	}{
		{
			name: "proxied public client over https", remoteAddr: proxyPeer,
			headers:  map[string]string{forwardedFor: publicClient, forwardedProto: "https"},
			wantPeer: auth.Peer{Address: netip.MustParseAddr(publicClient), Class: auth.AddressRemote}, wantSecure: true,
		},
		{
			name: "proxied lan client over http", remoteAddr: proxyPeer,
			headers:  map[string]string{forwardedFor: lanClient},
			wantPeer: auth.Peer{Address: netip.MustParseAddr(lanClient), Class: auth.AddressLAN},
		},
		{
			name: "proxy without the header keeps its address and is remote", remoteAddr: proxyPeer,
			headers:  map[string]string{forwardedProto: "https"},
			wantPeer: auth.Peer{Address: netip.MustParseAddr(proxyAddress), Class: auth.AddressRemote}, wantSecure: true,
		},
		{
			name: "spoofing public peer gets no secure cookie", remoteAddr: publicPeer,
			headers:  map[string]string{forwardedFor: lanClient, forwardedProto: "https"},
			wantPeer: auth.Peer{Address: netip.MustParseAddr(publicClient), Class: auth.AddressRemote},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var (
				gotPeer   auth.Peer
				gotOrigin clientorigin.Origin
			)

			handler := http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
				gotPeer = restapi.PeerFrom(request.Context())
				gotOrigin = clientorigin.From(request.Context())
			})

			chainCase := proxyChainCase{remoteAddr: test.remoteAddr, method: http.MethodPost, target: apiV1AuthSession, headers: test.headers}
			options, _ := proxyChainOptions(chainCase, handler)

			recorder := serveChain(t, options, newProxyChainRequest(t, chainCase))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d %s, want 200", recorder.Code, recorder.Body)
			}

			if gotPeer != test.wantPeer {
				t.Errorf("peer = %+v, want %+v", gotPeer, test.wantPeer)
			}

			if gotOrigin.Secure != test.wantSecure {
				t.Errorf("Secure = %t, want %t", gotOrigin.Secure, test.wantSecure)
			}
		})
	}
}

// TestSameOriginUsesTheResolvedSchemeAndTheHostHeader: behind a TLS-terminating trusted
// proxy the browser's Origin is https; X-Forwarded-Host never replaces Host.
func TestSameOriginUsesTheResolvedSchemeAndTheHostHeader(t *testing.T) {
	t.Parallel()

	const publicHost = "media.example"

	viaTLSProxy := func(origin string, extra map[string]string) map[string]string {
		headers := map[string]string{forwardedFor: publicClient, forwardedProto: "https", headerOrigin: origin}
		for name, value := range extra {
			headers[name] = value
		}

		return headers
	}

	tests := []proxyChainCase{
		{name: "https origin behind a tls proxy", remoteAddr: proxyPeer, headers: viaTLSProxy("https://"+publicHost, nil), wantStatus: http.StatusOK},
		{name: "http origin behind a tls proxy", remoteAddr: proxyPeer, headers: viaTLSProxy("http://"+publicHost, nil), wantStatus: http.StatusForbidden},
		{
			name: "http origin behind a plain proxy", remoteAddr: proxyPeer,
			headers: map[string]string{forwardedFor: publicClient, headerOrigin: "http://" + publicHost}, wantStatus: http.StatusOK,
		},
		{
			name: "https origin from an untrusted peer claiming https", remoteAddr: lanPeer,
			headers: viaTLSProxy("https://"+publicHost, nil), wantStatus: http.StatusForbidden,
		},
		{
			name: "forwarded host does not replace the host", remoteAddr: proxyPeer,
			headers:    viaTLSProxy("https://evil.example", map[string]string{"X-Forwarded-Host": "evil.example"}),
			wantStatus: http.StatusForbidden,
		},
		{
			name: "differing forwarded host leaves a matching origin alone", remoteAddr: proxyPeer,
			headers:    viaTLSProxy("https://"+publicHost, map[string]string{"X-Forwarded-Host": "evil.example"}),
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			test.method, test.target = http.MethodPost, apiV1AuthSession
			options, _ := proxyChainOptions(test, viewerEcho())

			request := newProxyChainRequest(t, test)
			request.Host = publicHost

			recorder := serveChain(t, options, request)
			if recorder.Code != test.wantStatus {
				t.Errorf("sign-in with %v = %d %s, want %d", test.headers, recorder.Code, recorder.Body, test.wantStatus)
			}
		})
	}
}

// TestProxyGuardLogsNameClientAndProxy: a refusal logs the resolved client and the proxy,
// never a header value verbatim.
func TestProxyGuardLogsNameClientAndProxy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		header   string
		lanOnly  bool
		want     []string
		wantNone []string
	}{
		{
			name: "manage guard", header: "evil-left-entry, " + publicClient + ":4711",
			want:     []string{"remote_address=" + publicClient, "proxy_address=" + proxyAddress},
			wantNone: []string{"evil-left-entry", ":4711"},
		},
		{
			name: "lan guard", header: publicClient, lanOnly: true,
			want: []string{refusedMessage, "remote_address=" + publicClient, "proxy_address=" + proxyAddress},
		},
		{
			name: "fail closed logs the socket only", header: "evil-entry",
			want: []string{"remote_address=" + proxyPeer}, wantNone: []string{"evil-entry", "proxy_address="},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			chainCase := proxyChainCase{
				remoteAddr: proxyPeer, method: http.MethodPost, target: apiV1Libraries, bearer: adminToken,
				headers: map[string]string{forwardedFor: test.header}, lanOnly: test.lanOnly,
			}
			options, logs := proxyChainOptions(chainCase, viewerEcho())

			if recorder := serveChain(t, options, newProxyChainRequest(t, chainCase)); recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403", recorder.Code)
			}

			assertLogParts(t, logs.String(), test.want, test.wantNone)
		})
	}
}

// assertLogParts checks that logs contains every wanted part and none of the unwanted ones.
func assertLogParts(t *testing.T, logs string, wanted, unwanted []string) {
	t.Helper()

	for _, want := range wanted {
		if !strings.Contains(logs, want) {
			t.Errorf("log %q lacks %q", logs, want)
		}
	}

	for _, part := range unwanted {
		if strings.Contains(logs, part) {
			t.Errorf("log %q contains %q", logs, part)
		}
	}
}

// TestProxyOverALoopbackConnection runs over a real socket with the loopback address as the
// trusted proxy: the client named in X-Forwarded-For decides, not the loopback peer.
func TestProxyOverALoopbackConnection(t *testing.T) {
	t.Parallel()

	logger, _ := newDebugLogger()
	options := Options{
		Logger: logger, APIHandler: viewerEcho(), AllowPublicClients: true,
		TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("::1/128")},
		Authentication: Authentication{Sessions: &fakeResolver{}, Setup: fakeSetup{required: true}},
	}

	testServer := httptest.NewServer(newTestChain(t, options))
	t.Cleanup(testServer.Close)

	tests := []struct {
		name       string
		target     string
		forwarded  string
		wantStatus int
	}{
		{name: "public client, setup", target: apiV1Setup, forwarded: publicClient, wantStatus: http.StatusForbidden},
		{name: "public client, manage route", target: apiV1Libraries, forwarded: publicClient, wantStatus: http.StatusForbidden},
		{name: "no header, setup", target: apiV1Setup, wantStatus: http.StatusForbidden},
		{name: "lan client, setup", target: apiV1Setup, forwarded: lanClient, wantStatus: http.StatusOK},
		{name: "lan client, manage route", target: apiV1Libraries, forwarded: lanClient, wantStatus: http.StatusOK},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := postOverLoopback(t, testServer, test.target, test.forwarded)
			if status != test.wantStatus {
				t.Errorf("POST %s forwarded for %q = %d, want %d", test.target, test.forwarded, status, test.wantStatus)
			}
		})
	}
}

// postOverLoopback sends an admin's JSON POST, with X-Forwarded-For when forwarded is set.
func postOverLoopback(t *testing.T, testServer *httptest.Server, target, forwarded string) int {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, testServer.URL+target, strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Content-Type", jsonType)
	request.Header.Set("Authorization", "Bearer "+adminToken)

	if forwarded != "" {
		request.Header.Set(forwardedFor, forwarded)
	}

	response, err := testServer.Client().Do(request)
	if err != nil {
		t.Fatalf("POST over loopback: %v", err)
	}

	if closeErr := response.Body.Close(); closeErr != nil {
		t.Logf("close body: %v", closeErr)
	}

	return response.StatusCode
}

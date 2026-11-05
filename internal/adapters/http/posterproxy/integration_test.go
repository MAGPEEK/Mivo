package posterproxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/posterproxy"
	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
)

const loopbackHost = "127.0.0.1"

type loopbackResolver struct{}

func (loopbackResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr(loopbackHost)}, nil
}

// upstream stands in for the TMDB image host and records the headers it receives.
type upstream struct {
	*httptest.Server

	mutex   sync.Mutex
	headers []http.Header
}

func newUpstream(t *testing.T, body []byte) *upstream {
	t.Helper()

	server := &upstream{}
	server.Server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		server.mutex.Lock()
		server.headers = append(server.headers, request.Header.Clone())
		server.mutex.Unlock()

		_, _ = writer.Write(body)
	}))
	t.Cleanup(server.Close)

	return server
}

// TestPosterProxyForwardsNothingOfTheBrowserRequest wires the proxy like production: the
// artwork fetcher's preview through the hardened outbound client against a stand-in TMDB
// image host. The browser's cookie, authorization and referrer never reach the provider.
func TestPosterProxyForwardsNothingOfTheBrowserRequest(t *testing.T) {
	t.Parallel()

	body := readImage(t, "poster.jpg")
	server := newUpstream(t, body)
	base, _ := url.Parse(server.URL)

	fetcher, err := artworkfetch.New(artworkfetch.Options{
		DataDir: t.TempDir(),
		TMDB: artworkfetch.Provider{
			Client: outbound.New(outbound.Options{
				AllowedHosts: []string{loopbackHost}, MaxAttempts: 1, AllowInsecureLoopback: true, Resolver: loopbackResolver{},
			}),
			ImageURL:     func(ref enrich.ArtworkRef) (string, error) { return base.String() + "/t/p/w780" + ref.Path, nil },
			AllowedHosts: []string{loopbackHost},
		},
		AllowInsecureLoopback: true,
	})
	if err != nil {
		t.Fatalf("artworkfetch.New: %v", err)
	}

	proxy, handler, _ := newProxy(t, posterproxy.Options{Source: fetcher})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, proxy.PosterURL(poster("/a.jpg")), nil)
	request.Header.Set("Cookie", "session=browser-secret")
	request.Header.Set("Authorization", "Bearer browser-secret")
	request.Header.Set("Referer", "http://mivo.local:8460/items/x")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK || recorder.Body.Len() != len(body) {
		t.Fatalf("answer = %d (%d bytes), want the picture", recorder.Code, recorder.Body.Len())
	}

	server.mutex.Lock()
	defer server.mutex.Unlock()

	if len(server.headers) != 1 {
		t.Fatalf("upstream requests = %d, want 1", len(server.headers))
	}

	for _, name := range []string{"Cookie", "Authorization", "Referer"} {
		if value := server.headers[0].Get(name); value != "" {
			t.Errorf("upstream received %s: %q", name, value)
		}
	}
}

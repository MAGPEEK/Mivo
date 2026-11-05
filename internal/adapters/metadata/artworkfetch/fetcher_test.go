package artworkfetch_test

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	fixtureDir   = "../../../../testdata/providers/images"
	loopbackHost = "127.0.0.1"
	// archiveHost stands in for a Cover Art Archive redirect target; the test resolver maps
	// it to loopback.
	archiveHost = "ia800.archive.org"
	// foreignHost is a second name for the loopback server that no allow-list contains.
	foreignHost = "localhost"
)

// loopbackResolver resolves every name to 127.0.0.1, so redirects to *.archive.org reach
// the test server without any network.
type loopbackResolver struct{}

func (loopbackResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr(loopbackHost)}, nil
}

// fixtureServer serves routes and counts requests per path.
type fixtureServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newFixtureServer(t *testing.T, routes map[string]http.HandlerFunc) *fixtureServer {
	t.Helper()

	server := &fixtureServer{hits: map[string]int{}}
	server.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.mu.Lock()
		server.hits[r.URL.Path]++
		server.mu.Unlock()

		handler, ok := routes[r.URL.Path]
		if !ok {
			http.NotFound(w, r)

			return
		}

		handler(w, r)
	}))
	t.Cleanup(server.Close)

	return server
}

func (server *fixtureServer) hitCount(path string) int {
	server.mu.Lock()
	defer server.mu.Unlock()

	return server.hits[path]
}

// url returns the server URL for path using host instead of 127.0.0.1.
func (server *fixtureServer) url(host, path string) string {
	parsed, _ := url.Parse(server.URL)

	return "http://" + host + ":" + parsed.Port() + path
}

func serveBytes(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		// Lie about the type: the fetcher must sniff the bytes itself.
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(body)
	}
}

func redirectTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target, http.StatusFound)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS(fixtureDir), name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return data
}

type harness struct {
	fetcher *artworkfetch.Fetcher
	server  *fixtureServer
	dataDir string
}

// newHarness wires the fetcher like production (one outbound client per provider with the
// ADR 0031 redirect rules) but against the loopback server over plain HTTP.
func newHarness(t *testing.T, routes map[string]http.HandlerFunc) harness {
	t.Helper()

	server := newFixtureServer(t, routes)
	dataDir := t.TempDir()
	clientFor := func(redirectHosts []string) *outbound.Client {
		return outbound.New(outbound.Options{
			AllowedHosts:          []string{loopbackHost},
			RedirectHosts:         redirectHosts,
			MaxAttempts:           1,
			AllowInsecureLoopback: true,
			Resolver:              loopbackResolver{},
		})
	}

	fetcher, err := artworkfetch.New(artworkfetch.Options{
		DataDir: dataDir,
		TMDB: artworkfetch.Provider{
			Client:       clientFor([]string{loopbackHost}),
			ImageURL:     tmdbStyleURL(server.URL),
			AllowedHosts: []string{loopbackHost},
		},
		CoverArt: artworkfetch.Provider{
			Client:       clientFor(artworkfetch.CoverArtRedirectHosts()),
			AllowedHosts: []string{loopbackHost},
		},
		AllowInsecureLoopback: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return harness{fetcher: fetcher, server: server, dataDir: dataDir}
}

// tmdbStyleURL stands in for the TMDB adapter's ImageURL: backdrops at w1280, everything
// else at original size, below base.
func tmdbStyleURL(base string) artworkfetch.URLBuilder {
	return func(ref enrich.ArtworkRef) (string, error) {
		size := "original"
		if ref.Kind == artwork.KindBackdrop {
			size = "w1280"
		}

		return base + "/t/p/" + size + ref.Path, nil
	}
}

func tmdbRef(path string) enrich.ArtworkRef {
	return enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: path}
}

func coverRef(rawURL string) enrich.ArtworkRef {
	return enrich.ArtworkRef{Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster, Path: rawURL}
}

func (h harness) artworkFiles(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(h.dataDir, artworkfetch.ArtworkDirectory))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		t.Fatalf("read artwork directory: %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func (h harness) requireNoFiles(t *testing.T) {
	t.Helper()

	if files := h.artworkFiles(t); len(files) != 0 {
		t.Fatalf("artwork directory = %v, want empty", files)
	}
}

func (h harness) requireStored(t *testing.T, stored enrich.Stored, extension string, want []byte) {
	t.Helper()

	name := strings.TrimPrefix(stored.DataPath, artworkfetch.ArtworkDirectory+"/")
	tag, gotExtension, _ := strings.Cut(name, ".")

	if len(tag) != artwork.TagLength || gotExtension != extension || !filepath.IsLocal(stored.DataPath) {
		t.Fatalf("DataPath = %q, want artwork/<%d hex>.%s", stored.DataPath, artwork.TagLength, extension)
	}

	data, err := os.ReadFile(filepath.Join(h.dataDir, filepath.FromSlash(stored.DataPath)))
	if err != nil {
		t.Fatalf("read stored picture: %v", err)
	}

	if string(data) != string(want) || stored.Size != int64(len(want)) || stored.ModifiedAt.IsZero() {
		t.Fatalf("stored %d bytes (Size %d, ModifiedAt %v), want %d bytes", len(data), stored.Size, stored.ModifiedAt, len(want))
	}
}

package artworkfetch_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestFetchStoresEachFormat(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ fixture, extension string }{
		{"poster.jpg", "jpg"}, {"poster.png", "png"}, {"poster.webp", "webp"},
	} {
		t.Run(test.extension, func(t *testing.T) {
			t.Parallel()

			body := readFixture(t, test.fixture)
			h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/abc.jpg": serveBytes(body)})

			stored, err := h.fetcher.Fetch(t.Context(), tmdbRef("/abc.jpg"))
			if err != nil {
				t.Fatalf("Fetch: %v", err)
			}

			h.requireStored(t, stored, test.extension, body)
		})
	}
}

func TestFetchTwiceDownloadsOnce(t *testing.T) {
	t.Parallel()

	body := readFixture(t, "poster.png")
	h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/abc.jpg": serveBytes(body)})

	first, err := h.fetcher.Fetch(t.Context(), tmdbRef("/abc.jpg"))
	if err != nil {
		t.Fatalf("first Fetch: %v", err)
	}

	second, err := h.fetcher.Fetch(t.Context(), tmdbRef("/abc.jpg"))
	if err != nil {
		t.Fatalf("second Fetch: %v", err)
	}

	if first != second {
		t.Fatalf("second Fetch = %+v, want %+v", second, first)
	}

	if hits := h.server.hitCount("/t/p/original/abc.jpg"); hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}
}

func TestFetchConcurrentSameRefDownloadsOnce(t *testing.T) {
	t.Parallel()

	const workers = 16

	body := readFixture(t, "poster.jpg")
	h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/same.jpg": serveBytes(body)})

	results := make([]enrich.Stored, workers)
	errs := make([]error, workers)

	var group sync.WaitGroup
	for worker := range workers {
		group.Go(func() {
			results[worker], errs[worker] = h.fetcher.Fetch(context.Background(), tmdbRef("/same.jpg"))
		})
	}

	group.Wait()

	for worker := range workers {
		if errs[worker] != nil || results[worker] != results[0] {
			t.Fatalf("worker %d: %+v, %v; want %+v", worker, results[worker], errs[worker], results[0])
		}
	}

	if hits := h.server.hitCount("/t/p/original/same.jpg"); hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}

	if files := h.artworkFiles(t); len(files) != 1 {
		t.Fatalf("artwork files = %v, want exactly one (no temp leftovers)", files)
	}
}

func TestFetchRejectsBadPictures(t *testing.T) {
	t.Parallel()

	jpegBytes := readFixture(t, "poster.jpg")
	oversized := append(append([]byte{}, jpegBytes...), make([]byte, artworkfetch.MaxImageBytes)...)

	tests := []struct {
		name string
		body []byte
	}{
		{"wrong type", readFixture(t, "not-an-image.gif")},
		{"oversized", oversized},
		{"truncated", jpegBytes[:len(jpegBytes)-10]},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/bad.jpg": serveBytes(test.body)})

			_, err := h.fetcher.Fetch(t.Context(), tmdbRef("/bad.jpg"))
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Fetch error = %v, want ErrInvalid", err)
			}

			h.requireNoFiles(t)
		})
	}
}

func TestFetchCoverArtRedirects(t *testing.T) {
	t.Parallel()

	body := readFixture(t, "poster.jpg")
	routes := map[string]http.HandlerFunc{"/image.jpg": serveBytes(body)}
	h := newHarness(t, routes)
	routes["/release/abc/front"] = redirectTo(h.server.url(archiveHost, "/image.jpg"))
	routes["/release/abc/back"] = redirectTo(h.server.url(foreignHost, "/image.jpg"))

	stored, err := h.fetcher.Fetch(t.Context(), coverRef(h.server.URL+"/release/abc/front"))
	if err != nil {
		t.Fatalf("Fetch via *.archive.org redirect: %v", err)
	}

	h.requireStored(t, stored, "jpg", body)

	_, err = h.fetcher.Fetch(t.Context(), coverRef(h.server.URL+"/release/abc/back"))
	if !errors.Is(err, outbound.ErrHostNotAllowed) {
		t.Fatalf("Fetch via foreign redirect error = %v, want ErrHostNotAllowed", err)
	}

	if hits := h.server.hitCount("/image.jpg"); hits != 1 {
		t.Fatalf("image hits = %d, want 1 (foreign redirect must not be followed)", hits)
	}
}

func TestFetchTMDBRefusesRedirectToArchive(t *testing.T) {
	t.Parallel()

	routes := map[string]http.HandlerFunc{"/image.jpg": serveBytes(readFixture(t, "poster.jpg"))}
	h := newHarness(t, routes)
	routes["/t/p/original/moved.jpg"] = redirectTo(h.server.url(archiveHost, "/image.jpg"))

	_, err := h.fetcher.Fetch(t.Context(), tmdbRef("/moved.jpg"))
	if !errors.Is(err, outbound.ErrHostNotAllowed) {
		t.Fatalf("Fetch error = %v, want ErrHostNotAllowed", err)
	}

	h.requireNoFiles(t)
}

func TestFetchRefusesBadReferences(t *testing.T) {
	t.Parallel()

	h := newHarness(t, nil)
	tests := []struct {
		name string
		ref  enrich.ArtworkRef
		want error
	}{
		{"tmdb traversal", tmdbRef("/../../../etc/passwd.jpg"), domain.ErrInvalid},
		{"tmdb nested", tmdbRef("/a/../../b.jpg"), domain.ErrInvalid},
		{"cover traversal", coverRef(h.server.URL + "/release/../../../etc/passwd"), domain.ErrInvalid},
		{"cover encoded traversal", coverRef(h.server.URL + "/release/%2e%2e/%2e%2e/secret"), domain.ErrInvalid},
		{"cover foreign host", coverRef(h.server.url(foreignHost, "/release/abc/front")), domain.ErrForbidden},
		{"cover https elsewhere", coverRef("https://evil.example/x.jpg"), domain.ErrForbidden},
		{"unknown provider", enrich.ArtworkRef{Provider: "fanart", Path: "/a.jpg"}, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := h.fetcher.Fetch(t.Context(), test.ref)
			if !errors.Is(err, test.want) {
				t.Fatalf("Fetch error = %v, want %v", err, test.want)
			}
		})
	}

	h.requireNoFiles(t)
}

func TestFetchMapsServerErrors(t *testing.T) {
	t.Parallel()

	h := newHarness(t, map[string]http.HandlerFunc{
		"/t/p/original/gone.jpg":   func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusGone) },
		"/t/p/original/broken.jpg": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
	})

	tests := map[string]error{
		"/missing.jpg": domain.ErrNotFound,
		"/gone.jpg":    domain.ErrNotFound,
		"/broken.jpg":  domain.ErrUnavailable,
	}

	for path, want := range tests {
		if _, err := h.fetcher.Fetch(t.Context(), tmdbRef(path)); !errors.Is(err, want) {
			t.Errorf("Fetch(%s) error = %v, want %v", path, err, want)
		}
	}

	h.requireNoFiles(t)
}

func TestFetchDisabledProvider(t *testing.T) {
	t.Parallel()

	fetcher, err := artworkfetch.New(artworkfetch.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = fetcher.Fetch(t.Context(), tmdbRef("/a.jpg"))
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Fetch error = %v, want ErrUnavailable", err)
	}
}

func TestFetchTMDBWithoutURLBuilderIsUnavailable(t *testing.T) {
	t.Parallel()

	h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/a.jpg": serveBytes(readFixture(t, "poster.jpg"))})
	fetcher, err := artworkfetch.New(artworkfetch.Options{
		DataDir:               h.dataDir,
		TMDB:                  artworkfetch.Provider{Client: outbound.New(outbound.Options{}), AllowedHosts: []string{loopbackHost}},
		AllowInsecureLoopback: true,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := fetcher.Fetch(t.Context(), tmdbRef("/a.jpg")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("Fetch error = %v, want ErrUnavailable", err)
	}

	if hits := h.server.hitCount("/t/p/original/a.jpg"); hits != 0 {
		t.Fatalf("server hits = %d, want 0", hits)
	}
}

func TestFetchUsesURLBuilderPerKind(t *testing.T) {
	t.Parallel()

	posterBytes := readFixture(t, "poster.jpg")
	backdropBytes := readFixture(t, "poster.png")
	h := newHarness(t, map[string]http.HandlerFunc{
		"/t/p/original/same.jpg": serveBytes(posterBytes),
		"/t/p/w1280/same.jpg":    serveBytes(backdropBytes),
	})

	backdropRef := tmdbRef("/same.jpg")
	backdropRef.Kind = artwork.KindBackdrop

	poster, errPoster := h.fetcher.Fetch(t.Context(), tmdbRef("/same.jpg"))
	backdrop, errBackdrop := h.fetcher.Fetch(t.Context(), backdropRef)

	if err := errors.Join(errPoster, errBackdrop); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	h.requireStored(t, poster, "jpg", posterBytes)
	h.requireStored(t, backdrop, "png", backdropBytes)
}

func TestFetchChecksBuiltURL(t *testing.T) {
	t.Parallel()

	builderErr := errors.New("no size for kind")
	tests := []struct {
		name    string
		builder artworkfetch.URLBuilder
		want    error
	}{
		{"builder error", func(enrich.ArtworkRef) (string, error) { return "", builderErr }, builderErr},
		{"foreign host", func(ref enrich.ArtworkRef) (string, error) {
			return "https://evil.example/t/p/original" + ref.Path, nil
		}, domain.ErrForbidden},
		{"dot segments", func(enrich.ArtworkRef) (string, error) { return "http://127.0.0.1/t/../../secret.jpg", nil }, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fetcher, err := artworkfetch.New(artworkfetch.Options{
				DataDir:               t.TempDir(),
				TMDB:                  artworkfetch.Provider{Client: outbound.New(outbound.Options{}), ImageURL: test.builder, AllowedHosts: []string{loopbackHost}},
				AllowInsecureLoopback: true,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}

			if _, err := fetcher.Fetch(t.Context(), tmdbRef("/a.jpg")); !errors.Is(err, test.want) {
				t.Fatalf("Fetch error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestFetchCancelledContext(t *testing.T) {
	t.Parallel()

	h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/a.jpg": serveBytes(readFixture(t, "poster.jpg"))})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := h.fetcher.Fetch(ctx, tmdbRef("/a.jpg")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Fetch error = %v, want context.Canceled", err)
	}

	h.requireNoFiles(t)
}

func TestNewRequiresAbsoluteDataDir(t *testing.T) {
	t.Parallel()

	if _, err := artworkfetch.New(artworkfetch.Options{DataDir: "relative/data"}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("New error = %v, want ErrInvalid", err)
	}
}

func TestRemoveDeletesStoredPictures(t *testing.T) {
	t.Parallel()

	h := newHarness(t, map[string]http.HandlerFunc{
		"/t/p/original/a.jpg": serveBytes(readFixture(t, "poster.jpg")),
		"/t/p/original/b.jpg": serveBytes(readFixture(t, "poster.png")),
	})

	first, errFirst := h.fetcher.Fetch(t.Context(), tmdbRef("/a.jpg"))
	second, errSecond := h.fetcher.Fetch(t.Context(), tmdbRef("/b.jpg"))

	if err := errors.Join(errFirst, errSecond); err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if err := h.fetcher.Remove(t.Context(), first.DataPath, second.DataPath); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	h.requireNoFiles(t)

	if err := h.fetcher.Remove(t.Context(), first.DataPath); err != nil {
		t.Fatalf("Remove of a missing picture: %v", err)
	}
}

func TestRemoveRefusesForeignPaths(t *testing.T) {
	t.Parallel()

	dataDir := t.TempDir()
	keep := filepath.Join(dataDir, "mivo.db")

	if err := os.WriteFile(keep, []byte("db"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	fetcher, err := artworkfetch.New(artworkfetch.Options{DataDir: dataDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	for _, dataPath := range []string{"mivo.db", "artwork/../mivo.db", "../outside.jpg"} {
		if err := fetcher.Remove(t.Context(), dataPath); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("Remove(%q) error = %v, want ErrInvalid", dataPath, err)
		}
	}

	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("unrelated file was touched: %v", err)
	}
}

func TestRemoveWithoutDataDirectoryAndCancelled(t *testing.T) {
	t.Parallel()

	fetcher, err := artworkfetch.New(artworkfetch.Options{DataDir: filepath.Join(t.TempDir(), "missing")})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if err := fetcher.Remove(t.Context(), "artwork/0123456789abcdef.jpg"); err != nil {
		t.Fatalf("Remove without data directory: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := fetcher.Remove(ctx, "artwork/0123456789abcdef.jpg"); !errors.Is(err, context.Canceled) {
		t.Fatalf("Remove error = %v, want context.Canceled", err)
	}
}

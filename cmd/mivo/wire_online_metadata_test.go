package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/metadata/coverart"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	appimages "github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// TestImageServiceReadsTheFetchersArtworkDirectory: the image endpoint confines provider
// pictures to the folder the artwork fetcher writes.
func TestImageServiceReadsTheFetchersArtworkDirectory(t *testing.T) {
	t.Parallel()

	if appimages.DataArtworkDirectory != artworkfetch.ArtworkDirectory {
		t.Errorf("images reads %q, the fetcher writes %q", appimages.DataArtworkDirectory, artworkfetch.ArtworkDirectory)
	}
}

// loopbackResolver resolves every name to 127.0.0.1, so the real provider host names of the
// wiring can be served by one test server that tells them apart by the Host header.
type loopbackResolver struct{}

func (loopbackResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
}

// coverArtMirror plays coverartarchive.org, archive.org and an ia….us.archive.org mirror:
// the API answers 307 to archive.org/download, which answers 302 to the mirror (the real
// chain), and the mirror serves the listing JSON and the picture.
func coverArtMirror(t *testing.T) (port string) {
	t.Helper()

	picture := fakeJPEG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, _ := net.SplitHostPort(r.Host)
		_, port, _ := net.SplitHostPort(r.Host)

		switch host {
		case "coverartarchive.org":
			http.Redirect(w, r, "http://archive.org:"+port+"/download/mbid"+r.URL.Path, http.StatusTemporaryRedirect) //nolint:gosec // fake archive redirecting to itself
		case "archive.org":
			http.Redirect(w, r, "http://ia800.us.archive.org:"+port+r.URL.Path, http.StatusFound) //nolint:gosec // fake archive redirecting to itself
		case "ia800.us.archive.org":
			if strings.HasSuffix(r.URL.Path, ".jpg") {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(picture)

				return
			}

			writeFakeJSON(w, map[string]any{"images": []any{map[string]any{
				"approved": true, "front": true, "image": "https://coverartarchive.org/release/" + coverArtMBID + "/1.jpg",
			}}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	return serverURL.Port()
}

// coverArtMBID is a well-formed release ID for the fake archive.
const coverArtMBID = "76df3287-6cda-33eb-8e9a-044b5e15ffdd"

// TestCoverArtFollowsTheArchiveRedirectChain: both the cover listing (JSON) and the picture
// download follow coverartarchive.org → archive.org → ia….us.archive.org with the wiring's
// host lists.
func TestCoverArtFollowsTheArchiveRedirectChain(t *testing.T) {
	t.Parallel()

	port := coverArtMirror(t)
	app := application{
		logger: slog.New(slog.DiscardHandler),
		hooks:  runHooks{metadata: metadataHooks{allowInsecureLoopback: true, resolver: loopbackResolver{}}},
	}
	client := app.outboundClient(app.coverArtClientOptions())

	covers, err := coverart.New(coverart.Options{HTTP: client, BaseURL: "http://coverartarchive.org:" + port}).
		Covers(t.Context(), coverart.CoverRequest{ReleaseID: coverArtMBID})
	if err != nil || len(covers) == 0 {
		t.Fatalf("Covers() = %v, %v; want the listing through both redirects", covers, err)
	}

	fetcher, err := artworkfetch.New(artworkfetch.Options{
		DataDir:               t.TempDir(),
		CoverArt:              artworkfetch.Provider{Client: client, AllowedHosts: artworkfetch.CoverArtHosts()},
		AllowInsecureLoopback: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := fetcher.Fetch(t.Context(), enrich.ArtworkRef{
		Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster,
		Path: "http://coverartarchive.org:" + port + "/release/" + coverArtMBID + "/front.jpg",
	})
	if err != nil || stored.DataPath == "" {
		t.Fatalf("Fetch() = %+v, %v; want the picture through both redirects", stored, err)
	}
}

// TestProviderClientsRefuseRedirectsToForeignHosts checks the redirect restriction the wiring
// gives every provider client (ADR 0031 §1): a redirect may only lead to the provider's own
// hosts, here stood in for by the fake's address, never to another host.
func TestProviderClientsRefuseRedirectsToForeignHosts(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("answer"))
	}))
	t.Cleanup(target.Close)

	targetURL, err := url.Parse(target.URL)
	if err != nil {
		t.Fatal(err)
	}

	// "localhost" is loopback too, but no allowed redirect host.
	foreign := "http://localhost:" + targetURL.Port() + "/front.jpg"
	redirecting := httptest.NewServer(http.RedirectHandler(foreign, http.StatusFound))
	t.Cleanup(redirecting.Close)

	app := application{
		logger: slog.New(slog.DiscardHandler),
		hooks:  runHooks{metadata: metadataHooks{extraHosts: []string{"127.0.0.1"}, allowInsecureLoopback: true}},
	}

	clients := map[string]outbound.Options{
		"tmdb":        app.tmdbClientOptions(),
		"musicbrainz": app.musicBrainzClientOptions(),
		"coverart":    app.coverArtClientOptions(),
	}

	for name, options := range clients {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := app.outboundClient(options)

			if _, err := client.Get(t.Context(), target.URL+"/front.jpg", nil); err != nil {
				t.Fatalf("direct GET error = %v, want the allowed host served", err)
			}

			_, err := client.Get(t.Context(), redirecting.URL+"/release/x/front", nil)
			if !errors.Is(err, outbound.ErrHostNotAllowed) {
				t.Errorf("redirect to %s: error = %v, want ErrHostNotAllowed", foreign, err)
			}
		})
	}
}

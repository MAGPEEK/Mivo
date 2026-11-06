package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	appimages "github.com/magpeek/mivo/internal/app/images"
)

// TestImageServiceReadsTheFetchersArtworkDirectory: the image endpoint confines provider
// pictures to the folder the artwork fetcher writes.
func TestImageServiceReadsTheFetchersArtworkDirectory(t *testing.T) {
	t.Parallel()

	if appimages.DataArtworkDirectory != artworkfetch.ArtworkDirectory {
		t.Errorf("images reads %q, the fetcher writes %q", appimages.DataArtworkDirectory, artworkfetch.ArtworkDirectory)
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

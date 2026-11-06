package main

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/adapters/metadata/coverart"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
)

// TestCoverArtClientRefusesRedirectsToForeignHosts checks the redirect restriction the
// wiring gives the Cover Art Archive client (ADR 0031 §1): a redirect may only lead to the
// archive's hosts, here stood in for by the fake's own address.
func TestCoverArtClientRefusesRedirectsToForeignHosts(t *testing.T) {
	t.Parallel()

	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("picture"))
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
	client := app.outboundClient(outbound.Options{
		AllowedHosts:  app.allowedHosts(coverart.AllowedHosts()),
		RedirectHosts: app.allowedHosts(slices.Concat(coverart.RedirectHosts(), artworkfetch.CoverArtRedirectHosts())),
	})

	if _, err := client.Get(t.Context(), target.URL+"/front.jpg", nil); err != nil {
		t.Fatalf("direct GET error = %v, want the allowed host served", err)
	}

	_, err = client.Get(t.Context(), redirecting.URL+"/release/x/front", nil)
	if !errors.Is(err, outbound.ErrHostNotAllowed) {
		t.Errorf("redirect to %s: error = %v, want ErrHostNotAllowed", foreign, err)
	}
}

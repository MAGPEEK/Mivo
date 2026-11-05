package musicbrainz_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/metadata/musicbrainz"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		retryAfter string
		fixture    string
		want       error
		attempts   int
	}{
		{"503 with Retry-After is rate limiting", http.StatusServiceUnavailable, "1", "error_503.json", domain.ErrRateLimited, 2},
		{"503 without Retry-After", http.StatusServiceUnavailable, "", "error_503.json", domain.ErrUnavailable, 2},
		{"429", http.StatusTooManyRequests, "", "error_503.json", domain.ErrRateLimited, 2},
		{"500", http.StatusInternalServerError, "", "error_404.json", domain.ErrUnavailable, 1},
		{"400", http.StatusBadRequest, "", "error_404.json", domain.ErrUnavailable, 1},
		{"broken json", http.StatusOK, "", "broken.json", domain.ErrUnavailable, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFake(t, route{path: "/artist", status: test.status, retryAfter: test.retryAfter, fixture: test.fixture})

			_, err := newClient(fake, newFakeClock()).Search(t.Context(), enrich.MusicQuery{Kind: match.RefKindArtist, Title: "Radiohead"})
			if !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}

			if errors.Is(test.want, domain.ErrRateLimited) && errors.Is(err, domain.ErrUnavailable) {
				t.Error("rate limiting must not also read as unavailable")
			}

			if got := len(fake.recorded()); got != test.attempts {
				t.Errorf("attempts = %d, want %d", got, test.attempts)
			}
		})
	}
}

func TestPacerAllowsOneRequestPerSecond(t *testing.T) {
	fake := newFake(t, route{path: "/artist", fixture: "artist_search.json"})
	clock := newFakeClock()
	client := newClient(fake, clock)
	query := enrich.MusicQuery{Kind: match.RefKindArtist, Title: "Radiohead"}

	for range 3 {
		if _, err := client.Search(t.Context(), query); err != nil {
			t.Fatalf("Search: %v", err)
		}
	}

	if want := []time.Duration{time.Second, time.Second}; !slices.Equal(clock.pauses(), want) {
		t.Errorf("pauses = %v, want %v (first request immediately)", clock.pauses(), want)
	}

	clock.advance(musicbrainz.MinRequestInterval)

	if _, err := client.Search(t.Context(), query); err != nil {
		t.Fatalf("Search: %v", err)
	}

	if got := len(clock.pauses()); got != 2 {
		t.Errorf("pauses = %v, want no extra pause after a quiet second", clock.pauses())
	}
}

func TestPacerHonoursCancellation(t *testing.T) {
	fake := newFake(t, route{path: "/artist", fixture: "artist_search.json"})
	client := newClient(fake, newFakeClock())
	query := enrich.MusicQuery{Kind: match.RefKindArtist, Title: "Radiohead"}

	if _, err := client.Search(t.Context(), query); err != nil {
		t.Fatalf("Search: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := client.Search(ctx, query); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}

	if got := len(fake.recorded()); got != 1 {
		t.Errorf("requests = %d, want 1", got)
	}
}

func TestRealClockDefaults(t *testing.T) {
	fake := newFake(t, route{path: "/artist", fixture: "artist_search.json"})
	client := musicbrainz.New(musicbrainz.Options{HTTP: newOutbound(), BaseURL: fake.server.URL + apiPrefix + "/"})

	if _, err := client.Search(t.Context(), enrich.MusicQuery{Kind: match.RefKindArtist, Title: "Radiohead"}); err != nil {
		t.Fatalf("Search: %v", err)
	}
}

func TestWiringValues(t *testing.T) {
	if !slices.Equal(musicbrainz.AllowedHosts(), []string{"musicbrainz.org"}) {
		t.Errorf("AllowedHosts = %v", musicbrainz.AllowedHosts())
	}

	want := map[string]outbound.Rate{"musicbrainz.org": {PerSecond: 1, Burst: 1}}
	got := musicbrainz.HostRates()

	if len(got) != len(want) || got["musicbrainz.org"] != want["musicbrainz.org"] {
		t.Errorf("HostRates = %v, want %v", got, want)
	}
}

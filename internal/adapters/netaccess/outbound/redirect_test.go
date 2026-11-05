package outbound_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
)

// redirectPair starts origin A and other host B. B records whether a credential header ever
// reached it and serves "/end" with 200.
type redirectPair struct {
	origin, other *httptest.Server
	leaked        atomic.Bool
}

func newRedirectPair(t *testing.T, originRoutes, otherRoutes map[string]func() string) *redirectPair {
	t.Helper()

	pair := &redirectPair{}
	handler := func(routes map[string]func() string, record bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if record && r.Header.Get("X-Api-Key") != "" {
				pair.leaked.Store(true)
			}

			if next, ok := routes[r.URL.Path]; ok {
				http.Redirect(w, r, next(), http.StatusFound)
			}
		}
	}

	pair.origin = httptest.NewServer(handler(originRoutes, false))
	pair.other = httptest.NewServer(handler(otherRoutes, true))
	t.Cleanup(pair.origin.Close)
	t.Cleanup(pair.other.Close)

	return pair
}

func getWithKey(t *testing.T, client *outbound.Client, target string) error {
	t.Helper()

	_, err := client.Get(context.Background(), target, http.Header{"X-Api-Key": {"secret"}})

	return err
}

func TestCredentialHeaderNeverReachesOtherHostOnMultiHopRedirect(t *testing.T) {
	t.Parallel()

	var pair *redirectPair

	tests := map[string]struct {
		origin map[string]func() string
		other  map[string]func() string
	}{
		"A to B to B": {
			origin: map[string]func() string{"/start": func() string { return pair.other.URL + "/hop" }},
			other:  map[string]func() string{"/hop": func() string { return pair.other.URL + "/end" }},
		},
		"A to B to A": {
			origin: map[string]func() string{"/start": func() string { return pair.other.URL + "/hop" }},
			other:  map[string]func() string{"/hop": func() string { return pair.origin.URL + "/end" }},
		},
	}

	for name, tc := range tests {
		pair = newRedirectPair(t, tc.origin, tc.other)

		if err := getWithKey(t, testClient(newClock(), nil), pair.origin.URL+"/start"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if pair.leaked.Load() {
			t.Errorf("%s: X-Api-Key reached the other host", name)
		}
	}
}

func TestRedirectHosts(t *testing.T) {
	t.Parallel()

	var pair *redirectPair

	pair = newRedirectPair(t,
		map[string]func() string{
			"/listed":   func() string { return pair.other.URL + "/end" },
			"/unlisted": func() string { return strings.Replace(pair.other.URL, "127.0.0.1", "localhost", 1) },
			"/same":     func() string { return pair.origin.URL + "/end" },
		}, nil)

	client := testClient(newClock(), func(o *outbound.Options) {
		o.MaxAttempts = 1
		o.RedirectHosts = []string{"127.0.0.1"}
	})

	if err := getWithKey(t, client, pair.origin.URL+"/listed"); err != nil {
		t.Errorf("listed redirect host: %v", err)
	}

	if err := getWithKey(t, client, pair.origin.URL+"/same"); err != nil {
		t.Errorf("same-host redirect: %v", err)
	}

	if err := getWithKey(t, client, pair.origin.URL+"/unlisted"); !errors.Is(err, outbound.ErrHostNotAllowed) {
		t.Errorf("unlisted redirect host error = %v, want ErrHostNotAllowed", err)
	}
}

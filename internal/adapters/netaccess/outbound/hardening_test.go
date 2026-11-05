package outbound_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/domain"
)

type staticResolver struct{ addr string }

func (s staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return []netip.Addr{netip.MustParseAddr(s.addr)}, nil
}

func TestNonIdempotentMethodsAreNeverRetried(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		var calls atomic.Int32

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusServiceUnavailable)
		}))

		req, err := http.NewRequestWithContext(context.Background(), method, server.URL, strings.NewReader("x"))
		if err != nil {
			t.Fatal(err)
		}

		clock := newClock()
		if _, err := testClient(clock, nil).Do(context.Background(), req); !errors.Is(err, domain.ErrUnavailable) {
			t.Errorf("%s error = %v, want ErrUnavailable", method, err)
		}

		if calls.Load() != 1 || len(clock.sleeps) != 0 {
			t.Errorf("%s: calls=%d sleeps=%v, want a single attempt", method, calls.Load(), clock.sleeps)
		}

		server.Close()
	}
}

func TestHeadIsRetried(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
		}
	}))
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodHead, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := testClient(newClock(), nil).Do(context.Background(), req)
	if err != nil || resp.StatusCode != http.StatusOK || calls.Load() != 2 {
		t.Fatalf("HEAD = %v, %v after %d calls, want success on the second", resp, err, calls.Load())
	}
}

func TestAllowedHosts(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(server.Close) // parallel subtests outlive a deferred Close

	tests := []struct {
		name    string
		allowed []string
		url     string
		wantErr bool
	}{
		{"exact match", []string{"127.0.0.1"}, server.URL, false},
		{"other host refused", []string{"api.themoviedb.org"}, server.URL, true},
		{"suffix refuses the bare domain", []string{".archive.org"}, "http://archive.org/", true},
		{"non-ascii host refused", []string{"example.com"}, "http://bücher.example/", true},
		{"empty list allows any host", nil, server.URL, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := testClient(newClock(), func(o *outbound.Options) {
				o.AllowedHosts = tc.allowed
				o.MaxAttempts = 1
				o.Resolver = staticResolver{"10.0.0.1"} // proves refusals happen before any dial
				o.AllowInsecureLoopback = true
			})

			_, err := client.Get(context.Background(), tc.url, nil)
			if tc.wantErr && !errors.Is(err, outbound.ErrHostNotAllowed) {
				t.Errorf("error = %v, want ErrHostNotAllowed before dialing", err)
			}

			if !tc.wantErr && err != nil {
				t.Errorf("error = %v, want success", err)
			}
		})
	}
}

func TestAllowedSuffixAndNormalisation(t *testing.T) {
	t.Parallel()

	client := testClient(newClock(), func(o *outbound.Options) {
		o.AllowedHosts = []string{".Archive.org", "API.Example."}
		o.MaxAttempts = 1
		o.Resolver = staticResolver{"10.0.0.1"}
	})

	for _, target := range []string{"http://ia800.us.ARCHIVE.org/x", "http://api.example/", "http://Api.Example./"} {
		_, err := client.Get(context.Background(), target, nil)
		if !errors.Is(err, outbound.ErrBlockedAddress) {
			t.Errorf("%s: error = %v, want to pass the allow-list and stop at the SSRF guard", target, err)
		}
	}
}

func TestIdleHostsAreEvictedWhenTrackingIsFull(t *testing.T) {
	t.Parallel()

	client := testClient(newClock(), func(o *outbound.Options) {
		o.MaxAttempts = 1
		o.Resolver = staticResolver{"10.0.0.1"}
	})

	for i := range outbound.MaxTrackedHosts + 10 {
		_, err := client.Get(context.Background(), fmt.Sprintf("http://h%d.example/", i), nil)
		if !errors.Is(err, outbound.ErrBlockedAddress) {
			t.Fatalf("host %d: error = %v, want idle hosts evicted to make room", i, err)
		}
	}
}

func TestBusyHostsAreNotEvicted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	port := server.URL[strings.LastIndex(server.URL, ":")+1:]
	client := testClient(newClock(), func(o *outbound.Options) {
		o.MaxAttempts = 1
		o.BreakerThreshold = 100
		o.Resolver = staticResolver{"127.0.0.1"}
	})

	for i := range outbound.MaxTrackedHosts {
		_, err := client.Get(context.Background(), fmt.Sprintf("http://h%d.example:%s/", i, port), nil)
		if !errors.Is(err, domain.ErrUnavailable) {
			t.Fatalf("host %d: error = %v", i, err)
		}
	}

	_, err := client.Get(context.Background(), "http://one-too-many.example:"+port+"/", nil)
	if !errors.Is(err, outbound.ErrHostNotAllowed) {
		t.Errorf("error = %v, want ErrHostNotAllowed while every tracked host has failures", err)
	}

	if _, err := client.Get(context.Background(), "http://h0.example:"+port+"/", nil); !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("known host error = %v, want it still served", err)
	}
}

func TestPolicyErrorsDoNotTouchBreaker(t *testing.T) {
	t.Parallel()

	client := testClient(newClock(), func(o *outbound.Options) {
		o.MaxAttempts = 1
		o.BreakerThreshold = 1
		o.Resolver = staticResolver{"192.168.1.20"}
	})

	for range 3 {
		_, err := client.Get(context.Background(), "http://intranet.example/", nil)
		if !errors.Is(err, outbound.ErrBlockedAddress) {
			t.Fatalf("error = %v, want ErrBlockedAddress every time (breaker must stay closed)", err)
		}

		if strings.Contains(err.Error(), "192.168.1.20") {
			t.Errorf("error leaks the resolved address: %v", err)
		}
	}
}

func TestCrossHostRedirectStripsCustomHeaders(t *testing.T) {
	t.Parallel()

	var seen http.Header

	target := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = r.Header.Clone()
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer origin.Close()

	header := http.Header{}
	header.Set("X-Api-Key", "k")
	header.Set("X-Custom-Token", "t")
	header.Set("Accept", "application/json")

	if _, err := testClient(newClock(), nil).Get(context.Background(), origin.URL, header); err != nil {
		t.Fatal(err)
	}

	if seen.Get("X-Api-Key") != "" || seen.Get("X-Custom-Token") != "" {
		t.Errorf("credential headers followed a cross-host redirect: %v", seen)
	}

	if seen.Get("Accept") != "application/json" || seen.Get("User-Agent") == "" {
		t.Errorf("safe headers were dropped: %v", seen)
	}
}

func TestSameHostRedirectKeepsHeaders(t *testing.T) {
	t.Parallel()

	var seen http.Header

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)

			return
		}

		seen = r.Header.Clone()
	}))
	defer server.Close()

	header := http.Header{"X-Api-Key": {"k"}}
	if _, err := testClient(newClock(), nil).Get(context.Background(), server.URL+"/start", header); err != nil {
		t.Fatal(err)
	}

	if seen.Get("X-Api-Key") != "k" {
		t.Errorf("same-host redirect lost the header: %v", seen)
	}
}

func TestRetryAfterOverflowStopsRetrying(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "99999999999999999999")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	for _, value := range []string{"9223372036854775807", "99999999999"} {
		calls.Store(0)

		server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.Header().Set("Retry-After", value)
			w.WriteHeader(http.StatusTooManyRequests)
		})

		clock := newClock()
		_, err := testClient(clock, nil).Get(context.Background(), server.URL, nil)

		if !errors.Is(err, domain.ErrRateLimited) || calls.Load() != 1 || len(clock.sleeps) != 0 {
			t.Errorf("Retry-After %s: err=%v calls=%d sleeps=%v", value, err, calls.Load(), clock.sleeps)
		}
	}
}

func TestTotalTimeoutBoundsRetries(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := testClient(newClock(), func(o *outbound.Options) {
		o.TotalTimeout = 50 * time.Millisecond
		o.MaxAttempts = 10
		o.Sleep = func(ctx context.Context, _ time.Duration) error {
			<-ctx.Done()

			return ctx.Err()
		}
	})

	_, err := client.Get(context.Background(), server.URL, nil)
	if !errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("error = %v, want ErrUnavailable once the total deadline passes", err)
	}
}

func TestGetCopiesCallerHeaders(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	header := http.Header{"Accept": {"a"}}
	if _, err := testClient(newClock(), nil).Get(context.Background(), server.URL, header); err != nil {
		t.Fatal(err)
	}

	if len(header) != 1 || header.Get("User-Agent") != "" {
		t.Errorf("caller header was modified: %v", header)
	}
}

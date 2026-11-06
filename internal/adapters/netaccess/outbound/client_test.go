package outbound_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
	"github.com/magpeek/mivo/internal/domain"
)

type manualClock struct {
	now    time.Time
	sleeps []time.Duration
}

func (m *manualClock) Now() time.Time { return m.now }

func (m *manualClock) Sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	m.sleeps = append(m.sleeps, d)
	m.now = m.now.Add(d)

	return nil
}

func newClock() *manualClock {
	return &manualClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
}

func testClient(clock *manualClock, mutate func(*outbound.Options)) *outbound.Client {
	opts := outbound.Options{
		Version:               "1.2.3",
		AllowInsecureLoopback: true,
		Now:                   clock.Now,
		Sleep:                 clock.Sleep,
		Jitter:                func(d time.Duration) time.Duration { return d },
		BaseBackoff:           time.Second,
	}
	if mutate != nil {
		mutate(&opts)
	}

	return outbound.New(opts)
}

// redirectsTo lets redirects leave the original host for hosts (Options.RedirectHosts).
func redirectsTo(hosts ...string) func(*outbound.Options) {
	return func(o *outbound.Options) { o.RedirectHosts = hosts }
}

// TestRedirectsStayOnTheHostWithoutRedirectHosts: an empty RedirectHosts is the safe
// default – a redirect to the same host is followed, one to another host is refused.
func TestRedirectsStayOnTheHostWithoutRedirectHosts(t *testing.T) {
	t.Parallel()

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("other"))
	}))
	defer other.Close()

	var origin *httptest.Server

	origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/same":
			http.Redirect(w, r, origin.URL+"/final", http.StatusFound)
		case "/away":
			// localhost is loopback too, so only the host check can refuse it.
			http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1), http.StatusFound)
		default:
			_, _ = w.Write([]byte("done"))
		}
	}))
	defer origin.Close()

	client := testClient(newClock(), nil)

	if resp, err := client.Get(context.Background(), origin.URL+"/same", nil); err != nil || string(resp.Body) != "done" {
		t.Errorf("same-host redirect = %v, %v; want it followed", resp, err)
	}

	if _, err := client.Get(context.Background(), origin.URL+"/away", nil); !errors.Is(err, outbound.ErrHostNotAllowed) {
		t.Errorf("redirect to another host error = %v, want ErrHostNotAllowed", err)
	}
}

func TestGetSendsOnlyUserAgentAndNoCookies(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	var secondCookie atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "x", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		} else if len(r.Cookies()) > 0 {
			secondCookie.Store(true)
		}

		if got := r.Header.Get("User-Agent"); got != "Mivo/1.2.3 (+https://github.com/MAGPEEK/Mivo)" {
			t.Errorf("User-Agent = %q", got)
		}

		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := testClient(newClock(), nil)

	for range 2 {
		resp, err := client.Get(context.Background(), server.URL, nil)
		if err != nil || string(resp.Body) != "ok" {
			t.Fatalf("Get = %v, %v", resp, err)
		}
	}

	if secondCookie.Load() {
		t.Error("cookie was replayed: client must not keep a jar")
	}
}

func TestHTTPSOnlyWithoutOverride(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be contacted")
	}))
	defer server.Close()

	client := outbound.New(outbound.Options{})
	if _, err := client.Get(context.Background(), server.URL, nil); !errors.Is(err, outbound.ErrInsecureURL) {
		t.Fatalf("http url error = %v, want ErrInsecureURL", err)
	}
}

func TestLoopbackIsBlockedWithoutOverrideEvenOverHTTPS(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("server must not be contacted")
	}))
	defer server.Close()

	client := outbound.New(outbound.Options{MaxAttempts: 1})

	_, err := client.Get(context.Background(), server.URL, nil)
	if !errors.Is(err, outbound.ErrBlockedAddress) {
		t.Fatalf("loopback error = %v, want ErrBlockedAddress", err)
	}
}

func TestBodySizeCap(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", 11)))
	}))
	defer server.Close()

	client := testClient(newClock(), func(o *outbound.Options) { o.MaxBodyBytes = 10 })
	if _, err := client.Get(context.Background(), server.URL, nil); !errors.Is(err, outbound.ErrBodyTooLarge) {
		t.Fatalf("oversized body error = %v, want ErrBodyTooLarge", err)
	}

	small := testClient(newClock(), func(o *outbound.Options) { o.MaxBodyBytes = 11 })
	if resp, err := small.Get(context.Background(), server.URL, nil); err != nil || len(resp.Body) != 11 {
		t.Fatalf("body at the cap: %v, %v", resp, err)
	}
}

func TestRedirects(t *testing.T) {
	t.Parallel()

	var server *httptest.Server

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/final":
			_, _ = w.Write([]byte("done"))
		case "/private":
			http.Redirect(w, r, "http://10.0.0.1/secret", http.StatusFound)
		case "/ftp":
			http.Redirect(w, r, "ftp://example.com/x", http.StatusFound)
		default:
			http.Redirect(w, r, server.URL+r.URL.Path+"x", http.StatusFound) //nolint:gosec // test server redirecting to itself
		}
	}))
	defer server.Close()

	client := testClient(newClock(), func(o *outbound.Options) {
		o.MaxAttempts = 1
		o.RedirectHosts = []string{"10.0.0.1"}
	})

	if _, err := client.Get(context.Background(), server.URL+"/loop", nil); !errors.Is(err, outbound.ErrTooManyRedirects) {
		t.Errorf("redirect chain error = %v, want ErrTooManyRedirects", err)
	}

	if _, err := client.Get(context.Background(), server.URL+"/private", nil); !errors.Is(err, outbound.ErrBlockedAddress) {
		t.Errorf("redirect to private error = %v, want ErrBlockedAddress", err)
	}

	if _, err := client.Get(context.Background(), server.URL+"/ftp", nil); !errors.Is(err, outbound.ErrInsecureURL) {
		t.Errorf("redirect to ftp error = %v, want ErrInsecureURL", err)
	}
}

func TestRetryStatusesAndBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		wantCalls  int32
		wantErr    error
		wantSleeps []time.Duration
	}{
		{"503 retried then exhausted", 503, 3, domain.ErrUnavailable, []time.Duration{time.Second, 2 * time.Second}},
		{"502 retried", 502, 3, domain.ErrUnavailable, []time.Duration{time.Second, 2 * time.Second}},
		{"504 retried", 504, 3, domain.ErrUnavailable, []time.Duration{time.Second, 2 * time.Second}},
		{"429 maps to rate limited", 429, 3, domain.ErrRateLimited, []time.Duration{time.Second, 2 * time.Second}},
		{"404 not retried", 404, 1, nil, nil},
		{"400 not retried", 400, 1, nil, nil},
		{"500 not retried", 500, 1, nil, nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var calls atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
			}))
			defer server.Close()

			clock := newClock()
			resp, err := testClient(clock, nil).Get(context.Background(), server.URL, nil)

			if err == nil && tc.wantErr == nil && resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}

			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}

			if calls.Load() != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls.Load(), tc.wantCalls)
			}

			if !slices.Equal(clock.sleeps, tc.wantSleeps) {
				t.Errorf("sleeps = %v, want %v", clock.sleeps, tc.wantSleeps)
			}
		})
	}
}

func TestRetryRecoversAndHonoursRetryAfter(t *testing.T) {
	t.Parallel()

	clock := newClock()
	dateHeader := clock.now.Add(20 * time.Second).UTC().Format(http.TimeFormat)
	headers := []string{"7", dateHeader}

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := int(calls.Add(1))
		if n <= len(headers) {
			w.Header().Set("Retry-After", headers[n-1])
			w.WriteHeader(http.StatusTooManyRequests)

			return
		}

		_, _ = w.Write([]byte("fine"))
	}))
	defer server.Close()

	client := testClient(clock, func(o *outbound.Options) { o.MaxAttempts = 4 })

	resp, err := client.Get(context.Background(), server.URL, nil)
	if err != nil || string(resp.Body) != "fine" {
		t.Fatalf("Get = %v, %v", resp, err)
	}

	want := []time.Duration{7 * time.Second, 13 * time.Second} // the date is 20s out, 7s already slept
	for i := range want {
		if clock.sleeps[i] != want[i] {
			t.Errorf("sleep %d = %v, want %v", i, clock.sleeps[i], want[i])
		}
	}
}

func TestRetryAfterBeyondCapStopsRetrying(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "86400")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	clock := newClock()
	_, err := testClient(clock, nil).Get(context.Background(), server.URL, nil)

	if !errors.Is(err, domain.ErrRateLimited) || calls.Load() != 1 || len(clock.sleeps) != 0 {
		t.Errorf("err=%v calls=%d sleeps=%v, want ErrRateLimited after one call", err, calls.Load(), clock.sleeps)
	}

	// The enrichment queue reads the pause the server asked for (enrich.RetryAfterError).
	var rateLimit *outbound.RateLimitError
	if !errors.As(err, &rateLimit) || rateLimit.RetryAfter() != 24*time.Hour || strings.Contains(err.Error(), "%!") {
		t.Errorf("err = %v, want a *RateLimitError asking for 24h", err)
	}
}

func TestRateLimitWithoutRetryAfterCarriesNoPause(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, err := testClient(newClock(), func(o *outbound.Options) { o.MaxAttempts = 1 }).Get(context.Background(), server.URL, nil)

	var rateLimit *outbound.RateLimitError
	if !errors.Is(err, domain.ErrRateLimited) || errors.As(err, &rateLimit) {
		t.Errorf("err = %v, want ErrRateLimited without a pause", err)
	}
}

func TestNetworkErrorIsUnavailableAndRedactsKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	target := server.URL + "/search?query=x&api_key=SECRET123"
	server.Close()

	clock := newClock()
	_, err := testClient(clock, nil).Get(context.Background(), target, nil)

	if !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}

	if strings.Contains(err.Error(), "SECRET123") {
		t.Errorf("error leaks the api key: %v", err)
	}

	if len(clock.sleeps) != 2 {
		t.Errorf("sleeps = %v, want 2 backoff pauses", clock.sleeps)
	}
}

func TestStatusErrorRedactsKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	_, err := testClient(newClock(), nil).Get(context.Background(), server.URL+"/?api_key=SECRET123", nil)
	if err == nil || strings.Contains(err.Error(), "SECRET123") {
		t.Errorf("error = %v, want an error without the key", err)
	}
}

func TestCircuitBreakerOpensAndRecovers(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	var healthy atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)

		if !healthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()

	clock := newClock()
	client := testClient(clock, func(o *outbound.Options) {
		o.MaxAttempts = 1
		o.BreakerThreshold = 2
		o.BreakerCooldown = time.Minute
	})

	for range 2 {
		_, _ = client.Get(context.Background(), server.URL, nil)
	}

	before := calls.Load()

	_, err := client.Get(context.Background(), server.URL, nil)
	if !errors.Is(err, domain.ErrUnavailable) || calls.Load() != before {
		t.Fatalf("open breaker: err=%v, extra calls=%d, want immediate ErrUnavailable", err, calls.Load()-before)
	}

	healthy.Store(true)
	clock.now = clock.now.Add(time.Minute)

	if resp, err := client.Get(context.Background(), server.URL, nil); err != nil || resp.StatusCode != 200 {
		t.Fatalf("half-open probe: %v, %v", resp, err)
	}

	if _, err := client.Get(context.Background(), server.URL, nil); err != nil {
		t.Fatalf("breaker should be closed again: %v", err)
	}
}

func TestCancelledContextIsNotRetriedNorCounted(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()

	client := testClient(newClock(), func(o *outbound.Options) { o.BreakerThreshold = 1 })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.Get(ctx, server.URL, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Get error = %v, want context.Canceled", err)
	}

	if resp, err := client.Get(context.Background(), server.URL, nil); err != nil || resp.StatusCode != 200 {
		t.Fatalf("cancellation must not open the breaker: %v, %v", resp, err)
	}
}

func TestRateLimitDelaysRequests(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	clock := newClock()
	client := testClient(clock, func(o *outbound.Options) { o.DefaultRate = outbound.Rate{PerSecond: 2, Burst: 1} })

	for range 3 {
		if _, err := client.Get(context.Background(), server.URL, nil); err != nil {
			t.Fatal(err)
		}
	}

	if len(clock.sleeps) != 2 || clock.sleeps[0] != 500*time.Millisecond {
		t.Errorf("sleeps = %v, want two 500ms waits", clock.sleeps)
	}
}

func TestDoRejectsNonReplayableBody(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://127.0.0.1:1/",
		plainReader{strings.NewReader("data")})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testClient(newClock(), nil).Do(context.Background(), req); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

type plainReader struct{ *strings.Reader }

func TestRedactURL(t *testing.T) {
	t.Parallel()

	tests := map[string]string{ //nolint:gosec // fake credentials
		"https://api.example/3/search?query=a&api_key=abc": "https://api.example/3/search?api_key=REDACTED&query=a",
		"https://u:pw@api.example/x?Token=abc#frag":        "https://api.example/x?Token=REDACTED",
		"https://api.example/plain":                        "https://api.example/plain",
		"://bad":                                           "<invalid url>",
	}

	for in, want := range tests {
		if got := outbound.RedactURL(in); got != want {
			t.Errorf("RedactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUserAgentDefaultsVersion(t *testing.T) {
	t.Parallel()

	if ua := outbound.New(outbound.Options{}).UserAgent(); !strings.HasPrefix(ua, "Mivo/dev (+") {
		t.Errorf("UserAgent = %q", ua)
	}
}

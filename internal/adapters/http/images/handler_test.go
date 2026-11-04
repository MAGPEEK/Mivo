package images_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/images"
	appimages "github.com/magpeek/mivo/internal/app/images"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

const (
	imageID    = "7e6d5c4b-3a2f-4e1d-9c0b-a9f8e7d6c5b4"
	currentTag = "9f86d081884c7d65"
	imageETag  = `"` + currentTag + `-320"`
	imageURL   = "/api/v1/images/" + imageID + "?width=320&tag=" + currentTag
)

const imageContent = "0123456789abcdefghij"

type nopCloser struct{ io.ReadSeeker }

func (nopCloser) Close() error { return nil }

// fakeService returns a canned result and records the request it got.
type fakeService struct {
	err     error
	request appimages.Request
	calls   int
}

func (fake *fakeService) Image(_ context.Context, request appimages.Request) (appimages.Image, error) {
	fake.calls++
	fake.request = request

	if fake.err != nil {
		return appimages.Image{}, fake.err
	}

	return appimages.Image{
		Content:     nopCloser{strings.NewReader(imageContent)},
		Size:        int64(len(imageContent)),
		ContentType: "image/jpeg",
		ETag:        imageETag,
		Immutable:   request.Tag == currentTag,
	}, nil
}

type environment struct {
	service *fakeService
	logs    *bytes.Buffer
	mux     *http.ServeMux
}

func newEnvironment(t *testing.T) *environment {
	t.Helper()

	env := &environment{service: &fakeService{}, logs: &bytes.Buffer{}, mux: http.NewServeMux()}
	logger := slog.New(slog.NewTextHandler(env.logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handlers, err := images.New(images.Options{Images: env.service, Logger: logger})
	if err != nil {
		t.Fatalf("New error = %v", err)
	}

	for _, mount := range handlers.Mounts() {
		env.mux.Handle(mount.Pattern, mount.Handler)
	}

	return env
}

func (env *environment) do(method, target string, header ...string) *httptest.ResponseRecorder {
	request := httptest.NewRequestWithContext(context.Background(), method, target, nil)
	for index := 0; index+1 < len(header); index += 2 {
		request.Header.Set(header[index], header[index+1])
	}

	recorder := httptest.NewRecorder()
	env.mux.ServeHTTP(recorder, request)

	return recorder
}

func TestNewRequiresDependencies(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.DiscardHandler)
	for name, options := range map[string]images.Options{
		"no service": {Logger: logger},
		"no logger":  {Images: &fakeService{}},
	} {
		if _, err := images.New(options); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}

func TestServeImageHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		target       string
		cacheControl string
	}{
		{"current tag is immutable", imageURL, "private, max-age=31536000, immutable"},
		{"outdated tag", "/api/v1/images/" + imageID + "?width=320&tag=abc", "private, no-cache"},
		{"no tag", "/api/v1/images/" + imageID + "?width=320", "private, no-cache"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			response := env.do(http.MethodGet, test.target)

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", response.Code)
			}

			want := map[string]string{
				"Content-Type":           "image/jpeg",
				"Content-Length":         fmt.Sprint(len(imageContent)),
				"ETag":                   imageETag,
				"X-Content-Type-Options": "nosniff",
				"Cache-Control":          test.cacheControl,
			}
			for name, value := range want {
				if got := response.Header().Get(name); got != value {
					t.Errorf("%s = %q, want %q", name, got, value)
				}
			}

			if response.Body.String() != imageContent {
				t.Errorf("body = %q", response.Body.Bytes())
			}
		})
	}
}

func TestServeImagePassesCanonicalRequest(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	env.do(http.MethodGet, "/api/v1/images/"+strings.ToUpper(imageID)+"?width=640&tag="+currentTag)

	want := appimages.Request{ID: imageID, Width: artwork.Width640, Tag: currentTag}
	if env.service.request != want {
		t.Errorf("request = %+v, want %+v", env.service.request, want)
	}
}

func TestHeadHasHeadersWithoutBody(t *testing.T) {
	t.Parallel()

	response := newEnvironment(t).do(http.MethodHead, imageURL)

	if response.Code != http.StatusOK || response.Body.Len() != 0 {
		t.Fatalf("status = %d, body length = %d", response.Code, response.Body.Len())
	}

	if got := response.Header().Get("Content-Length"); got != fmt.Sprint(len(imageContent)) {
		t.Errorf("Content-Length = %q", got)
	}
}

func TestConditionalAndRangeRequests(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)

	notModified := env.do(http.MethodGet, imageURL, "If-None-Match", imageETag)
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Errorf("matching If-None-Match: status = %d, body = %d bytes", notModified.Code, notModified.Body.Len())
	}

	if got := notModified.Header().Get("Cache-Control"); got != "private, max-age=31536000, immutable" {
		t.Errorf("304 Cache-Control = %q", got)
	}

	stale := env.do(http.MethodGet, imageURL, "If-None-Match", `"other-320"`)
	if stale.Code != http.StatusOK {
		t.Errorf("other If-None-Match: status = %d, want 200", stale.Code)
	}

	partial := env.do(http.MethodGet, imageURL, "Range", "bytes=0-3")
	if partial.Code != http.StatusPartialContent || partial.Body.String() != "0123" {
		t.Errorf("range: status = %d, body = %q", partial.Code, partial.Body.String())
	}
}

func TestBadParameters(t *testing.T) {
	t.Parallel()

	base := "/api/v1/images/" + imageID
	tests := map[string]string{
		"no width":        base,
		"width not set":   base + "?width=321",
		"width leading 0": base + "?width=0320",
		"width text":      base + "?width=big",
		"tag uppercase":   base + "?width=320&tag=ABC",
		"tag not hex":     base + "?width=320&tag=xyz",
		"tag too long":    base + "?width=320&tag=" + strings.Repeat("a", 65),
		"id not a uuid":   "/api/v1/images/secret-name?width=320",
	}

	for name, target := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			response := env.do(http.MethodGet, target)

			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", response.Code)
			}

			body := response.Body.String()
			if !strings.Contains(body, `"code":"invalid"`) || strings.Contains(body, "secret-name") {
				t.Errorf("body = %s", body)
			}

			if env.service.calls != 0 {
				t.Error("service was called for an invalid request")
			}
		})
	}
}

func TestErrorMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		err        error
		status     int
		code       string
		retryAfter bool
	}{
		{"not found", fmt.Errorf("x: %w", domain.ErrNotFound), 404, "not_found", false},
		{"unsupported", fmt.Errorf("x: %w", domain.ErrUnsupported), 422, "unsupported", false},
		{"unavailable", fmt.Errorf("x: %w", domain.ErrUnavailable), 503, "unavailable", true},
		{
			"render timeout", fmt.Errorf("x: %w", errors.Join(context.DeadlineExceeded, domain.ErrUnavailable)),
			503, "unavailable", true,
		},
		{"stored data invalid is no client error", fmt.Errorf("/secret/path: %w", domain.ErrInvalid), 500, "internal_error", false},
		{"other", errors.New("/secret/path exploded"), 500, "internal_error", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := newEnvironment(t)
			env.service.err = test.err
			response := env.do(http.MethodGet, imageURL)

			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}

			body := response.Body.String()
			if !strings.Contains(body, `"code":"`+test.code+`"`) || strings.Contains(body, "secret") {
				t.Errorf("body = %s", body)
			}

			if got := response.Header().Get("Retry-After") != ""; got != test.retryAfter {
				t.Errorf("Retry-After present = %v, want %v", got, test.retryAfter)
			}
		})
	}
}

func TestCancelledRequestSendsNothingAndLogsNoError(t *testing.T) {
	t.Parallel()

	env := newEnvironment(t)
	env.service.err = fmt.Errorf("wait for render: %w", context.Canceled)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	request := httptest.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	recorder := httptest.NewRecorder()
	env.mux.ServeHTTP(recorder, request)

	if recorder.Body.Len() != 0 {
		t.Errorf("body = %q, want none", recorder.Body.String())
	}

	if strings.Contains(env.logs.String(), "level=ERROR") || strings.Contains(env.logs.String(), "level=WARN") {
		t.Errorf("logs = %s", env.logs.String())
	}
}

func TestOtherMethodsAreRejected(t *testing.T) {
	t.Parallel()

	if got := newEnvironment(t).do(http.MethodPost, imageURL).Code; got != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", got)
	}
}

func TestQuietPatternsMatchMountedRoute(t *testing.T) {
	t.Parallel()

	mounts := (&images.Handlers{}).Mounts()
	patterns := images.QuietPatterns()

	if len(patterns) != 1 || len(mounts) != 1 || patterns[0] != mounts[0].Pattern {
		t.Errorf("patterns = %v, mounts = %v", patterns, mounts)
	}
}

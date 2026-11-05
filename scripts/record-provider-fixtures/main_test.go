package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
)

const testKey = "secret-test-key"

// fakeGetter answers every request with body and status and remembers the URLs.
type fakeGetter struct {
	status  int
	body    string
	urls    []string
	headers []http.Header
}

func (fake *fakeGetter) Get(_ context.Context, rawURL string, header http.Header) (*outbound.Response, error) {
	fake.urls = append(fake.urls, rawURL)
	fake.headers = append(fake.headers, header)

	return &outbound.Response{StatusCode: fake.status, Body: []byte(fake.body)}, nil
}

func TestRecordWritesIndentedFixturesWithoutKey(t *testing.T) {
	out := t.TempDir()
	fake := &fakeGetter{status: http.StatusOK, body: `{"id":603}`}

	if err := record(t.Context(), recorder{http: fake, baseURL: "https://tmdb.example/3", key: testKey, out: out}); err != nil {
		t.Fatalf("record: %v", err)
	}

	if len(fake.urls) != len(fixtures()) || !strings.Contains(fake.urls[0], "api_key="+testKey) {
		t.Errorf("urls = %v", fake.urls)
	}

	written, err := os.ReadFile(filepath.Clean(filepath.Join(out, "movie_603.json")))
	if err != nil || string(written) != "{\n  \"id\": 603\n}\n" || strings.Contains(string(written), testKey) {
		t.Errorf("fixture = %q, %v", written, err)
	}
}

func TestRecordFailures(t *testing.T) {
	tests := []struct {
		name string
		run  recorder
	}{
		{"no key", recorder{http: &fakeGetter{status: http.StatusOK, body: "{}"}}},
		{"bad status", recorder{http: &fakeGetter{status: http.StatusUnauthorized}, key: testKey}},
		{"not json", recorder{http: &fakeGetter{status: http.StatusOK, body: "<html>"}, key: testKey}},
	}

	for _, test := range tests {
		test.run.out = t.TempDir()
		if err := record(t.Context(), test.run); err == nil {
			t.Errorf("%s: record succeeded", test.name)
		}
	}

	if err := record(t.Context(), recorder{}); !errors.Is(err, errNoKey) {
		t.Errorf("err = %v, want errNoKey", err)
	}
}

func TestRecordSendsReadAccessTokenAsBearer(t *testing.T) {
	const token = "eyJhbGciOiJIUzI1NiJ9.eyJhdWQiOiJ0ZXN0In0.c2lnbmF0dXJl" //nolint:gosec // fake token, not a credential

	fake := &fakeGetter{status: http.StatusOK, body: "{}"}

	if err := record(t.Context(), recorder{http: fake, baseURL: "https://tmdb.example/3", key: token, out: t.TempDir()}); err != nil {
		t.Fatalf("record: %v", err)
	}

	if strings.Contains(fake.urls[0], "api_key") || fake.headers[0].Get("Authorization") != "Bearer "+token {
		t.Errorf("url %q, authorization %q", fake.urls[0], fake.headers[0].Get("Authorization"))
	}
}

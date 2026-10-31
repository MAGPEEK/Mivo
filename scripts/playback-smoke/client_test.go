package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"
)

func TestProblemErrorsCarryCodeDetailAndRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		handler        http.HandlerFunc
		wantCode       string
		wantInMessage  string
		wantRetryAfter time.Duration
	}{
		{
			name: "problem document",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "3")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = fmt.Fprint(w, `{"code":"unavailable","detail":"shutting down","status":503}`)
			},
			wantCode: "unavailable", wantInMessage: "503 unavailable: shutting down", wantRetryAfter: 3 * time.Second,
		},
		{
			name: "plain text",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "soon")
				http.Error(w, "proxy error", http.StatusBadGateway)
			},
			wantInMessage: "502: proxy error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(test.handler)
			defer server.Close()

			client := newAPIClient(mustParseURL(t, server.URL), server.Client())
			err := client.call(t.Context(), apiCall{method: http.MethodGet, reference: "/x", want: http.StatusOK})

			problem, isProblem := errors.AsType[*problemError](err)
			if !isProblem || !errors.Is(err, errUnexpectedStatus) {
				t.Fatalf("call = %v, want a problemError", err)
			}

			if problemCode(err) != test.wantCode || problem.retryAfter != test.wantRetryAfter ||
				!strings.Contains(err.Error(), test.wantInMessage) {
				t.Errorf("error = %q (code %q, retry %s), want %q in it (code %q, retry %s)",
					err, problemCode(err), problem.retryAfter, test.wantInMessage, test.wantCode, test.wantRetryAfter)
			}
		})
	}
}

func TestCallReportsUndecodableBodiesAndBadReferences(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	client := newAPIClient(mustParseURL(t, server.URL), server.Client())

	var out struct{}

	if err := client.call(t.Context(), apiCall{method: http.MethodGet, reference: "/x", want: http.StatusOK, out: &out}); err == nil ||
		!strings.Contains(err.Error(), "decode the response") {
		t.Errorf("call with a broken body = %v, want a decode error", err)
	}

	if err := client.call(t.Context(), apiCall{method: http.MethodGet, reference: "%zz", want: http.StatusOK}); err == nil {
		t.Error("call with a malformed reference = nil, want an error")
	}

	if err := client.call(t.Context(), apiCall{method: http.MethodPost, reference: "/x", body: func() {}, want: http.StatusOK}); err == nil {
		t.Error("call with an unencodable body = nil, want an error")
	}

	if problemCode(errors.New("plain")) != "" {
		t.Error("problemCode of a plain error is not empty")
	}
}

func TestCreateSessionGivesUpWhenTheContextEndsDuringTheLimit(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"code":"session_limit_reached"}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	client := newAPIClient(mustParseURL(t, server.URL), server.Client())
	poll := pollSettings{interval: time.Millisecond, maxRetryAfter: 10 * time.Millisecond}

	if _, err := createSession(ctx, client, apiRequestForTest(), poll); err == nil {
		t.Error("createSession = nil error, want the cancellation")
	}
}

func TestFileIndexTellsAmbiguousFromMissing(t *testing.T) {
	t.Parallel()

	index := fileIndex{}
	first := uuid.New()
	index.add("a.mkv", first)
	index.add("b.mkv", uuid.New())
	index.add("b.mkv", uuid.New())

	if id, err := index.lookup("a.mkv"); err != nil || id != first {
		t.Errorf("lookup(a.mkv) = %s, %v; want %s", id, err, first)
	}

	if _, err := index.lookup("b.mkv"); !errors.Is(err, errAmbiguousFile) {
		t.Errorf("lookup(b.mkv) = %v, want errAmbiguousFile", err)
	}

	if _, err := index.lookup("c.mkv"); !errors.Is(err, errFileNotFound) {
		t.Errorf("lookup(c.mkv) = %v, want errFileNotFound", err)
	}
}

func TestPollWaitHonoursHintsAndCancellation(t *testing.T) {
	t.Parallel()

	poll := pollSettings{interval: time.Hour, maxRetryAfter: time.Millisecond}
	if err := poll.wait(t.Context(), time.Hour); err != nil {
		t.Errorf("wait with a capped hint = %v, want nil", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := poll.wait(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("wait after cancellation = %v, want context.Canceled", err)
	}
}

func TestLimitedWriterRejectsOversizedWrites(t *testing.T) {
	t.Parallel()

	var text strings.Builder

	writer := &limitedWriter{writer: &text, remaining: 4}

	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatalf("first write: %v", err)
	}

	if _, err := writer.Write([]byte("de")); !errors.Is(err, errTooLarge) {
		t.Errorf("second write = %v, want errTooLarge", err)
	}
}

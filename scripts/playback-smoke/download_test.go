package main

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWholeFileSize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		contentRange string
		want         int64
		wantErr      bool
	}{
		{contentRange: "bytes 0-1023/1024", want: 1024},
		{contentRange: "bytes 0-0/1", want: 1},
		{contentRange: "", wantErr: true},
		{contentRange: "bytes 1-1023/1024", wantErr: true},
		{contentRange: "bytes 0-511/1024", wantErr: true},
		{contentRange: "bytes 0-1023/*", wantErr: true},
		{contentRange: "bytes */1024", wantErr: true},
		{contentRange: "bytes 0-x/1024", wantErr: true},
	}

	for _, test := range tests {
		size, err := wholeFileSize(test.contentRange)
		if (err != nil) != test.wantErr || size != test.want {
			t.Errorf("wholeFileSize(%q) = %d, %v; want %d, error %t", test.contentRange, size, err, test.want, test.wantErr)
		}

		if err != nil && !errors.Is(err, errBadContentRange) {
			t.Errorf("wholeFileSize(%q) error = %v, want errBadContentRange", test.contentRange, err)
		}
	}
}

func TestDownloadDirectRequiresACompleteRangeAnswer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{
			name: "no range support",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, "whole file")
			},
			wantErr: errUnexpectedStatus,
		},
		{
			name: "short body",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-9/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = fmt.Fprint(w, "12345")
			},
			wantErr: errShortBody,
		},
		{
			name: "partial range",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Range", "bytes 0-4/10")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = fmt.Fprint(w, "12345")
			},
			wantErr: errBadContentRange,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(test.handler)
			defer server.Close()

			client := newAPIClient(mustParseURL(t, server.URL), server.Client())

			err := streamDownloader{client: client}.direct(t.Context(), mustParseURL(t, server.URL+"/file"), &bytes.Buffer{})
			if !errors.Is(err, test.wantErr) {
				t.Errorf("direct = %v, want %v", err, test.wantErr)
			}
		})
	}
}

func TestDownloadHLSStopsAtAnEndlessPlaylist(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "#EXTM3U\n"+strings.Repeat("#EXT-X-COMMENT\n", maxPlaylistBytes/len("#EXT-X-COMMENT\n")+1))
	}))
	defer server.Close()

	client := newAPIClient(mustParseURL(t, server.URL), server.Client())
	poll := pollSettings{interval: time.Millisecond, maxRetryAfter: time.Millisecond}

	err := streamDownloader{client: client, poll: poll}.hls(t.Context(), mustParseURL(t, server.URL+"/index.m3u8"), &bytes.Buffer{})
	if !errors.Is(err, errTooLarge) {
		t.Errorf("hls = %v, want errTooLarge", err)
	}
}

func TestDownloadHLSReportsABrokenPlaylist(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "<html>not a playlist</html>")
	}))
	defer server.Close()

	client := newAPIClient(mustParseURL(t, server.URL), server.Client())
	poll := pollSettings{interval: time.Millisecond, maxRetryAfter: time.Millisecond}

	err := streamDownloader{client: client, poll: poll}.hls(t.Context(), mustParseURL(t, server.URL+"/index.m3u8"), &bytes.Buffer{})
	if !errors.Is(err, errNotAPlaylist) {
		t.Errorf("hls = %v, want errNotAPlaylist", err)
	}
}

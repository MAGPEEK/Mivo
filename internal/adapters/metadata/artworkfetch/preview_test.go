package artworkfetch_test

import (
	"bytes"
	"errors"
	"net/http"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/artworkfetch"
	"github.com/magpeek/mivo/internal/domain"
)

func TestPreviewReturnsThePictureWithoutStoringIt(t *testing.T) {
	t.Parallel()

	body := readFixture(t, "poster.jpg")
	h := newHarness(t, map[string]http.HandlerFunc{"/t/p/original/abc.jpg": serveBytes(body)})

	got, err := h.fetcher.Preview(t.Context(), tmdbRef("/abc.jpg"))
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}

	if !bytes.Equal(got, body) {
		t.Errorf("Preview returned %d bytes, want the %d fixture bytes", len(got), len(body))
	}

	h.requireNoFiles(t)
}

func TestPreviewOfADisabledProviderIsNoOutage(t *testing.T) {
	t.Parallel()

	fetcher, err := artworkfetch.New(artworkfetch.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = fetcher.Preview(t.Context(), tmdbRef("/abc.jpg"))
	if !errors.Is(err, domain.ErrConflict) || errors.Is(err, domain.ErrUnavailable) {
		t.Errorf("Preview error = %v, want ErrConflict only", err)
	}
}

func TestPreviewRefusesWhatFetchRefuses(t *testing.T) {
	t.Parallel()

	h := newHarness(t, map[string]http.HandlerFunc{
		"/t/p/original/bad.jpg":   serveBytes(readFixture(t, "not-an-image.gif")),
		"/t/p/original/gone.jpg":  func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNotFound) },
		"/t/p/original/error.jpg": func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusBadGateway) },
	})

	tests := []struct {
		name   string
		call   func() error
		target error
	}{
		{"cover art is not previewed", func() error {
			_, err := h.fetcher.Preview(t.Context(), coverRef(h.server.url("coverartarchive.org", "/x.jpg")))

			return err
		}, domain.ErrInvalid},
		{"path outside the image base", func() error {
			_, err := h.fetcher.Preview(t.Context(), tmdbRef("/../etc/passwd"))

			return err
		}, domain.ErrInvalid},
		{"no image", func() error {
			_, err := h.fetcher.Preview(t.Context(), tmdbRef("/bad.jpg"))

			return err
		}, domain.ErrInvalid},
		{"missing", func() error {
			_, err := h.fetcher.Preview(t.Context(), tmdbRef("/gone.jpg"))

			return err
		}, domain.ErrNotFound},
		{"server error", func() error {
			_, err := h.fetcher.Preview(t.Context(), tmdbRef("/error.jpg"))

			return err
		}, domain.ErrUnavailable},
	}
	for _, test := range tests {
		if err := test.call(); !errors.Is(err, test.target) {
			t.Errorf("%s: Preview error = %v, want %v", test.name, err, test.target)
		}
	}

	h.requireNoFiles(t)
}

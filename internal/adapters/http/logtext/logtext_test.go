package logtext_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/logtext"
	"github.com/magpeek/mivo/internal/adapters/http/requestid"
)

// tenKibibytes is the size of the hostile client values in these tests.
const tenKibibytes = 10 << 10

func TestTruncate(t *testing.T) {
	t.Parallel()

	fits := strings.Repeat("a", logtext.MaxBytes)
	ascii := strings.Repeat("a", logtext.MaxBytes+1)
	// "€" is three bytes, so a cut at a fixed byte offset lands inside a sequence.
	multiByte := strings.Repeat("€", logtext.MaxBytes)
	kept := logtext.MaxBytes - len(logtext.TruncationMarker)

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "short", value: "/api/v1/health", want: "/api/v1/health"},
		{name: "fits exactly", value: fits, want: fits},
		{name: "one byte too long", value: ascii, want: ascii[:kept] + logtext.TruncationMarker},
		{name: "multi-byte", value: multiByte, want: strings.Repeat("€", kept/len("€")) + logtext.TruncationMarker},
		{name: "10 KiB", value: strings.Repeat("p", tenKibibytes), want: strings.Repeat("p", kept) + logtext.TruncationMarker},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := logtext.Truncate(test.value)
			if got != test.want {
				t.Errorf("Truncate = %q, want %q", got, test.want)
			}

			assertBounded(t, got, logtext.MaxBytes)
		})
	}
}

func TestTruncateKeepsInvalidUTF8Bounded(t *testing.T) {
	t.Parallel()

	// Continuation bytes only: no rune start to back off to, so the cut must still end.
	value := strings.Repeat("\x80", logtext.MaxBytes+1)

	got := logtext.Truncate(value)
	if len(got) > logtext.MaxBytes || !strings.HasSuffix(got, logtext.TruncationMarker) {
		t.Errorf("Truncate = %q (%d bytes), want at most %d bytes ending in the marker", got, len(got), logtext.MaxBytes)
	}
}

func TestTruncateMiddle(t *testing.T) {
	t.Parallel()

	fits := strings.Repeat("a", logtext.MaxBytes)
	cause := ": disk on fire"
	quotedValue := "open " + strings.Repeat("r", tenKibibytes) + cause
	kept := logtext.MaxBytes - len(logtext.TruncationMarker)

	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "empty", value: "", want: ""},
		{name: "fits exactly", value: fits, want: fits},
		{
			name: "keeps start and cause", value: quotedValue,
			want: "open " + strings.Repeat("r", kept/2-len("open ")) + logtext.TruncationMarker + strings.Repeat("r", kept-kept/2-len(cause)) + cause,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := logtext.TruncateMiddle(test.value)
			if got != test.want {
				t.Errorf("TruncateMiddle = %q, want %q", got, test.want)
			}

			assertBounded(t, got, logtext.MaxBytes)
		})
	}
}

func TestTruncateMiddleToNeverSplitsRunes(t *testing.T) {
	t.Parallel()

	// "€" is three bytes, so both cuts land inside a sequence for most limits.
	value := strings.Repeat("€", tenKibibytes)

	for limit := len(logtext.TruncationMarker); limit < 40; limit++ {
		got := logtext.TruncateMiddleTo(value, limit)
		assertBounded(t, got, limit)

		if !strings.Contains(got, logtext.TruncationMarker) {
			t.Errorf("TruncateMiddleTo(limit %d) = %q, want the marker", limit, got)
		}
	}
}

func TestTruncateMiddleToBelowTheMarker(t *testing.T) {
	t.Parallel()

	for limit := -1; limit < len(logtext.TruncationMarker); limit++ {
		got := logtext.TruncateMiddleTo("long value", limit)
		if len(got) > max(limit, 0) {
			t.Errorf("TruncateMiddleTo(limit %d) = %q, want at most %d bytes", limit, got, max(limit, 0))
		}
	}

	// Continuation bytes only: there is no rune start to cut at.
	got := logtext.TruncateMiddleTo(strings.Repeat("\x80", 20), 10)
	if len(got) > 10 {
		t.Errorf("TruncateMiddleTo(invalid UTF-8) = %q, want at most 10 bytes", got)
	}
}

func TestRequestAttributesCutClientText(t *testing.T) {
	t.Parallel()

	longMethod := strings.Repeat("M", tenKibibytes)
	longPath := "/" + strings.Repeat("p", tenKibibytes)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, longPath, nil)
	request.Method = longMethod
	request = request.WithContext(requestid.NewContext(request.Context(), "req-7"))

	var output strings.Builder

	logger := slog.New(slog.NewTextHandler(&output, nil))
	logger.Info("x", logtext.RequestAttributes(request, logtext.Error(errors.New(strings.Repeat("e", tenKibibytes)+" cause")))...)

	record := output.String()
	for _, want := range []string{
		"method=" + logtext.Truncate(longMethod), "path=" + logtext.Truncate(longPath), "request_id=req-7", "cause",
	} {
		if !strings.Contains(record, want) {
			t.Errorf("record %q lacks %q", record, want)
		}
	}

	for _, uncut := range []string{"M", "p", "e"} {
		if strings.Contains(record, strings.Repeat(uncut, logtext.MaxBytes)) {
			t.Errorf("record of %d bytes holds an uncut %q value", len(record), uncut)
		}
	}
}

// assertBounded checks that got has at most limit bytes of valid UTF-8.
func assertBounded(t *testing.T, got string, limit int) {
	t.Helper()

	if len(got) > limit || !utf8.ValidString(got) {
		t.Errorf("%q: %d bytes, valid UTF-8 %v; want at most %d valid bytes", got, len(got), utf8.ValidString(got), limit)
	}
}

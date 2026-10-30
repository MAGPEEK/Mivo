package statuswriter_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/statuswriter"
)

func TestRecorderKeepsFirstFinalStatus(t *testing.T) {
	t.Parallel()

	recorder := statuswriter.New(httptest.NewRecorder())
	if recorder.HeadersSent() || recorder.StatusOrOK() != http.StatusOK {
		t.Fatalf("fresh recorder: sent %v, status %d, want not sent and 200", recorder.HeadersSent(), recorder.StatusOrOK())
	}

	recorder.WriteHeader(http.StatusEarlyHints)

	if recorder.HeadersSent() {
		t.Error("an informational status counts as sent headers")
	}

	recorder.WriteHeader(http.StatusCreated)
	recorder.WriteHeader(http.StatusConflict)

	if got := recorder.StatusOrOK(); got != http.StatusCreated || !recorder.HeadersSent() {
		t.Errorf("status = %d, sent %v, want the first final status 201 and sent", got, recorder.HeadersSent())
	}
}

// TestRecorderTreatsSwitchingProtocolsAsFinal follows net/http, which sends the header block
// with a 101 and ignores every later WriteHeader: after a 101 the status can no longer change.
func TestRecorderTreatsSwitchingProtocolsAsFinal(t *testing.T) {
	t.Parallel()

	recorder := statuswriter.New(httptest.NewRecorder())
	recorder.WriteHeader(http.StatusSwitchingProtocols)

	if !recorder.HeadersSent() || recorder.StatusOrOK() != http.StatusSwitchingProtocols {
		t.Fatalf("after 101: sent %v, status %d, want sent and 101", recorder.HeadersSent(), recorder.StatusOrOK())
	}

	recorder.WriteHeader(http.StatusOK)

	if got := recorder.StatusOrOK(); got != http.StatusSwitchingProtocols {
		t.Errorf("status = %d, want the first final status 101", got)
	}
}

func TestRecorderCountsBodyAndRecordsImplicitOK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		write func(recorder *statuswriter.Recorder) error
	}{
		{name: "write", write: func(recorder *statuswriter.Recorder) error {
			_, err := recorder.Write([]byte("hello"))

			return err
		}},
		{name: "read from", write: func(recorder *statuswriter.Recorder) error {
			_, err := recorder.ReadFrom(strings.NewReader("hello"))

			return err
		}},
		{name: "io.Copy", write: func(recorder *statuswriter.Recorder) error {
			_, err := io.Copy(recorder, strings.NewReader("hello"))

			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			underlying := httptest.NewRecorder()
			recorder := statuswriter.New(underlying)

			if err := test.write(recorder); err != nil {
				t.Fatalf("write: %v", err)
			}

			if recorder.StatusOrOK() != http.StatusOK || !recorder.HeadersSent() {
				t.Errorf("status = %d, sent %v, want implicit 200", recorder.StatusOrOK(), recorder.HeadersSent())
			}

			if recorder.Written() != int64(len("hello")) || underlying.Body.String() != "hello" {
				t.Errorf("written = %d, body %q, want 5 and hello", recorder.Written(), underlying.Body)
			}
		})
	}
}

func TestRecorderUsesUnderlyingReaderFrom(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		copy func(recorder *statuswriter.Recorder, source io.Reader) (int64, error)
	}{
		{name: "ReadFrom", copy: func(recorder *statuswriter.Recorder, source io.Reader) (int64, error) {
			return recorder.ReadFrom(source)
		}},
		// http.ServeContent copies with io.CopyN, which reaches ReadFrom through io.Copy.
		{name: "io.CopyN", copy: func(recorder *statuswriter.Recorder, source io.Reader) (int64, error) {
			return io.CopyN(recorder, source, int64(len("file")))
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			underlying := &readerFromWriter{ResponseWriter: httptest.NewRecorder()}
			recorder := statuswriter.New(underlying)

			copied, err := test.copy(recorder, strings.NewReader("file"))
			if err != nil {
				t.Fatalf("copy: %v", err)
			}

			if !underlying.readFromCalled {
				t.Error("the wrapped writer's ReadFrom was bypassed, which disables sendfile")
			}

			if copied != int64(len("file")) || recorder.Written() != copied {
				t.Errorf("copied %d, recorded %d, want both 4", copied, recorder.Written())
			}
		})
	}
}

func TestRecorderFlushThroughResponseController(t *testing.T) {
	t.Parallel()

	underlying := httptest.NewRecorder()
	recorder := statuswriter.New(underlying)

	if err := http.NewResponseController(recorder).Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	if !underlying.Flushed || !recorder.HeadersSent() || recorder.StatusOrOK() != http.StatusOK {
		t.Errorf("flushed %v, sent %v, status %d, want a flushed implicit 200", underlying.Flushed, recorder.HeadersSent(), recorder.StatusOrOK())
	}

	if recorder.Unwrap() != underlying {
		t.Error("Unwrap() does not return the wrapped writer")
	}
}

func TestRecorderFlushWithoutSupportSendsNothing(t *testing.T) {
	t.Parallel()

	recorder := statuswriter.New(plainWriter{header: http.Header{}})

	if err := recorder.FlushError(); !errors.Is(err, http.ErrNotSupported) {
		t.Errorf("FlushError = %v, want http.ErrNotSupported", err)
	}

	recorder.Flush()

	if recorder.HeadersSent() {
		t.Error("a failed flush counts as sent headers")
	}
}

func TestRecorderPassesWriteErrorThrough(t *testing.T) {
	t.Parallel()

	failure := errors.New("connection reset")
	recorder := statuswriter.New(failingWriter{ResponseWriter: httptest.NewRecorder(), err: failure})

	if _, err := recorder.Write([]byte("x")); !errors.Is(err, failure) {
		t.Errorf("Write error = %v, want %v", err, failure)
	}

	if recorder.Written() != 0 {
		t.Errorf("written = %d, want 0 after a failed write", recorder.Written())
	}
}

type readerFromWriter struct {
	http.ResponseWriter
	readFromCalled bool
}

func (writer *readerFromWriter) ReadFrom(source io.Reader) (int64, error) {
	writer.readFromCalled = true

	return io.Copy(io.Discard, source)
}

// plainWriter supports neither flushing nor unwrapping.
type plainWriter struct {
	header http.Header
}

func (writer plainWriter) Header() http.Header { return writer.header }

func (plainWriter) Write(body []byte) (int, error) { return len(body), nil }

func (plainWriter) WriteHeader(int) {}

type failingWriter struct {
	http.ResponseWriter
	err error
}

func (writer failingWriter) Write([]byte) (int, error) {
	return 0, writer.err
}

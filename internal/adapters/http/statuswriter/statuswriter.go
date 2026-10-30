// Package statuswriter records what a handler sent through an http.ResponseWriter. The
// server middleware (access log, panic recovery) and the REST API (skipping a problem once
// the status line is out) both need it, and restapi must not import the server package.
package statuswriter

import (
	"io"
	"net/http"
)

// Recorder wraps an http.ResponseWriter and remembers the first final status and the number
// of body bytes written through it, ReadFrom included. It stays transparent for
// http.ResponseController (Flush, deadlines) and keeps the io.ReaderFrom fast path
// (sendfile) of the wrapped writer.
type Recorder struct {
	http.ResponseWriter
	status  int
	written int64
}

// New returns a Recorder around writer.
func New(writer http.ResponseWriter) *Recorder {
	return &Recorder{ResponseWriter: writer}
}

// WriteHeader records the first final status.
func (recorder *Recorder) WriteHeader(status int) {
	if recorder.status == 0 && isFinal(status) {
		recorder.status = status
	}

	recorder.ResponseWriter.WriteHeader(status)
}

// isFinal reports whether net/http treats status as the final status line: every status
// from 200 on, and 101 Switching Protocols, after which no other header block may follow.
// Other 1xx statuses are informational and leave the status open.
func isFinal(status int) bool {
	return status >= http.StatusOK || status == http.StatusSwitchingProtocols
}

// Write records an implicit 200 when no status was written before, as net/http does.
func (recorder *Recorder) Write(body []byte) (int, error) {
	recorder.recordImplicitOK()

	written, err := recorder.ResponseWriter.Write(body)
	recorder.written += int64(written)

	// A transparent decorator: the caller sees the underlying writer's result unchanged.
	return written, err
}

// ReadFrom copies source into the wrapped writer. It calls the wrapped writer's own ReadFrom
// directly when there is one, so net/http can still use sendfile for file bodies; io.Copy
// would prefer the source's WriteTo and lose that path.
func (recorder *Recorder) ReadFrom(source io.Reader) (int64, error) {
	recorder.recordImplicitOK()

	var (
		copied int64
		err    error
	)

	if readerFrom, ok := recorder.ResponseWriter.(io.ReaderFrom); ok {
		copied, err = readerFrom.ReadFrom(source)
	} else {
		copied, err = io.Copy(recorder.ResponseWriter, source)
	}

	recorder.written += copied

	// A transparent decorator: the caller sees the underlying writer's result unchanged.
	return copied, err
}

// FlushError flushes the wrapped writer through http.ResponseController. A successful flush
// sends the headers, so it records an implicit 200 like Write.
func (recorder *Recorder) FlushError() error {
	err := http.NewResponseController(recorder.ResponseWriter).Flush()
	if err == nil {
		recorder.recordImplicitOK()
	}

	// A transparent decorator: the caller sees the underlying writer's result unchanged.
	return err
}

// Flush implements http.Flusher for handlers that type-assert it. The interface has no
// error result; callers that need it use http.ResponseController, which calls FlushError.
func (recorder *Recorder) Flush() {
	_ = recorder.FlushError()
}

// Unwrap lets http.ResponseController reach the wrapped writer (deadlines, hijacking).
func (recorder *Recorder) Unwrap() http.ResponseWriter {
	return recorder.ResponseWriter
}

// HeadersSent reports whether the status line has been sent, after which the status can no
// longer change.
func (recorder *Recorder) HeadersSent() bool {
	return recorder.status != 0
}

// StatusOrOK returns the recorded status, or 200 for a handler that sent nothing, which is
// what net/http answers in that case.
func (recorder *Recorder) StatusOrOK() int {
	if recorder.status == 0 {
		return http.StatusOK
	}

	return recorder.status
}

// Written returns the number of body bytes written through the recorder.
func (recorder *Recorder) Written() int64 {
	return recorder.written
}

func (recorder *Recorder) recordImplicitOK() {
	if recorder.status == 0 {
		recorder.status = http.StatusOK
	}
}

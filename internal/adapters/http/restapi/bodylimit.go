package restapi

import (
	"errors"
	"io"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// MaxRequestBodyBytes limits every API request body (ADR 0024). The largest body of the
// contract, a playback request with a bounded client profile, stays far below it.
const MaxRequestBodyBytes = 64 << 10

// limitRequestBodies rejects a body that announces more than MaxRequestBodyBytes at once
// and cuts off longer bodies of unknown length while they are read. A handler that hits
// the cut-off gets an *http.MaxBytesError; the generated code reports it to
// writeRequestError, which answers 413 as well. How long a body may take is bounded by the
// server's per-request read deadline (server.BodyReadTimeout), which also covers the bytes
// net/http discards after an early 413.
//
// The limit runs before routing, so an oversized body sent to an unknown route gets 413
// rather than 404. That is accepted: the answer is correct either way, and checking the
// length first never reads a byte of an oversized body.
func limitRequestBodies(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.ContentLength > MaxRequestBodyBytes {
			WriteProblem(writer, payloadTooLargeProblem())

			return
		}

		if request.Body != nil && request.Body != http.NoBody {
			request.Body = http.MaxBytesReader(writer, request.Body, MaxRequestBodyBytes)
		}

		next.ServeHTTP(writer, request)
	})
}

// isBodyTooLarge reports whether err comes from reading past the body limit.
func isBodyTooLarge(err error) bool {
	var tooLarge *http.MaxBytesError

	return errors.As(err, &tooLarge)
}

// remainingBodyTooLarge reads the rest of a body the decoder rejected, to tell a malformed
// body (400) from an oversized one (413): a chunked upload of non-JSON text fails to decode
// long before it reaches the limit. limitRequestBodies and the server's read deadline
// already bound the body in size and time; the local limit keeps the drain bounded even for
// a body limitRequestBodies did not wrap.
func remainingBodyTooLarge(request *http.Request) bool {
	if request.Body == nil {
		return false
	}

	_, err := io.Copy(io.Discard, io.LimitReader(request.Body, MaxRequestBodyBytes+1))

	return isBodyTooLarge(err)
}

func payloadTooLargeProblem() apigen.Problem {
	return NewProblem(http.StatusRequestEntityTooLarge, CodePayloadTooLarge)
}

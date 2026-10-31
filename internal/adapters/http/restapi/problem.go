package restapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// ProblemMediaType is the content type of every error response (RFC 9457).
const ProblemMediaType = "application/problem+json"

// problemTypeBlank is the RFC 9457 type for problems that need no more than the status
// title; clients distinguish them by Problem.Code.
const problemTypeBlank = "about:blank"

// Stable machine-readable problem codes (api/openapi.yaml, Problem.code). Clients translate
// them, so a published code never changes its meaning.
const (
	// CodeBadRequest reports a request the server could not parse or bind.
	CodeBadRequest = "bad_request"
	// CodeInvalid reports input that violates a domain rule (domain.ErrInvalid).
	CodeInvalid = "invalid"
	// CodeForbidden reports a caller that may not perform the operation (domain.ErrForbidden).
	CodeForbidden = "forbidden"
	// CodeNotFound reports a missing entity or an unknown API route.
	CodeNotFound = "not_found"
	// CodeMethodNotAllowed reports a known API route requested with an unsupported method.
	CodeMethodNotAllowed = "method_not_allowed"
	// CodeConflict reports a clash with the current state (domain.ErrConflict).
	CodeConflict = "conflict"
	// CodePayloadTooLarge reports a request body above MaxRequestBodyBytes (413).
	CodePayloadTooLarge = "payload_too_large"
	// CodeUnsupported reports well-formed input the server cannot handle, for example a
	// file that could not be probed (domain.ErrUnsupported, 422).
	CodeUnsupported = "unsupported"
	// CodeNoCompatibleFormat reports that the client plays no format the server can
	// produce for the file (422).
	CodeNoCompatibleFormat = "no_compatible_format"
	// CodeTranscodeUnavailable reports that playback needs transcoding the server cannot
	// do (422).
	CodeTranscodeUnavailable = "transcode_unavailable"
	// CodeInternal reports a server-side failure. Its details are logged, never sent.
	CodeInternal = "internal_error"
	// CodeUnavailable reports a temporarily unavailable resource, for example a server that
	// is shutting down (domain.ErrUnavailable, 503).
	CodeUnavailable = "unavailable"
	// CodeSessionLimitReached reports that no further playback session may start now
	// (503 with Retry-After).
	CodeSessionLimitReached = "session_limit_reached"
	// CodeStreamNotReady reports a stream without its first segment yet (503 with
	// Retry-After).
	CodeStreamNotReady = "stream_not_ready"
)

// errorMapping maps one domain sentinel error to its HTTP status and problem code.
type errorMapping struct {
	target error
	status int32
	code   string
}

// domainErrorMappings is checked in order; the first sentinel the error wraps wins. The
// playback sentinels come first because each wraps a generic one below (specific before
// generic): ErrNoCompatibleFormat and ErrTranscodeUnavailable wrap domain.ErrUnsupported,
// ErrSessionLimitReached wraps domain.ErrUnavailable.
func domainErrorMappings() []errorMapping {
	return []errorMapping{
		{target: playback.ErrNoCompatibleFormat, status: http.StatusUnprocessableEntity, code: CodeNoCompatibleFormat},
		{target: playback.ErrTranscodeUnavailable, status: http.StatusUnprocessableEntity, code: CodeTranscodeUnavailable},
		{target: appplayback.ErrSessionLimitReached, status: http.StatusServiceUnavailable, code: CodeSessionLimitReached},
		{target: domain.ErrNotFound, status: http.StatusNotFound, code: CodeNotFound},
		{target: domain.ErrForbidden, status: http.StatusForbidden, code: CodeForbidden},
		{target: domain.ErrConflict, status: http.StatusConflict, code: CodeConflict},
		{target: domain.ErrInvalid, status: http.StatusBadRequest, code: CodeInvalid},
		{target: domain.ErrUnsupported, status: http.StatusUnprocessableEntity, code: CodeUnsupported},
		{target: domain.ErrUnavailable, status: http.StatusServiceUnavailable, code: CodeUnavailable},
	}
}

// NewProblem returns a problem with the standard title of status and no detail. A detail
// is left out on purpose: error texts may carry paths or driver internals.
func NewProblem(status int32, code string) apigen.Problem {
	return apigen.Problem{
		Type:   problemTypeBlank,
		Title:  http.StatusText(int(status)),
		Status: status,
		Code:   code,
	}
}

// InternalProblem returns the 500 problem. Callers log the cause themselves.
func InternalProblem() apigen.Problem {
	return NewProblem(http.StatusInternalServerError, CodeInternal)
}

// ProblemForError maps a domain error to its problem: the playback failures
// no_compatible_format and transcode_unavailable 422 and session_limit_reached 503, then
// ErrNotFound 404, ErrForbidden 403, ErrConflict 409, ErrInvalid 400, ErrUnsupported 422,
// ErrUnavailable 503 and anything else 500. Only this adapter maps domain errors to HTTP
// (ADR 0017). A session_limit_reached answer also needs Retry-After, which the caller sets.
func ProblemForError(err error) apigen.Problem {
	for _, mapping := range domainErrorMappings() {
		if errors.Is(err, mapping.target) {
			return NewProblem(mapping.status, mapping.code)
		}
	}

	return InternalProblem()
}

// WriteProblem sends problem as application/problem+json with its status.
func WriteProblem(writer http.ResponseWriter, problem apigen.Problem) {
	body, err := json.Marshal(problem)
	if err != nil {
		// A Problem holds only strings and integers, so this cannot happen; the status
		// alone still tells the client what went wrong.
		writer.WriteHeader(int(problem.Status))

		return
	}

	header := writer.Header()
	header.Del("Content-Length")
	header.Set("Content-Type", ProblemMediaType)
	writer.WriteHeader(int(problem.Status))

	// A failed write means the client has gone away; there is nobody left to tell.
	_, _ = writer.Write(body)
}

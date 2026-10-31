package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	contentTypeJSON = "application/json"
	// maxErrorBodyBytes bounds the response body kept for an error message.
	maxErrorBodyBytes = 4096
)

var errUnexpectedStatus = errors.New("unexpected HTTP status")

// apiClient calls the public HTTP API of a running server. It only knows the contract
// (api/openapi.yaml), never the server's internals.
type apiClient struct {
	base *url.URL
	http *http.Client
}

func newAPIClient(base *url.URL, httpClient *http.Client) *apiClient {
	return &apiClient{base: base, http: httpClient}
}

// problemError is a failed API call. code, detail and retryAfter are filled from an RFC 9457
// problem body and the Retry-After header when the server sent them.
type problemError struct {
	method     string
	target     string
	status     int
	code       string
	detail     string
	retryAfter time.Duration
}

func (e *problemError) Error() string {
	message := fmt.Sprintf("%s %s: %s %d", e.method, e.target, errUnexpectedStatus, e.status)

	if e.code != "" {
		message += " " + e.code
	}

	if e.detail != "" {
		message += ": " + e.detail
	}

	return message
}

func (e *problemError) Unwrap() error {
	return errUnexpectedStatus
}

// problemCode returns the problem code of a failed call, or "" for other errors.
func problemCode(err error) string {
	if problem, ok := errors.AsType[*problemError](err); ok {
		return problem.code
	}

	return ""
}

// resolve turns a URI reference of the API (a root-relative streamUrl, a relative segment
// name) into an absolute URL, per RFC 3986 against base.
func resolve(base *url.URL, reference string) (*url.URL, error) {
	target, err := base.Parse(reference)
	if err != nil {
		return nil, fmt.Errorf("resolve %q against %s: %w", reference, base, err)
	}

	return target, nil
}

// apiCall is one JSON request: body is encoded when set, out is decoded from a response with
// status want.
type apiCall struct {
	method    string
	reference string
	body      any
	want      int
	out       any
}

func (c *apiClient) call(ctx context.Context, request apiCall) error {
	target, err := resolve(c.base, request.reference)
	if err != nil {
		return err
	}

	var body io.Reader

	if request.body != nil {
		encoded, err := json.Marshal(request.body)
		if err != nil {
			return fmt.Errorf("encode the body of %s %s: %w", request.method, target, err)
		}

		body = bytes.NewReader(encoded)
	}

	httpRequest, err := http.NewRequestWithContext(ctx, request.method, target.String(), body)
	if err != nil {
		return fmt.Errorf("build %s %s: %w", request.method, target, err)
	}

	httpRequest.Header.Set("Accept", contentTypeJSON)

	if body != nil {
		httpRequest.Header.Set("Content-Type", contentTypeJSON)
	}

	response, err := c.http.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("%s %s: %w", request.method, target, err)
	}

	defer closeBody(response)

	return decodeResponse(response, request.want, request.out)
}

func decodeResponse(response *http.Response, want int, out any) error {
	if response.StatusCode != want {
		return newProblemError(response)
	}

	if out == nil {
		return nil
	}

	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		return fmt.Errorf("decode the response of %s %s: %w", response.Request.Method, response.Request.URL, err)
	}

	return nil
}

// newProblemError describes a response with an unexpected status. A body that is no
// problem document still ends up in the detail, so the message shows what the server said.
func newProblemError(response *http.Response) *problemError {
	problem := &problemError{
		method: response.Request.Method, target: response.Request.URL.String(), status: response.StatusCode,
		retryAfter: retryAfter(response.Header),
	}

	// The body only enriches the message; a failed read leaves the status as the error.
	data, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))

	var document struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}

	if err := json.Unmarshal(data, &document); err == nil {
		problem.code, problem.detail = document.Code, document.Detail
	} else {
		problem.detail = strings.TrimSpace(string(data))
	}

	return problem
}

// retryAfter reads Retry-After in seconds; a missing or malformed value is 0.
func retryAfter(header http.Header) time.Duration {
	seconds, err := strconv.Atoi(header.Get("Retry-After"))
	if err != nil || seconds < 0 {
		return 0
	}

	return time.Duration(seconds) * time.Second
}

// closeBody drains a little of the body so the connection can be reused, then closes it.
func closeBody(response *http.Response) {
	// Errors while discarding or closing a body that was already read change nothing.
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, maxErrorBodyBytes))
	_ = response.Body.Close()
}

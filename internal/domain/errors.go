package domain

import "errors"

// Sentinel errors shared by all layers. Use cases and adapters wrap them with context
// (fmt.Errorf("load identity: %w", ErrNotFound)); only adapters/http maps them to status codes,
// so callers must test with errors.Is, never by comparing messages.
var (
	// ErrNotFound reports that a requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict reports that an operation clashes with the current state, for example a
	// duplicate entity.
	ErrConflict = errors.New("conflict")
	// ErrInvalid reports input that violates a domain rule, for example a malformed ID.
	ErrInvalid = errors.New("invalid")
	// ErrForbidden reports that the caller is not allowed to perform the operation.
	ErrForbidden = errors.New("forbidden")
	// ErrUnsupported reports well-formed input the server cannot handle, for example a
	// media file that could not be probed or a format no client profile can play. Unlike
	// ErrInvalid, the request itself breaks no rule.
	ErrUnsupported = errors.New("unsupported")
	// ErrUnavailable reports that a resource the operation needs is temporarily unavailable,
	// for example an unmounted library folder, a reached session limit or a server that is
	// shutting down. Retrying later may succeed.
	ErrUnavailable = errors.New("unavailable")
)

package restapi

import (
	"fmt"
	"slices"
)

// operationFailure prepares a handler error for writeResponseError, which maps it with
// ProblemForError and logs it with the request's cut path. The status is decided per
// operation, as GetSystemInfo does: an error keeps its domain sentinel only when its problem
// status is one of published, the statuses the contract documents for the operation (for
// example 404 for an unknown ID). Any other failure is a server fault (500), whatever
// sentinel it wraps: a catalog read gets no client value besides its IDs, so for example
// domain.ErrInvalid from it means unreadable stored data, not a bad request.
//
// A failure after the request context ended needs no special case: writeResponseError
// sees the done context and writes nothing.
func operationFailure(operation string, err error, published ...int32) error {
	if slices.Contains(published, ProblemForError(err).Status) {
		return fmt.Errorf("%s: %w", operation, err)
	}

	return fmt.Errorf("%s: %w", operation, &serverFault{cause: err})
}

// serverFault hides the sentinels of its cause from errors.Is, so ProblemForError answers
// 500, while the log still gets the cause's full text.
type serverFault struct {
	cause error
}

func (fault *serverFault) Error() string {
	return fault.cause.Error()
}

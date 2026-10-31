package scan

import (
	"context"
	"errors"
	"fmt"
)

// Causes logged when a scan fails. The wrapped errors of the walker, resolver and prober
// may contain the absolute library root or tool paths, so the finish line logs these fixed
// texts (or texts built from root-relative paths and catalog errors) instead.
const (
	causeRootUnavailable   = "library folder is missing or unreadable"
	causeProberUnavailable = "the probing tool cannot be started"
	causeWalkFailed        = "walking the library folder failed"
	causeUnexpected        = "unexpected error"
)

// failure is an error that ends a scan with a specific error code. cause is safe to log:
// it never contains an absolute path.
type failure struct {
	code  ErrorCode
	cause string
	err   error
}

func (scanFailure *failure) Error() string {
	return fmt.Sprintf("%s: %v", scanFailure.code, scanFailure.err)
}

func (scanFailure *failure) Unwrap() error {
	return scanFailure.err
}

// fail wraps err so that the scan ends with code; cause is what the finish line logs.
func fail(code ErrorCode, cause string, err error) error {
	return &failure{code: code, cause: cause, err: err}
}

// describeEnd maps the error that ended a scan to its code and a loggable cause; canceled
// tells whether the scan context was done when the scan ended. Cancellation wins over the
// code of the step it interrupted, because a shutdown is not the step's fault. The context
// is consulted besides the error because not every interrupted step wraps context.Canceled:
// SQLite reports an interrupted statement as a plain "interrupted" error (ADR 0022 notes).
func describeEnd(err error, canceled bool) (ErrorCode, string) {
	if err == nil {
		return ErrorCodeNone, ""
	}

	if canceled || errors.Is(err, context.Canceled) {
		return ErrorCodeCanceled, ""
	}

	if scanFailure, ok := errors.AsType[*failure](err); ok {
		return scanFailure.code, scanFailure.cause
	}

	return ErrorCodeFailed, causeUnexpected
}

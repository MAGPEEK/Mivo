package playback

import (
	"errors"
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
)

// FailureCode is the stable, machine-readable reason Decide found no plan. Clients translate
// it; adapters/http answers it as the problem code.
type FailureCode string

// The failure codes of ADR 0023.
const (
	// FailureNoCompatibleFormat: the client can play neither the file nor anything the
	// server can produce from it.
	FailureNoCompatibleFormat FailureCode = "no_compatible_format"
	// FailureTranscodeUnavailable: the client could play a remux or transcode, but the
	// server's FFmpeg cannot produce it (missing FFmpeg, muxer or encoder).
	FailureTranscodeUnavailable FailureCode = "transcode_unavailable"
)

// Sentinel errors of Decide. Both wrap domain.ErrUnsupported, so callers that only know the
// shared sentinels still classify them; errors.Is with these tells the two apart.
var (
	// ErrNoCompatibleFormat wraps domain.ErrUnsupported with FailureNoCompatibleFormat.
	ErrNoCompatibleFormat = fmt.Errorf("%s: %w", FailureNoCompatibleFormat, domain.ErrUnsupported)
	// ErrTranscodeUnavailable wraps domain.ErrUnsupported with FailureTranscodeUnavailable.
	ErrTranscodeUnavailable = fmt.Errorf("%s: %w", FailureTranscodeUnavailable, domain.ErrUnsupported)
)

// FailureCodeOf returns the failure code of an error from Decide. The second result is
// false for every other error, including nil.
func FailureCodeOf(err error) (FailureCode, bool) {
	switch {
	case errors.Is(err, ErrNoCompatibleFormat):
		return FailureNoCompatibleFormat, true
	case errors.Is(err, ErrTranscodeUnavailable):
		return FailureTranscodeUnavailable, true
	default:
		return "", false
	}
}

package media

import (
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// ProbeError is the stable code stored for a file that ffprobe could not describe. Clients
// translate it; new codes may be added, so readers must tolerate unknown values.
type ProbeError string

// Probe error codes.
const (
	// ProbeErrorNone marks a successfully probed file.
	ProbeErrorNone ProbeError = ""
	// ProbeErrorFailed means ffprobe exited with an error or its output was unusable.
	ProbeErrorFailed ProbeError = "probe_failed"
	// ProbeErrorTimedOut means ffprobe did not finish within the per-file timeout.
	ProbeErrorTimedOut ProbeError = "probe_timed_out"
)

// File is one media file of a library together with its technical description.
type File struct {
	// ID, ItemID and LibraryID are canonical UUID texts assigned by the use cases.
	ID        string
	ItemID    string
	LibraryID string
	// RelativePath is slash-separated and relative to the library root. It was validated
	// by library.ParseRelativePath at the boundary; this package treats it as opaque.
	RelativePath string
	// Size is in bytes.
	Size       int64
	ModifiedAt time.Time
	// Description is the zero value when ProbeError is set.
	Description Description
	ProbeError  ProbeError
}

// Probed reports whether ffprobe described the file successfully.
func (file File) Probed() bool {
	return file.ProbeError == ProbeErrorNone
}

// Validate rejects a negative size, an empty relative path, an invalid description of a
// probed file and streams on a file whose probe failed. Errors wrap domain.ErrInvalid.
func (file File) Validate() error {
	if file.Size < 0 {
		return fmt.Errorf("validate file %s: negative size %d: %w", file.ID, file.Size, domain.ErrInvalid)
	}

	if file.RelativePath == "" {
		return fmt.Errorf("validate file %s: empty relative path: %w", file.ID, domain.ErrInvalid)
	}

	if !file.Probed() {
		return file.validateFailedProbe()
	}

	if err := file.Description.Validate(); err != nil {
		return fmt.Errorf("validate file %s: %w", file.ID, err)
	}

	return nil
}

// validateFailedProbe keeps half-probed data out of the catalog: a file either has a full
// description or a probe error, never stale streams next to an error.
func (file File) validateFailedProbe() error {
	if len(file.Description.Streams) > 0 {
		return fmt.Errorf("validate file %s: probe error %q with %d streams: %w",
			file.ID, file.ProbeError, len(file.Description.Streams), domain.ErrInvalid)
	}

	return nil
}

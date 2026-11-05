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
	// ProbeErrorDiscEncrypted, ProbeErrorDiscUnsupported and ProbeErrorDiscNoTitle are the
	// codes of the disc errors (DiscError, ADR 0029): a disc whose main title is CSS or AACS
	// protected, cannot be read, or does not exist.
	ProbeErrorDiscEncrypted   ProbeError = "disc_encrypted"
	ProbeErrorDiscUnsupported ProbeError = "disc_unsupported"
	ProbeErrorDiscNoTitle     ProbeError = "disc_no_title"
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
	// Layout says how the media is stored; empty means a single file, like LayoutFile. A disc
	// (Layout.IsDisc) is never played directly; RelativePath then names the BDMV or VIDEO_TS
	// folder or the image file, as the walker reported it (see Layout.DiscRoot).
	Layout Layout
	// Description is the zero value when ProbeError is set.
	Description Description
	ProbeError  ProbeError
}

// Probed reports whether ffprobe described the file successfully.
func (file File) Probed() bool {
	return file.ProbeError == ProbeErrorNone
}

// Validate rejects a negative size, an empty relative path, an unknown layout, an invalid
// description of a probed file and streams on a file whose probe failed. Errors wrap
// domain.ErrInvalid.
func (file File) Validate() error {
	if file.Size < 0 {
		return fmt.Errorf("validate file %s: negative size %d: %w", file.ID, file.Size, domain.ErrInvalid)
	}

	if file.RelativePath == "" {
		return fmt.Errorf("validate file %s: empty relative path: %w", file.ID, domain.ErrInvalid)
	}

	if file.Layout != "" && !file.Layout.Valid() {
		return fmt.Errorf("validate file %s: unknown layout %q: %w", file.ID, file.Layout, domain.ErrInvalid)
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

package media

import (
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

// Layout says how a media file is stored; the values match media_files.layout (ADR 0029).
type Layout string

// The known layouts.
const (
	// LayoutFile is a single video file.
	LayoutFile Layout = "file"
	// LayoutVideoTS is a DVD folder (VIDEO_TS).
	LayoutVideoTS Layout = "video_ts"
	// LayoutBDMV is a Blu-ray folder (BDMV).
	LayoutBDMV Layout = "bdmv"
	// LayoutISO is a disc image file.
	LayoutISO Layout = "iso"
)

// Valid reports whether the layout is one of the known values.
func (layout Layout) Valid() bool {
	switch layout {
	case LayoutFile, LayoutVideoTS, LayoutBDMV, LayoutISO:
		return true
	default:
		return false
	}
}

// IsDisc reports whether the layout is a disc (folder or image). Discs are never played directly.
func (layout Layout) IsDisc() bool {
	return layout.Valid() && layout != LayoutFile
}

// DiscRoot returns the path the main title of a disc is resolved and read from, given the
// slash-separated path the walker reported: for a disc folder the folder that holds BDMV or
// VIDEO_TS (so the disc resolver also sees an AACS folder next to BDMV, and the clip paths
// of the title start with "BDMV/" or "VIDEO_TS/"), for an image or a single file the path
// itself. A disc folder directly in the library root yields "."; it is never resolved,
// because naming.PlaceDisc leaves such a disc unmatched, so it is neither probed nor played.
func (layout Layout) DiscRoot(relativePath string) string {
	if layout == LayoutVideoTS || layout == LayoutBDMV {
		return path.Dir(relativePath)
	}

	return relativePath
}

// DiscSource is the main title of a disc inside a library folder, as the prober and the
// keyframe locator read it.
type DiscSource struct {
	// RelativePath is the disc root (Layout.DiscRoot), slash-separated and relative to the
	// library folder; the clip paths of Title are relative to it.
	RelativePath string
	// Layout is LayoutVideoTS, LayoutBDMV or LayoutISO.
	Layout Layout
	// Title lists the clips in playback order.
	Title DiscTitle
}

// DiscClip is one piece of a disc title: a file inside a disc folder, or a byte extent of an
// ISO image.
type DiscClip struct {
	// Path is the clip file, slash-separated and relative to the disc root; set for folder discs.
	Path string
	// Offset and Length locate the clip inside an ISO image in bytes; set for ISO discs.
	Offset, Length int64
	// Start is the presentation time inside the clip where the title's span begins (a Blu-ray
	// playlist item's IN_time); 0 plays the clip from its beginning. With a positive Duration
	// the span ends at Start + Duration, so a player cuts the clip to it instead of playing
	// the parts the playlist skips.
	Start    time.Duration
	Duration time.Duration
}

// Validate requires exactly one of Path or an extent with a positive length, and a
// non-negative offset, start and duration. Errors wrap domain.ErrInvalid.
func (clip DiscClip) Validate() error {
	hasPath := clip.Path != ""
	hasExtent := clip.Length != 0 || clip.Offset != 0

	switch {
	case hasPath == hasExtent:
		return fmt.Errorf("validate disc clip: need exactly one of path or extent: %w", domain.ErrInvalid)
	case hasExtent && (clip.Offset < 0 || clip.Length <= 0):
		return fmt.Errorf("validate disc clip %d+%d: bad extent: %w", clip.Offset, clip.Length, domain.ErrInvalid)
	case clip.Duration < 0:
		return fmt.Errorf("validate disc clip: negative duration: %w", domain.ErrInvalid)
	case clip.Start < 0:
		return fmt.Errorf("validate disc clip: negative start: %w", domain.ErrInvalid)
	}

	return nil
}

// DiscTitle is the ordered list of clips that make up the main feature of a disc.
type DiscTitle []DiscClip

// Duration is the summed clip duration.
func (title DiscTitle) Duration() time.Duration {
	var total time.Duration
	for _, clip := range title {
		total += clip.Duration
	}

	return total
}

// Validate requires at least one clip and valid clips. An empty title wraps ErrDiscNoTitle.
func (title DiscTitle) Validate() error {
	if len(title) == 0 {
		return fmt.Errorf("validate disc title: %w", ErrDiscNoTitle)
	}

	for index, clip := range title {
		if err := clip.Validate(); err != nil {
			return fmt.Errorf("disc title clip %d: %w", index, err)
		}
	}

	return nil
}

// DiscError is a disc failure with a stable code that clients translate.
type DiscError struct {
	Code string
	base error
}

// Error returns the stable code.
func (err *DiscError) Error() string { return err.Code }

// Unwrap exposes the generic domain error the HTTP layer maps.
func (err *DiscError) Unwrap() error { return err.base }

// Is matches another DiscError with the same code, so wrapped copies compare equal.
func (err *DiscError) Is(target error) bool {
	other, ok := target.(*DiscError)

	return ok && other.Code == err.Code
}

// Disc error codes and sentinels.
var (
	// ErrDiscEncrypted marks a CSS or AACS protected disc (never decrypted).
	ErrDiscEncrypted = &DiscError{Code: "disc_encrypted", base: domain.ErrUnsupported}
	// ErrDiscUnsupported marks a disc structure that cannot be read.
	ErrDiscUnsupported = &DiscError{Code: "disc_unsupported", base: domain.ErrUnsupported}
	// ErrDiscNoTitle marks a disc without a usable main title.
	ErrDiscNoTitle = &DiscError{Code: "disc_no_title", base: domain.ErrNotFound}
)

// DiscProbeError returns the probe error a disc failure is stored with (the code of a
// wrapped DiscError), or false.
func DiscProbeError(err error) (ProbeError, bool) {
	code, found := DiscErrorCode(err)

	return ProbeError(code), found
}

// DiscErrorCode returns the code of a wrapped DiscError, or false.
func DiscErrorCode(err error) (string, bool) {
	var discErr *DiscError
	if errors.As(err, &discErr) {
		return discErr.Code, true
	}

	return "", false
}

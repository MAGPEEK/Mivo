package disc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ResolveImage selects the main title of a disc image (BDMV preferred over VIDEO_TS at the
// image root) and returns it as image byte extents. A clip's duration is split over its
// extents by length; extents that continue each other are merged, so a contiguous DVD feature
// becomes one clip. A disc that looks CSS or AACS protected fails with media.ErrDiscEncrypted.
// Corrupt image structures met on the way are reported as disc_unsupported.
func ResolveImage(ctx context.Context, img Image) (media.DiscTitle, error) {
	title, err := resolveImage(ctx, img)
	if err != nil {
		return nil, classifyImageError(ctx, err)
	}

	return title, nil
}

// classifyImageError tags a corrupt-image error (iso.ErrInvalidImage) with ErrDiscUnsupported
// so clients get a stable code. Cancellation and errors that already carry a disc code pass
// unchanged.
func classifyImageError(ctx context.Context, err error) error {
	if _, coded := media.DiscErrorCode(err); coded || ctx.Err() != nil ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		!errors.Is(err, iso.ErrInvalidImage) {
		return err
	}

	return fmt.Errorf("%w: %w", media.ErrDiscUnsupported, err)
}

func resolveImage(ctx context.Context, img Image) (media.DiscTitle, error) {
	files := newFileTree(ctx, img)
	layout, err := imageLayout(files)
	if err != nil {
		return nil, err
	}
	clips, err := selectTitle(files, layout)
	if err != nil {
		return nil, err
	}
	if err := files.checkEncryption(layout, clips); err != nil {
		return nil, err
	}

	var title media.DiscTitle
	for _, clip := range clips {
		extentClips, err := clipExtents(img, clip)
		if err != nil {
			return nil, err
		}
		for _, extentClip := range extentClips {
			title = appendMerged(title, extentClip)
		}
	}

	return validated(title)
}

// imageLayout detects the disc structure at the image root.
func imageLayout(files *fileTree) (media.Layout, error) {
	for _, candidate := range []struct {
		folder string
		layout media.Layout
	}{{bdmvFolderName, media.LayoutBDMV}, {videoTSFolderName, media.LayoutVideoTS}} {
		_, found, err := files.childDir(".", candidate.folder)
		if err != nil {
			return "", fmt.Errorf("detect disc image layout: %w", err)
		}
		if found {
			return candidate.layout, nil
		}
	}

	return "", fmt.Errorf("detect disc image layout: no BDMV or VIDEO_TS folder: %w", media.ErrDiscUnsupported)
}

// clipExtents maps one clip file to its non-empty image extents.
func clipExtents(img Image, clip titleClip) ([]media.DiscClip, error) {
	extents, err := img.Extents(clip.path)
	if err != nil {
		return nil, fmt.Errorf("locate disc clip %s: %w", clip.path, err)
	}
	lengths := make([]int64, len(extents))
	for index, extent := range extents {
		lengths[index] = extent.Length
	}
	shares := splitDuration(clip.duration, lengths)

	located := make([]media.DiscClip, 0, len(extents))
	for index, extent := range extents {
		if extent.Length > 0 {
			located = append(located, media.DiscClip{Offset: extent.Offset, Length: extent.Length, Duration: shares[index]})
		}
	}
	if len(located) == 0 {
		return nil, fmt.Errorf("locate disc clip %s: no data: %w", clip.path, media.ErrDiscNoTitle)
	}

	return located, nil
}

// appendMerged appends clip, merging it into the last clip when it starts where that ends.
func appendMerged(title media.DiscTitle, clip media.DiscClip) media.DiscTitle {
	if last := len(title) - 1; last >= 0 && title[last].Offset+title[last].Length == clip.Offset {
		title[last].Length += clip.Length
		title[last].Duration += clip.Duration
		return title
	}

	return append(title, clip)
}

// splitDuration divides total over weights proportionally. float64 avoids the int64 overflow
// of duration*weight; the last share takes the remainder so the shares sum to total exactly.
func splitDuration(total time.Duration, weights []int64) []time.Duration {
	shares := make([]time.Duration, len(weights))
	if len(weights) == 0 {
		return shares
	}
	var weightSum float64
	for _, weight := range weights {
		weightSum += float64(max(weight, 0))
	}

	var assigned time.Duration
	last := len(weights) - 1
	for index := range last {
		if weightSum > 0 {
			share := time.Duration(float64(total) * (float64(max(weights[index], 0)) / weightSum))
			shares[index] = min(share, total-assigned) // float rounding never overshoots total
		}
		assigned += shares[index]
	}
	shares[last] = total - assigned

	return shares
}

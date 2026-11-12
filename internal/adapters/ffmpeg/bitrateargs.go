package ffmpeg

import (
	"fmt"
	"strconv"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Bit-rate cap of a quality step (ADR 0030 section 6).
const (
	// minVideoBitRate is the smallest video budget (200 kbit/s): below it the picture is
	// unwatchable, so a cap that leaves less (after the audio) is raised to this value instead
	// of starving the encoder. The stream then exceeds such a cap by design.
	minVideoBitRate = 200 * bitsPerKilobit
	// maxBitRateCap bounds a request's cap (1 Gbit/s, far above any real stream) so a corrupt
	// value cannot reach the arguments.
	maxBitRateCap = 1000 * 1000 * bitsPerKilobit
	// bufferSizeFactor sizes the rate-control buffer as a multiple of the maximum rate, which
	// lets constrained CRF spend a short burst on a complex scene without breaking the cap.
	bufferSizeFactor = 2
)

// videoBudget returns the video bit rate in bits per second a stream capped at capBitsPerSecond
// leaves after audioBitsPerSecond, never below minVideoBitRate.
func videoBudget(capBitsPerSecond, audioBitsPerSecond int64) int64 {
	return max(capBitsPerSecond-audioBitsPerSecond, minVideoBitRate)
}

// bitrateArguments returns the rate-control options of a capped libx264 encode: constant
// quality (-crf, kept by the caller) bounded by -maxrate and a -bufsize of twice that, so
// the keyframe-aligned segments of ADR 0024 stay as they are. It returns nothing without a
// cap and an error for a negative or absurd one.
func bitrateArguments(capBitsPerSecond, audioBitsPerSecond int64) ([]string, error) {
	if capBitsPerSecond < 0 || capBitsPerSecond > maxBitRateCap {
		return nil, fmt.Errorf("video: bit-rate cap %d is outside 0..%d: %w", capBitsPerSecond, maxBitRateCap, domain.ErrInvalid)
	}

	if capBitsPerSecond == playback.OriginalQuality {
		return nil, nil
	}

	maxRate := videoBudget(capBitsPerSecond, audioBitsPerSecond) / bitsPerKilobit

	return []string{
		"-maxrate", strconv.FormatInt(maxRate, 10) + "k",
		"-bufsize", strconv.FormatInt(maxRate*bufferSizeFactor, 10) + "k",
	}, nil
}

// audioBitsPerSecond is the bit rate the audio of the plan takes from the cap: the AAC rate
// of its channel layout (a copied stream is assumed to need as much), 0 without audio.
func (streams hlsStreams) audioBitsPerSecond() int64 {
	if streams.audio == nil {
		return 0
	}

	return aacBitsPerSecond(streams.audio.TargetChannels)
}

package playback

import "github.com/magpeek/mivo/internal/domain/media"

// ReasonBitrateCap says that Request.MaxBitrate lies below the bit rate of the source, so the video is
// re-encoded although the client plays it (ADR 0030 section 6).
const ReasonBitrateCap ReasonCode = "bitrate_cap"

// OriginalQuality is the Request.MaxBitrate that plays the source as it is.
const OriginalQuality int64 = 0

// unknownBitrate is what the probe reports when the container does not tell a bit rate.
const unknownBitrate int64 = 0

// bitsPerMegabit converts the ladder's bit rates, which are named in Mbit/s.
const bitsPerMegabit int64 = 1_000_000

// Output heights of the quality steps.
const (
	ladderHeight2160 = 2160
	ladderHeight1080 = 1080
	ladderHeight720  = 720
	ladderHeight480  = 480
)

// Bit rates of the quality steps in Mbit/s.
const (
	ladderMegabits20 int64 = 20
	ladderMegabits12 int64 = 12
	ladderMegabits8  int64 = 8
	ladderMegabits4  int64 = 4
	ladderMegabits2  int64 = 2
	ladderMegabits1  int64 = 1
)

// LadderStep is one quality step a client offers (PL-10): the bit rate of the whole stream
// and the greatest height the video is encoded at for it.
type LadderStep struct {
	// MaxBitrate is in bits per second and is what a client sends as maxBitRate.
	MaxBitrate int64
	// Height is always even, because it becomes the height of H.264 4:2:0.
	Height int
}

// Ladder returns the fixed quality steps from the highest to the lowest (ADR 0030 section 6).
// Every call returns a fresh slice, so callers cannot change the steps of each other.
func Ladder() []LadderStep {
	return []LadderStep{
		{MaxBitrate: ladderMegabits20 * bitsPerMegabit, Height: ladderHeight2160},
		{MaxBitrate: ladderMegabits12 * bitsPerMegabit, Height: ladderHeight1080},
		{MaxBitrate: ladderMegabits8 * bitsPerMegabit, Height: ladderHeight1080},
		{MaxBitrate: ladderMegabits4 * bitsPerMegabit, Height: ladderHeight720},
		{MaxBitrate: ladderMegabits2 * bitsPerMegabit, Height: ladderHeight480},
		{MaxBitrate: ladderMegabits1 * bitsPerMegabit, Height: ladderHeight480},
	}
}

// ladderStep returns the step of a cap: the highest step that stays within it, so a cap
// between two steps gets the size of the lower one, and the lowest step for a cap below the
// ladder, because no smaller size is defined.
func ladderStep(maxBitrate int64) LadderStep {
	steps := Ladder()

	for _, step := range steps {
		if step.MaxBitrate <= maxBitrate {
			return step
		}
	}

	return steps[len(steps)-1]
}

// bitrateCapReasons returns bitrate_cap when the request's cap forces a re-encode of the
// video. The video's own bit rate decides; Matroska rarely stores it, then the bit rate of the
// whole file stands in. When neither is known the cap is honoured: a transcode that was not
// needed costs CPU, an original far above the cap stalls on the link the user chose the cap
// for.
func bitrateCapReasons(input Input, video media.Stream) []Reason {
	maxBitrate := input.Request.MaxBitrate
	if maxBitrate == OriginalQuality {
		return nil
	}

	source := video.BitRate
	if source == unknownBitrate {
		source = input.Description.BitRate
	}

	if source != unknownBitrate && source <= maxBitrate {
		return nil
	}

	return []Reason{{Code: ReasonBitrateCap, Scope: ScopeVideo}}
}

// transcodeHeightCap is the greatest height of a transcoded video: the operator's limit and
// the step of the request's cap, whichever is lower. A source below it keeps its height
// (targetSize never scales up), so a 1920x800 film is not shrunk for a 1080p step.
func transcodeHeightCap(input Input) int {
	heightCap := input.Policy.MaxTranscodeHeight
	if input.Request.MaxBitrate == OriginalQuality {
		return heightCap
	}

	stepHeight := ladderStep(input.Request.MaxBitrate).Height
	if heightCap == noHeightCap || stepHeight < heightCap {
		return stepHeight
	}

	return heightCap
}

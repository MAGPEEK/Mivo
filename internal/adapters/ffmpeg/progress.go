package ffmpeg

import (
	"bytes"
	"math"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

const (
	// progressFlag makes FFmpeg print key=value blocks about its progress to progressTarget,
	// its standard output; noStatsFlag turns off the human-readable line it would otherwise
	// write to the error output, whose tail is kept for failure logs.
	progressFlag   = "-progress"
	progressTarget = "pipe:1"
	noStatsFlag    = "-nostats"

	// progressInterval is the shortest time between two published snapshots (ADR 0030 §7).
	// FFmpeg reports about twice as often.
	progressInterval = time.Second
	// maxProgressLineBytes bounds a kept line. FFmpeg's progress lines are far shorter;
	// anything longer is not progress and is dropped.
	maxProgressLineBytes = 512

	// The keys of a progress block that are read. FFmpeg also prints total_size, frame
	// counts and one quantiser per stream; the output bit rate it derives from total_size is
	// read from progressKeyBitRate, so none of those is kept.
	progressKeyOutputTime = "out_time_us"
	progressKeySpeed      = "speed"
	progressKeyFrameRate  = "fps"
	progressKeyBitRate    = "bitrate"
	// progressKeyState is the last line of every block.
	progressKeyState = "progress"
	// progressStateContinue and progressStateEnd are the values of progressKeyState: more
	// blocks follow, or this was the last one.
	progressStateContinue = "continue"
	progressStateEnd      = "end"

	progressSeparator = "="
	lineEnd           = '\n'
	// speedSuffix and bitRateSuffix follow the numbers of the speed ("1.02x") and the bit
	// rate ("1234.5kbits/s"). A value FFmpeg does not know is "N/A" and has neither.
	speedSuffix    = "x"
	bitRateSuffix  = "kbits/s"
	bitsPerKilobit = 1000
)

// withProgressArguments puts the global options that make FFmpeg report its progress in
// front of arguments.
func withProgressArguments(arguments []string) []string {
	return append([]string{progressFlag, progressTarget, noStatsFlag}, arguments...)
}

// progressTracker reads FFmpeg's -progress output as the standard output writer of the
// process and keeps the latest snapshot. Write never fails and keeps at most one short line,
// so the pipe is always drained and FFmpeg never blocks on it. Progress lines are not logged:
// there are two blocks per second and session.
//
// Write is called by one goroutine, the output copier of os/exec; snapshot is safe to call
// from any goroutine at any time.
type progressTracker struct {
	now      func() time.Time
	interval time.Duration
	latest   atomic.Pointer[appplayback.Progress]

	// The fields below belong to the writing goroutine.

	// line is the line read so far; overlong is set while the rest of a line that exceeded
	// maxProgressLineBytes is skipped.
	line     []byte
	overlong bool
	// block holds the values read so far. A value FFmpeg reports as unknown keeps the last
	// known one: at the end of a transcode the speed is "N/A" for several blocks.
	block appplayback.Progress
	// published is when the latest snapshot was stored; zero before the first.
	published time.Time
}

func newProgressTracker() *progressTracker {
	return &progressTracker{
		now:      time.Now,
		interval: progressInterval,
		line:     make([]byte, 0, maxProgressLineBytes),
	}
}

// snapshot returns the latest published progress without blocking; zero before the first block.
func (t *progressTracker) snapshot() appplayback.Progress {
	if latest := t.latest.Load(); latest != nil {
		return *latest
	}

	return appplayback.Progress{}
}

// Write implements io.Writer for the standard output of FFmpeg. It always accepts the whole
// chunk: an error would make os/exec stop reading the pipe.
func (t *progressTracker) Write(chunk []byte) (int, error) {
	rest := chunk

	for len(rest) > 0 {
		end := bytes.IndexByte(rest, lineEnd)
		if end < 0 {
			t.keep(rest)

			break
		}

		t.keep(rest[:end])
		t.endLine()

		rest = rest[end+1:]
	}

	return len(chunk), nil
}

// keep appends a part of the current line, or starts skipping the line when it gets too long.
func (t *progressTracker) keep(part []byte) {
	if t.overlong || len(t.line)+len(part) > maxProgressLineBytes {
		t.overlong = true
		t.line = t.line[:0]

		return
	}

	t.line = append(t.line, part...)
}

func (t *progressTracker) endLine() {
	if !t.overlong {
		t.apply(string(t.line))
	}

	t.line = t.line[:0]
	t.overlong = false
}

// apply reads one line into the current block. Lines that are malformed, carry an unknown key
// or a value FFmpeg does not know ("N/A") change nothing.
func (t *progressTracker) apply(line string) {
	key, value, found := strings.Cut(strings.TrimSpace(line), progressSeparator)
	if !found {
		return
	}

	value = strings.TrimSpace(value)

	switch key {
	case progressKeyOutputTime:
		if outputTime, valid := parseOutputTime(value); valid {
			t.block.OutputTime = outputTime
		}
	case progressKeySpeed:
		if speed, valid := parseSuffixedNumber(value, speedSuffix); valid {
			t.block.Speed = speed
		}
	case progressKeyFrameRate:
		if frameRate, valid := parseNonNegativeNumber(value); valid {
			t.block.FPS = frameRate
		}
	case progressKeyBitRate:
		if bitRate, valid := parseBitRate(value); valid {
			t.block.BitRate = bitRate
		}
	case progressKeyState:
		t.endBlock(value)
	}
}

// endBlock publishes the block unless the last snapshot is younger than the interval. The
// last block of a run is always published, because no later one would make up for it.
func (t *progressTracker) endBlock(state string) {
	now := t.now()
	if state != progressStateEnd && !t.published.IsZero() && now.Sub(t.published) < t.interval {
		return
	}

	snapshot := t.block
	t.latest.Store(&snapshot)
	t.published = now
}

// parseOutputTime reads a count of microseconds that fits a time.Duration.
func parseOutputTime(value string) (time.Duration, bool) {
	microseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || microseconds < 0 || microseconds > math.MaxInt64/int64(time.Microsecond) {
		return 0, false
	}

	return time.Duration(microseconds) * time.Microsecond, true
}

// parseBitRate reads "1234.5kbits/s" as bits per second.
func parseBitRate(value string) (int64, bool) {
	kilobits, valid := parseSuffixedNumber(value, bitRateSuffix)
	if !valid {
		return 0, false
	}

	bits := math.Round(kilobits * bitsPerKilobit)
	if bits >= math.MaxInt64 {
		return 0, false
	}

	return int64(bits), true
}

// parseSuffixedNumber reads a number followed by its unit; FFmpeg pads the number with spaces.
func parseSuffixedNumber(value, suffix string) (float64, bool) {
	number, found := strings.CutSuffix(value, suffix)
	if !found {
		return 0, false
	}

	return parseNonNegativeNumber(strings.TrimSpace(number))
}

func parseNonNegativeNumber(value string) (float64, bool) {
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, false
	}

	return number, true
}

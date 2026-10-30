package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/magpeek/mivo/internal/domain"
)

const (
	// keyframeTimeout bounds a whole keyframe search; the viewer waits for it on every seek.
	keyframeTimeout = 20 * time.Second
	// keyframeSearchWindow is how far before the position the first search starts. It covers
	// the keyframe intervals of common encodes (2–10 s).
	keyframeSearchWindow = 10 * time.Second
	// keyframeWideSearchWindow is the fallback for long GOPs (broadcast recordings, some
	// HEVC encodes). Beyond it the seek starts at the beginning of the file.
	keyframeWideSearchWindow = 60 * time.Second
	// keyframeEndMargin reads a little past the position, so a keyframe exactly at the
	// position is decoded even when the end check of ffprobe cuts the interval early.
	keyframeEndMargin = time.Second
	// seekVideoSelector selects the first video stream that is not cover art. It is meant to
	// match the stream FFmpeg seeks on when -ss is used with stream copy (its default stream,
	// av_find_default_stream_index), whose keyframes decide where the copy starts. It may
	// differ from media.Description.PrimaryVideo, which prefers the default disposition.
	seekVideoSelector = "V:0"
	keyframeEntries   = "frame=key_frame,pts_time,best_effort_timestamp_time:format=start_time"
)

// keyframeSearchWindows are tried in order until one contains a keyframe.
func keyframeSearchWindows() []time.Duration {
	return []time.Duration{keyframeSearchWindow, keyframeWideSearchWindow}
}

// KeyframeAtOrBefore returns the position of the last video keyframe at or before position,
// measured like position from the start of the file (the format start time, which is
// ≈ 1.4 s for MPEG-TS). A stream-copy session started with -ss at the returned position
// begins exactly there (ADR 0024). When no keyframe is found within the search windows the
// result is 0, the start of the file. Errors wrap domain.ErrInvalid for a relative path,
// context errors on timeout or cancellation, domain.ErrUnavailable when ffprobe cannot be
// started and domain.ErrUnsupported when it failed on the file. A file without a format
// start time also fails with domain.ErrUnsupported: ffprobe rejects the relative
// -read_intervals offsets the search uses ("current timestamp is not defined"), so callers
// that can live without an exact keyframe fall back to position 0.
func (p *Prober) KeyframeAtOrBefore(ctx context.Context, absolutePath string, position time.Duration) (time.Duration, error) {
	input, err := inputArguments(absolutePath)
	if err != nil {
		return 0, fmt.Errorf("find keyframe: %w", err)
	}

	if position <= 0 {
		return 0, nil
	}

	searchContext, cancel := context.WithTimeout(ctx, p.keyframeTimeout)
	defer cancel()

	for _, window := range keyframeSearchWindows() {
		keyframe, found, err := p.searchKeyframe(searchContext, input, position, window)
		if err != nil {
			return 0, fmt.Errorf("find keyframe before %s in %s: %w", position, filepath.Base(absolutePath), err)
		}

		if found {
			return keyframe, nil
		}

		// This window already started at the beginning of the file; a wider one reads
		// the same frames again.
		if window >= position {
			break
		}
	}

	return 0, nil
}

// searchKeyframe decodes only the keyframes of the seek video stream between
// position - window and position. input is the result of inputArguments.
func (p *Prober) searchKeyframe(ctx context.Context, input []string, position, window time.Duration) (time.Duration, bool, error) {
	arguments := append([]string{
		"-v", "error", "-hide_banner", "-select_streams", seekVideoSelector, "-skip_frame", "nokey",
		"-show_entries", keyframeEntries, "-read_intervals", readInterval(position, window),
		"-print_format", "json",
	}, input...)

	output, err := p.ffprobe.output(ctx, arguments)
	if err != nil {
		return 0, false, classifyRunError(err)
	}

	return keyframeInOutput(output, position)
}

// readInterval builds ffprobe's -read_intervals value. Both ends are offsets: a start with
// "+" counts from the format start time, so positions need no conversion, and an end with
// "+" counts from the start of the interval. ffprobe may seek to a keyframe after the
// interval start, which is why the interval begins a whole window before the position.
func readInterval(position, window time.Duration) string {
	start := max(position-window, 0)
	length := position - start + keyframeEndMargin

	return "+" + formatSeconds(start) + "%+" + formatSeconds(length)
}

// formatSeconds prints a non-negative duration as decimal seconds with microsecond
// precision, the resolution of FFmpeg's time base AV_TIME_BASE.
func formatSeconds(duration time.Duration) string {
	return fmt.Sprintf("%d.%06d", duration/time.Second, (duration%time.Second)/time.Microsecond)
}

// keyframeOutput is the part of ffprobe's -show_entries frame=…:format=start_time JSON
// the search reads.
type keyframeOutput struct {
	Frames []keyframeFrame `json:"frames"`
	Format *struct {
		StartTime string `json:"start_time"`
	} `json:"format"`
}

type keyframeFrame struct {
	// KeyFrame is nil when ffprobe omits the field; -skip_frame nokey already dropped the
	// other frames then.
	KeyFrame                *int   `json:"key_frame"`
	PresentationTime        string `json:"pts_time"`
	BestEffortTimestampTime string `json:"best_effort_timestamp_time"`
}

// timestamp prefers the best-effort timestamp, which FFmpeg fills in for containers
// without presentation timestamps (AVI).
func (frame keyframeFrame) timestamp() (time.Duration, bool) {
	if timestamp, ok := parseSeconds(frame.BestEffortTimestampTime); ok {
		return timestamp, true
	}

	return parseSeconds(frame.PresentationTime)
}

// keyframeInOutput returns the last keyframe at or before position, relative to the format
// start time. The second result is false when the output holds no such keyframe.
func keyframeInOutput(output []byte, position time.Duration) (time.Duration, bool, error) {
	var parsed keyframeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return 0, false, fmt.Errorf("decode ffprobe frames: %w: %w", domain.ErrUnsupported, err)
	}

	var start time.Duration
	if parsed.Format != nil {
		start, _ = parseSeconds(parsed.Format.StartTime)
	}

	target := start + position

	var (
		last  time.Duration
		found bool
	)

	for _, frame := range parsed.Frames {
		if frame.KeyFrame != nil && *frame.KeyFrame == 0 {
			continue
		}

		if timestamp, ok := frame.timestamp(); ok && timestamp <= target && (!found || timestamp > last) {
			last, found = timestamp, true
		}
	}

	if !found {
		return 0, false, nil
	}

	// A keyframe before the format start time (other streams start later) is the start.
	return max(last-start, 0), true, nil
}

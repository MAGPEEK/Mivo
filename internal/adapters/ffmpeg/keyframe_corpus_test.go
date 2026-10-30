package ffmpeg_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
)

const (
	// seekKeyframeInterval is the GOP length of the corpus seek clips (M2-06).
	seekKeyframeInterval = 2 * time.Second
	// keyframeTolerance absorbs timestamp rounding of the container time bases.
	keyframeTolerance = time.Millisecond
	// firstKeyframeProbe is a position just after the first keyframe of every seek clip.
	firstKeyframeProbe = 500 * time.Millisecond
)

// Positions are measured from the format start time, like FFmpeg's -ss. That start differs
// from the first keyframe: the MPEG-TS clip starts at pts ≈ 1.43 s with its first keyframe
// 10 ms later (M2-06), and FFmpeg 7.1 starts the Matroska clip at -0.021 s because of the
// AAC priming samples while 9.0 reports 0. Expected keyframes are therefore computed from
// the first keyframe, not from 0.
func TestKeyframeAtOrBeforeOnSeekClips(t *testing.T) {
	ffprobePath, manifest := requireCorpus(t)
	prober := ffmpeg.NewProber(ffprobePath)

	for _, name := range []string{"video-mkv-h264-aac-long", "video-ts-mpeg2-mp2-long"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assertKeyframeGrid(t, prober, corpusPath(t, corpusCaseByName(t, manifest, name).File))
		})
	}
}

// assertKeyframeGrid checks that the keyframes of a seek clip lie seekKeyframeInterval apart
// from the first one.
func assertKeyframeGrid(t *testing.T, prober *ffmpeg.Prober, path string) {
	t.Helper()

	first := keyframeAt(t, prober, path, firstKeyframeProbe)
	if first < 0 || first >= seekKeyframeInterval/2 {
		t.Fatalf("first keyframe at %s, want close to the start", first)
	}

	for _, position := range []time.Duration{2 * time.Second, 2500 * time.Millisecond, 21 * time.Second, 29 * time.Second} {
		intervals := (position - first) / seekKeyframeInterval
		want := first + intervals*seekKeyframeInterval

		if got := keyframeAt(t, prober, path, position); (got-want).Abs() > keyframeTolerance || got > position {
			t.Errorf("keyframe at or before %s = %s, want %s (first keyframe %s)", position, got, want, first)
		}
	}

	// Beyond the end the last keyframe is the answer.
	if got, last := keyframeAt(t, prober, path, 45*time.Second), keyframeAt(t, prober, path, 29900*time.Millisecond); got != last {
		t.Errorf("keyframe beyond the end = %s, want the last keyframe %s", got, last)
	}
}

func keyframeAt(t *testing.T, prober *ffmpeg.Prober, path string, position time.Duration) time.Duration {
	t.Helper()

	keyframe, err := prober.KeyframeAtOrBefore(t.Context(), path, position)
	if err != nil {
		t.Fatalf("KeyframeAtOrBefore(%s): %v", position, err)
	}

	return keyframe
}

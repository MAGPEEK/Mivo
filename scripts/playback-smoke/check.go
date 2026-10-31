package main

import (
	"fmt"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// The transcode targets of milestone 2 (ADR 0023), written out so the smoke test states the
// promise instead of repeating the engine's constants.
const (
	transcodedVideoCodec   = "h264"
	transcodedVideoProfile = "High"
	transcodedPixelFormat  = "yuv420p"
	transcodedAudioCodec   = "aac"
)

// exactStartTolerance absorbs the float seconds of the contract when a start must be exact.
const exactStartTolerance = time.Millisecond

// planMismatches compares mode, stream actions, reasons and warnings of the session with
// the expectation. fatal is true when mode or an action differs: the output checks would
// only repeat that mismatch.
func planMismatches(session apigen.PlaybackSession, want corpustest.Expectation) (mismatches []string, fatal bool) {
	video, audio := streamAction(session, apigen.MediaStreamKindVideo), streamAction(session, apigen.MediaStreamKindAudio)

	if string(session.Mode) != string(want.Mode) || video != string(want.Video) || audio != string(want.Audio) {
		mismatches = append(mismatches, fmt.Sprintf("plan = %s (video %q, audio %q), want %s (video %q, audio %q)",
			session.Mode, video, audio, want.Mode, want.Video, want.Audio))
		fatal = true
	}

	if got, expected := reasonCodes(session.Reasons), wantReasons(want.Reasons); !slices.Equal(got, expected) {
		mismatches = append(mismatches, fmt.Sprintf("reasons = %v, want %v", got, expected))
	}

	if got, expected := reasonCodes(session.Warnings), wantWarnings(want.Warnings); !slices.Equal(got, expected) {
		mismatches = append(mismatches, fmt.Sprintf("warnings = %v, want %v", got, expected))
	}

	return mismatches, fatal
}

// streamAction returns the action of the first stream decision of the kind, or "".
func streamAction(session apigen.PlaybackSession, kind apigen.MediaStreamKind) string {
	for _, decision := range session.Streams {
		if decision.Kind == kind {
			return string(decision.Action)
		}
	}

	return ""
}

// reasonCodes renders reasons as "scope:code" in order, as playback.Decide reports them.
func reasonCodes(reasons []apigen.PlaybackReason) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, string(reason.Scope)+":"+reason.Code)
	}

	return codes
}

func wantReasons(reasons []playback.Reason) []string {
	codes := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		codes = append(codes, string(reason.Scope)+":"+string(reason.Code))
	}

	return codes
}

func wantWarnings(warnings []playback.Warning) []string {
	codes := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		codes = append(codes, string(warning.Scope)+":"+string(warning.Code))
	}

	return codes
}

// startMismatch checks the reported start: exact for a re-encoded video or a run from the
// beginning; a copied video starts at a keyframe whose position depends on the format start
// time FFmpeg versions report differently (−0.021 s in 7.1, 0 in 9.0 for the long MKV).
func startMismatch(session apigen.PlaybackSession, smoke smokeCase) (string, bool) {
	tolerance := exactStartTolerance
	if smoke.start > 0 && smoke.want.Video == playback.ActionCopy {
		tolerance = corpustest.KeyframeTolerance
	}

	start := seconds(session.StartPositionSeconds)
	if (start - smoke.wantStart).Abs() <= tolerance {
		return "", false
	}

	return fmt.Sprintf("start position = %s for a request at %s, want %s (±%s)",
		start, smoke.start, smoke.wantStart, tolerance), true
}

// outputMismatches checks the probed stream: the target codecs of re-encoded streams, the
// source codecs of copied ones, and the length of the clip after the session's start.
func outputMismatches(smoke smokeCase, start time.Duration, output outputSummary) []string {
	var mismatches []string

	wantVideo, wantAudio := smoke.clip.Expected.Video.Codec, smoke.clip.defaultAudioCodec()

	if smoke.want.Video == playback.ActionTranscode {
		wantVideo = transcodedVideoCodec

		if output.videoProfile != transcodedVideoProfile || output.pixelFormat != transcodedPixelFormat {
			mismatches = append(mismatches, fmt.Sprintf("transcoded video = profile %q, %s; want %s, %s",
				output.videoProfile, output.pixelFormat, transcodedVideoProfile, transcodedPixelFormat))
		}
	}

	if smoke.want.Audio == playback.ActionTranscode {
		wantAudio = transcodedAudioCodec
	}

	if output.videoCodec != wantVideo || output.audioCodec != wantAudio {
		mismatches = append(mismatches, fmt.Sprintf("output codecs = video %q, audio %q; want video %q, audio %q",
			output.videoCodec, output.audioCodec, wantVideo, wantAudio))
	}

	wantDuration := smoke.clip.duration() - start
	if (output.duration - wantDuration).Abs() > corpustest.DurationTolerance {
		mismatches = append(mismatches, fmt.Sprintf("output duration = %s, want %s (±%s)",
			output.duration.Round(time.Millisecond), wantDuration, corpustest.DurationTolerance))
	}

	return mismatches
}

func seconds(value float64) time.Duration {
	return time.Duration(value * float64(time.Second))
}

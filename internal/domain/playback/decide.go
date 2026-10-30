package playback

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// streamPlanner decides one selected stream of an HLS plan.
type streamPlanner func(stream media.Stream, input Input) (StreamDecision, []Reason, error)

// Decide picks the cheapest mode the client plays and the server can produce, following the
// rules of ADR 0023 in order: stream selection, direct play, remux, transcode.
//
// Errors wrap domain.ErrInvalid for input that breaks a rule (profile, description, start
// position, unknown audio index, policy), ErrNoCompatibleFormat when the client can play
// nothing the server could deliver, ErrTranscodeUnavailable when the server's FFmpeg cannot
// produce what the client plays, and domain.ErrUnsupported for a file without audio and
// video.
func Decide(input Input) (Plan, error) {
	if err := input.validate(); err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	chosen, err := selectStreams(input.Description, input.Request)
	if err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	if !chosen.hasVideo && !chosen.hasAudio {
		return Plan{}, fmt.Errorf("decide playback: file has neither video nor audio: %w", domain.ErrUnsupported)
	}

	reasons := directPlayReasons(chosen, input.Description.Container, input.Profile)
	if len(reasons) == 0 {
		return directPlayPlan(input.Description.Container, chosen), nil
	}

	plan, err := hlsPlan(input, chosen, reasons)
	if err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	return plan, nil
}

func directPlayPlan(container media.Container, chosen selection) Plan {
	plan := Plan{
		Mode: ModeDirectPlay, Delivery: DeliveryFile,
		SourceContainer: container, TargetContainer: container,
		Warnings: subtitleWarnings(chosen),
	}

	if chosen.hasVideo {
		plan.Streams = append(plan.Streams, copyDecision(chosen.video))
	}

	if chosen.hasAudio {
		plan.Streams = append(plan.Streams, copyDecision(chosen.audio))
	}

	return plan
}

// hlsPlan builds a remux or transcode plan. directPlayReasons explain why the original file
// is not served; the copy checks add why a stream is re-encoded. Failures follow one
// precedence: every client limit (no_compatible_format) before any server limit
// (transcode_unavailable).
func hlsPlan(input Input, chosen selection, directPlayReasons []Reason) (Plan, error) {
	if input.Profile.HLS.Delivery == HLSDeliveryNone {
		return Plan{}, fmt.Errorf("client plays no HLS and not the original file: %w", ErrNoCompatibleFormat)
	}

	plan := Plan{
		Mode: ModeRemux, Delivery: DeliveryHLS,
		SourceContainer: input.Description.Container, TargetContainer: media.ContainerMP4,
		Reasons: directPlayReasons,
	}

	planned := []struct {
		present bool
		stream  media.Stream
		decide  streamPlanner
	}{
		{present: chosen.hasVideo, stream: chosen.video, decide: planVideo},
		{present: chosen.hasAudio, stream: chosen.audio, decide: planAudio},
	}

	for _, entry := range planned {
		if !entry.present {
			continue
		}

		decision, reasons, err := entry.decide(entry.stream, input)
		if err != nil {
			return Plan{}, err
		}

		plan.add(decision, reasons)
	}

	// Server limits are checked only after every stream passed the client checks, so a
	// client that cannot play the result hears no_compatible_format, never a hint that a
	// better FFmpeg would help.
	if err := missingServerCapability(input.Capabilities, plan.Streams); err != nil {
		return Plan{}, err
	}

	plan.Warnings = hlsWarnings(plan, chosen)

	return plan, nil
}

// add records a stream decision; one re-encoded stream makes the whole plan a transcode.
func (plan *Plan) add(decision StreamDecision, reasons []Reason) {
	plan.Streams = append(plan.Streams, decision)
	plan.Reasons = appendUnique(plan.Reasons, reasons...)

	if decision.Action == ActionTranscode {
		plan.Mode = ModeTranscode
	}
}

// hlsWarnings lists the degradations of a transcoded video and the ignored subtitles. A
// copied HDR video keeps its signal, so only a transcode loses it.
func hlsWarnings(plan Plan, chosen selection) []Warning {
	var warnings []Warning

	if video, found := plan.Video(); found && video.Action == ActionTranscode {
		if isHDR(chosen.video) {
			warnings = append(warnings, Warning{Code: WarningHDRNotToneMapped, Scope: ScopeVideo})
		}

		if video.Downscaled {
			warnings = append(warnings, Warning{Code: WarningVideoDownscaled, Scope: ScopeVideo})
		}
	}

	return append(warnings, subtitleWarnings(chosen)...)
}

// subtitleWarnings reports subtitles, which milestone 2 ignores in every mode.
func subtitleWarnings(chosen selection) []Warning {
	if !chosen.hasSubtitles {
		return nil
	}

	return []Warning{{Code: WarningSubtitlesNotSupported, Scope: ScopeSubtitle}}
}

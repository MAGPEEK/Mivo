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
// position, unknown audio index or subtitle track id, invalid external subtitles, policy),
// ErrNoCompatibleFormat when the client can play
// nothing the server could deliver, ErrTranscodeUnavailable when the server's FFmpeg cannot
// produce what the client plays, and domain.ErrUnsupported for a file without audio and
// video.
func Decide(input Input) (Plan, error) {
	if err := input.validate(); err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	chosen, err := selectStreams(input)
	if err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	if !chosen.hasVideo && !chosen.hasAudio {
		return Plan{}, fmt.Errorf("decide playback: file has neither video nor audio: %w", domain.ErrUnsupported)
	}

	subtitles, err := planSubtitle(input, chosen)
	if err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	reasons := sourceReasons(input, chosen)
	if subtitles.burnsIn() {
		reasons = append(reasons, Reason{Code: ReasonSubtitleBurnIn, Scope: ScopeSubtitle})
	}

	if len(reasons) == 0 {
		return directPlayPlan(input.Description.Container, chosen, subtitles), nil
	}

	plan, err := hlsPlan(input, chosen, hlsCause{reasons: reasons, subtitles: subtitles})
	if err != nil {
		return Plan{}, fmt.Errorf("decide playback: %w", err)
	}

	return plan, nil
}

// sourceReasons explains why the source is not played directly. A disc is never: its title
// is a list of clips that only FFmpeg joins (ADR 0029), so its one reason is disc_source and
// the container and codec checks of direct play do not apply; the copy checks of the HLS
// plan still add why a stream is re-encoded.
func sourceReasons(input Input, chosen selection) []Reason {
	if input.Layout.IsDisc() {
		return []Reason{{Code: ReasonDiscSource, Scope: ScopeContainer}}
	}

	return directPlayReasons(input, chosen)
}

// directPlayPlan serves the original file. A sidecar subtitle does not change that: the
// client loads it next to the file.
func directPlayPlan(container media.Container, chosen selection, subtitles subtitlePlan) Plan {
	plan := Plan{
		Mode: ModeDirectPlay, Delivery: DeliveryFile,
		SourceContainer: container, TargetContainer: container,
		Warnings: subtitles.warnings, Subtitle: subtitles.decision,
	}

	if chosen.hasVideo {
		plan.Streams = append(plan.Streams, copyDecision(chosen.video))
	}

	if chosen.hasAudio {
		plan.Streams = append(plan.Streams, copyDecision(chosen.audio))
	}

	return plan
}

// hlsCause is why a file is not played directly, with the subtitle the HLS plan carries.
type hlsCause struct {
	// reasons explain why the original file is not served, a burn-in included.
	reasons   []Reason
	subtitles subtitlePlan
}

// videoPlanner is planVideo, or its burn-in variant that always re-encodes.
func (cause hlsCause) videoPlanner() streamPlanner {
	if cause.subtitles.burnsIn() {
		return planBurnInVideo
	}

	return planVideo
}

// hlsPlan builds a remux or transcode plan. The cause explains why the original file is not
// served; the copy checks add why a stream is re-encoded. Failures follow one precedence:
// every client limit (no_compatible_format) before any server limit (transcode_unavailable).
func hlsPlan(input Input, chosen selection, cause hlsCause) (Plan, error) {
	if input.Profile.HLS.Delivery == HLSDeliveryNone {
		return Plan{}, fmt.Errorf("client plays no HLS and not the original file: %w", ErrNoCompatibleFormat)
	}

	plan := Plan{
		Mode: ModeRemux, Delivery: DeliveryHLS,
		SourceContainer: input.Description.Container, TargetContainer: media.ContainerMP4,
		Reasons: cause.reasons, Subtitle: cause.subtitles.decision,
	}

	planned := []struct {
		present bool
		stream  media.Stream
		decide  streamPlanner
	}{
		{present: chosen.hasVideo, stream: chosen.video, decide: cause.videoPlanner()},
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

	if cause.subtitles.burnsIn() {
		if err := missingTextBurnIn(input.Capabilities, chosen.subtitle); err != nil {
			return Plan{}, err
		}
	}

	plan.Warnings = append(hlsWarnings(plan, chosen), cause.subtitles.warnings...)

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

// hlsWarnings lists the degradations of a transcoded video. A copied HDR video keeps its
// signal, so only a transcode loses it.
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

	return warnings
}

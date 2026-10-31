package restapi

import (
	"fmt"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// newPlaybackSession maps an app session to the contract 1:1. Enumerations and codes are
// passed through as strings, so values a later domain version adds reach clients, which
// tolerate them (ADR 0023). The lists are never nil: the contract requires arrays, and
// direct-play plans carry nil reasons and warnings. The streamUrl is the app's, unchanged.
// An ID that is no UUID or a stream index the contract cannot carry breaks the contract and
// is reported as an error (a server fault).
func newPlaybackSession(session appplayback.Session) (apigen.PlaybackSession, error) {
	id, err := uuid.Parse(session.ID)
	if err != nil {
		return apigen.PlaybackSession{}, fmt.Errorf("convert session id %q: %w", session.ID, err)
	}

	fileID, err := uuid.Parse(session.FileID)
	if err != nil {
		return apigen.PlaybackSession{}, fmt.Errorf("convert file id %q of session %s: %w", session.FileID, session.ID, err)
	}

	plan := session.Plan

	streams, err := newStreamDecisions(plan.Streams)
	if err != nil {
		return apigen.PlaybackSession{}, fmt.Errorf("convert plan of session %s: %w", session.ID, err)
	}

	return apigen.PlaybackSession{
		ID:                   id,
		FileID:               fileID,
		State:                apigen.PlaybackSessionState(session.State),
		FailureCode:          optionalString(session.FailureCode),
		Mode:                 apigen.PlaybackMode(plan.Mode),
		Delivery:             apigen.PlaybackDelivery(plan.Delivery),
		StreamURL:            session.StreamURL,
		StartPositionSeconds: durationSeconds(session.StartPosition),
		DurationSeconds:      optionalDurationSeconds(session.Duration),
		SourceContainer:      string(plan.SourceContainer),
		TargetContainer:      string(plan.TargetContainer),
		Streams:              streams,
		Reasons:              newPlaybackReasons(plan.Reasons),
		Warnings:             newPlaybackWarnings(plan.Warnings),
	}, nil
}

// newStreamDecisions fails for a source index the contract cannot carry (streamIndex). A
// size or channel count the plan does not know (0), or one that does not apply to the
// stream kind, such as the width of audio, is omitted.
func newStreamDecisions(decisions []playback.StreamDecision) ([]apigen.StreamDecision, error) {
	converted := make([]apigen.StreamDecision, 0, len(decisions))

	for _, decision := range decisions {
		sourceIndex, err := streamIndex(decision.SourceIndex)
		if err != nil {
			return nil, err
		}

		converted = append(converted, apigen.StreamDecision{
			Kind:           apigen.MediaStreamKind(decision.Kind),
			SourceIndex:    sourceIndex,
			Action:         apigen.StreamAction(decision.Action),
			SourceCodec:    decision.SourceCodec,
			TargetCodec:    decision.TargetCodec,
			TargetWidth:    optionalInt32(decision.TargetWidth),
			TargetHeight:   optionalInt32(decision.TargetHeight),
			TargetChannels: optionalInt32(decision.TargetChannels),
			Encoder:        optionalString(decision.Encoder),
		})
	}

	return converted, nil
}

func newPlaybackReasons(reasons []playback.Reason) []apigen.PlaybackReason {
	converted := make([]apigen.PlaybackReason, 0, len(reasons))

	for _, reason := range reasons {
		converted = append(converted, apigen.PlaybackReason{
			Code:  string(reason.Code),
			Scope: apigen.PlaybackReasonScope(reason.Scope),
		})
	}

	return converted
}

func newPlaybackWarnings(warnings []playback.Warning) []apigen.PlaybackReason {
	converted := make([]apigen.PlaybackReason, 0, len(warnings))

	for _, warning := range warnings {
		converted = append(converted, apigen.PlaybackReason{
			Code:  string(warning.Code),
			Scope: apigen.PlaybackReasonScope(warning.Scope),
		})
	}

	return converted
}

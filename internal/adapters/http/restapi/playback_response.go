package restapi

import (
	"fmt"
	"math"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// newPlaybackSession maps an app session to the contract 1:1. Enumerations and codes are
// passed through as strings, so values a later domain version adds reach clients, which
// tolerate them (ADR 0023). The lists are never nil: the contract requires arrays, and
// direct-play plans carry nil reasons and warnings. The streamUrl is the app's, unchanged.
// An ID that is no UUID breaks the contract and is reported as an error (a server fault).
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

	return apigen.PlaybackSession{
		ID:                   id,
		FileID:               fileID,
		State:                apigen.PlaybackSessionState(session.State),
		FailureCode:          optionalString(session.FailureCode),
		Mode:                 apigen.PlaybackMode(plan.Mode),
		Delivery:             apigen.PlaybackDelivery(plan.Delivery),
		StreamURL:            session.StreamURL,
		StartPositionSeconds: session.StartPosition.Seconds(),
		DurationSeconds:      knownDurationSeconds(session.Duration),
		SourceContainer:      string(plan.SourceContainer),
		TargetContainer:      string(plan.TargetContainer),
		Streams:              newStreamDecisions(plan.Streams),
		Reasons:              newPlaybackReasons(plan.Reasons),
		Warnings:             newPlaybackWarnings(plan.Warnings),
	}, nil
}

// knownDurationSeconds omits an unknown (zero) duration, as the contract asks.
func knownDurationSeconds(duration time.Duration) *float64 {
	if duration <= 0 {
		return nil
	}

	seconds := duration.Seconds()

	return &seconds
}

func newStreamDecisions(decisions []playback.StreamDecision) []apigen.StreamDecision {
	converted := make([]apigen.StreamDecision, 0, len(decisions))

	for _, decision := range decisions {
		converted = append(converted, apigen.StreamDecision{
			Kind:           apigen.MediaStreamKind(decision.Kind),
			SourceIndex:    contractInt32(decision.SourceIndex),
			Action:         apigen.StreamAction(decision.Action),
			SourceCodec:    decision.SourceCodec,
			TargetCodec:    decision.TargetCodec,
			TargetWidth:    knownQuantity(decision.TargetWidth),
			TargetHeight:   knownQuantity(decision.TargetHeight),
			TargetChannels: knownQuantity(decision.TargetChannels),
			Encoder:        optionalString(decision.Encoder),
		})
	}

	return converted
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

// knownQuantity omits a size or channel count the plan does not know (0), and the fields
// that do not apply to a stream kind, such as the width of audio.
func knownQuantity(value int) *int32 {
	if value <= 0 {
		return nil
	}

	converted := contractInt32(value)

	return &converted
}

// contractInt32 narrows a stream index, size or channel count to the contract's int32.
// Probed values are far below the limit; clamping keeps a corrupt one from wrapping into a
// negative number.
func contractInt32(value int) int32 {
	return int32(min(max(value, math.MinInt32), math.MaxInt32))
}

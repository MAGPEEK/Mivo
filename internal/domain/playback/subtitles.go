package playback

import (
	"fmt"
	"slices"

	"github.com/magpeek/mivo/internal/domain/media"
)

// SubtitleFormatWebVTT is the one format of the sidecar route: the server converts every text
// subtitle to WebVTT, so a client that lists it renders every text track (ADR 0030 §3).
const SubtitleFormatWebVTT = "webvtt"

// SubtitleDelivery is how a plan shows its subtitle (API SubtitleDeliveryMode).
type SubtitleDelivery string

// The subtitle deliveries of ADR 0030 §3.
const (
	// SubtitleOff shows no subtitle.
	SubtitleOff SubtitleDelivery = "off"
	// SubtitleSidecar: the client loads the track as WebVTT and renders it; the video and
	// audio decisions are untouched.
	SubtitleSidecar SubtitleDelivery = "sidecar"
	// SubtitleBurnIn: the server draws the track into the video, which forces a transcode.
	SubtitleBurnIn SubtitleDelivery = "burn_in"
)

// SubtitleDecision is what a plan does with subtitles (API SubtitleDecision).
type SubtitleDecision struct {
	Mode SubtitleDelivery
	// TrackID and Format (SubtitleTrack.Format) name the shown track; both are empty when the
	// mode is SubtitleOff.
	TrackID string
	Format  string
}

// rendersSubtitle reports a track the client shows from the sidecar route. Image tracks never
// qualify: they have no text to convert.
func (profile ClientProfile) rendersSubtitle(track SubtitleTrack) bool {
	return track.TextBased && slices.Contains(profile.SubtitleFormats, SubtitleFormatWebVTT)
}

// renderableSubtitles keeps the tracks the client renders itself. The default policy picks
// only from these, so it never causes a burn-in and with it a transcode (ADR 0030 Notes
// (M4-02b)).
func (profile ClientProfile) renderableSubtitles(tracks []SubtitleTrack) []SubtitleTrack {
	var kept []SubtitleTrack

	for _, track := range tracks {
		if profile.rendersSubtitle(track) {
			kept = append(kept, track)
		}
	}

	return kept
}

// subtitlePlan is the subtitle part of a plan.
type subtitlePlan struct {
	decision SubtitleDecision
	warnings []Warning
}

func (subtitles subtitlePlan) burnsIn() bool {
	return subtitles.decision.Mode == SubtitleBurnIn
}

// planSubtitle decides how the selected track is shown: as a sidecar when the client renders
// it, otherwise burned in. Only a track the request names gets here unrendered. A burn-in
// that is not possible is dropped with a warning when the request allows it; otherwise the
// plan keeps it, so that the HLS plan fails in its usual order (client limits before server
// limits) with the problem a transcode gets.
func planSubtitle(input Input, chosen selection) (subtitlePlan, error) {
	if !chosen.hasSubtitle {
		return subtitlePlan{decision: SubtitleDecision{Mode: SubtitleOff}}, nil
	}

	shown := SubtitleDecision{TrackID: chosen.subtitle.ID, Format: chosen.subtitle.Format}

	if input.Profile.rendersSubtitle(chosen.subtitle) {
		shown.Mode = SubtitleSidecar

		return subtitlePlan{decision: shown}, nil
	}

	obstacle := burnInObstacle(input, chosen)

	if obstacle != nil && input.Request.AllowDegradeSubtitles {
		return subtitlePlan{
			decision: SubtitleDecision{Mode: SubtitleOff},
			warnings: []Warning{{Code: WarningSubtitleBurnInUnavailable, Scope: ScopeSubtitle}},
		}, nil
	}

	// Without a picture no later check would fail, so this obstacle is reported here.
	if !chosen.hasVideo {
		return subtitlePlan{}, obstacle
	}

	shown.Mode = SubtitleBurnIn

	return subtitlePlan{decision: shown}, nil
}

// burnInObstacle returns why the subtitle cannot be drawn into the video, client limits
// first: nothing to draw on, no HLS, no playable transcode target, then a server that cannot
// encode video. nil means burn-in works. The session limit is not known here (app/playback).
func burnInObstacle(input Input, chosen selection) error {
	switch {
	case !chosen.hasVideo:
		return fmt.Errorf("subtitle %s needs burn-in, but the file has no video: %w",
			chosen.subtitle.ID, ErrNoCompatibleFormat)
	case input.Profile.HLS.Delivery == HLSDeliveryNone:
		return fmt.Errorf("subtitle %s needs burn-in, but the client plays no HLS: %w",
			chosen.subtitle.ID, ErrNoCompatibleFormat)
	case !input.Profile.playsTranscodedVideo():
		return fmt.Errorf("subtitle %s needs burn-in, but %w", chosen.subtitle.ID, errNoVideoTarget())
	case !input.Capabilities.HLSMuxing || chooseVideoEncoder(input).Name == "":
		return fmt.Errorf("subtitle %s needs burn-in, but the server cannot encode video: %w",
			chosen.subtitle.ID, ErrTranscodeUnavailable)
	default:
		return missingTextBurnIn(input.Capabilities, chosen.subtitle)
	}
}

// missingTextBurnIn is the server limit of drawing a text subtitle: it needs libass, while
// picture subtitles only need the overlay filter every build has. The HLS plan checks it with
// the other server limits, after the client limits.
func missingTextBurnIn(capabilities ServerCapabilities, track SubtitleTrack) error {
	if track.TextBased && !capabilities.BurnInText {
		return fmt.Errorf("subtitle %s needs text burn-in, but the server cannot draw text: %w",
			track.ID, ErrTranscodeUnavailable)
	}

	return nil
}

// planBurnInVideo is planVideo for a video that gets a subtitle drawn in: a video the client
// would play as it is must be re-encoded all the same, because a copied stream cannot carry
// an overlay. The audio is not affected and may still be copied.
func planBurnInVideo(stream media.Stream, input Input) (StreamDecision, []Reason, error) {
	decision, reasons, err := planVideo(stream, input)
	if err != nil || decision.Action == ActionTranscode {
		return decision, reasons, err
	}

	return transcodeVideoDecision(stream, input)
}

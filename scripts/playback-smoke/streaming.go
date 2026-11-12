package main

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// subtitleCaseBase is the corpus case with text subtitles the subtitle case plays.
	subtitleCaseBase = "subtitle-mkv-srt-ass"
	// capCaseBitRate is the bit rate cap of the cap case: 2 Mbit/s, a typical limit of a
	// remote client (ADR 0030 section 6).
	capCaseBitRate int64 = 2_000_000

	sidecarSuffix = "+sidecar"
	capSuffix     = "@2Mbit/s"

	webVTTSignature  = "WEBVTT"
	maxSubtitleBytes = 4 << 20
	mediaFilesPath   = "/api/v1/media-files/"
)

// streamingCases are the cases of the streaming extension (M4-15) for the firefox-desktop
// profile: one text subtitle delivered as a sidecar, and one transcode under a 2 Mbit/s cap.
// They reuse corpus cases, so a corpus without the base clips yields no case; the profile's
// own expectation table still guards the corpus against shrinking.
func streamingCases(cases []smokeCase, profile string) []smokeCase {
	if profile != corpustest.FirefoxDesktop {
		return nil
	}

	var extra []smokeCase

	if index := slices.IndexFunc(cases, func(smoke smokeCase) bool { return smoke.label == subtitleCaseBase }); index >= 0 {
		sidecar := cases[index]
		sidecar.label += sidecarSuffix
		sidecar.sidecarSubtitle = true
		extra = append(extra, sidecar)
	}

	index := slices.IndexFunc(cases, func(smoke smokeCase) bool {
		return smoke.start == 0 && smoke.want.Video == playback.ActionTranscode
	})
	if index >= 0 {
		capped := cases[index]
		capped.label += capSuffix
		capped.maxBitRate = capCaseBitRate
		extra = append(extra, capped)
	}

	return extra
}

// subtitleRequest adds the subtitle selection to the request of a sidecar case: the profile
// renders WebVTT itself, and the first text track of the file is named.
func (runner *smokeRunner) subtitleRequest(request apigen.PlaybackRequest) (apigen.PlaybackRequest, error) {
	track, found := runner.tracks[request.FileID]
	if !found {
		return request, fmt.Errorf("%w: file %s lists no text subtitle track", errNoSubtitleTrack, request.FileID)
	}

	request.SubtitleTrackID = &track
	request.Profile.SubtitleFormats = &[]apigen.MediaIdentifier{playback.SubtitleFormatWebVTT}

	return request, nil
}

// verifyStreaming checks what the extra cases promise beyond the plan: the sidecar delivery
// and a working WebVTT download, or an output bit rate within the cap.
func (runner *smokeRunner) verifyStreaming(ctx context.Context, smoke smokeCase, session apigen.PlaybackSession, outcome *caseOutcome) {
	if smoke.sidecarSubtitle {
		runner.verifySidecar(ctx, session, outcome)
	}

	if smoke.maxBitRate > 0 && (session.OutputBitRate == nil || *session.OutputBitRate > smoke.maxBitRate) {
		outcome.fail("output bit rate = %s, want at most %d", describeBitRate(session.OutputBitRate), smoke.maxBitRate)
	}
}

func describeBitRate(rate *int64) string {
	if rate == nil {
		return "not reported"
	}

	return fmt.Sprint(*rate)
}

func (runner *smokeRunner) verifySidecar(ctx context.Context, session apigen.PlaybackSession, outcome *caseOutcome) {
	if session.Subtitle == nil || session.Subtitle.Mode != apigen.SubtitleDeliveryModeSidecar || session.Subtitle.TrackID == nil {
		outcome.fail("subtitle = %+v, want a sidecar", session.Subtitle)

		return
	}

	reference := mediaFilesPath + session.FileID.String() + "/subtitles/" + url.PathEscape(*session.Subtitle.TrackID) + ".vtt"

	target, err := resolve(runner.client.base, reference)
	if err != nil {
		outcome.fail("%v", err)

		return
	}

	var body bytes.Buffer

	limited := &limitedWriter{writer: &body, remaining: maxSubtitleBytes}
	if err := runner.downloader().fetchStreamFile(ctx, target, limited); err != nil {
		outcome.fail("download the subtitle: %v", err)

		return
	}

	if !strings.HasPrefix(body.String(), webVTTSignature) {
		outcome.fail("subtitle download is no WebVTT (%d bytes)", body.Len())
	}
}

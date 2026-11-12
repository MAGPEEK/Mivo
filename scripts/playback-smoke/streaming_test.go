package main

import (
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func baseCases() []smokeCase {
	return []smokeCase{
		{label: "plain", want: corpustest.Expectation{Video: playback.ActionCopy}},
		{label: subtitleCaseBase, want: corpustest.Expectation{Video: playback.ActionCopy}},
		{label: "seek-case@10s", start: 10 * time.Second, want: corpustest.Expectation{Video: playback.ActionTranscode}},
		{label: "reencoded", want: corpustest.Expectation{Video: playback.ActionTranscode}},
	}
}

// TestStreamingCasesAddTheSubtitleAndTheCapCaseForFirefoxOnly (M4-15).
func TestStreamingCasesAddTheSubtitleAndTheCapCaseForFirefoxOnly(t *testing.T) {
	t.Parallel()

	extra := streamingCases(baseCases(), corpustest.FirefoxDesktop)
	if len(extra) != 2 {
		t.Fatalf("streamingCases = %+v, want the subtitle and the cap case", extra)
	}

	if extra[0].label != subtitleCaseBase+sidecarSuffix || !extra[0].sidecarSubtitle {
		t.Errorf("subtitle case = %+v", extra[0])
	}

	if extra[1].label != "reencoded"+capSuffix || extra[1].maxBitRate != capCaseBitRate || extra[1].start != 0 {
		t.Errorf("cap case = %+v, want the re-encoded clip from its beginning under 2 Mbit/s", extra[1])
	}

	if other := streamingCases(baseCases(), corpustest.ChromiumDesktop); len(other) != 0 {
		t.Errorf("chromium cases = %+v, want none", other)
	}

	if none := streamingCases(nil, corpustest.FirefoxDesktop); len(none) != 0 {
		t.Errorf("cases without the base clips = %+v, want none", none)
	}
}

func TestSidecarRequestNamesTheFirstTextTrackAndTheWebVTTFormat(t *testing.T) {
	t.Parallel()

	file := uuid.New()
	runner := &smokeRunner{tracks: trackIndex{file: "s:2"}}

	request, err := runner.request(file, smokeCase{sidecarSubtitle: true, maxBitRate: capCaseBitRate})
	if err != nil || request.SubtitleTrackID == nil || *request.SubtitleTrackID != "s:2" {
		t.Fatalf("request = %+v, %v, want track s:2", request, err)
	}

	if request.Profile.SubtitleFormats == nil || (*request.Profile.SubtitleFormats)[0] != playback.SubtitleFormatWebVTT {
		t.Errorf("subtitle formats = %v, want webvtt", request.Profile.SubtitleFormats)
	}

	if request.MaxBitRate == nil || *request.MaxBitRate != capCaseBitRate {
		t.Errorf("max bit rate = %v", request.MaxBitRate)
	}

	if _, err := runner.request(uuid.New(), smokeCase{sidecarSubtitle: true}); !errors.Is(err, errNoSubtitleTrack) {
		t.Errorf("request for a file without a text track = %v, want errNoSubtitleTrack", err)
	}
}

func TestFirstTextTrackSkipsPictureSubtitles(t *testing.T) {
	t.Parallel()

	tracks := []apigen.SubtitleTrack{{ID: "s:1"}, {ID: "s:2", TextBased: true}, {ID: "x:3", TextBased: true}}

	if got := firstTextTrack(apigen.MediaFile{SubtitleTracks: &tracks}); got != "s:2" {
		t.Errorf("firstTextTrack = %q, want s:2", got)
	}

	if got := firstTextTrack(apigen.MediaFile{}); got != "" {
		t.Errorf("firstTextTrack without tracks = %q, want none", got)
	}
}

package playback_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func trackID(id string) *string {
	return &id
}

// TestDecideAppliesTheAudioPreference: without a request the profile's language list picks
// the audio track; a track that is not first still never plays directly (FIX-audio-codecs),
// and an explicit index wins over the preference.
func TestDecideAppliesTheAudioPreference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		languages    []string
		request      *int
		wantIndex    int
		wantNotFirst bool
	}{
		{name: "no preference plays the default German track", wantIndex: 1},
		{name: "German preference", languages: []string{"de"}, wantIndex: 1},
		{name: "English preference", languages: []string{"en"}, wantIndex: 2, wantNotFirst: true},
		{name: "request overrides the preference", languages: []string{"en"}, request: audioIndex(1), wantIndex: 1},
		{name: "request of the second track", languages: []string{"de"}, request: audioIndex(2), wantIndex: 2, wantNotFirst: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(germanDualLanguageClip())
			input.Profile.Tracks.AudioLanguages = test.languages
			input.Request.AudioStreamIndex = test.request

			plan := mustDecide(t, input)
			audioDecision, _ := plan.Audio()

			if audioDecision.SourceIndex != test.wantIndex {
				t.Errorf("audio source = %d, want %d", audioDecision.SourceIndex, test.wantIndex)
			}

			if got := slices.Contains(plan.Reasons, audioTrackReason()); got != test.wantNotFirst {
				t.Errorf("reasons %v contain audio_track_not_first = %v, want %v", plan.Reasons, got, test.wantNotFirst)
			}
		})
	}
}

// TestDecideKeepsDirectPlayForStereoFirst: without default flags and language preference the
// first track wins over a 5.1 second track, so the file still plays directly.
func TestDecideKeepsDirectPlayForStereoFirst(t *testing.T) {
	t.Parallel()

	input := referenceInput(clip(media.ContainerMP4, h264High(0),
		surround(1, "aac", stereoChannels), surround(2, "aac", surround51)))

	plan := mustDecide(t, input)
	audioDecision, _ := plan.Audio()

	if plan.Mode != playback.ModeDirectPlay || audioDecision.SourceIndex != 1 {
		t.Errorf("plan mode %s with audio %d, want direct play of audio 1", plan.Mode, audioDecision.SourceIndex)
	}
}

// TestDecideAcceptsKnownSubtitleTracks: a request may name an embedded or external track or
// switch subtitles off; the preference never fails a decision.
func TestDecideAcceptsKnownSubtitleTracks(t *testing.T) {
	t.Parallel()

	external := playback.SubtitleTrack{ID: "x:0a1b2c", Language: "deu", External: true, TextBased: true}

	tests := []struct {
		name       string
		request    *string
		preference playback.TrackPreference
	}{
		{name: "embedded track", request: trackID("s:3")},
		{name: "external track", request: trackID(external.ID)},
		{name: "explicit off", request: trackID("")},
		{name: "forced preference", preference: playback.TrackPreference{SubtitleMode: playback.SubtitleModeForced}},
		{name: "always preference", preference: playback.TrackPreference{SubtitleMode: playback.SubtitleModeAlways, SubtitleLanguages: []string{"de"}}},
		{name: "always preference without a match", preference: playback.TrackPreference{SubtitleMode: playback.SubtitleModeAlways, SubtitleLanguages: []string{"fr"}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(clip(media.ContainerMKV, h264High(0),
				asDefault(languageAudio(1, "aac", "eng", stereoChannels)),
				languageSubtitle(2, "subrip", "eng"), forcedSubtitle(3, "eng")))
			input.ExternalSubtitles = []playback.SubtitleTrack{external}
			input.Profile.Tracks = test.preference
			input.Request.SubtitleTrackID = test.request

			mustDecide(t, input)
		})
	}
}

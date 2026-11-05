package playback_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Channel counts of common audio layouts.
const (
	monoChannels     = 1
	stereoChannels   = 2
	surround30       = 3
	surround51       = 6
	surround71       = 8
	unknownChannels  = 0
	firstAudioIndex  = 1
	secondAudioIndex = 2
)

func audioChannelsReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonAudioChannelsNotSupported, Scope: playback.ScopeAudio}
}

// surround is an audio stream with the given channel count.
func surround(index int, codec string, channels int) media.Stream {
	stream := audio(index, codec)
	stream.Channels = channels

	return stream
}

// TestDecideAudioChannels: browsers decode at most 5.1 (Firefox 159 fails on AAC 7.1 with "no
// decoder found" although canPlayType answers "probably"), so more channels are never copied;
// a re-encode keeps mono and stereo, turns every surround layout into AAC 5.1 and an unknown
// layout into stereo.
func TestDecideAudioChannels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		stream       media.Stream
		wantAction   playback.Action
		wantChannels int
		wantReasons  []playback.Reason
	}{
		{
			name: "AAC mono is copied", stream: surround(1, "aac", monoChannels),
			wantAction: copied, wantChannels: monoChannels,
		},
		{
			name: "AAC 5.1 is copied", stream: surround(1, "aac", surround51),
			wantAction: copied, wantChannels: surround51,
		},
		{
			name: "AAC of unknown layout is copied", stream: surround(1, "aac", unknownChannels),
			wantAction: copied, wantChannels: unknownChannels,
		},
		{
			name: "AAC 7.1 is re-encoded to 5.1", stream: surround(1, "aac", surround71),
			wantAction: transcoded, wantChannels: surround51, wantReasons: []playback.Reason{audioChannelsReason()},
		},
		{
			name: "FLAC 7.1 is re-encoded to 5.1", stream: surround(1, "flac", surround71),
			wantAction: transcoded, wantChannels: surround51, wantReasons: []playback.Reason{audioChannelsReason()},
		},
		{
			name: "AC-3 mono stays mono", stream: surround(1, "ac3", monoChannels),
			wantAction: transcoded, wantChannels: monoChannels, wantReasons: []playback.Reason{audioCodecReason()},
		},
		{
			name: "AC-3 stereo stays stereo", stream: surround(1, "ac3", stereoChannels),
			wantAction: transcoded, wantChannels: stereoChannels, wantReasons: []playback.Reason{audioCodecReason()},
		},
		{
			name: "AC-3 3.0 becomes 5.1", stream: surround(1, "ac3", surround30),
			wantAction: transcoded, wantChannels: surround51, wantReasons: []playback.Reason{audioCodecReason()},
		},
		{
			name: "E-AC-3 5.1 stays 5.1", stream: surround(1, "eac3", surround51),
			wantAction: transcoded, wantChannels: surround51, wantReasons: []playback.Reason{audioCodecReason()},
		},
		{
			name: "TrueHD 7.1 becomes 5.1", stream: surround(1, "truehd", surround71),
			wantAction: transcoded, wantChannels: surround51, wantReasons: []playback.Reason{audioCodecReason()},
		},
		{
			name: "DTS of unknown layout becomes stereo", stream: surround(1, "dts", unknownChannels),
			wantAction: transcoded, wantChannels: stereoChannels, wantReasons: []playback.Reason{audioCodecReason()},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plan := mustDecide(t, referenceInput(clip(media.ContainerMP4, h264High(0), test.stream)))

			decision, found := plan.Audio()
			if !found || decision.Action != test.wantAction || decision.TargetChannels != test.wantChannels {
				t.Errorf("audio = %s with %d channels (present %t), want %s with %d",
					decision.Action, decision.TargetChannels, found, test.wantAction, test.wantChannels)
			}

			if !slices.Equal(plan.Reasons, test.wantReasons) {
				t.Errorf("Reasons = %v, want %v", plan.Reasons, test.wantReasons)
			}
		})
	}
}

// TestDecideDirectPlayNeedsTheFirstAudioTrack: Chrome and Edge 156 play the first audio track
// of a Matroska file and Firefox 159 that of Matroska and MP4 files, whatever the default flag
// says; only the selected stream's codec is checked, so any other selection is remuxed.
func TestDecideDirectPlayNeedsTheFirstAudioTrack(t *testing.T) {
	t.Parallel()

	defaultSecond := clip(media.ContainerMP4, h264High(0), audio(firstAudioIndex, "ac3"), asDefault(audio(secondAudioIndex, "aac")))
	bothAAC := clip(media.ContainerMP4, h264High(0), audio(firstAudioIndex, "aac"), asDefault(audio(secondAudioIndex, "aac")))

	tests := []struct {
		name        string
		description media.Description
		request     *int
		wantMode    playback.Mode
		wantIndex   int
		wantReasons []playback.Reason
	}{
		{
			name: "the default flag on the second track remuxes it", description: defaultSecond,
			wantMode: playback.ModeRemux, wantIndex: secondAudioIndex, wantReasons: []playback.Reason{audioTrackReason()},
		},
		{
			name: "a request for the first track plays the file", description: bothAAC, request: audioIndex(firstAudioIndex),
			wantMode: playback.ModeDirectPlay, wantIndex: firstAudioIndex,
		},
		{
			name: "a request for the default second track remuxes it", description: bothAAC, request: audioIndex(secondAudioIndex),
			wantMode: playback.ModeRemux, wantIndex: secondAudioIndex, wantReasons: []playback.Reason{audioTrackReason()},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			input := referenceInput(test.description)
			input.Request.AudioStreamIndex = test.request
			plan := mustDecide(t, input)

			decision, _ := plan.Audio()
			if plan.Mode != test.wantMode || decision.SourceIndex != test.wantIndex || decision.Action != copied {
				t.Errorf("plan = %s copying audio %d (%s), want %s copying %d",
					plan.Mode, decision.SourceIndex, decision.Action, test.wantMode, test.wantIndex)
			}

			if !slices.Equal(plan.Reasons, test.wantReasons) {
				t.Errorf("Reasons = %v, want %v", plan.Reasons, test.wantReasons)
			}
		})
	}
}

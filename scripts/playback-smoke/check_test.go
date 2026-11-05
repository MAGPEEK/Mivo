package main

import (
	"testing"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestChannelMismatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		sourceChannels int
		action         playback.Action
		outputChannels int
		wantMismatch   bool
	}{
		{name: "copied 5.1 keeps its channels", sourceChannels: 6, action: playback.ActionCopy, outputChannels: 6},
		{name: "re-encoded 7.1 becomes 5.1", sourceChannels: 8, action: playback.ActionTranscode, outputChannels: 6},
		{name: "re-encoded stereo stays stereo", sourceChannels: 2, action: playback.ActionTranscode, outputChannels: 2},
		{
			name: "re-encoded 5.1 downmixed to stereo", sourceChannels: 6, action: playback.ActionTranscode,
			outputChannels: 2, wantMismatch: true,
		},
		{name: "silent output without audio", sourceChannels: 2, action: playback.ActionCopy, wantMismatch: true},
		{name: "manifest without channels", action: playback.ActionTranscode, outputChannels: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			smoke := smokeCase{
				clip: corpusCase{Expected: caseProperties{Audio: []streamProperties{
					{Codec: "ac3", Channels: test.sourceChannels},
				}}},
				want: corpustest.Expectation{Audio: test.action},
			}

			if mismatch, found := channelMismatch(smoke, outputSummary{audioChannels: test.outputChannels}); found != test.wantMismatch {
				t.Errorf("channelMismatch = %q (%t), want a mismatch: %t", mismatch, found, test.wantMismatch)
			}
		})
	}
}

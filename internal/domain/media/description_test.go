package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestDescriptionValidateAcceptsProbedFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		description media.Description
	}{
		{name: "hdr10 mkv", description: hdr10Description()},
		{name: "unknown container without streams", description: media.Description{Container: media.ContainerUnknown}},
		{name: "zero duration and bit rate", description: media.Description{Container: media.ContainerAVI}},
		{
			name: "sparse stream indexes",
			description: media.Description{
				Container: media.ContainerMPEGTS,
				Streams: []media.Stream{
					{Index: 3, Kind: media.StreamKindVideo},
					{Index: 1, Kind: media.StreamKindAudio},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if err := test.description.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestDescriptionValidateRejectsBrokenValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*media.Description)
	}{
		{name: "negative duration", mutate: func(d *media.Description) { d.Duration = -time.Second }},
		{name: "negative bit rate", mutate: func(d *media.Description) { d.BitRate = -1 }},
		{name: "empty container", mutate: func(d *media.Description) { d.Container = "" }},
		{name: "foreign container", mutate: func(d *media.Description) { d.Container = "matroska,webm" }},
		{name: "duplicate stream index", mutate: func(d *media.Description) { d.Streams[1].Index = 0 }},
		{name: "negative stream index", mutate: func(d *media.Description) { d.Streams[0].Index = -1 }},
		{name: "unknown stream kind", mutate: func(d *media.Description) { d.Streams[0].Kind = "data" }},
		{name: "empty stream kind", mutate: func(d *media.Description) { d.Streams[0].Kind = "" }},
		{name: "negative width", mutate: func(d *media.Description) { d.Streams[0].Width = -1920 }},
		{name: "negative height", mutate: func(d *media.Description) { d.Streams[0].Height = -1080 }},
		{name: "negative bit depth", mutate: func(d *media.Description) { d.Streams[0].BitDepth = -10 }},
		{name: "negative level", mutate: func(d *media.Description) { d.Streams[0].Level = -99 }},
		{name: "negative stream bit rate", mutate: func(d *media.Description) { d.Streams[0].BitRate = -1 }},
		{name: "negative channels", mutate: func(d *media.Description) { d.Streams[1].Channels = -2 }},
		{name: "negative sample rate", mutate: func(d *media.Description) { d.Streams[1].SampleRate = -48000 }},
		{name: "unknown hdr format", mutate: func(d *media.Description) { d.Streams[0].HDR = "dolby_vision" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			description := hdr10Description()
			test.mutate(&description)

			if err := description.Validate(); !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("Validate() error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestPrimaryVideo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		streams   []media.Stream
		wantIndex int
		wantOK    bool
	}{
		{
			name:      "single video",
			streams:   []media.Stream{video(0), audio(1)},
			wantIndex: 0, wantOK: true,
		},
		{
			name:      "cover art first is skipped",
			streams:   []media.Stream{attachedPicture(0, true), video(1), audio(2)},
			wantIndex: 1, wantOK: true,
		},
		{
			name:      "default video preferred",
			streams:   []media.Stream{video(0), withDefault(video(1)), audio(2)},
			wantIndex: 1, wantOK: true,
		},
		{
			name:      "default cover art is not preferred",
			streams:   []media.Stream{video(0), attachedPicture(1, true)},
			wantIndex: 0, wantOK: true,
		},
		{
			name:      "first of several non-default videos",
			streams:   []media.Stream{audio(0), video(1), video(2)},
			wantIndex: 1, wantOK: true,
		},
		{name: "only cover art", streams: []media.Stream{audio(0), attachedPicture(1, false)}},
		{name: "audio only", streams: []media.Stream{audio(0)}},
		{name: "no streams"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			description := media.Description{Container: media.ContainerMKV, Streams: test.streams}
			stream, ok := description.PrimaryVideo()
			assertSelected(t, "PrimaryVideo", selection{stream: stream, ok: ok}, expectedSelection{index: test.wantIndex, ok: test.wantOK})
		})
	}
}

func TestDefaultAudio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		streams   []media.Stream
		wantIndex int
		wantOK    bool
	}{
		{
			name:      "first audio without default flags",
			streams:   []media.Stream{video(0), audio(1), audio(2)},
			wantIndex: 1, wantOK: true,
		},
		{
			name:      "default audio preferred",
			streams:   []media.Stream{video(0), audio(1), withDefault(audio(2))},
			wantIndex: 2, wantOK: true,
		},
		{
			name:      "default on other kinds is ignored",
			streams:   []media.Stream{withDefault(video(0)), withDefault(subtitle(1)), audio(2)},
			wantIndex: 2, wantOK: true,
		},
		{name: "video only", streams: []media.Stream{video(0)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			description := media.Description{Container: media.ContainerMKV, Streams: test.streams}
			stream, ok := description.DefaultAudio()
			assertSelected(t, "DefaultAudio", selection{stream: stream, ok: ok}, expectedSelection{index: test.wantIndex, ok: test.wantOK})
		})
	}
}

func TestAudioByIndex(t *testing.T) {
	t.Parallel()

	description := media.Description{
		Container: media.ContainerMKV,
		Streams:   []media.Stream{video(0), audio(1), subtitle(2), audio(4)},
	}

	for _, index := range []int{1, 4} {
		stream, err := description.AudioByIndex(index)
		if err != nil {
			t.Fatalf("AudioByIndex(%d) error = %v, want nil", index, err)
		}

		if stream.Index != index || stream.Kind != media.StreamKindAudio {
			t.Errorf("AudioByIndex(%d) = stream %d of kind %q", index, stream.Index, stream.Kind)
		}
	}

	for _, index := range []int{0, 2, 3, 5, -1} {
		stream, err := description.AudioByIndex(index)
		if !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("AudioByIndex(%d) error = %v, want ErrNotFound", index, err)
		}

		if stream.Kind != "" {
			t.Errorf("AudioByIndex(%d) = stream of kind %q on error, want zero stream", index, stream.Kind)
		}
	}
}

// selection is the result of a stream selection helper.
type selection struct {
	stream media.Stream
	ok     bool
}

// expectedSelection is the stream index a selection helper should return, if any.
type expectedSelection struct {
	index int
	ok    bool
}

func assertSelected(t *testing.T, helper string, got selection, want expectedSelection) {
	t.Helper()

	stream, ok := got.stream, got.ok
	wantIndex, wantOK := want.index, want.ok

	if ok != wantOK {
		t.Fatalf("%s() ok = %t, want %t", helper, ok, wantOK)
	}

	if ok && stream.Index != wantIndex {
		t.Errorf("%s() = stream %d, want %d", helper, stream.Index, wantIndex)
	}

	if !ok && stream.Kind != "" {
		t.Errorf("%s() = stream of kind %q when nothing matches, want zero stream", helper, stream.Kind)
	}
}

// hdr10Description mirrors the corpus clip video/hevc_main10_hdr10_ac3.mkv.
func hdr10Description() media.Description {
	return media.Description{
		Container:  media.ContainerMKV,
		FormatName: probeMatroska,
		Duration:   5 * time.Second,
		BitRate:    2_000_000,
		Streams: []media.Stream{
			{
				Index: 0, Kind: media.StreamKindVideo, Codec: "hevc", Profile: "Main 10", Level: 120,
				PixelFormat: "yuv420p10le", BitDepth: 10, Width: 1280, Height: 720,
				FrameRate:      media.NewFrameRate(25, 1),
				ColorPrimaries: "bt2020", ColorTransfer: "smpte2084", ColorSpace: "bt2020nc",
				HDR: media.HDRFormatHDR10, Default: true,
			},
			{
				Index: 1, Kind: media.StreamKindAudio, Codec: "ac3", Channels: 2,
				ChannelLayout: "stereo", SampleRate: 48_000, Language: "eng", Default: true,
				BitRate: 192_000,
			},
		},
	}
}

func video(index int) media.Stream {
	return media.Stream{Index: index, Kind: media.StreamKindVideo, Codec: "h264"}
}

func audio(index int) media.Stream {
	return media.Stream{Index: index, Kind: media.StreamKindAudio, Codec: "aac"}
}

func subtitle(index int) media.Stream {
	return media.Stream{Index: index, Kind: media.StreamKindSubtitle, Codec: "subrip"}
}

func attachedPicture(index int, isDefault bool) media.Stream {
	return media.Stream{
		Index: index, Kind: media.StreamKindVideo, Codec: "mjpeg",
		AttachedPicture: true, Default: isDefault,
	}
}

func withDefault(stream media.Stream) media.Stream {
	stream.Default = true

	return stream
}

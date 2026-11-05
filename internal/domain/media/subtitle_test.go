package media_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
)

func TestStreamIsBitmapSubtitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stream media.Stream
		want   bool
	}{
		{name: "DVD subtitle", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "dvd_subtitle"}, want: true},
		{name: "Blu-ray PGS", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "hdmv_pgs_subtitle"}, want: true},
		{name: "DVB subtitle", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "dvb_subtitle"}, want: true},
		{name: "XSUB", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "xsub"}, want: true},
		{name: "upper-case codec", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "DVD_SUBTITLE"}, want: true},
		{name: "SubRip is text", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "subrip"}, want: false},
		{name: "ASS is text", stream: media.Stream{Kind: media.StreamKindSubtitle, Codec: "ass"}, want: false},
		{name: "unknown codec", stream: media.Stream{Kind: media.StreamKindSubtitle}, want: false},
		{name: "video stream", stream: media.Stream{Kind: media.StreamKindVideo, Codec: "dvd_subtitle"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := test.stream.IsBitmapSubtitle(); got != test.want {
				t.Errorf("IsBitmapSubtitle() = %v, want %v", got, test.want)
			}
		})
	}
}

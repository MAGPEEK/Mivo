package media_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/media"
)

func TestClassifyHDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		stream media.Stream
		want   media.HDRFormat
	}{
		{
			name:   "hdr10 corpus clip",
			stream: videoStream("bt2020", "smpte2084"),
			want:   media.HDRFormatHDR10,
		},
		{
			name:   "hlg with bt2020",
			stream: videoStream("bt2020", "arib-std-b67"),
			want:   media.HDRFormatHLG,
		},
		{
			name:   "hlg without primaries",
			stream: videoStream("", "arib-std-b67"),
			want:   media.HDRFormatHLG,
		},
		{
			name:   "pq without bt2020 primaries",
			stream: videoStream("bt709", "smpte2084"),
			want:   media.HDRFormatNone,
		},
		{
			name:   "bt2020 sdr",
			stream: videoStream("bt2020", "bt2020-10"),
			want:   media.HDRFormatNone,
		},
		{
			name:   "bt709 sdr",
			stream: videoStream("bt709", "bt709"),
			want:   media.HDRFormatNone,
		},
		{
			name:   "untagged",
			stream: videoStream("", ""),
			want:   media.HDRFormatNone,
		},
		{
			name:   "upper-case tags",
			stream: videoStream("BT2020", "SMPTE2084"),
			want:   media.HDRFormatHDR10,
		},
		{
			name: "audio stream with colour tags",
			stream: media.Stream{
				Kind: media.StreamKindAudio, ColorPrimaries: "bt2020", ColorTransfer: "smpte2084",
			},
			want: media.HDRFormatNone,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := media.ClassifyHDR(test.stream); got != test.want {
				t.Errorf("ClassifyHDR(%+v) = %q, want %q", test.stream, got, test.want)
			}
		})
	}
}

func TestHDRFormatIsHDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		format media.HDRFormat
		want   bool
	}{
		{format: media.HDRFormatNone, want: false},
		{format: media.HDRFormatHDR10, want: true},
		{format: media.HDRFormatHLG, want: true},
	}

	for _, test := range tests {
		if got := test.format.IsHDR(); got != test.want {
			t.Errorf("HDRFormat(%q).IsHDR() = %t, want %t", test.format, got, test.want)
		}
	}
}

func videoStream(primaries, transfer string) media.Stream {
	return media.Stream{
		Kind: media.StreamKindVideo, Codec: "hevc",
		ColorPrimaries: primaries, ColorTransfer: transfer,
	}
}

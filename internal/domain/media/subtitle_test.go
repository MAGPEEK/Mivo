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

func TestSubtitleFormatForExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		extension string
		want      media.SubtitleFormat
		wantKnown bool
		wantText  bool
	}{
		{extension: "srt", want: media.SubtitleFormatSubRip, wantKnown: true, wantText: true},
		{extension: ".srt", want: media.SubtitleFormatSubRip, wantKnown: true, wantText: true},
		{extension: ".SRT", want: media.SubtitleFormatSubRip, wantKnown: true, wantText: true},
		{extension: "ass", want: media.SubtitleFormatASS, wantKnown: true, wantText: true},
		{extension: "ssa", want: media.SubtitleFormatSSA, wantKnown: true, wantText: true},
		{extension: "vtt", want: media.SubtitleFormatWebVTT, wantKnown: true, wantText: true},
		{extension: "sub", want: media.SubtitleFormatVobSub, wantKnown: true},
		{extension: "Idx", want: media.SubtitleFormatVobSub, wantKnown: true},
		{extension: "sup", want: media.SubtitleFormatPGS, wantKnown: true},
		{extension: "txt"},
		{extension: "mkv"},
		{extension: ""},
		{extension: "."},
		{extension: "..srt"},
	}

	for _, test := range tests {
		t.Run(test.extension, func(t *testing.T) {
			t.Parallel()

			got, known := media.SubtitleFormatForExtension(test.extension)
			if got != test.want || known != test.wantKnown {
				t.Errorf("SubtitleFormatForExtension(%q) = %q, %t, want %q, %t",
					test.extension, got, known, test.want, test.wantKnown)
			}

			if got.TextBased() != test.wantText {
				t.Errorf("%q.TextBased() = %t, want %t", got, got.TextBased(), test.wantText)
			}
		})
	}
}

// The image formats carry the ffprobe codec names, so an external picture subtitle is
// recognised by the same rule as an embedded one.
func TestImageSubtitleFormatsAreBitmapCodecs(t *testing.T) {
	t.Parallel()

	for _, format := range []media.SubtitleFormat{media.SubtitleFormatVobSub, media.SubtitleFormatPGS} {
		stream := media.Stream{Kind: media.StreamKindSubtitle, Codec: string(format)}
		if !stream.IsBitmapSubtitle() {
			t.Errorf("format %q is not a bitmap subtitle codec", format)
		}
	}
}

func TestExternalSubtitleTrackID(t *testing.T) {
	t.Parallel()

	// The ids are the first 12 hex digits of the SHA-1 of the path. Clients remember a track
	// id per item, so these values must never change.
	tests := []struct {
		name         string
		relativePath string
		want         string
	}{
		{name: "published SHA-1 test vector", relativePath: "abc", want: "x:a9993e364706"},
		{name: "empty path", relativePath: "", want: "x:da39a3ee5e6b"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			subtitle := media.ExternalSubtitle{RelativePath: test.relativePath}
			if got := subtitle.TrackID(); got != test.want {
				t.Errorf("TrackID() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestExternalSubtitleTrackIDDependsOnThePathOnly(t *testing.T) {
	t.Parallel()

	english := media.ExternalSubtitle{RelativePath: "Films/Movie/Movie.en.srt", Language: "en"}
	german := media.ExternalSubtitle{RelativePath: "Films/Movie/Movie.de.srt", Language: "de"}
	retagged := media.ExternalSubtitle{RelativePath: "Films/Movie/Movie.en.srt", Language: "eng", Forced: true}

	if english.TrackID() == german.TrackID() {
		t.Errorf("two paths share the track id %q", english.TrackID())
	}

	if english.TrackID() != retagged.TrackID() {
		t.Errorf("TrackID() = %q and %q for one path, want one id", english.TrackID(), retagged.TrackID())
	}
}

func TestExternalSubtitleUnder(t *testing.T) {
	t.Parallel()

	subtitle := media.ExternalSubtitle{
		RelativePath: "Subs/Movie.es.srt", Format: media.SubtitleFormatSubRip, Language: "es", Forced: true, TextBased: true,
	}

	tests := []struct {
		name   string
		folder string
		want   string
	}{
		{name: "library root", folder: "", want: "Subs/Movie.es.srt"},
		{name: "folder", folder: "Films/Movie", want: "Films/Movie/Subs/Movie.es.srt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			want := subtitle
			want.RelativePath = test.want

			if got := subtitle.Under(test.folder); got != want {
				t.Errorf("Under(%q) = %+v, want %+v", test.folder, got, want)
			}
		})
	}
}

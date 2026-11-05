package media

import (
	"slices"
	"strings"
)

// bitmapSubtitleCodecs are ffprobe codec names of subtitles stored as pictures: DVD (VobSub),
// Blu-ray (PGS), DVB and DivX XSUB. They cannot become WebVTT; showing them means burning
// them into a transcoded video.
func bitmapSubtitleCodecs() []string {
	return []string{"dvd_subtitle", "hdmv_pgs_subtitle", "dvb_subtitle", "xsub"}
}

// IsBitmapSubtitle reports a subtitle stream whose codec stores pictures instead of text,
// such as the subtitles of a DVD or Blu-ray disc.
func (stream Stream) IsBitmapSubtitle() bool {
	return stream.Kind == StreamKindSubtitle &&
		slices.Contains(bitmapSubtitleCodecs(), strings.ToLower(stream.Codec))
}

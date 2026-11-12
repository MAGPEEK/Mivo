package media

import (
	"crypto/sha1" //nolint:gosec // names a track (task M4-05b fixes SHA-1); it protects nothing
	"encoding/hex"
	"slices"
	"strings"
)

// SubtitleFormat is the format of a subtitle file next to a media file. The values are the
// ffprobe codec names, so embedded and external subtitles share one vocabulary.
type SubtitleFormat string

// Subtitle file formats Mivo picks up next to a media file (ADR 0030 §3).
const (
	SubtitleFormatSubRip SubtitleFormat = "subrip"
	SubtitleFormatASS    SubtitleFormat = "ass"
	SubtitleFormatSSA    SubtitleFormat = "ssa"
	SubtitleFormatWebVTT SubtitleFormat = "webvtt"
	// SubtitleFormatVobSub is a DVD picture subtitle: an .idx index with its .sub pictures.
	SubtitleFormatVobSub SubtitleFormat = "dvd_subtitle"
	// SubtitleFormatPGS is a Blu-ray picture subtitle (.sup).
	SubtitleFormatPGS SubtitleFormat = "hdmv_pgs_subtitle"
)

// UndeterminedLanguage is the ISO 639-2 code of a subtitle whose language is not known.
const UndeterminedLanguage = "und"

const (
	extensionSeparator = "."
	pathSeparator      = "/"
	// externalTrackPrefix marks the track id of a subtitle file; embedded streams use "s:".
	externalTrackPrefix = "x:"
	// externalTrackDigestLength is the number of hex digits of the path digest in a track
	// id. 48 bits keep the ids of the few subtitle files of one media file apart.
	externalTrackDigestLength = 12
)

// SubtitleFormatForExtension returns the subtitle format of a file extension, with or
// without the leading dot and in any case. A .sub file counts as the picture half of a
// VobSub pair; a text subtitle with that extension (MicroDVD) is not supported.
func SubtitleFormatForExtension(extension string) (SubtitleFormat, bool) {
	switch strings.ToLower(strings.TrimPrefix(extension, extensionSeparator)) {
	case "srt":
		return SubtitleFormatSubRip, true
	case "ass":
		return SubtitleFormatASS, true
	case "ssa":
		return SubtitleFormatSSA, true
	case "vtt":
		return SubtitleFormatWebVTT, true
	case "idx", "sub":
		return SubtitleFormatVobSub, true
	case "sup":
		return SubtitleFormatPGS, true
	default:
		return "", false
	}
}

// TextBased reports a format that stores text and can be delivered as WebVTT. The picture
// formats and an unknown format need burn-in.
func (format SubtitleFormat) TextBased() bool {
	switch format {
	case SubtitleFormatSubRip, SubtitleFormatASS, SubtitleFormatSSA, SubtitleFormatWebVTT:
		return true
	case SubtitleFormatVobSub, SubtitleFormatPGS:
		return false
	default:
		return false
	}
}

// ExternalSubtitle is a subtitle file found next to a media file, with what its name says
// about it (ADR 0030 §3).
type ExternalSubtitle struct {
	// RelativePath uses "/" and is relative to the folder of the media file when the name
	// was parsed; Under moves it below a library root.
	RelativePath string
	Format       SubtitleFormat
	// Language is "ll" or "lll", optionally with script and region ("pt-BR"), or "und".
	Language string
	// Title holds the parts of the name that are neither language nor flag, for example
	// "commentary"; empty for most files.
	Title   string
	Forced  bool
	SDH     bool
	Default bool
	// TextBased is false for picture subtitles (VobSub, PGS), which need burn-in.
	TextBased bool
}

// TrackID is the stable id of the subtitle as a track of its media file: "x:" and the
// first hex digits of the SHA-1 of RelativePath. It changes when the file is renamed or
// moved, so call it on the path that is stored (see Under).
func (subtitle ExternalSubtitle) TrackID() string {
	digest := sha1.Sum([]byte(subtitle.RelativePath)) //nolint:gosec // an id, not a security boundary

	return externalTrackPrefix + hex.EncodeToString(digest[:])[:externalTrackDigestLength]
}

// Under returns the subtitle with its path placed below folder, the path of the media
// file's folder relative to the library root; an empty folder is the root itself.
func (subtitle ExternalSubtitle) Under(folder string) ExternalSubtitle {
	if folder != "" {
		subtitle.RelativePath = folder + pathSeparator + subtitle.RelativePath
	}

	return subtitle
}

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

package media

import "strings"

// HDRFormat names the high dynamic range format of a video stream. The empty value means
// SDR or unknown.
type HDRFormat string

// The HDR formats detected in milestone 2. Dolby Vision is out of scope (see the package
// documentation).
const (
	HDRFormatNone  HDRFormat = ""
	HDRFormatHDR10 HDRFormat = "hdr10"
	HDRFormatHLG   HDRFormat = "hlg"
)

// ffprobe colour tags that identify HDR signals.
const (
	colorPrimariesBT2020 = "bt2020"
	transferPQ           = "smpte2084"
	transferHLG          = "arib-std-b67"
)

// IsHDR reports whether the format is a high dynamic range format.
func (format HDRFormat) IsHDR() bool {
	return format != HDRFormatNone
}

// isKnown reports whether the format is one of the declared constants.
func (format HDRFormat) isKnown() bool {
	switch format {
	case HDRFormatNone, HDRFormatHDR10, HDRFormatHLG:
		return true
	default:
		return false
	}
}

// ClassifyHDR derives the HDR format from the colour tags of a video stream: BT.2020
// primaries with the PQ transfer (smpte2084) are HDR10, the HLG transfer (arib-std-b67) is
// HLG whatever the primaries say. PQ without BT.2020 primaries is not classified, because
// such tags come from broken muxers far more often than from real HDR masters. Static
// mastering metadata is not required: many HDR10 files lack it and still need the HDR
// handling. Non-video streams are never HDR.
func ClassifyHDR(stream Stream) HDRFormat {
	if stream.Kind != StreamKindVideo {
		return HDRFormatNone
	}

	transfer := strings.ToLower(stream.ColorTransfer)

	switch {
	case transfer == transferHLG:
		return HDRFormatHLG
	case transfer == transferPQ && strings.ToLower(stream.ColorPrimaries) == colorPrimariesBT2020:
		return HDRFormatHDR10
	default:
		return HDRFormatNone
	}
}

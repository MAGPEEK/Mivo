package main

import "slices"

// Feature tags (task M4-03). A tag names what a case is an input for, so a test asks the
// manifest for "every imagesub case" instead of repeating file names. Known gaps are tagged
// too: a test that selects by tag then sees that the format exists but has no file.
const (
	// tagTextSubtitle: text subtitles a player gets as WebVTT (SRT, ASS, mov_text, WebVTT).
	tagTextSubtitle = "textsub"
	// tagStyledSubtitle: ASS with styles and override tags, which a WebVTT conversion loses.
	tagStyledSubtitle = "styledsub"
	// tagImageSubtitle: bitmap subtitles (PGS, VobSub, DVB), which must be burned in.
	tagImageSubtitle = "imagesub"
	// tagMultiAudio: more than one audio track, for track selection.
	tagMultiAudio = "multiaudio"
	// tagCommentary: an audio track titled as a commentary.
	tagCommentary = "commentary"
	// tagSurround: audio with more than two channels.
	tagSurround = "surround"
	// tagHiRes: lossless audio above CD resolution.
	tagHiRes = "hires"
	// tagDSD: 1-bit DSD audio.
	tagDSD = "dsd"
	// tagToneMapping: HDR video a client without HDR needs tone-mapped.
	tagToneMapping = "tonemap"
	// tagLong: clips long enough to seek in and to cut trickplay thumbnails from.
	tagLong = "long"
)

// Names of the cases task M4-03 added.
const (
	caseStyledSubtitles = "tracks-mkv-ass-styled-srt"
	caseCommentary      = "tracks-mkv-eng-deu51-commentary"
	casePGS             = "tracks-mkv-pgs"
	caseDVDSubtitle     = "tracks-mkv-dvdsub"
	caseToneMapping     = "hdr-mkv-hevc10-hdr10-ac3-20s"
)

// caseTags is the one table that maps case names to their feature tags. Alternatives of one
// name share its tags.
func caseTags() map[string][]string {
	return map[string][]string{
		"subtitle-sidecar-srt-vtt-ass": {tagTextSubtitle},
		"subtitle-mkv-srt-ass":         {tagTextSubtitle},
		"subtitle-mp4-movtext":         {tagTextSubtitle},
		caseStyledSubtitles:            {tagTextSubtitle, tagStyledSubtitle},

		"subtitle-sup-pgs": {tagImageSubtitle},
		"subtitle-vobsub":  {tagImageSubtitle},
		"subtitle-ts-dvb":  {tagImageSubtitle},
		casePGS:            {tagImageSubtitle},
		caseDVDSubtitle:    {tagImageSubtitle},

		"multitrack-mkv-eng-deu":                {tagMultiAudio, tagTextSubtitle},
		"multitrack-mkv-dts-ac3-deu-eng":        {tagMultiAudio, tagSurround},
		"multitrack-mp4-ac3-aac-default-second": {tagMultiAudio},
		caseCommentary:                          {tagMultiAudio, tagCommentary, tagSurround},

		"video-mkv-h264-ac3-51":    {tagSurround},
		"video-mkv-h264-eac3-51":   {tagSurround},
		"video-mp4-h264-eac3-51":   {tagSurround},
		"video-mkv-h264-dts-51":    {tagSurround},
		"video-mkv-h264-truehd-51": {tagSurround},
		"video-mkv-h264-opus-51":   {tagSurround},
		"video-mp4-h264-aac-51":    {tagSurround},
		"video-mp4-h264-aac-71":    {tagSurround},
		"video-mkv-h264-flac-71":   {tagSurround},

		"audio-flac-24bit-96k": {tagHiRes},
		"audio-dsf-dsd":        {tagDSD},
		"audio-dff-dsd":        {tagDSD},

		"video-mkv-hevc10-hdr10-ac3": {tagToneMapping},
		caseToneMapping:              {tagToneMapping, tagLong},

		"video-mkv-h264-aac" + seekCaseSuffix: {tagLong},
		"video-ts-mpeg2-mp2" + seekCaseSuffix: {tagLong},
	}
}

// withTags returns the cases with the tags of caseTags, sorted so the manifest is stable.
func withTags(cases []mediaCase) []mediaCase {
	tags := caseTags()

	for index := range cases {
		cases[index].Tags = slices.Sorted(slices.Values(tags[cases[index].Name]))
	}

	return cases
}

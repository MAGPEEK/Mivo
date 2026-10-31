package corpustest

import (
	"maps"
	"slices"
	"time"

	"github.com/magpeek/mivo/internal/domain/playback"
)

// Names of the reference profiles of ADR 0025 (playback.ClientProfile.Name of the
// playbacktest profiles); they are the profile keys of the table.
const (
	ChromiumDesktop = "chromium-desktop"
	FirefoxDesktop  = "firefox-desktop"
)

// Tolerances of the output checks (ADR 0025).
const (
	// DurationTolerance is how far the length of produced output may differ from the
	// manifest's duration minus the actual start: half a second absorbs the frame and audio
	// frame granularity at both ends and the container quirks of the legacy formats.
	DurationTolerance = 500 * time.Millisecond
	// KeyframeTolerance is how far a reported keyframe start may differ from the nominal
	// keyframe of a seek clip. FFmpeg versions disagree on the format start time (7.1
	// reports −0.021 s for the long MKV, 9.0 reports 0), and keyframe positions are measured
	// from it, so they are never compared exactly.
	KeyframeTolerance = 100 * time.Millisecond
)

// Positions of the seek expectations: the seek clips are 30 s long with a keyframe every
// 2 s (M2-06), so 21 s lies one second after the keyframe at 20 s.
const (
	seekRequestedPosition = 21 * time.Second
	seekKeyframePosition  = 20 * time.Second
)

// Manifest case names the seek expectations refer to.
const (
	caseLongMatroska        = "video-mkv-h264-aac-long"
	caseLongTransportStream = "video-ts-mpeg2-mp2-long"
)

// Expectation is the plan a reference profile must get for a corpus case from a server with
// full capabilities (playbacktest.FullCapabilities) and the 1080 height cap. Every corpus
// video has one video and one selected audio stream, so both actions are always set.
type Expectation struct {
	Mode  playback.Mode
	Video playback.Action
	Audio playback.Action
	// Reasons and Warnings are compared in order, as playback.Decide reports them.
	Reasons  []playback.Reason
	Warnings []playback.Warning
}

// SeekExpectation is a session of a seek clip started in the middle.
type SeekExpectation struct {
	// Case is the manifest case name and Profile the reference profile name; Expected
	// returns the plan.
	Case    string
	Profile string
	// Requested is the start position the client asks for.
	Requested time.Duration
	// ActualStart is the start the session reports: the keyframe at or before Requested
	// when the video is copied (within KeyframeTolerance), exactly Requested when it is
	// re-encoded (ADR 0024).
	ActualStart time.Duration
}

// caseExpectation holds the expectations of one corpus case for both reference profiles.
type caseExpectation struct {
	chromium Expectation
	firefox  Expectation
}

// Expected returns the expectation of a manifest case for a reference profile name. The
// second result is false for an unknown case or profile. The caller owns the result.
func Expected(caseName, profileName string) (Expectation, bool) {
	entry, found := expectations()[caseName]
	if !found {
		return Expectation{}, false
	}

	switch profileName {
	case ChromiumDesktop:
		return entry.chromium, true
	case FirefoxDesktop:
		return entry.firefox, true
	default:
		return Expectation{}, false
	}
}

// CaseNames returns the names of every case the table covers, sorted.
func CaseNames() []string {
	return slices.Sorted(maps.Keys(expectations()))
}

// SeekExpectations returns the seek sessions of ADR 0025: the long MKV is remuxed and starts
// at the keyframe before the requested position, the long MPEG-TS is transcoded and starts
// exactly there.
func SeekExpectations() []SeekExpectation {
	return []SeekExpectation{
		{
			Case: caseLongMatroska, Profile: ChromiumDesktop,
			Requested: seekRequestedPosition, ActualStart: seekKeyframePosition,
		},
		{
			Case: caseLongTransportStream, Profile: ChromiumDesktop,
			Requested: seekRequestedPosition, ActualStart: seekRequestedPosition,
		},
	}
}

// expectations builds the table; every call returns new values, so callers may change them.
func expectations() map[string]caseExpectation {
	table := videoExpectations()
	maps.Copy(table, trackExpectations())

	return table
}

// videoExpectations mirror corpusVideoRows of the M2-09 format matrix.
func videoExpectations() map[string]caseExpectation {
	return map[string]caseExpectation{
		"video-mkv-h264-aac":           both(remux(containerReason())),
		"video-mkv-hevc10-ac3":         both(standardTranscode()),
		"video-mkv-hevc10-hdr10-ac3":   both(standardTranscode().withWarnings(hdrWarning())),
		"video-mp4-h264-aac-faststart": both(directPlay()),
		"video-m4v-h264-aac":           both(directPlay()),
		"video-mov-h264-pcm":           both(transcodeAudioOnly()),
		"video-3gp-h263-aac":           both(transcodeVideoOnly()),
		"video-avi-xvid-mp3":           both(transcodeVideoOnly()),
		"video-avi-divx-mp3":           both(transcodeVideoOnly()),
		"video-ts-mpeg2-mp2":           both(standardTranscode()),
		"video-m2ts-h264-ac3":          both(transcodeAudioOnly()),
		"video-mts-h264-ac3":           both(transcodeAudioOnly()),
		"video-webm-vp9-opus":          both(directPlay()),
		"video-flv-h264-aac":           both(remux(containerReason())),
		// Only Firefox plays Theora in Ogg; Chromium gets neither codec inside HLS.
		"video-ogv-theora-vorbis": {chromium: standardTranscode(), firefox: directPlay()},
		"video-webm-av1-opus":     both(directPlay()),
		"video-mpg-mpeg2-mp2":     both(standardTranscode()),
		"video-vob-mpeg2-ac3":     both(standardTranscode()),
		"video-wmv-wmv2-wmav2":    both(standardTranscode()),
		"video-mxf-mpeg2-pcm":     both(standardTranscode()),
		"video-rm-rv20-ra144":     both(standardTranscode()),
		"video-rmvb-rv20-ra144":   both(standardTranscode()),
		// A known gap of the corpus (ADR 0021): no FFmpeg can produce it, the entry documents
		// the mode a real file gets.
		"video-rmvb-rv40-cook":  both(standardTranscode()),
		caseLongMatroska:        both(remux(containerReason())),
		caseLongTransportStream: both(standardTranscode()),
	}
}

// trackExpectations mirror corpusTrackRows of the M2-09 format matrix: embedded subtitles
// are ignored in milestone 2 with a warning, sidecar files are not part of the media file.
func trackExpectations() map[string]caseExpectation {
	return map[string]caseExpectation{
		"subtitle-sidecar-srt-vtt-ass": both(remux(containerReason())),
		"subtitle-mkv-srt-ass":         both(remux(containerReason()).withWarnings(subtitlesWarning())),
		"subtitle-mp4-movtext":         both(directPlay(subtitlesWarning())),
		"multitrack-mkv-eng-deu":       both(remux(containerReason()).withWarnings(subtitlesWarning())),
	}
}

func both(expected Expectation) caseExpectation {
	return caseExpectation{chromium: expected, firefox: expected.clone()}
}

func (expected Expectation) clone() Expectation {
	expected.Reasons = slices.Clone(expected.Reasons)
	expected.Warnings = slices.Clone(expected.Warnings)

	return expected
}

func (expected Expectation) withWarnings(warnings ...playback.Warning) Expectation {
	expected.Warnings = append(slices.Clone(expected.Warnings), warnings...)

	return expected
}

func directPlay(warnings ...playback.Warning) Expectation {
	return Expectation{
		Mode: playback.ModeDirectPlay, Video: playback.ActionCopy, Audio: playback.ActionCopy, Warnings: warnings,
	}
}

func remux(reasons ...playback.Reason) Expectation {
	return Expectation{Mode: playback.ModeRemux, Video: playback.ActionCopy, Audio: playback.ActionCopy, Reasons: reasons}
}

func transcode(video, audio playback.Action, reasons ...playback.Reason) Expectation {
	return Expectation{Mode: playback.ModeTranscode, Video: video, Audio: audio, Reasons: reasons}
}

// standardTranscode is the plan of a file whose container and both codecs are foreign to
// the reference profiles (MPEG-2, HEVC, WMV, RealMedia, ...).
func standardTranscode() Expectation {
	return transcode(playback.ActionTranscode, playback.ActionTranscode,
		containerReason(), videoCodecReason(), audioCodecReason())
}

// transcodeVideoOnly re-encodes a foreign video codec and copies the audio.
func transcodeVideoOnly() Expectation {
	return transcode(playback.ActionTranscode, playback.ActionCopy, containerReason(), videoCodecReason())
}

// transcodeAudioOnly copies H.264 and re-encodes a foreign audio codec such as AC-3 or PCM.
func transcodeAudioOnly() Expectation {
	return transcode(playback.ActionCopy, playback.ActionTranscode, containerReason(), audioCodecReason())
}

func containerReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonContainerNotSupported, Scope: playback.ScopeContainer}
}

func videoCodecReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonVideoCodecNotSupported, Scope: playback.ScopeVideo}
}

func audioCodecReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonAudioCodecNotSupported, Scope: playback.ScopeAudio}
}

func hdrWarning() playback.Warning {
	return playback.Warning{Code: playback.WarningHDRNotToneMapped, Scope: playback.ScopeVideo}
}

func subtitlesWarning() playback.Warning {
	return playback.Warning{Code: playback.WarningSubtitlesNotSupported, Scope: playback.ScopeSubtitle}
}

package playback_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const pgsCodec = "hdmv_pgs_subtitle"

func webVTT() playback.SubtitleOptions {
	return playback.SubtitleOptions{Formats: []string{playback.SubtitleFormatWebVTT}}
}

func degradable(options playback.SubtitleOptions) playback.SubtitleOptions {
	options.AllowDegrade = true

	return options
}

func trackRequest(id string) playback.Request {
	return playback.Request{SubtitleTrackID: &id}
}

func burnInReason() playback.Reason {
	return playback.Reason{Code: playback.ReasonSubtitleBurnIn, Scope: playback.ScopeSubtitle}
}

func burnInUnavailableWarning() playback.Warning {
	return playback.Warning{Code: playback.WarningSubtitleBurnInUnavailable, Scope: playback.ScopeSubtitle}
}

func forced(stream media.Stream, language string) media.Stream {
	stream.Forced = true
	stream.Language = language

	return stream
}

func inLanguage(stream media.Stream, language string) media.Stream {
	stream.Language = language

	return stream
}

func sidecar(trackID, format string) playback.SubtitleDecision {
	return playback.SubtitleDecision{Mode: playback.SubtitleSidecar, TrackID: trackID, Format: format}
}

func burnIn(trackID, format string) playback.SubtitleDecision {
	return playback.SubtitleDecision{Mode: playback.SubtitleBurnIn, TrackID: trackID, Format: format}
}

func subtitlesOff() playback.SubtitleDecision {
	return playback.SubtitleDecision{Mode: playback.SubtitleOff}
}

// subtitleCase is one decision with the subtitle the plan must report. The zero profile and
// capabilities mean the Chromium reference profile on a server with every encoder.
type subtitleCase struct {
	name         string
	description  media.Description
	layout       media.Layout
	external     []playback.SubtitleTrack
	profile      *playback.ClientProfile
	capabilities *playback.ServerCapabilities
	options      playback.SubtitleOptions
	request      playback.Request
	want         outcome
	subtitle     playback.SubtitleDecision
}

func (testCase subtitleCase) input() playback.Input {
	input := playback.Input{
		Description:       testCase.description,
		Layout:            testCase.layout,
		ExternalSubtitles: testCase.external,
		Subtitles:         testCase.options,
		Profile:           playbacktest.ChromiumDesktop(),
		Capabilities:      playbacktest.FullCapabilities(),
		Request:           testCase.request,
		Policy:            playback.Policy{MaxTranscodeHeight: referenceMaxHeight},
	}

	if testCase.profile != nil {
		input.Profile = *testCase.profile
	}

	if testCase.capabilities != nil {
		input.Capabilities = *testCase.capabilities
	}

	return input
}

func runSubtitleCases(t *testing.T, cases []subtitleCase) {
	t.Helper()

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			plan, err := playback.Decide(testCase.input())

			assertOutcome(t, plan, err, testCase.want)

			if testCase.want.err == nil && plan.Subtitle != testCase.subtitle {
				t.Errorf("Subtitle = %+v, want %+v", plan.Subtitle, testCase.subtitle)
			}
		})
	}
}

func mp4With(subtitles ...media.Stream) media.Description {
	streams := append([]media.Stream{h264High(0), inLanguage(audio(1, "aac"), "eng")}, subtitles...)

	return clip(media.ContainerMP4, streams...)
}

func alwaysEnglish(profile playback.ClientProfile) *playback.ClientProfile {
	profile.Tracks = playback.TrackPreference{
		SubtitleMode: playback.SubtitleModeAlways, SubtitleLanguages: []string{"eng"},
	}

	return &profile
}

func TestDecideSubtitleDefaultPolicy(t *testing.T) {
	t.Parallel()

	english := alwaysEnglish(playbacktest.ChromiumDesktop())

	runSubtitleCases(t, []subtitleCase{
		{
			name:        "file without subtitles",
			description: mp4With(), options: webVTT(),
			want: directPlay(), subtitle: subtitlesOff(),
		},
		{
			name:        "preferred text track is a sidecar and keeps direct play",
			description: mp4With(inLanguage(subtitle(2, "subrip"), "eng")),
			profile:     english, options: webVTT(),
			want: directPlay(), subtitle: sidecar("s:2", "subrip"),
		},
		{
			name:        "preferred ASS track is a sidecar",
			description: mp4With(inLanguage(subtitle(2, "ass"), "eng")),
			profile:     english, options: webVTT(),
			want: directPlay(), subtitle: sidecar("s:2", "ass"),
		},
		{
			name:        "a client that renders nothing gets no subtitle and no warning",
			description: mp4With(inLanguage(subtitle(2, "subrip"), "eng")),
			profile:     english,
			want:        directPlay(), subtitle: subtitlesOff(),
		},
		{
			name:        "preferred image track is never burned in by default",
			description: mp4With(inLanguage(subtitle(2, pgsCodec), "eng")),
			profile:     english, options: webVTT(),
			want: directPlay(), subtitle: subtitlesOff(),
		},
		{
			name:        "forced image track in the audio language stays off",
			description: mp4With(forced(subtitle(2, pgsCodec), "eng")),
			options:     webVTT(),
			want:        directPlay(), subtitle: subtitlesOff(),
		},
		{
			name:        "forced text track in the audio language is a sidecar",
			description: mp4With(forced(subtitle(2, pgsCodec), "eng"), forced(subtitle(3, "subrip"), "eng")),
			options:     webVTT(),
			want:        directPlay(), subtitle: sidecar("s:3", "subrip"),
		},
		{
			name:        "the renderable track wins over a better ranked image track",
			description: mp4With(asDefault(inLanguage(subtitle(2, pgsCodec), "eng")), inLanguage(subtitle(3, "subrip"), "eng")),
			profile:     english, options: webVTT(),
			want: directPlay(), subtitle: sidecar("s:3", "subrip"),
		},
		{
			name:        "preferred external text track is a sidecar",
			description: mp4With(),
			external:    []playback.SubtitleTrack{{ID: "x:abc", Language: "eng", Format: "subrip", External: true, TextBased: true}},
			profile:     english, options: webVTT(),
			want: directPlay(), subtitle: sidecar("x:abc", "subrip"),
		},
		{
			name:        "a remux keeps its mode with a sidecar",
			description: clip(media.ContainerMKV, h264High(0), audio(1, "aac"), inLanguage(subtitle(2, "subrip"), "eng")),
			profile:     english, options: webVTT(),
			want: remux(containerReason()), subtitle: sidecar("s:2", "subrip"),
		},
	})
}

func TestDecideSubtitleRequested(t *testing.T) {
	t.Parallel()

	hdr := clip(media.ContainerMKV, hevcHDR10(0), audio(1, "aac"), subtitle(2, pgsCodec))

	runSubtitleCases(t, []subtitleCase{
		{
			name:        "empty id switches subtitles off",
			description: mp4With(forced(subtitle(2, "subrip"), "eng")),
			options:     webVTT(), request: trackRequest(""),
			want: directPlay(), subtitle: subtitlesOff(),
		},
		{
			name:        "text track the client renders",
			description: mp4With(subtitle(2, "mov_text")),
			options:     webVTT(), request: trackRequest("s:2"),
			want: directPlay(), subtitle: sidecar("s:2", "mov_text"),
		},
		{
			name:        "image track is burned in and the audio is still copied",
			description: mp4With(subtitle(2, pgsCodec)),
			options:     webVTT(), request: trackRequest("s:2"),
			want:     transcode(transcoded, copied, burnInReason()),
			subtitle: burnIn("s:2", pgsCodec),
		},
		{
			name:        "text track of a client that renders nothing is burned in",
			description: mp4With(subtitle(2, "subrip")),
			request:     trackRequest("s:2"),
			want:        transcode(transcoded, copied, burnInReason()),
			subtitle:    burnIn("s:2", "subrip"),
		},
		{
			name:        "a format list without webvtt renders no text track",
			description: mp4With(subtitle(2, "subrip")),
			options:     playback.SubtitleOptions{Formats: []string{"ttml"}}, request: trackRequest("s:2"),
			want:     transcode(transcoded, copied, burnInReason()),
			subtitle: burnIn("s:2", "subrip"),
		},
		{
			name:        "external image track is burned in",
			description: mp4With(),
			external:    []playback.SubtitleTrack{{ID: "x:1f", Format: "dvd_subtitle", External: true}},
			options:     webVTT(), request: trackRequest("x:1f"),
			want:     transcode(transcoded, copied, burnInReason()),
			subtitle: burnIn("x:1f", "dvd_subtitle"),
		},
		{
			name:        "burn-in reason follows the container reason",
			description: clip(media.ContainerMKV, h264High(0), audio(1, "ac3"), subtitle(2, pgsCodec)),
			options:     webVTT(), request: trackRequest("s:2"),
			want:     transcode(transcoded, transcoded, containerReason(), burnInReason(), audioCodecReason()),
			subtitle: burnIn("s:2", pgsCodec),
		},
		{
			name:        "video that is re-encoded anyway keeps its own reason",
			description: clip(media.ContainerMKV, hevcMain10(0), audio(1, "aac"), subtitle(2, pgsCodec)),
			options:     webVTT(), request: trackRequest("s:2"),
			want:     transcode(transcoded, copied, containerReason(), burnInReason(), videoCodecReason()),
			subtitle: burnIn("s:2", pgsCodec),
		},
		{
			name:        "burned-in HDR video is not tone mapped",
			description: hdr, options: webVTT(), request: trackRequest("s:2"),
			want: transcode(transcoded, copied, containerReason(), burnInReason(), videoCodecReason()).
				withWarnings(hdrWarning()),
			subtitle: burnIn("s:2", pgsCodec),
		},
		{
			name: "disc subtitle is burned in", layout: media.LayoutBDMV,
			description: clip(media.ContainerM2TS, h264High(0), audio(1, "aac"), subtitle(2, pgsCodec)),
			options:     webVTT(), request: trackRequest("s:2"),
			want:     transcode(transcoded, copied, discReason(), burnInReason()),
			subtitle: burnIn("s:2", pgsCodec),
		},
		{
			name:        "unknown track id",
			description: mp4With(subtitle(2, "subrip")),
			options:     webVTT(), request: trackRequest("s:9"),
			want: failure(domain.ErrInvalid),
		},
	})
}

func TestDecideSubtitleBurnInUnavailable(t *testing.T) {
	t.Parallel()

	noEncoders := playbacktest.NoEncoders()
	noFFmpeg := playback.ServerCapabilities{}

	noHLS := playbacktest.ChromiumDesktop()
	noHLS.HLS.Delivery = playback.HLSDeliveryNone

	vp9Only := playbacktest.ChromiumDesktop()
	vp9Only.HLS.VideoCodecs = []string{"vp9"}

	pgsFile := mp4With(subtitle(2, pgsCodec))
	audioOnly := clip(media.ContainerMP4, audio(0, "aac"))
	externalText := []playback.SubtitleTrack{{ID: "x:aa", Format: "subrip", External: true, TextBased: true}}
	degraded := directPlay(burnInUnavailableWarning())

	runSubtitleCases(t, []subtitleCase{
		{
			name:        "server without encoders refuses",
			description: pgsFile, capabilities: &noEncoders,
			options: webVTT(), request: trackRequest("s:2"),
			want: failure(playback.ErrTranscodeUnavailable),
		},
		{
			name:        "server without encoders degrades when allowed",
			description: pgsFile, capabilities: &noEncoders,
			options: degradable(webVTT()), request: trackRequest("s:2"),
			want: degraded, subtitle: subtitlesOff(),
		},
		{
			name:        "server without FFmpeg degrades when allowed",
			description: pgsFile, capabilities: &noFFmpeg,
			options: degradable(webVTT()), request: trackRequest("s:2"),
			want: degraded, subtitle: subtitlesOff(),
		},
		{
			name:        "client without HLS refuses",
			description: pgsFile, profile: &noHLS,
			options: webVTT(), request: trackRequest("s:2"),
			want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name:        "client without HLS degrades when allowed",
			description: pgsFile, profile: &noHLS,
			options: degradable(webVTT()), request: trackRequest("s:2"),
			want: degraded, subtitle: subtitlesOff(),
		},
		{
			name:        "client that plays no H.264 inside HLS refuses",
			description: pgsFile, profile: &vp9Only,
			options: webVTT(), request: trackRequest("s:2"),
			want: failure(playback.ErrNoCompatibleFormat),
		},
		{
			name:        "client that plays no H.264 inside HLS degrades when allowed",
			description: pgsFile, profile: &vp9Only,
			options: degradable(webVTT()), request: trackRequest("s:2"),
			want: degraded, subtitle: subtitlesOff(),
		},
		{
			name:        "audio has no picture to draw on",
			description: audioOnly, external: externalText,
			request: trackRequest("x:aa"),
			want:    failure(playback.ErrNoCompatibleFormat),
		},
		{
			name:        "audio degrades when allowed",
			description: audioOnly, external: externalText,
			options: degradable(playback.SubtitleOptions{}), request: trackRequest("x:aa"),
			want: outcome{
				mode: playback.ModeDirectPlay, audio: copied,
				warnings: []playback.Warning{burnInUnavailableWarning()},
			},
			subtitle: subtitlesOff(),
		},
		{
			name:        "degrading is not used while burn-in works",
			description: pgsFile,
			options:     degradable(webVTT()), request: trackRequest("s:2"),
			want:     transcode(transcoded, copied, burnInReason()),
			subtitle: burnIn("s:2", pgsCodec),
		},
		{
			name:        "a client limit of the audio is reported before the missing encoder",
			description: clip(media.ContainerMKV, h264High(0), audio(1, "ac3"), subtitle(2, pgsCodec)),
			profile:     withoutHLSAudio(playbacktest.ChromiumDesktop()), capabilities: &noEncoders,
			options: webVTT(), request: trackRequest("s:2"),
			want: failure(playback.ErrNoCompatibleFormat),
		},
	})
}

func withoutHLSAudio(profile playback.ClientProfile) *playback.ClientProfile {
	profile.HLS.AudioCodecs = []string{"opus"}

	return &profile
}

func TestDecideRejectsInvalidSubtitleFormats(t *testing.T) {
	t.Parallel()

	tooMany := slices.Repeat([]string{playback.SubtitleFormatWebVTT}, 9)

	for name, formats := range map[string][]string{
		"more than eight formats": tooMany,
		"upper-case format":       {"WebVTT"},
		"empty format":            {""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := playback.Decide(playback.Input{
				Description:  mp4With(),
				Profile:      playbacktest.ChromiumDesktop(),
				Capabilities: playbacktest.FullCapabilities(),
				Subtitles:    playback.SubtitleOptions{Formats: formats},
			})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("Decide() error = %v, want %v", err, domain.ErrInvalid)
			}
		})
	}
}

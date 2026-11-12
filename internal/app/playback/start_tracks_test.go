package playback_test

import (
	"context"
	"sync"
	"testing"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

const (
	subtitledFileID = "f2000000-0000-4000-8000-000000000001"
	sourceBitRate   = int64(6_000_000)
	pgsTrackID      = "s:3"
	textTrackID     = "s:2"
	externalTrackID = "x:0123456789abcdef"
	twoMegabits     = int64(2_000_000)
)

// fakeSubtitles returns fixed external tracks or an error.
type fakeSubtitles struct {
	tracks []playback.SubtitleTrack
	err    error
}

func (source fakeSubtitles) Tracks(context.Context, string) ([]playback.SubtitleTrack, error) {
	return source.tracks, source.err
}

// recordingHardware records the APIs marked degraded.
type recordingHardware struct {
	mutex    sync.Mutex
	degraded []playback.HardwareAPI
}

func (hardware *recordingHardware) MarkDegraded(api playback.HardwareAPI) {
	hardware.mutex.Lock()
	defer hardware.mutex.Unlock()

	hardware.degraded = append(hardware.degraded, api)
}

func (hardware *recordingHardware) marked() []playback.HardwareAPI {
	hardware.mutex.Lock()
	defer hardware.mutex.Unlock()

	return append([]playback.HardwareAPI(nil), hardware.degraded...)
}

// subtitledEnvironment adds a Matroska file with H.264, AAC, a text and a PGS track.
func subtitledEnvironment() *environment {
	env := newEnvironment()

	text := media.Stream{Index: 2, Kind: media.StreamKindSubtitle, Codec: "subrip", Language: "eng"}
	pgs := media.Stream{Index: 3, Kind: media.StreamKindSubtitle, Codec: "hdmv_pgs_subtitle", Language: "eng"}
	file := catalogFile(subtitledFileID, "movies/subtitled.mkv", media.ContainerMKV,
		h264Video(0), audio(1, "aac"), text, pgs)
	file.File.Description.BitRate = sourceBitRate
	env.files.files[subtitledFileID] = file

	return env
}

// subtitleRequest plays the subtitled file for a client that renders WebVTT.
func subtitleRequest(trackID *string) appplayback.StartRequest {
	profile := playbacktest.ChromiumDesktop()
	profile.SubtitleFormats = []string{playback.SubtitleFormatWebVTT}

	return appplayback.StartRequest{FileID: subtitledFileID, Profile: profile, SubtitleTrackID: trackID}
}

func ptr(text string) *string { return &text }

func TestStartSubtitleChoices(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		trackID    *string
		external   []playback.SubtitleTrack
		wantMode   playback.SubtitleDelivery
		wantTrack  string
		wantPlan   playback.Mode
		wantBurnIn bool
	}{
		{name: "off", trackID: ptr(""), wantMode: playback.SubtitleOff, wantPlan: playback.ModeRemux},
		{name: "sidecar", trackID: ptr(textTrackID), wantMode: playback.SubtitleSidecar, wantTrack: textTrackID, wantPlan: playback.ModeRemux},
		{
			name: "external sidecar", trackID: ptr(externalTrackID),
			external: []playback.SubtitleTrack{{ID: externalTrackID, Format: "subrip", External: true, TextBased: true}},
			wantMode: playback.SubtitleSidecar, wantTrack: externalTrackID, wantPlan: playback.ModeRemux,
		},
		{
			name: "burn-in", trackID: ptr(pgsTrackID), wantMode: playback.SubtitleBurnIn, wantTrack: pgsTrackID,
			wantPlan: playback.ModeTranscode, wantBurnIn: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := subtitledEnvironment()
			env.subtitles = fakeSubtitles{tracks: test.external}

			session := mustStart(t, env.service(t), subtitleRequest(test.trackID))

			if session.SubtitleMode != test.wantMode || session.SubtitleTrackID != test.wantTrack || session.Plan.Mode != test.wantPlan {
				t.Fatalf("mode %s track %q plan %s; want %s %q %s",
					session.SubtitleMode, session.SubtitleTrackID, session.Plan.Mode, test.wantMode, test.wantTrack, test.wantPlan)
			}

			job := env.producer.jobs()[0].job
			if gotBurnIn := job.BurnInSubtitle != nil; gotBurnIn != test.wantBurnIn {
				t.Fatalf("Job.BurnInSubtitle = %+v, want burn-in %t", job.BurnInSubtitle, test.wantBurnIn)
			}

			if test.wantBurnIn && job.BurnInSubtitle.ID != test.wantTrack {
				t.Errorf("burn-in track = %s, want %s", job.BurnInSubtitle.ID, test.wantTrack)
			}
		})
	}
}

func TestStartSubtitleTrackLookupFailure(t *testing.T) {
	t.Parallel()

	env := subtitledEnvironment()
	env.subtitles = fakeSubtitles{err: errBoom}

	_, err := env.service(t).Start(adminContext(t), subtitleRequest(nil))

	assertWrappedError(t, err, errBoom, "subtitle tracks")
}

func TestStartUnknownSubtitleTrackIsInvalid(t *testing.T) {
	t.Parallel()

	env := subtitledEnvironment()

	_, err := env.service(t).Start(adminContext(t), subtitleRequest(ptr("s:99")))

	assertWrappedError(t, err, domain.ErrInvalid, "s:99")
}

func TestStartBitrateCapAndReportedRates(t *testing.T) {
	t.Parallel()

	env := subtitledEnvironment()
	startRequest := subtitleRequest(ptr(""))
	startRequest.MaxBitrate = twoMegabits

	session := mustStart(t, env.service(t), startRequest)

	if session.Plan.Mode != playback.ModeTranscode {
		t.Fatalf("mode = %s, want transcode for a cap below the source", session.Plan.Mode)
	}

	if session.SourceBitRate != sourceBitRate || session.OutputBitRate != twoMegabits {
		t.Errorf("rates source %d output %d, want %d and %d", session.SourceBitRate, session.OutputBitRate, sourceBitRate, twoMegabits)
	}

	if job := env.producer.jobs()[0].job; job.MaxBitrate != twoMegabits || job.Encoder.Name != playback.EncoderLibx264 {
		t.Errorf("job cap %d encoder %q, want %d and %s", job.MaxBitrate, job.Encoder.Name, twoMegabits, playback.EncoderLibx264)
	}
}

func TestStartWithoutCapReportsTheSourceRateAsOutput(t *testing.T) {
	t.Parallel()

	env := subtitledEnvironment()

	session := mustStart(t, env.service(t), subtitleRequest(ptr("")))

	if session.SourceBitRate != sourceBitRate || session.OutputBitRate != sourceBitRate {
		t.Errorf("rates source %d output %d, want both %d", session.SourceBitRate, session.OutputBitRate, sourceBitRate)
	}
}

// fillTranscodeLimit starts maxTranscodes sessions that re-encode.
func fillTranscodeLimit(t *testing.T, service *appplayback.Service) {
	t.Helper()

	for range maxTranscodes {
		mustStart(t, service, request(transcodeFileID))
	}
}

func TestStartBurnInAtTheReencodingLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		allowDegrade bool
		limitReached bool
		wantErr      bool
		wantMode     playback.SubtitleDelivery
		wantWarning  bool
	}{
		{name: "limit free", wantMode: playback.SubtitleBurnIn},
		{name: "limit reached refuses", limitReached: true, wantErr: true},
		{name: "limit reached degrades", limitReached: true, allowDegrade: true, wantMode: playback.SubtitleOff, wantWarning: true},
		{name: "limit free keeps the burn-in when degrading is allowed", allowDegrade: true, wantMode: playback.SubtitleBurnIn},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := subtitledEnvironment()
			service := env.service(t)

			if test.limitReached {
				fillTranscodeLimit(t, service)
			}

			startRequest := subtitleRequest(ptr(pgsTrackID))
			startRequest.AllowDegradeSubtitles = test.allowDegrade

			session, err := service.Start(adminContext(t), startRequest)
			if test.wantErr {
				assertWrappedError(t, err, appplayback.ErrSessionLimitReached, "re-encoding sessions")

				return
			}

			if err != nil {
				t.Fatalf("Start error = %v", err)
			}

			assertSubtitleOutcome(t, session, test.wantMode, test.wantWarning)
		})
	}
}

func assertSubtitleOutcome(t *testing.T, session appplayback.Session, wantMode playback.SubtitleDelivery, wantWarning bool) {
	t.Helper()

	if session.SubtitleMode != wantMode {
		t.Errorf("SubtitleMode = %s, want %s", session.SubtitleMode, wantMode)
	}

	if got := hasSubtitleWarning(session.Plan); got != wantWarning {
		t.Errorf("subtitle_burn_in_unavailable warning = %t, want %t", got, wantWarning)
	}
}

func hasSubtitleWarning(plan playback.Plan) bool {
	for _, warning := range plan.Warnings {
		if warning.Code == playback.WarningSubtitleBurnInUnavailable {
			return true
		}
	}

	return false
}

func TestStartReplacementWithBurnIn(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		fillLimit    bool
		allowDegrade bool
		wantErr      bool
		wantOldAlive bool
		wantMode     playback.SubtitleDelivery
	}{
		// A remux session does not hold a re-encoding slot, so the burn-in needs a free one.
		{name: "limit free", wantMode: playback.SubtitleBurnIn},
		{name: "limit reached refused keeps the old session", fillLimit: true, wantErr: true, wantOldAlive: true},
		{name: "limit reached degraded replaces", fillLimit: true, allowDegrade: true, wantMode: playback.SubtitleOff},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			env := subtitledEnvironment()
			service := env.service(t)
			old := mustStart(t, service, subtitleRequest(ptr("")))

			if test.fillLimit {
				fillTranscodeLimit(t, service)
			}

			startRequest := subtitleRequest(ptr(pgsTrackID))
			startRequest.ReplacesSessionID = old.ID
			startRequest.AllowDegradeSubtitles = test.allowDegrade

			session, err := service.Start(adminContext(t), startRequest)
			if (err != nil) != test.wantErr {
				t.Fatalf("Start error = %v, want error %t", err, test.wantErr)
			}

			assertReplacedSession(t, service, old.ID, test.wantOldAlive)

			if production := env.producer.productionOf(old.ID); test.wantOldAlive && production.stopCount() != 0 {
				t.Error("the replaced session was stopped although the start failed")
			}

			if err == nil {
				assertSubtitleOutcome(t, session, test.wantMode, test.wantMode == playback.SubtitleOff)
			}
		})
	}
}

// assertReplacedSession checks that the replaced session still serves, and is not stopped, or
// is gone, depending on whether the replacing start succeeded.
func assertReplacedSession(t *testing.T, service *appplayback.Service, oldID string, wantAlive bool) {
	t.Helper()

	_, err := service.Get(adminContext(t), oldID)
	if alive := err == nil; alive != wantAlive {
		t.Errorf("old session alive = %t, want %t", alive, wantAlive)
	}
}

func TestStartReplacementOfATranscodeWithBurnInNeedsNoFreeSlot(t *testing.T) {
	t.Parallel()

	env := subtitledEnvironment()
	service := env.service(t)
	old := mustStart(t, service, subtitleRequest(ptr(pgsTrackID)))
	mustStart(t, service, request(transcodeFileID))

	startRequest := subtitleRequest(ptr(pgsTrackID))
	startRequest.ReplacesSessionID = old.ID
	startRequest.MaxBitrate = twoMegabits

	session := mustStart(t, service, startRequest)

	if session.SubtitleMode != playback.SubtitleBurnIn || session.OutputBitRate != twoMegabits {
		t.Errorf("session mode %s rate %d, want burn-in at %d", session.SubtitleMode, session.OutputBitRate, twoMegabits)
	}

	if _, err := service.Get(adminContext(t), old.ID); err == nil {
		t.Error("the replaced session is still visible")
	}
}

func hardwareEnvironment() (*environment, *recordingHardware) {
	env := subtitledEnvironment()
	env.options.Capabilities.HardwareEncoders = []playback.HardwareEncoder{
		{API: playback.HardwareAPIVAAPI, Name: "h264_vaapi", Codec: playback.TranscodeVideoCodec, Device: "/dev/dri/renderD128", Usable: true},
	}

	hardware := &recordingHardware{}
	env.hardware = hardware

	return env, hardware
}

func TestStartHardwareFallbackMarksTheEncoderDegraded(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, _ *environment) {
		env, hardware := hardwareEnvironment()
		env.producer.fallback = true
		startRequest := subtitleRequest(ptr(""))
		startRequest.MaxBitrate = twoMegabits

		mustStart(t, env.service(t), startRequest)
		env.passTime(0)

		if job := env.producer.jobs()[0].job; !job.Encoder.Hardware || job.Encoder.API != playback.HardwareAPIVAAPI {
			t.Errorf("job encoder = %+v, want the VAAPI encoder", job.Encoder)
		}

		if got := hardware.marked(); len(got) != 1 || got[0] != playback.HardwareAPIVAAPI {
			t.Errorf("degraded APIs = %v, want [vaapi]", got)
		}
	})
}

func TestStartHardwareWithoutFallbackStaysClean(t *testing.T) {
	t.Parallel()

	inBubble(t, func(t *testing.T, _ *environment) {
		env, hardware := hardwareEnvironment()
		startRequest := subtitleRequest(ptr(""))
		startRequest.MaxBitrate = twoMegabits

		mustStart(t, env.service(t), startRequest)
		env.passTime(0)

		if got := hardware.marked(); len(got) != 0 {
			t.Errorf("degraded APIs = %v, want none", got)
		}
	})
}

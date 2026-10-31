package corpustest_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// maxParallelSessions bounds the sessions that run at the same time. Each one may run an
// FFmpeg; four keep a desktop busy without starving the race detector in Docker, and stay
// below the playback service's fixed cap of eight FFmpeg sessions.
const maxParallelSessions = 4

// cataloguedCase is a generated video case together with its media file ID in the catalog.
type cataloguedCase struct {
	clip   corpusCase
	fileID string
}

// sessionSlots is a semaphore of maxParallelSessions slots shared by every subtest.
type sessionSlots chan struct{}

func newSessionSlots() sessionSlots {
	return make(sessionSlots, maxParallelSessions)
}

// hold takes a slot until the subtest and its cleanups (which stop its session) are done:
// cleanups run in reverse order, so the slot is given back last.
func (slots sessionSlots) hold(t *testing.T) {
	t.Helper()

	slots <- struct{}{}

	t.Cleanup(func() { <-slots })
}

func referenceProfiles() []playback.ClientProfile {
	return []playback.ClientProfile{playbacktest.ChromiumDesktop(), playbacktest.FirefoxDesktop()}
}

// TestCorpusVideosPlayWithTheExpectedMode is the milestone 2 promise (ADR 0025): the whole
// generated corpus is scanned with the real adapters, and every video case gets the
// expected mode for each reference profile and produces playable output. The same catalog
// then serves a server without encoders (exactly the transcode cases fail with
// transcode_unavailable) and a browser that reported HLS without any codec (no HLS plan).
func TestCorpusVideosPlayWithTheExpectedMode(t *testing.T) {
	environment := requireCorpus(t)
	requireTranscoding(t, environment.ffmpegPath)

	began := time.Now()
	cases := videoCases(t, environment.manifest)
	graph := newCorpusGraph(t, environment)
	videos := catalogue(t, cases, graph.scanCorpus(t))
	slots := newSessionSlots()

	full := graph.newPlaybackService(t, playbacktest.FullCapabilities())

	t.Run("full capabilities", func(t *testing.T) {
		playEveryVideo(t, sessionGroup{graph: graph, service: full, slots: slots}, videos)
		playSeeks(t, sessionGroup{graph: graph, service: full, slots: slots}, videos)
	})

	t.Run("no encoders", func(t *testing.T) {
		withoutEncoders := graph.newPlaybackService(t, playbacktest.NoEncoders())
		decideWithoutEncoders(t, sessionGroup{graph: graph, service: withoutEncoders, slots: slots}, videos)
	})

	t.Run("hls without codecs", func(t *testing.T) {
		decideWithoutHLSCodecs(t, sessionGroup{graph: graph, service: full, slots: slots}, videos)
	})

	assertWorkspaceEmpty(t, graph.transcodeRoot)
	t.Logf("corpus playback of %d video cases (FFmpeg %s corpus) took %s",
		len(videos), environment.manifest.FFmpegVersion, time.Since(began).Round(time.Millisecond))
}

// catalogue pairs every case with its catalogued file and fails for a case the scan missed.
func catalogue(t *testing.T, cases []corpusCase, fileIDs map[string]string) []cataloguedCase {
	t.Helper()

	videos := make([]cataloguedCase, 0, len(cases))

	for _, clip := range cases {
		fileID, found := fileIDs[clip.File]
		if !found {
			t.Fatalf("the scan did not catalogue %s (%s)", clip.File, clip.Name)
		}

		videos = append(videos, cataloguedCase{clip: clip, fileID: fileID})
	}

	return videos
}

// sessionGroup is what the subtests of one group share.
type sessionGroup struct {
	graph   *corpusGraph
	service *appplayback.Service
	slots   sessionSlots
}

// playEveryVideo plays each video case from the start with each reference profile and
// checks mode, reasons and output.
func playEveryVideo(t *testing.T, group sessionGroup, videos []cataloguedCase) {
	t.Helper()

	for _, video := range videos {
		for _, profile := range referenceProfiles() {
			want, _ := corpustest.Expected(video.clip.Name, profile.Name)
			run := sessionRun{group: group, video: video, profile: profile, want: want}

			t.Run(video.clip.Name+"/"+profile.Name, func(t *testing.T) {
				t.Parallel()
				group.slots.hold(t)

				playAndVerify(t, run)
			})
		}
	}
}

// playSeeks starts the seek clips in the middle: a remux starts at the keyframe before the
// requested position, a transcode exactly there (ADR 0024).
func playSeeks(t *testing.T, group sessionGroup, videos []cataloguedCase) {
	t.Helper()

	for _, seek := range corpustest.SeekExpectations() {
		want, _ := corpustest.Expected(seek.Case, seek.Profile)

		run := sessionRun{
			group: group, video: videoNamed(t, videos, seek.Case), profile: profileNamed(t, seek.Profile), want: want,
			start: seek.Requested, wantStart: seek.ActualStart,
		}

		t.Run(seek.Case+"/"+seek.Profile+"/from "+seek.Requested.String(), func(t *testing.T) {
			t.Parallel()
			group.slots.hold(t)

			playAndVerify(t, run)
		})
	}
}

// decideWithoutEncoders uses a server whose FFmpeg cannot encode: exactly the transcode
// cases fail with transcode_unavailable, every other case keeps its plan.
func decideWithoutEncoders(t *testing.T, group sessionGroup, videos []cataloguedCase) {
	t.Helper()

	for _, video := range videos {
		for _, profile := range referenceProfiles() {
			run := decisionRun{group: group, video: video, profile: profile}
			run.want, _ = corpustest.Expected(video.clip.Name, profile.Name)

			if run.want.Mode == playback.ModeTranscode {
				run.wantFailure = playback.FailureTranscodeUnavailable
			}

			t.Run(video.clip.Name+"/"+profile.Name, func(t *testing.T) {
				t.Parallel()
				group.slots.hold(t)

				decideOnly(t, run)
			})
		}
	}
}

// decideWithoutHLSCodecs uses a browser that offers HLS but whose codec probes all failed
// (M2-14 test r2): files it plays directly keep direct play, every other file fails with
// no_compatible_format instead of producing a stream the browser cannot play.
func decideWithoutHLSCodecs(t *testing.T, group sessionGroup, videos []cataloguedCase) {
	t.Helper()

	for _, delivery := range []playback.HLSDelivery{playback.HLSDeliveryMSE, playback.HLSDeliveryNative} {
		profile := playbacktest.ChromiumDesktop()
		profile.HLS = playback.HLSProfile{Delivery: delivery}

		for _, video := range videos {
			run := decisionRun{group: group, video: video, profile: profile}
			run.want, _ = corpustest.Expected(video.clip.Name, corpustest.ChromiumDesktop)

			if run.want.Mode != playback.ModeDirectPlay {
				run.wantFailure = playback.FailureNoCompatibleFormat
			}

			t.Run(video.clip.Name+"/hls "+string(delivery), func(t *testing.T) {
				t.Parallel()
				group.slots.hold(t)

				decideOnly(t, run)
			})
		}
	}
}

// videoNamed returns the catalogued case with the given name or fails.
func videoNamed(t *testing.T, videos []cataloguedCase, name string) cataloguedCase {
	t.Helper()

	for _, video := range videos {
		if video.clip.Name == name {
			return video
		}
	}

	t.Fatalf("corpus case %s was not generated; regenerate the corpus with `go run ./scripts/gen-testmedia`", name)

	return cataloguedCase{}
}

// profileNamed returns the reference profile with the given name or fails.
func profileNamed(t *testing.T, name string) playback.ClientProfile {
	t.Helper()

	for _, profile := range referenceProfiles() {
		if profile.Name == name {
			return profile
		}
	}

	t.Fatalf("no reference profile is named %q", name)

	return playback.ClientProfile{}
}

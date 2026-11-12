package ffmpeg_test

import (
	"testing"
	"time"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// progressPollInterval is how often the test asks for the progress of the real FFmpeg.
const progressPollInterval = 50 * time.Millisecond

// awaitProgress polls until wanted holds for the production's progress. Once the process has
// ended the last snapshot is final, so it is checked one more time and the wait ends.
func awaitProgress(
	t *testing.T, production appplayback.Production, wanted func(appplayback.Progress) bool,
) appplayback.Progress {
	t.Helper()

	reporter, reports := production.(appplayback.ProgressReporter)
	if !reports {
		t.Fatalf("%T does not implement appplayback.ProgressReporter", production)
	}

	timeout := time.After(hlsRunTimeout)

	for {
		if progress := reporter.Progress(); wanted(progress) {
			return progress
		}

		select {
		case <-production.Done():
			progress := reporter.Progress()
			if !wanted(progress) {
				t.Fatalf("FFmpeg ended with the progress %+v", progress)
			}

			return progress
		case <-timeout:
			t.Fatalf("progress = %+v after %s", reporter.Progress(), hlsRunTimeout)
		case <-time.After(progressPollInterval):
		}
	}
}

func TestProducerReportsTheSpeedOfATranscode(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	t.Parallel()

	job := corpusJob(t, tools, manifest, hlsCorpusRun{
		name: "video-ts-mpeg2-mp2-long", profile: playbacktest.ChromiumDesktop(),
		wantMode: playback.ModeTranscode, wantVideo: playback.ActionTranscode, wantAudio: playback.ActionTranscode,
	})
	production := startCorpusProduction(t, corpusProducer(t, tools), job)

	progress := awaitProgress(t, production, func(progress appplayback.Progress) bool {
		return progress.Speed > 0 && progress.OutputTime > 0
	})
	t.Logf("transcode progress: %+v", progress)

	readyOrFail(t, production)
}

func TestProducerReportsTheProgressOfARemux(t *testing.T) {
	tools, manifest := requireHLSTools(t)
	t.Parallel()

	job := corpusJob(t, tools, manifest, hlsCorpusRun{
		name: "video-mkv-h264-aac", profile: playbacktest.ChromiumDesktop(),
		wantMode: playback.ModeRemux, wantVideo: playback.ActionCopy, wantAudio: playback.ActionCopy,
	})
	production := startCorpusProduction(t, corpusProducer(t, tools), job)

	// A short remux ends within one block: the last block must still be published.
	progress := awaitProgress(t, production, func(progress appplayback.Progress) bool {
		return progress.OutputTime > 0
	})
	t.Logf("remux progress: %+v", progress)

	readyOrFail(t, production)
}

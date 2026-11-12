package ffmpeg

import (
	"log/slog"
	"path/filepath"
	"slices"
	"testing"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// fallbackReporter is what the playback service asks a production (M4-13a).
type fallbackReporter interface{ Fallback() bool }

// hardwareProducerTest is a producerTest whose job transcodes the video with NVENC and records
// the arguments of every FFmpeg run.
func hardwareProducerTest(t *testing.T, mode string, hardware bool) (producerTest, func() [][]string) {
	t.Helper()

	test := newProducerTest(t, mode)
	argumentsPath := filepath.Join(t.TempDir(), "arguments.jsonl")
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	encoder := hardwareEncoderForTest()
	test.producer.hardware = []playback.HardwareEncoder{encoder}

	video := playback.StreamDecision{
		Kind: media.StreamKindVideo, SourceIndex: 0, Action: playback.ActionTranscode,
		SourceCodec: "mpeg4", TargetCodec: "h264", Encoder: playback.EncoderLibx264,
	}

	if hardware {
		video.Encoder, video.Hardware = encoder.Name, true
	}

	test.job.Plan = playback.Plan{
		Mode: playback.ModeTranscode, Delivery: playback.DeliveryHLS,
		Streams: []playback.StreamDecision{
			video,
			{Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionCopy, TargetCodec: "aac"},
		},
	}

	return test, func() [][]string { return readRecordedCalls(t, argumentsPath) }
}

// reportedFallback asks the production what the playback service asks it.
func reportedFallback(t *testing.T, production appplayback.Production) bool {
	t.Helper()

	reporter, ok := production.(fallbackReporter)
	if !ok {
		t.Fatal("the production does not report a fallback")
	}

	return reporter.Fallback()
}

func hardwareEncoderForTest() playback.HardwareEncoder {
	return playback.HardwareEncoder{API: playback.HardwareAPINVENC, Name: nvencName, Codec: "h264", Usable: true}
}

func TestHardwareFailureBeforeTheFirstSegmentFallsBackToSoftware(t *testing.T) {
	test, calls := hardwareProducerTest(t, fakeModeHLSFailHardware, true)
	production := test.start(t.Context(), t)

	if err := readyWithin(t, production, readyDeadline); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	reporter, ok := production.(fallbackReporter)
	if !ok || !reporter.Fallback() {
		t.Fatalf("Fallback = false after the hardware run failed, want true (reporter: %v)", ok)
	}

	recorded := calls()
	if len(recorded) != 2 {
		t.Fatalf("FFmpeg ran %d times, want 2: %v", len(recorded), recorded)
	}

	if !slices.Contains(recorded[0], nvencName) || slices.Contains(recorded[0], "libx264") {
		t.Errorf("first run = %v, want the hardware encoder", recorded[0])
	}

	if !slices.Contains(recorded[1], "libx264") || slices.Contains(recorded[1], nvencName) {
		t.Errorf("second run = %v, want libx264", recorded[1])
	}

	assertFallbackLogged(t, test.logs)
}

func assertFallbackLogged(t *testing.T, logs *syncBuffer) {
	t.Helper()

	if warnings := logs.warnings(t); len(warnings) != 0 {
		t.Errorf("the retry that worked logged warnings: %v", warnings)
	}

	for _, record := range logs.records(t) {
		if record[slog.LevelKey] == "INFO" && record["playback_id"] == testSessionID {
			return
		}
	}

	t.Errorf("no info record of the software retry: %v", logs.records(t))
}

func TestWorkingHardwareRunDoesNotFallBack(t *testing.T) {
	test, calls := hardwareProducerTest(t, fakeModeHLSLive, true)
	production := test.start(t.Context(), t)

	if err := readyWithin(t, production, readyDeadline); err != nil {
		t.Fatalf("Ready: %v", err)
	}

	if reportedFallback(t, production) {
		t.Error("Fallback = true although the hardware run produced a segment")
	}

	if got := len(calls()); got != 1 {
		t.Errorf("FFmpeg ran %d times, want 1", got)
	}
}

func TestSoftwareTranscodeNeverRetries(t *testing.T) {
	test, calls := hardwareProducerTest(t, fakeModeHLSFailAlways, false)
	production := test.start(t.Context(), t)

	if err := readyWithin(t, production, readyDeadline); err == nil {
		t.Fatal("Ready succeeded for a failing software run")
	}

	if reportedFallback(t, production) {
		t.Error("Fallback = true for a software run")
	}

	if got := len(calls()); got != 1 {
		t.Errorf("FFmpeg ran %d times, want 1", got)
	}
}

func TestSoftwareRetryThatFailsToo(t *testing.T) {
	test, calls := hardwareProducerTest(t, fakeModeHLSFailAlways, true)
	production := test.start(t.Context(), t)

	if err := readyWithin(t, production, readyDeadline); err == nil {
		t.Fatal("Ready succeeded although both runs failed")
	}

	if !reportedFallback(t, production) {
		t.Error("Fallback = false although the software retry ran")
	}

	if got := len(calls()); got != 2 {
		t.Errorf("FFmpeg ran %d times, want 2 (no retry loop)", got)
	}

	if got := len(test.logs.warnings(t)); got != 1 {
		t.Errorf("warnings = %d, want the one of the failed retry", got)
	}
}

func TestStoppedHardwareRunDoesNotFallBack(t *testing.T) {
	test, _ := hardwareProducerTest(t, fakeModeHLSHeaderOnly, true)
	production := test.start(t.Context(), t)

	production.Stop()
	awaitDone(t, production)

	if reportedFallback(t, production) {
		t.Error("Fallback = true after Stop")
	}
}

func TestSoftwarePlanLeavesTheOriginalAlone(t *testing.T) {
	test, _ := hardwareProducerTest(t, fakeModeHLSLive, true)
	plan := test.job.Plan

	software := softwarePlan(plan)

	if video, _ := software.Video(); video.Hardware || video.Encoder != playback.EncoderLibx264 {
		t.Errorf("software video = %+v", video)
	}

	if video, _ := plan.Video(); !video.Hardware || video.Encoder != nvencName {
		t.Errorf("the original plan changed: %+v", video)
	}
}

package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// Case names of the corpus the end-to-end tests use; their expectations come from corpustest.
const (
	caseDirectPlay = "video-mp4-h264-aac-faststart"
	caseRemux      = "video-mkv-h264-aac"
	caseTranscode  = "video-ts-mpeg2-mp2"
	caseLongRemux  = "video-mkv-h264-aac-long"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	return parsed
}

func passingManifest(t *testing.T) string {
	t.Helper()

	return writeManifest(t,
		testManifest{name: caseDirectPlay, category: "video", file: "video/direct.mp4", videoCodec: "h264", audioCodec: "aac", durationSeconds: 2},
		testManifest{name: caseRemux, category: "video", file: "video/remux.mkv", videoCodec: "h264", audioCodec: "aac", durationSeconds: 2},
		testManifest{name: caseTranscode, category: "video", file: "video/transcode.ts", videoCodec: "mpeg2video", audioCodec: "mp2", durationSeconds: 2},
		testManifest{name: caseLongRemux, category: "video", file: "video/long.mkv", videoCodec: "h264", audioCodec: "aac", durationSeconds: 30},
		// Audio files and images are no library videos and get no case.
		testManifest{name: "audio-mp3", category: "audio", file: "audio/a.mp3", audioCodec: "mp3", durationSeconds: 2},
	)
}

func passingFiles(t *testing.T) []*fakeFile {
	t.Helper()

	long := newFakeFile("long.mkv", sessionFor(expected(t, caseLongRemux), apigen.PlaybackDeliveryHls), "LONG-INIT", "LONG-SEG")
	long.seekStart, long.seekParts = 20.021, []string{"SEEK-INIT", "SEEK-SEG"}

	return []*fakeFile{
		newFakeFile("direct.mp4", sessionFor(expected(t, caseDirectPlay), apigen.PlaybackDeliveryFile), "DIRECT-FILE-BYTES"),
		newFakeFile("remux.mkv", sessionFor(expected(t, caseRemux), apigen.PlaybackDeliveryHls), "REMUX-INIT", "REMUX-SEG0", "REMUX-SEG1"),
		newFakeFile("transcode.ts", sessionFor(expected(t, caseTranscode), apigen.PlaybackDeliveryHls), "TC-INIT", "TC-SEG"),
		long,
	}
}

func passingSummaries() map[string]outputSummary {
	transcoded := outputSummary{videoCodec: "h264", videoProfile: "High", pixelFormat: "yuv420p", audioCodec: "aac"}
	transcoded.duration = 2 * time.Second

	return map[string]outputSummary{
		"DIRECT-FILE-BYTES": {videoCodec: "h264", audioCodec: "aac", duration: 2 * time.Second},
		joined("REMUX-INIT", "REMUX-SEG0", "REMUX-SEG1"): {videoCodec: "h264", audioCodec: "aac", duration: 2020 * time.Millisecond},
		joined("TC-INIT", "TC-SEG"):                      transcoded,
		joined("LONG-INIT", "LONG-SEG"):                  {videoCodec: "h264", audioCodec: "aac", duration: 30 * time.Second},
		joined("SEEK-INIT", "SEEK-SEG"):                  {videoCodec: "h264", audioCodec: "aac", duration: 10 * time.Second},
	}
}

func TestSmokePassesAgainstAServerThatMatchesTheExpectations(t *testing.T) {
	t.Parallel()

	fake, server := newFakeServer(t, passingFiles(t)...)
	fake.sessionLimitOnce = true

	var out bytes.Buffer

	err := smoke(t.Context(), testOptions(t, server, passingManifest(t)), testDependencies(t, passingSummaries()), &out)
	if err != nil {
		t.Fatalf("smoke = %v, want nil; output:\n%s", err, out.String())
	}

	text := out.String()
	for _, want := range []string{
		"server Fake 1.2.3", "5 cases", "scanned 1 libraries, 4 file names catalogued",
		"ok   " + caseDirectPlay, "original file, range 206",
		"ok   " + caseRemux + " ", "ok   " + caseTranscode, "video transcode h264, audio transcode aac",
		"ok   " + caseLongRemux + "@21s", "starts at 20.021 s",
		"summary: 5 passed, 0 failed, 0 not run of 5 cases (profile chromium-desktop)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.sessions) != 0 || len(fake.stopped) != 5 {
		t.Errorf("open sessions = %d, stopped = %d; want 0 and 5", len(fake.sessions), len(fake.stopped))
	}
}

func TestSmokeReportsEveryMismatchAndFails(t *testing.T) {
	t.Parallel()

	files := passingFiles(t)

	// The direct-play file gets a remux plan: a fatal plan mismatch.
	files[0].session = sessionFor(expected(t, caseRemux), apigen.PlaybackDeliveryHls)
	// The remux session's FFmpeg fails.
	files[1].streamStatus, files[1].failureCode = http.StatusInternalServerError, "ffmpeg_failed"
	// The transcode case is missing from the catalog.
	files = append(files[:2], files[3])
	// The seek session starts too far from the keyframe.
	files[2].seekStart = 21.5

	fake, server := newFakeServer(t, files...)

	summaries := passingSummaries()
	summaries[joined("LONG-INIT", "LONG-SEG")] = outputSummary{videoCodec: "hevc", audioCodec: "aac", duration: 25 * time.Second}

	var out bytes.Buffer

	err := smoke(t.Context(), testOptions(t, server, passingManifest(t)), testDependencies(t, summaries), &out)
	if !errors.Is(err, errCasesFailed) {
		t.Fatalf("smoke = %v, want errCasesFailed; output:\n%s", err, out.String())
	}

	text := out.String()
	for _, want := range []string{
		"FAIL " + caseDirectPlay, "plan = remux", "reasons = [container:container_not_supported], want []",
		"FAIL " + caseRemux + " ", "500", "failure code ffmpeg_failed",
		"FAIL " + caseTranscode, "no catalogued file with this name: transcode.ts",
		"FAIL " + caseLongRemux + " ", `output codecs = video "hevc"`, "output duration = 25s, want 30s",
		"FAIL " + caseLongRemux + "@21s", "start position = 21.5s",
		"summary: 0 passed, 5 failed, 0 not run of 5 cases",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()

	if len(fake.sessions) != 0 {
		t.Errorf("%d sessions were left open", len(fake.sessions))
	}
}

func TestSmokeStopsBeforeCasesWhenTheServerCannotScan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prepare func(fake *fakeServer)
		wantErr error
	}{
		{name: "scan failed", prepare: func(fake *fakeServer) { fake.scanState = apigen.ScanStateFailed }, wantErr: errScanFailed},
		{name: "no library", prepare: func(fake *fakeServer) { fake.noLibrary = true }, wantErr: errNoLibraries},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake, server := newFakeServer(t, passingFiles(t)...)
			test.prepare(fake)

			var out bytes.Buffer

			err := smoke(t.Context(), testOptions(t, server, passingManifest(t)), testDependencies(t, passingSummaries()), &out)
			if !errors.Is(err, test.wantErr) {
				t.Errorf("smoke = %v, want %v", err, test.wantErr)
			}

			if strings.Contains(out.String(), "summary:") {
				t.Errorf("a run that could not scan printed a summary:\n%s", out.String())
			}
		})
	}
}

func TestSmokeFailsWithoutAServer(t *testing.T) {
	t.Parallel()

	_, server := newFakeServer(t)
	opts := testOptions(t, server, passingManifest(t))
	server.Close()

	if err := smoke(t.Context(), opts, testDependencies(t, nil), &bytes.Buffer{}); err == nil ||
		!strings.Contains(err.Error(), "reach the server") {
		t.Errorf("smoke = %v, want an error that names the unreachable server", err)
	}
}

func TestSmokeRejectsAnUnknownProfileAndAMissingManifest(t *testing.T) {
	t.Parallel()

	_, server := newFakeServer(t)

	unknown := testOptions(t, server, passingManifest(t))
	unknown.profile = "netscape"

	if err := smoke(t.Context(), unknown, testDependencies(t, nil), &bytes.Buffer{}); !errors.Is(err, errUnknownProfile) {
		t.Errorf("smoke with an unknown profile = %v, want errUnknownProfile", err)
	}

	missing := testOptions(t, server, "does-not-exist/manifest.json")
	if err := smoke(t.Context(), missing, testDependencies(t, nil), &bytes.Buffer{}); err == nil {
		t.Error("smoke without a manifest = nil, want an error")
	}
}

func TestPlayAllCountsCasesAfterCancellationAsNotRun(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	var out bytes.Buffer

	runner := &smokeRunner{profile: apigen.ClientProfile{Name: "chromium-desktop"}}
	err := runner.playAll(ctx, []smokeCase{{label: "a"}, {label: "b"}}, &out)
	if !errors.Is(err, errCasesFailed) || !strings.Contains(out.String(), "0 passed, 0 failed, 2 not run of 2 cases") {
		t.Errorf("playAll = %v with output %q, want errCasesFailed and two cases not run", err, out.String())
	}
}

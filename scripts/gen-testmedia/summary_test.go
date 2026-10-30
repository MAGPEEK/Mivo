package main

import (
	"errors"
	"testing"
)

func TestWriteSummaryCountsEveryCategoryAndSkipKind(t *testing.T) {
	t.Parallel()

	audio := func(name string) mediaCase {
		c := sampleCase(name, "audio/"+name+".flac")
		c.Category = categoryAudio

		return c
	}

	result := newManifest("9.0.2", []manifestEntry{
		newBuiltEntry(sampleCase("video-a", "video/a.mkv")),
		newEntry(sampleCase("video-b", "video/b.mkv"), statusFailed, "boom"),
		newSkippedEntry(sampleCase("video-c", "video/c.mkv"), skipMissingCapability, "no encoder"),
		newSkippedEntry(sampleCase("video-d", "video/d.mkv"), skipKnownGap, "gap"),
		newBuiltEntry(audio("audio-a")),
		newKeptEntry(audio("audio-d"), nil),
		newKeptEntry(audio("audio-e"), []string{"flac"}),
		newSkippedEntry(audio("audio-b"), skipNotSelected, "not selected"),
		newSkippedEntry(audio("audio-c"), skipInterrupted, interruptedReason),
	})

	var output testWriter
	if err := writeSummary(&output, result, "out/manifest.json"); err != nil {
		t.Fatalf("writeSummary: %v", err)
	}

	const want = "ffmpeg 9.0.2\n" +
		"  audio  1 generated, 2 kept, 0 skipped, 1 not selected, 0 gaps, 0 failed, 1 interrupted\n" +
		"  video  1 generated, 0 kept, 1 skipped, 0 not selected, 1 gaps, 1 failed\n" +
		"  total  2 generated, 2 kept, 1 skipped, 1 not selected, 1 gaps, 1 failed, 1 interrupted\n" +
		"manifest: out/manifest.json\n"

	if output.text != want {
		t.Errorf("summary =\n%s\nwant\n%s", output.text, want)
	}
}

func TestWriteSummaryReportsWriteErrors(t *testing.T) {
	t.Parallel()

	output := testWriter{err: errors.New("disk full")}
	if err := writeSummary(&output, newManifest("x", nil), "m"); err == nil {
		t.Error("writeSummary ignored the write error")
	}
}

// testWriter collects written text or fails with err.
type testWriter struct {
	text string
	err  error
}

func (w *testWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}

	w.text += string(p)

	return len(p), nil
}

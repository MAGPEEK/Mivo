package main

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// copySample is a case whose built file is copied to two more paths in other folders.
func copySample() mediaCase {
	c := sampleCase("shows-a", "shows/a/e1.mkv")
	c.Copies = []string{"shows/a/e2.mkv", "shows/b/Season 01/e3.mkv"}
	c.Sidecars = []sidecarFile{{Output: "shows/a/info/tvshow.nfo", Content: "nfo"}}

	return c
}

func TestGenerateWritesCopiesAndSidecarsInOtherFolders(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)

	entry := gen.generate(context.Background(), plannedCase{Case: copySample(), Action: actionBuild})
	if entry.Status != statusGenerated {
		t.Fatalf("entry = %+v", entry)
	}

	for _, file := range []string{"shows/a/e2.mkv", "shows/b/Season 01/e3.mkv"} {
		if got := readOutput(t, gen.outDir, file); got != fakeMediaContent {
			t.Errorf("%s = %q, want a copy of the built file", file, got)
		}
	}

	if got := readOutput(t, gen.outDir, "shows/a/info/tvshow.nfo"); got != "nfo" {
		t.Errorf("sidecar = %q", got)
	}

	if encodes := len(ffmpeg.encodedOutputs()); encodes != 1 {
		t.Errorf("%d encodes, want one for all copies", encodes)
	}
}

func TestGenerateRestoresOrReplacesCopies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		force     bool
		wantCopy2 string
	}{
		{name: "kept file restores only missing copies", force: false, wantCopy2: "old copy"},
		{name: "force replaces every copy", force: true, wantCopy2: fakeMediaContent},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gen, _ := newTestGenerator(t, newFixtureFFmpeg(t), test.force)
			writeFile(t, filepath.Join(gen.outDir, "shows", "a", "e1.mkv"), fakeMediaContent)
			writeFile(t, filepath.Join(gen.outDir, "shows", "a", "e2.mkv"), "old copy")

			if entry := gen.generate(context.Background(), plannedCase{Case: copySample()}); entry.Status != statusGenerated {
				t.Fatalf("entry = %+v", entry)
			}

			if got := readOutput(t, gen.outDir, "shows/a/e2.mkv"); got != test.wantCopy2 {
				t.Errorf("existing copy = %q, want %q", got, test.wantCopy2)
			}

			if got := readOutput(t, gen.outDir, "shows/b/Season 01/e3.mkv"); got != fakeMediaContent {
				t.Errorf("missing copy = %q, want it restored", got)
			}
		})
	}
}

func TestGenerateFailsWhenACopyOrSidecarCannotBeWritten(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		blocker    string
		wantReason string
	}{
		{name: "copy", blocker: "b", wantReason: "copy shows/b/Season 01/e3.mkv"},
		{name: "sidecar", blocker: "a/info", wantReason: "sidecar shows/a/info/tvshow.nfo"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			gen, _ := newTestGenerator(t, newFixtureFFmpeg(t), false)
			// A file where the folder should be makes the write fail.
			writeFile(t, filepath.Join(gen.outDir, "shows", filepath.FromSlash(test.blocker)), "not a folder")

			entry := gen.generate(context.Background(), plannedCase{Case: copySample(), Action: actionBuild})
			if entry.Status != statusFailed || !strings.Contains(entry.Reason, test.wantReason) {
				t.Errorf("entry = %+v, want a failure naming %q", entry, test.wantReason)
			}
		})
	}
}

func TestCopyFileAtomicallyReportsAMissingSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := copyFileAtomically(filepath.Join(dir, "missing"), filepath.Join(dir, "copy")); err == nil {
		t.Error("copying a missing file succeeded")
	}
}

func TestGenerateResolvesTheCoverBelowTheOutputDirectory(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)
	gen, _ := newTestGenerator(t, ffmpeg, false)
	writeFile(t, filepath.Join(gen.outDir, "movies", "poster.jpg"), "jpeg")

	c := sampleCase("movies-cover", "movies/b.mp4")
	c.Muxer = "mp4"
	c.Cover = "movies/poster.jpg"

	if entry := gen.generate(context.Background(), plannedCase{Case: c, Action: actionBuild}); entry.Status != statusGenerated {
		t.Fatalf("entry = %+v", entry)
	}

	inputs := ffmpeg.textInputs()
	if len(inputs) != 1 || inputs[0].path != filepath.Join(gen.outDir, "movies", "poster.jpg") || inputs[0].content != "jpeg" {
		t.Errorf("ffmpeg read %+v, want the cover below the output directory", inputs)
	}
}

func TestRequireCoversSkipsCasesWithoutTheirCover(t *testing.T) {
	t.Parallel()

	gen, _ := newTestGenerator(t, newFixtureFFmpeg(t), false)

	withCover := func(name, cover string) mediaCase {
		c := sampleCase(name, "movies/"+name+".mkv")
		c.Cover = cover

		return c
	}

	writeFile(t, filepath.Join(gen.outDir, "movies", "poster.jpg"), "jpeg")
	writeFile(t, filepath.Join(gen.outDir, "movies", "kept.mkv"), "old")

	plans := gen.requireCovers([]plannedCase{
		{Case: withCover("present", "movies/poster.jpg"), Action: actionBuild},
		{Case: withCover("missing", "movies/none.jpg"), Action: actionBuild},
		{Case: withCover("kept", "movies/none.jpg"), Action: actionBuild},
		{Case: withCover("unselected", "movies/none.jpg"), Action: actionKeepOnly, Reason: "not selected"},
	})

	got := make([]string, 0, len(plans))

	for _, plan := range plans {
		entry := gen.generate(context.Background(), plan)
		got = append(got, fmt.Sprintf("%s|%s|%s|%s", entry.Name, entry.progressLabel(), entry.SkipKind, entry.Reason))
	}

	want := []string{
		"present|generated||",
		"missing|skipped|missing-input|cover image movies/none.jpg was not built",
		"kept|kept||",
		"unselected|skipped|not-selected|not selected",
	}

	if !slices.Equal(got, want) {
		t.Errorf("entries =\n%q\nwant\n%q", got, want)
	}
}

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
)

func TestReadManifestRejectsBrokenFiles(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	broken := filepath.Join(directory, "broken.json")
	oldSchema := filepath.Join(directory, "old.json")

	for path, content := range map[string]string{broken: "{", oldSchema: `{"schemaVersion":1}`} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	for _, path := range []string{filepath.Join(directory, "missing.json"), broken, oldSchema} {
		if _, err := readManifest(path); err == nil {
			t.Errorf("readManifest(%s) = nil error, want an error", filepath.Base(path))
		}
	}
}

func TestSmokeCasesCoverEveryGeneratedVideo(t *testing.T) {
	t.Parallel()

	manifest, err := readManifest(passingManifest(t))
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	manifest.Cases = append(manifest.Cases,
		corpusCase{Name: "video-rmvb-rv40-cook", Category: "video", Status: "skipped", Expected: caseProperties{Video: &streamProperties{}}},
		corpusCase{Name: "image-png", Category: "image", Status: statusGenerated, Expected: caseProperties{Video: &streamProperties{}}},
	)

	cases, err := smokeCases(manifest, corpustest.ChromiumDesktop)
	if err != nil {
		t.Fatalf("smokeCases: %v", err)
	}

	labels := make([]string, 0, len(cases))
	for _, smoke := range cases {
		labels = append(labels, smoke.label)
	}

	want := []string{caseDirectPlay, caseRemux, caseTranscode, caseLongRemux, caseLongRemux + "@21s"}
	if strings.Join(labels, ",") != strings.Join(want, ",") {
		t.Errorf("cases = %q, want %q", labels, want)
	}

	seek := cases[len(cases)-1]
	if seek.start != 21*time.Second || seek.wantStart != 20*time.Second || seek.clip.fileName() != "long.mkv" {
		t.Errorf("seek case = start %s, want start %s, file %s; want 21s, 20s, long.mkv", seek.start, seek.wantStart, seek.clip.fileName())
	}

	firefox, err := smokeCases(manifest, corpustest.FirefoxDesktop)
	if err != nil || len(firefox) != len(want)-1 {
		t.Errorf("firefox cases = %d, %v; want %d without seek cases", len(firefox), err, len(want)-1)
	}
}

func TestSmokeCasesFailForAVideoWithoutExpectation(t *testing.T) {
	t.Parallel()

	manifest, err := readManifest(writeManifest(t,
		testManifest{name: "video-new-format", category: "video", file: "video/new.xyz", videoCodec: "xyz", durationSeconds: 2},
	))
	if err != nil {
		t.Fatalf("readManifest: %v", err)
	}

	if _, err := smokeCases(manifest, corpustest.ChromiumDesktop); !errors.Is(err, errMissingExpectations) ||
		!strings.Contains(err.Error(), "video-new-format") {
		t.Errorf("smokeCases = %v, want errMissingExpectations naming the case", err)
	}
}

func TestDefaultAudioCodecOfAClipWithoutAudio(t *testing.T) {
	t.Parallel()

	if codec := (corpusCase{}).defaultAudioCodec(); codec != "" {
		t.Errorf("defaultAudioCodec = %q, want empty", codec)
	}
}

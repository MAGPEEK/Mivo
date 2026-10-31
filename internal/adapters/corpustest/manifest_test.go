package corpustest_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
)

const (
	// corpusDirectory is the generated test corpus seen from this package (ADR 0021).
	corpusDirectory  = "../../../testdata/media"
	manifestFileName = "manifest.json"
	// manifestSchemaVersion is the corpus manifest version this test understands.
	manifestSchemaVersion = 2
	statusGenerated       = "generated"
)

// corpusManifest holds the fields of testdata/media/manifest.json this test needs.
// scripts/gen-testmedia is package main and cannot be imported, so the shape is repeated.
type corpusManifest struct {
	SchemaVersion int          `json:"schemaVersion"`
	FFmpegVersion string       `json:"ffmpegVersion"`
	Cases         []corpusCase `json:"cases"`
}

type corpusCase struct {
	Name     string         `json:"name"`
	Category string         `json:"category"`
	File     string         `json:"file"`
	Status   string         `json:"status"`
	Expected caseProperties `json:"expected"`
}

type caseProperties struct {
	DurationSeconds float64            `json:"durationSeconds"`
	Video           *streamProperties  `json:"video"`
	Audio           []streamProperties `json:"audio"`
}

type streamProperties struct {
	Codec string `json:"codec"`
}

// videoCategories are the manifest categories whose files are videos (ADR 0025); image
// cases also describe a video stream but are no library videos.
func videoCategories() []string {
	return []string{"video", "subtitle", "multitrack"}
}

// isGeneratedVideo reports whether the case is a generated library video, which the
// expectation table must cover.
func (c corpusCase) isGeneratedVideo() bool {
	return c.Status == statusGenerated && c.Expected.Video != nil && slices.Contains(videoCategories(), c.Category)
}

// duration is the nominal length of the clip.
func (c corpusCase) duration() time.Duration {
	return time.Duration(c.Expected.DurationSeconds * float64(time.Second))
}

// defaultAudioCodec is the codec of the first audio stream, the default track of every
// corpus case.
func (c corpusCase) defaultAudioCodec() string {
	if len(c.Expected.Audio) == 0 {
		return ""
	}

	return c.Expected.Audio[0].Codec
}

// corpusEnvironment is what the integration test needs from the machine.
type corpusEnvironment struct {
	ffmpegPath  string
	ffprobePath string
	// root is the absolute corpus directory, used as the library root.
	root     string
	manifest corpusManifest
}

// requireCorpus returns the local FFmpeg tools and the corpus manifest, or skips the test
// with the reason: under -short, without ffmpeg or ffprobe on PATH, or without a manifest.
func requireCorpus(t *testing.T) corpusEnvironment {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping the corpus playback integration test in -short mode")
	}

	ffmpegPath, ffmpegErr := exec.LookPath("ffmpeg")
	ffprobePath, ffprobeErr := exec.LookPath("ffprobe")

	if ffmpegErr != nil || ffprobeErr != nil {
		t.Skip("skipping the corpus playback integration test: ffmpeg or ffprobe is not on PATH " +
			"(install FFmpeg, see docs/development.md)")
	}

	root, err := filepath.Abs(corpusDirectory)
	if err != nil {
		t.Fatalf("resolve the corpus directory: %v", err)
	}

	return corpusEnvironment{
		ffmpegPath: ffmpegPath, ffprobePath: ffprobePath, root: root,
		manifest: readManifest(t, root),
	}
}

func readManifest(t *testing.T, root string) corpusManifest {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(root, manifestFileName)) //nolint:gosec // fixed corpus path
	if errors.Is(err, fs.ErrNotExist) {
		t.Skip("skipping the corpus playback integration test: testdata/media/manifest.json is missing " +
			"(generate it with `go run ./scripts/gen-testmedia`)")
	}

	if err != nil {
		t.Fatalf("read the corpus manifest: %v", err)
	}

	var manifest corpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("decode the corpus manifest: %v", err)
	}

	if manifest.SchemaVersion != manifestSchemaVersion {
		t.Fatalf("the corpus manifest has schema version %d, want %d; regenerate it with `go run ./scripts/gen-testmedia`",
			manifest.SchemaVersion, manifestSchemaVersion)
	}

	return manifest
}

// videoCases returns the generated video cases of the manifest the expectation table
// covers. It fails for every generated video case the table misses, so the table grows with
// the corpus (ADR 0025), and for every table entry that names no manifest case, so a typo or
// a removed case cannot hide in the table.
func videoCases(t *testing.T, manifest corpusManifest) []corpusCase {
	t.Helper()

	var cases []corpusCase

	for _, entry := range manifest.Cases {
		if !entry.isGeneratedVideo() {
			continue
		}

		if hasExpectations(t, entry.Name) {
			cases = append(cases, entry)
		}
	}

	for _, name := range corpustest.CaseNames() {
		if !slices.ContainsFunc(manifest.Cases, func(entry corpusCase) bool { return entry.Name == name }) {
			t.Errorf("corpustest/expectations.go has an entry for %s, which the corpus manifest does not list "+
				"(fix the name, or regenerate the corpus with `go run ./scripts/gen-testmedia`)", name)
		}
	}

	if len(cases) == 0 {
		t.Fatal("the corpus manifest lists no generated video case; regenerate it with `go run ./scripts/gen-testmedia`")
	}

	return cases
}

// hasExpectations reports whether the table covers the case for every reference profile and
// fails the test for each profile it misses.
func hasExpectations(t *testing.T, name string) bool {
	t.Helper()

	covered := true

	for _, profile := range referenceProfileNames() {
		if _, found := corpustest.Expected(name, profile); !found {
			t.Errorf("generated video case %s has no expectation for %s in corpustest/expectations.go", name, profile)

			covered = false
		}
	}

	return covered
}

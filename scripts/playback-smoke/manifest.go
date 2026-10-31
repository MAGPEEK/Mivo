package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
)

const (
	// manifestSchemaVersion is the corpus manifest version this tool understands.
	manifestSchemaVersion = 2
	statusGenerated       = "generated"
)

// errMissingExpectations reports generated video cases the expectation table does not cover.
var errMissingExpectations = errors.New("generated video cases without an expectation in corpustest/expectations.go")

// corpusManifest holds the fields of testdata/media/manifest.json the smoke test needs.
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

// videoCategories are the manifest categories whose files are library videos (ADR 0025);
// image cases also describe a video stream but are no videos.
func videoCategories() []string {
	return []string{"video", "subtitle", "multitrack"}
}

func (c corpusCase) isGeneratedVideo() bool {
	return c.Status == statusGenerated && c.Expected.Video != nil && slices.Contains(videoCategories(), c.Category)
}

// fileName is the name the API reports for the case's file (MediaFile.fileName).
func (c corpusCase) fileName() string {
	return path.Base(c.File)
}

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

// readManifest reads and decodes the corpus manifest.
func readManifest(manifestPath string) (corpusManifest, error) {
	data, err := os.ReadFile(manifestPath) //nolint:gosec // path of the -manifest flag of a developer tool
	if err != nil {
		return corpusManifest{}, fmt.Errorf("read the corpus manifest (generate it with `go run ./scripts/gen-testmedia`): %w", err)
	}

	var manifest corpusManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return corpusManifest{}, fmt.Errorf("decode the corpus manifest %s: %w", manifestPath, err)
	}

	if manifest.SchemaVersion != manifestSchemaVersion {
		return corpusManifest{}, fmt.Errorf("the corpus manifest %s has schema version %d, want %d; regenerate it with `go run ./scripts/gen-testmedia`",
			manifestPath, manifest.SchemaVersion, manifestSchemaVersion)
	}

	return manifest, nil
}

// smokeCase is one session the smoke test plays and checks.
type smokeCase struct {
	// label names the case in the output: the manifest case name, plus the start position
	// for a seek case.
	label string
	clip  corpusCase
	want  corpustest.Expectation
	// start is the requested position and wantStart the start the session must report;
	// both are 0 for a run from the beginning.
	start     time.Duration
	wantStart time.Duration
}

// smokeCases returns a case for every generated video of the manifest and every seek
// expectation of the profile whose clip was generated. A generated video the table misses
// is an error, so the smoke test cannot silently shrink with the corpus.
func smokeCases(manifest corpusManifest, profile string) ([]smokeCase, error) {
	var (
		cases   []smokeCase
		missing []string
	)

	for _, clip := range manifest.Cases {
		if !clip.isGeneratedVideo() {
			continue
		}

		want, found := corpustest.Expected(clip.Name, profile)
		if !found {
			missing = append(missing, clip.Name)

			continue
		}

		cases = append(cases, smokeCase{label: clip.Name, clip: clip, want: want})
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("%w (profile %s): %s", errMissingExpectations, profile, strings.Join(missing, ", "))
	}

	return append(cases, seekCases(manifest, profile)...), nil
}

func seekCases(manifest corpusManifest, profile string) []smokeCase {
	var cases []smokeCase

	for _, seek := range corpustest.SeekExpectations() {
		index := slices.IndexFunc(manifest.Cases, func(clip corpusCase) bool {
			return clip.Name == seek.Case && clip.isGeneratedVideo()
		})

		want, found := corpustest.Expected(seek.Case, seek.Profile)
		if seek.Profile != profile || index < 0 || !found {
			continue
		}

		cases = append(cases, smokeCase{
			label: fmt.Sprintf("%s@%s", seek.Case, seek.Requested),
			clip:  manifest.Cases[index], want: want,
			start: seek.Requested, wantStart: seek.ActualStart,
		})
	}

	return cases
}

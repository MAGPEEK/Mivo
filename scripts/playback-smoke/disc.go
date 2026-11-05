package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"slices"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
)

// discCategory marks the case built from the disc part of the library manifest.
const discCategory = "disc"

var errNoDiscCase = errors.New("the library manifest has no playable disc case")

// libraryDiscManifest holds the fields of the disc part of testdata/library/manifest.json
// (M3-16i) the smoke test needs; scripts/gen-testmedia cannot be imported.
type libraryDiscManifest struct {
	Discs struct {
		Items []discRow `json:"items"`
	} `json:"discs"`
}

type discRow struct {
	Path      string `json:"path"`
	ErrorCode string `json:"errorCode"`
	MainTitle struct {
		DurationSeconds float64 `json:"durationSeconds"`
	} `json:"mainTitle"`
	Streams *struct {
		Video string   `json:"video"`
		Audio []string `json:"audio"`
	} `json:"streams"`
}

// discCases returns the disc case of the profile: the clear DVD folder of the library
// manifest at manifestPath (corpustest.DiscVideoTS), played through the session API like a
// file. An empty path means no disc case, for a server that does not serve the discs.
func discCases(manifestPath, profile string) ([]smokeCase, error) {
	if manifestPath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(manifestPath) //nolint:gosec // path of the -disc-manifest flag of a developer tool
	if err != nil {
		return nil, fmt.Errorf("read the library manifest (generate it with `go run ./scripts/gen-testmedia`): %w", err)
	}

	var manifest libraryDiscManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("decode the library manifest %s: %w", manifestPath, err)
	}

	index := slices.IndexFunc(manifest.Discs.Items, func(row discRow) bool { return row.Path == corpustest.DiscVideoTS })
	if index < 0 {
		return nil, fmt.Errorf("%w: %s lists no disc %q", errNoDiscCase, manifestPath, corpustest.DiscVideoTS)
	}

	row := manifest.Discs.Items[index]

	want, found := corpustest.ExpectedDisc(row.Path, profile)
	if !found || row.ErrorCode != "" || row.Streams == nil || len(row.Streams.Audio) == 0 {
		return nil, fmt.Errorf("%w: %q (error code %q, expectation for %s found: %t)",
			errNoDiscCase, row.Path, row.ErrorCode, profile, found)
	}

	return []smokeCase{{label: discCategory + " " + row.Path, clip: row.asCase(), want: want}}, nil
}

// asCase describes the disc like a corpus file: the API names a disc folder after the folder
// that holds VIDEO_TS (MediaFile.fileName), and the output checks compare the main title's
// duration and codecs.
func (row discRow) asCase() corpusCase {
	return corpusCase{
		Name: row.Path, Category: discCategory, File: path.Dir(row.Path), Status: statusGenerated,
		Expected: caseProperties{
			DurationSeconds: row.MainTitle.DurationSeconds,
			Video:           &streamProperties{Codec: row.Streams.Video},
			Audio:           []streamProperties{{Codec: row.Streams.Audio[0]}},
		},
	}
}

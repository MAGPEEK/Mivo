package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// discLibraryName is the name the test gives the movies library of the disc corpus.
	discLibraryName = "Discs"
	// discDurationSlack allows for the encoder delay and the last frame of a probed title.
	discDurationSlack = 600 * time.Millisecond
	// videoTSLayout is the manifest layout of a DVD folder, whose title the test plays.
	videoTSLayout = "video_ts"
)

// discCorpusManifest holds the disc part of testdata/library/manifest.json (M3-16i).
type discCorpusManifest struct {
	Discs struct {
		Path  string             `json:"path"`
		Items []discManifestItem `json:"items"`
	} `json:"discs"`
}

type discManifestItem struct {
	Title     string `json:"title"`
	Path      string `json:"path"`
	Layout    string `json:"layout"`
	ErrorCode string `json:"errorCode"`
	MainTitle struct {
		DurationSeconds float64 `json:"durationSeconds"`
	} `json:"mainTitle"`
}

// TestRunServesTheDiscCorpus is the disc walking skeleton (M3-16k): a movies library of DVD
// and Blu-ray folders and an ISO image is scanned at startup with the disc ports wired; clear
// discs are movies with the main title's duration, the encrypted stand-ins are unavailable
// with their disc code, and the DVD title plays through a session to its first segment.
func TestRunServesTheDiscCorpus(t *testing.T) {
	t.Parallel()

	corpus := copyLibraryCorpus(t)
	discs := readDiscManifest(t)
	environment := isolatedEnvironment(t)
	writeConfigFile(t, environment[paths.ConfigDirEnv()], fmt.Sprintf(
		"scan:\n  watch: false\n  interval: 0\nlibraries:\n  - name: %s\n    kind: movies\n    path: '%s'\n",
		discLibraryName, filepath.Join(corpus.root, discs.Discs.Path)))

	server := startTestServer(t, environment, runHooks{})
	libraryID := awaitScannedLibraries(t, server.baseURL, 1, time.Time{})[discLibraryName].ID.String()
	movies := listItems(t, server.baseURL, libraryID)

	if len(movies) != len(discs.Discs.Items) {
		t.Errorf("disc library lists %d movies, want one per disc (%d): %v", len(movies), len(discs.Discs.Items), itemTitles(movies))
	}

	for _, want := range discs.Discs.Items {
		file := discFile(t, server.baseURL, findByTitle(t, movies, want.Title))
		assertDiscFile(t, want, file)

		if want.Layout == videoTSLayout && want.ErrorCode == "" {
			assertDiscPlays(t, server.baseURL, file)
		}
	}

	server.stop(t)
	assertNoSessionDirectories(t, environment[envPathsTranscode])
}

// readDiscManifest reads the disc part of the manifest the corpus copy came from.
func readDiscManifest(t *testing.T) discCorpusManifest {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(libraryCorpusDirectory, libraryManifestFile))
	if err != nil {
		t.Fatalf("read the library manifest: %v", err)
	}

	var manifest discCorpusManifest
	if err := json.Unmarshal(content, &manifest); err != nil || len(manifest.Discs.Items) == 0 {
		t.Fatalf("library manifest has no discs (error %v); regenerate it with `go run ./scripts/gen-testmedia`", err)
	}

	return manifest
}

// discFile returns the one media file of a disc movie.
func discFile(t *testing.T, baseURL string, movie apigen.ItemSummary) apigen.MediaFile {
	t.Helper()

	var detail apigen.ItemDetail

	getJSON(t, baseURL+"/api/v1/items/"+movie.ID.String(), &detail)

	if len(detail.Files) != 1 {
		t.Fatalf("disc movie %q has %d files, want 1", movie.Title, len(detail.Files))
	}

	return detail.Files[0]
}

// assertDiscFile checks a clear disc for the main title's duration and an encrypted stand-in
// for its disc error code.
func assertDiscFile(t *testing.T, want discManifestItem, file apigen.MediaFile) {
	t.Helper()

	if want.ErrorCode != "" {
		if file.Available || file.ErrorCode == nil || *file.ErrorCode != want.ErrorCode {
			t.Errorf("disc %q: available %t, error code %v, want unavailable with %q",
				want.Path, file.Available, derefText(file.ErrorCode), want.ErrorCode)
		}

		return
	}

	if !file.Available || file.DurationSeconds == nil {
		t.Errorf("disc %q: available %t, error code %v, want an available title with a duration",
			want.Path, file.Available, derefText(file.ErrorCode))

		return
	}

	wantDuration := time.Duration(want.MainTitle.DurationSeconds * float64(time.Second))
	duration := time.Duration(*file.DurationSeconds * float64(time.Second))

	if (duration - wantDuration).Abs() > discDurationSlack {
		t.Errorf("disc %q lasts %s, want %s", want.Path, duration, wantDuration)
	}
}

// assertDiscPlays starts a session for a disc and fetches its first segment: a disc is never
// played directly, so its MPEG-2 and AC-3 are transcoded for the H.264/AAC test profile.
func assertDiscPlays(t *testing.T, baseURL string, file apigen.MediaFile) {
	t.Helper()

	session := createSession(t, baseURL, file.ID.String(), http.StatusCreated)
	if session.Mode != apigen.PlaybackModeTranscode {
		t.Fatalf("disc session mode = %s, want transcode", session.Mode)
	}

	playlist := fetchBody(t, baseURL+session.StreamURL, http.StatusOK)
	if !strings.Contains(playlist, "seg-00000.m4s") {
		t.Fatalf("disc playlist lacks the first segment:\n%s", playlist)
	}

	fetchBody(t, strings.TrimSuffix(baseURL+session.StreamURL, "index.m3u8")+"seg-00000.m4s", http.StatusOK)
}

func derefText(value *string) string {
	if value == nil {
		return "<nil>"
	}

	return *value
}

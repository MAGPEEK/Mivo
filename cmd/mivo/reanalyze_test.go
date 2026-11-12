package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	// reanalyzedSeconds is the length of the clip that replaces the one the scan probed.
	reanalyzedSeconds = "3"
	// secondsTolerance absorbs the container rounding of the clip length.
	secondsTolerance = 0.5
)

// writeTestMovieOfLength is writeTestMovie with a chosen length.
func writeTestMovieOfLength(t *testing.T, ffmpegPath, target, seconds string) {
	t.Helper()

	source := fmt.Sprintf("testsrc=duration=%s:size=%s:rate=%s", seconds, testVideoSize, testVideoRate)
	command := exec.CommandContext( //nolint:gosec // G204: a test helper with a looked-up tool
		t.Context(), ffmpegPath, "-v", "error", "-f", "lavfi", "-i", source,
		"-c:v", "mpeg4", "-an", "-y", target)

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("make test clip %s: %v: %s", target, err, output)
	}
}

// primaryFileOf reads the item and returns its first media file.
func primaryFileOf(t *testing.T, baseURL, itemID string) apigen.MediaFile {
	t.Helper()

	var detail apigen.ItemDetail

	getJSON(t, baseURL+"/api/v1/items/"+itemID, &detail)

	if len(detail.Files) == 0 {
		t.Fatalf("item %s has no files", itemID)
	}

	return detail.Files[0]
}

// awaitFile polls the item until its primary file satisfies done.
func awaitFile(t *testing.T, baseURL, itemID string, done func(apigen.MediaFile) bool) apigen.MediaFile {
	t.Helper()

	deadline := time.Now().Add(scanTimeout)
	file := primaryFileOf(t, baseURL, itemID)

	for !done(file) && time.Now().Before(deadline) {
		time.Sleep(managementPollInterval)

		file = primaryFileOf(t, baseURL, itemID)
	}

	if !done(file) {
		t.Fatalf("file of item %s did not reach the expected state within %s: %+v", itemID, scanTimeout, file)
	}

	return file
}

func TestReanalyzeItemProbesAChangedFileAndMarksAVanishedOneUnavailable(t *testing.T) {
	t.Parallel()

	ffmpegPath := requireFFmpegForManagement(t)
	fixture := newManagementEnvironment(t, nil)
	movie := filepath.Join(fixture.browseRoot, "movies-a", "Alpha (2020).mp4")
	writeTestMovie(t, ffmpegPath, movie)
	fixture.start(t, runHooks{})

	baseURL := fixture.server.baseURL
	root, _ := listRoots(t, baseURL)
	created := createMoviesLibrary(t, baseURL, root, "movies-a")
	awaitLibraryItems(t, baseURL, created.ID.String(), []string{"Alpha"})

	itemID := listItems(t, baseURL, created.ID.String())[0].ID.String()
	reanalyzeURL := baseURL + "/api/v1/items/" + itemID + "/reanalyze"

	// The file changed on disk: the re-analysis reads the new length, the item stays.
	writeTestMovieOfLength(t, ffmpegPath, movie, reanalyzedSeconds)

	if status, body := call(t, http.MethodPost, reanalyzeURL, ""); status != http.StatusAccepted {
		t.Fatalf("POST reanalyze = %d (%s), want 202", status, body)
	}

	awaitFile(t, baseURL, itemID, func(file apigen.MediaFile) bool {
		return file.DurationSeconds != nil && *file.DurationSeconds > 1+secondsTolerance
	})

	// The file vanished: the item stays and its file is unavailable.
	if err := os.Remove(movie); err != nil {
		t.Fatalf("remove clip: %v", err)
	}

	awaitReanalysisAccepted(t, reanalyzeURL)

	file := awaitFile(t, baseURL, itemID, func(file apigen.MediaFile) bool { return !file.Available })
	if file.ErrorCode == nil || *file.ErrorCode != "probe_failed" {
		t.Errorf("error code = %v, want probe_failed", file.ErrorCode)
	}

	if status, _ := call(t, http.MethodPost, baseURL+"/api/v1/items/00000000-0000-4000-8000-000000000000/reanalyze", ""); status != http.StatusNotFound {
		t.Errorf("POST reanalyze of an unknown item = %d, want 404", status)
	}

	fixture.server.stop(t)
}

// awaitReanalysisAccepted posts until the request is accepted: the earlier run may still be
// finishing, which answers 409.
func awaitReanalysisAccepted(t *testing.T, address string) {
	t.Helper()

	deadline := time.Now().Add(scanTimeout)

	for time.Now().Before(deadline) {
		status, body := call(t, http.MethodPost, address, "")
		if status == http.StatusAccepted {
			return
		}

		if status != http.StatusConflict {
			t.Fatalf("POST reanalyze = %d (%s), want 202", status, body)
		}

		time.Sleep(managementPollInterval)
	}

	t.Fatalf("POST reanalyze was not accepted within %s", scanTimeout)
}

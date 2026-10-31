package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	// corpusDirectory is the generated test corpus seen from this package (ADR 0021).
	corpusDirectory = "../../testdata/media"
	// corpusDirectPlayClip is H.264/AAC in MP4, which the test profile plays directly.
	corpusDirectPlayClip = "video/h264_aac_faststart.mp4"
	// corpusRemuxClip is H.264/AAC in Matroska, which the test profile only plays as HLS.
	corpusRemuxClip = "video/h264_aac.mkv"

	// testProfile plays H.264/AAC in MP4 and over HLS, like every desktop browser, but no
	// Matroska.
	testProfile = `{"name":"cmd-test","directPlay":[{"container":"mp4","videoCodecs":["h264"],"audioCodecs":["aac"]}],` +
		`"hls":{"delivery":"mse","videoCodecs":["h264"],"audioCodecs":["aac"]}}`

	// rangeLength is the byte range the direct-play check requests.
	rangeLength = 100
	// maxStreamBytes bounds what the test reads of one stream response.
	maxStreamBytes = 64 << 20
)

// TestRunScansAndPlaysTheCorpus is the walking skeleton of milestone 2: a configured library
// is scanned at startup and listed, an MP4 plays directly with Range support, an MKV is
// remuxed to HLS, and the shutdown removes every session directory.
func TestRunScansAndPlaysTheCorpus(t *testing.T) {
	t.Parallel()

	libraryRoot := corpusLibrary(t)
	environment := isolatedEnvironment(t)
	environment[envLibraries] = libraryRoot
	environment[envLogLevel] = "info"

	server := startTestServer(t, environment, runHooks{})
	files := scannedFiles(t, server.baseURL)

	direct := createSession(t, server.baseURL, files[filepath.Base(corpusDirectPlayClip)], http.StatusCreated)
	if direct.Mode != apigen.PlaybackModeDirectPlay {
		t.Fatalf("MP4 session mode = %s, want direct_play", direct.Mode)
	}

	assertRangeServed(t, server.baseURL+direct.StreamURL)

	remux := createSession(t, server.baseURL, files[filepath.Base(corpusRemuxClip)], http.StatusCreated)
	if remux.Mode != apigen.PlaybackModeRemux {
		t.Fatalf("MKV session mode = %s, want remux", remux.Mode)
	}

	playlist := fetchBody(t, server.baseURL+remux.StreamURL, http.StatusOK)
	if !strings.Contains(playlist, "seg-00000.m4s") {
		t.Fatalf("playlist lacks the first segment:\n%s", playlist)
	}

	fetchBody(t, strings.TrimSuffix(server.baseURL+remux.StreamURL, "index.m3u8")+"seg-00000.m4s", http.StatusOK)

	// The remux session keeps running: the shutdown must stop its FFmpeg and remove its
	// directory.
	stopStarted := time.Now()
	server.stop(t)

	if took := time.Since(stopStarted); took > shutdownBudget {
		t.Errorf("shutdown took %s, want at most %s", took, shutdownBudget)
	}

	assertNoSessionDirectories(t, environment[envPathsTranscode])
	assertStreamRequestsQuiet(t, server.stderr.String(), direct.ID.String(), remux.ID.String())
}

// TestRunWithoutFFmpegRefusesRemux: with ffprobe but no ffmpeg the server scans and plays
// directly, while a file that needs HLS answers transcode_unavailable.
func TestRunWithoutFFmpegRefusesRemux(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	environment[envLibraries] = corpusLibrary(t)
	environment[envToolsFFmpeg] = missingToolPath(t, "ffmpeg")

	server := startTestServer(t, environment, runHooks{})
	files := scannedFiles(t, server.baseURL)

	direct := createSession(t, server.baseURL, files[filepath.Base(corpusDirectPlayClip)], http.StatusCreated)
	assertRangeServed(t, server.baseURL+direct.StreamURL)

	problem := createProblem(t, server.baseURL, files[filepath.Base(corpusRemuxClip)])
	if problem.Status != http.StatusUnprocessableEntity || problem.Code != "transcode_unavailable" {
		t.Errorf("MKV session problem = %+v, want 422 transcode_unavailable", problem)
	}

	server.stop(t)
}

// corpusLibrary copies the two corpus clips into a fresh library folder, or skips the test
// when FFmpeg or the corpus is missing.
func corpusLibrary(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping FFmpeg integration test: %s is not on PATH (see docs/development.md)", tool)
		}
	}

	corpus, err := os.OpenRoot(corpusDirectory)
	if err != nil {
		t.Skipf("skipping FFmpeg corpus test: %s is missing (run `go run ./scripts/gen-testmedia`)", corpusDirectory)
	}

	defer func() { _ = corpus.Close() }()

	library, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatalf("open library root: %v", err)
	}

	defer func() { _ = library.Close() }()

	for _, clip := range []string{corpusDirectPlayClip, corpusRemuxClip} {
		content, err := corpus.ReadFile(clip)
		if err != nil {
			t.Skipf("skipping FFmpeg corpus test: %s is missing (run `go run ./scripts/gen-testmedia`)", clip)
		}

		if err := library.WriteFile(path.Base(clip), content, 0o600); err != nil {
			t.Fatalf("copy %s: %v", clip, err)
		}
	}

	return library.Name()
}

// scannedFiles waits for the startup scan and returns the media file IDs by file name.
func scannedFiles(t *testing.T, baseURL string) map[string]string {
	t.Helper()

	scanned := awaitScannedLibrary(t, baseURL)
	if scanned.Scan.ErrorCode != nil || scanned.Scan.Counts.Added != 2 {
		t.Fatalf("scan = %+v, want two added files without an error", scanned.Scan)
	}

	var page apigen.ItemPage

	getJSON(t, baseURL+"/api/v1/libraries/"+scanned.ID.String()+"/items", &page)

	files := map[string]string{}

	for _, item := range page.Items {
		var detail apigen.ItemDetail

		getJSON(t, baseURL+"/api/v1/items/"+item.ID.String(), &detail)

		for _, file := range detail.Files {
			files[file.FileName] = file.ID.String()
		}
	}

	if len(files) != 2 {
		t.Fatalf("catalogued files = %v, want both clips", files)
	}

	return files
}

func createSession(t *testing.T, baseURL, fileID string, wantStatus int) apigen.PlaybackSession {
	t.Helper()

	status, body := postPlaybackRequest(t, baseURL, fileID)
	if status != wantStatus {
		t.Fatalf("POST session for %s = %d, want %d: %s", fileID, status, wantStatus, body)
	}

	var session apigen.PlaybackSession
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("decode session %s: %v", body, err)
	}

	return session
}

func createProblem(t *testing.T, baseURL, fileID string) apigen.Problem {
	t.Helper()

	_, body := postPlaybackRequest(t, baseURL, fileID)

	var problem apigen.Problem
	if err := json.Unmarshal(body, &problem); err != nil {
		t.Fatalf("decode problem %s: %v", body, err)
	}

	return problem
}

func postPlaybackRequest(t *testing.T, baseURL, fileID string) (int, []byte) {
	t.Helper()

	payload := `{"fileId":"` + fileID + `","profile":` + testProfile + `}`

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+"/api/v1/playback/sessions",
		bytes.NewReader([]byte(payload)))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST session: %v", err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if err != nil {
		t.Fatalf("read session response: %v", err)
	}

	return response.StatusCode, body
}

// assertRangeServed requests the first bytes of a direct-play stream like a video element.
func assertRangeServed(t *testing.T, url string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Range", "bytes=0-99")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	if response.StatusCode != http.StatusPartialContent || len(body) != rangeLength ||
		!strings.HasPrefix(response.Header.Get("Content-Range"), "bytes 0-99/") || response.Header.Get("Content-Type") != "video/mp4" {
		t.Errorf("GET %s with Range = %d %q %q and %d bytes, want 206 video/mp4 with %d bytes",
			url, response.StatusCode, response.Header.Get("Content-Type"), response.Header.Get("Content-Range"), len(body), rangeLength)
	}
}

func fetchBody(t *testing.T, url string, wantStatus int) string {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	if response.StatusCode != wantStatus || len(body) == 0 {
		t.Fatalf("GET %s = %d with %d bytes, want %d with a body", url, response.StatusCode, len(body), wantStatus)
	}

	return string(body)
}

// assertNoSessionDirectories checks that the shutdown left <transcode>/sessions empty.
func assertNoSessionDirectories(t *testing.T, transcodeDir string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(transcodeDir, "sessions"))
	if err != nil {
		t.Fatalf("read sessions directory: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("sessions directory holds %d entries after shutdown, want none", len(entries))
	}
}

// assertStreamRequestsQuiet checks that successful stream requests stay out of the info log
// (ADR 0024) while the session starts are recorded there.
func assertStreamRequestsQuiet(t *testing.T, logs string, sessionIDs ...string) {
	t.Helper()

	for _, id := range sessionIDs {
		assertLogLine(t, logs, "level=INFO", `msg="playback session started"`, "session="+id)

		if strings.Contains(logs, "path=/api/v1/playback/sessions/"+id+"/") {
			t.Errorf("a stream request of session %s reached the info log:\n%s", id, logs)
		}
	}
}

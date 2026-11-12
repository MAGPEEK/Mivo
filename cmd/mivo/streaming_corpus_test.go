package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// The streaming checkpoint of milestone 4 (M4-15): the real binary over the test corpus, with
// the subtitle, track, bit rate and transcoder behaviour of ADR 0030.

const (
	// streamingProfile plays H.264/AAC in MP4 and Matroska, over HLS, and renders WebVTT
	// subtitles itself, like a desktop browser with a Matroska-capable player.
	streamingProfile = `{"name":"cmd-streaming","directPlay":[` +
		`{"container":"mp4","videoCodecs":["h264"],"audioCodecs":["aac"]},` +
		`{"container":"mkv","videoCodecs":["h264"],"audioCodecs":["aac"]}],` +
		`"hls":{"delivery":"mse","videoCodecs":["h264"],"audioCodecs":["aac"]},"subtitleFormats":["webvtt"]}`

	subsLibraryDirectory = "../../testdata/library/subs"
	subsMovieFile        = "Movie (2020).mkv"
	pgsClip              = "tracks/h264_aac_pgs.mkv"
	multiAudioClip       = "multitrack/h264_aac2_srt2.mkv"
	hiBitrateClipName    = "hibitrate.mp4"
	longClipName         = "long.mp4"

	// capBitRate is the bit rate cap of the client in bits per second (4 Mbit/s).
	capBitRate = 4_000_000
	// hiBitrate is the bit rate the generated clip is encoded at, well above the cap.
	hiBitrate = "8M"
	// longClipSeconds is the length of the clip the throttle test needs: far more segments than
	// the lead the throttle allows.
	longClipSeconds = 240

	// pauseWait bounds the wait for the throttle to pause a session.
	pauseWait = 60 * time.Second
)

type streamingServer struct {
	testServer
	environment map[string]string
	files       map[string]apigen.MediaFile
}

// streamingLibrary builds a library folder from the subtitle corpus, the PGS and multi-audio
// clips, and clips ffmpeg generates for the bit rate and throttle cases; it skips the test
// without FFmpeg or the corpus.
func streamingLibrary(t *testing.T, generated ...string) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping FFmpeg integration test: %s is not on PATH (see docs/development.md)", tool)
		}
	}

	if _, err := os.Stat(filepath.Join(subsLibraryDirectory, subsMovieFile)); err != nil {
		t.Skipf("skipping streaming corpus test: %s is missing (run `go run ./scripts/gen-testmedia`)", subsLibraryDirectory)
	}

	library := t.TempDir()
	copyTree(t, subsLibraryDirectory, library)

	for _, clip := range []string{pgsClip, multiAudioClip} {
		copyFile(t, filepath.Join(corpusDirectory, filepath.FromSlash(clip)), filepath.Join(library, filepath.Base(clip)))
	}

	for _, name := range generated {
		generateClip(t, filepath.Join(library, name), name)
	}

	return library
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()

	err := filepath.WalkDir(from, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err //nolint:wrapcheck // test helper
		}

		relative, relErr := filepath.Rel(from, path)
		if relErr != nil {
			return relErr //nolint:wrapcheck // test helper
		}

		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(to, relative), 0o750) //nolint:wrapcheck // test helper
		}

		copyFile(t, path, filepath.Join(to, relative))

		return nil
	})
	if err != nil {
		t.Fatalf("copy %s: %v", from, err)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()

	content, err := os.ReadFile(from) //nolint:gosec // test helper reading corpus paths it built
	if err != nil {
		t.Skipf("skipping streaming corpus test: %s is missing (run `go run ./scripts/gen-testmedia`)", from)
	}

	if err := os.WriteFile(to, content, 0o600); err != nil { //nolint:gosec // test helper writing into a temp library
		t.Fatalf("write %s: %v", to, err)
	}
}

// generateClip writes a test clip with ffmpeg: a constant 8 Mbit/s H.264/AAC clip of two
// seconds (a source above the 4 Mbit/s cap) or a long, small one (the throttle needs far more
// segments than its lead).
func generateClip(t *testing.T, path, name string) {
	t.Helper()

	arguments := []string{
		"-hide_banner", "-loglevel", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
	}

	switch name {
	case hiBitrateClipName:
		arguments = append(arguments, "-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-b:v", hiBitrate, "-minrate", hiBitrate,
			"-maxrate", hiBitrate, "-bufsize", hiBitrate, "-x264-params", "nal-hrd=cbr:force-cfr=1", "-pix_fmt", "yuv420p")
	default:
		arguments = []string{
			"-hide_banner", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
			"-t", fmt.Sprint(longClipSeconds), "-c:v", "libx264", "-preset", "ultrafast", "-g", "50", "-pix_fmt", "yuv420p",
		}
	}

	arguments = append(arguments, "-c:a", "aac", "-movflags", "+faststart", path)

	if output, err := exec.CommandContext(t.Context(), "ffmpeg", arguments...).CombinedOutput(); err != nil {
		t.Fatalf("generate %s: %v\n%s", name, err, output)
	}
}

// startStreamingServer starts a server over the library and waits for its scan.
func startStreamingServer(t *testing.T, library string, extra map[string]string) streamingServer {
	t.Helper()

	environment := isolatedEnvironment(t)
	environment[envLibraries] = library

	for key, value := range extra {
		environment[key] = value
	}

	server := streamingServer{testServer: startTestServer(t, environment, runHooks{}), environment: environment}
	scanned := awaitScannedLibrary(t, server.baseURL)

	if scanned.Scan.ErrorCode != nil {
		t.Fatalf("scan = %+v, want no error", scanned.Scan)
	}

	server.files = libraryMediaFiles(t, server.baseURL, scanned.ID.String())

	return server
}

// libraryMediaFiles returns the media files of a library by file name.
func libraryMediaFiles(t *testing.T, baseURL, libraryID string) map[string]apigen.MediaFile {
	t.Helper()

	var page apigen.ItemPage

	getJSON(t, baseURL+"/api/v1/libraries/"+libraryID+"/items?limit=200", &page)

	files := map[string]apigen.MediaFile{}

	for _, item := range page.Items {
		var detail apigen.ItemDetail

		getJSON(t, baseURL+"/api/v1/items/"+item.ID.String(), &detail)

		for _, file := range detail.Files {
			files[file.FileName] = file
		}
	}

	return files
}

func (server streamingServer) file(t *testing.T, name string) apigen.MediaFile {
	t.Helper()

	file, found := server.files[name]
	if !found {
		t.Fatalf("catalogued files = %v, want %s", fileNames(server.files), name)
	}

	return file
}

func fileNames(files map[string]apigen.MediaFile) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}

	return names
}

// sessionOptions are the optional fields of a playback request.
type sessionOptions struct {
	subtitleTrackID  *string
	audioStreamIndex *int
	maxBitRate       int64
}

func (server streamingServer) startSession(t *testing.T, fileID string, options sessionOptions) (int, []byte) {
	t.Helper()

	request := map[string]any{"fileId": fileID, "profile": json.RawMessage(streamingProfile)}

	if options.subtitleTrackID != nil {
		request["subtitleTrackId"] = *options.subtitleTrackID
	}

	if options.audioStreamIndex != nil {
		request["audioStreamIndex"] = *options.audioStreamIndex
	}

	if options.maxBitRate > 0 {
		request["maxBitRate"] = options.maxBitRate
	}

	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	return postJSON(t, server.baseURL+"/api/v1/playback/sessions", payload)
}

func (server streamingServer) mustStartSession(t *testing.T, fileID string, options sessionOptions) apigen.PlaybackSession {
	t.Helper()

	status, body := server.startSession(t, fileID, options)
	if status != http.StatusCreated {
		t.Fatalf("POST session = %d, want 201: %s", status, body)
	}

	var session apigen.PlaybackSession
	if err := json.Unmarshal(body, &session); err != nil {
		t.Fatalf("decode session %s: %v", body, err)
	}

	return session
}

func postJSON(t *testing.T, url string, payload []byte) (int, []byte) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	return response.StatusCode, body
}

func hasReason(reasons []apigen.PlaybackReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}

	return false
}

func trackOf(t *testing.T, file apigen.MediaFile, match func(apigen.SubtitleTrack) bool) apigen.SubtitleTrack {
	t.Helper()

	if file.SubtitleTracks != nil {
		for _, track := range *file.SubtitleTracks {
			if match(track) {
				return track
			}
		}
	}

	t.Fatalf("file %s has no matching subtitle track in %+v", file.FileName, file.SubtitleTracks)

	return apigen.SubtitleTrack{}
}

func segmentURL(playlistURL string) string {
	return strings.TrimSuffix(playlistURL, "index.m3u8") + "seg-00000.m4s"
}

// TestStreamingSidecarSubtitleIsDirectPlayWithAWebVTTDownload: (a) an SRT next to the movie is
// scanned, listed as a track and served as WebVTT while the video plays directly.
func TestStreamingSidecarSubtitleIsDirectPlayWithAWebVTTDownload(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t), nil)
	movie := server.file(t, subsMovieFile)

	english := trackOf(t, movie, func(track apigen.SubtitleTrack) bool {
		return track.External && track.Language != nil && *track.Language == "en" && track.TextBased
	})

	session := server.mustStartSession(t, movie.ID.String(), sessionOptions{subtitleTrackID: &english.ID})

	if session.Mode != apigen.PlaybackModeDirectPlay || session.Subtitle == nil ||
		session.Subtitle.Mode != apigen.SubtitleDeliveryModeSidecar {
		t.Fatalf("session = mode %s subtitle %+v, want direct play with a sidecar", session.Mode, session.Subtitle)
	}

	assertRangeServedAs(t, server.baseURL+session.StreamURL, "video/x-matroska")

	body := fetchBody(t, server.baseURL+"/api/v1/media-files/"+movie.ID.String()+"/subtitles/"+english.ID+".vtt", http.StatusOK)
	if !strings.HasPrefix(body, "WEBVTT") || !strings.Contains(body, "-->") {
		t.Errorf("subtitle download is no WebVTT with cues:\n%.200s", body)
	}

	// A second request is served from the cache with the same content.
	if again := fetchBody(t, server.baseURL+"/api/v1/media-files/"+movie.ID.String()+"/subtitles/"+english.ID+".vtt", http.StatusOK); again != body {
		t.Error("the cached subtitle differs from the first conversion")
	}

	for _, language := range []string{"de", "fr", "es"} {
		trackOf(t, movie, func(track apigen.SubtitleTrack) bool {
			return track.External && track.Language != nil && *track.Language == language
		})
	}

	server.stop(t)
}

// assertRangeServedAs is assertRangeServed for another content type.
func assertRangeServedAs(t *testing.T, url, contentType string) {
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

	body, _ := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if response.StatusCode != http.StatusPartialContent || len(body) != rangeLength || response.Header.Get("Content-Type") != contentType {
		t.Errorf("GET %s with Range = %d %q and %d bytes, want 206 %s with %d bytes",
			url, response.StatusCode, response.Header.Get("Content-Type"), len(body), contentType, rangeLength)
	}
}

// TestStreamingPGSSubtitleIsBurnedIn: (b) a picture subtitle forces a transcode with the
// reason subtitle_burn_in, and FFmpeg produces the first segment with the overlay.
func TestStreamingPGSSubtitleIsBurnedIn(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t), nil)
	clip := server.file(t, filepath.Base(pgsClip))

	pgs := trackOf(t, clip, func(track apigen.SubtitleTrack) bool { return !track.TextBased && !track.External })

	session := server.mustStartSession(t, clip.ID.String(), sessionOptions{subtitleTrackID: &pgs.ID})

	if session.Mode != apigen.PlaybackModeTranscode || !hasReason(session.Reasons, "subtitle_burn_in") ||
		session.Subtitle == nil || session.Subtitle.Mode != apigen.SubtitleDeliveryModeBurnIn {
		t.Fatalf("session = mode %s reasons %+v subtitle %+v, want a transcode with subtitle_burn_in", session.Mode, session.Reasons, session.Subtitle)
	}

	fetchBody(t, server.baseURL+session.StreamURL, http.StatusOK)
	fetchBody(t, segmentURL(server.baseURL+session.StreamURL), http.StatusOK)

	server.stop(t)
}

// TestStreamingTextBurnInIsRefusedUntilTheHardenedPathExists: the producer burns in picture
// subtitles only, so a text track the client cannot render answers transcode_unavailable, or
// degrades to no subtitle when the request allows it (M4-15 safe default).
func TestStreamingTextBurnInIsRefusedUntilTheHardenedPathExists(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t), nil)
	movie := server.file(t, subsMovieFile)

	english := trackOf(t, movie, func(track apigen.SubtitleTrack) bool {
		return track.External && track.Language != nil && *track.Language == "en"
	})

	// A profile that does not render WebVTT would need the text burned in.
	payload := fmt.Sprintf(`{"fileId":%q,"subtitleTrackId":%q,"profile":{"name":"no-subs","directPlay":[`+
		`{"container":"mkv","videoCodecs":["h264"],"audioCodecs":["aac"]}],"hls":{"delivery":"mse","videoCodecs":["h264"],"audioCodecs":["aac"]}}}`,
		movie.ID.String(), english.ID)

	status, body := postJSON(t, server.baseURL+"/api/v1/playback/sessions", []byte(payload))
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(body), "transcode_unavailable") {
		t.Errorf("text burn-in = %d %s, want 422 transcode_unavailable", status, body)
	}

	degrading := strings.Replace(payload, `"subtitleTrackId"`, `"allowDegradeSubtitles":true,"subtitleTrackId"`, 1)

	status, body = postJSON(t, server.baseURL+"/api/v1/playback/sessions", []byte(degrading))
	if status != http.StatusCreated || !strings.Contains(string(body), "subtitle_burn_in_unavailable") {
		t.Errorf("degrading text burn-in = %d %s, want 201 with subtitle_burn_in_unavailable", status, body)
	}

	server.stop(t)
}

// TestStreamingSecondAudioTrackIsPlayed: (c) audioStreamIndex selects the second audio stream
// and FFmpeg maps exactly it.
func TestStreamingSecondAudioTrackIsPlayed(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t), nil)
	clip := server.file(t, filepath.Base(multiAudioClip))

	var audio []int

	for _, stream := range clip.Streams {
		if stream.Kind == apigen.MediaStreamKindAudio {
			audio = append(audio, int(stream.Index))
		}
	}

	if len(audio) < 2 {
		t.Fatalf("audio streams = %v, want two", audio)
	}

	session := server.mustStartSession(t, clip.ID.String(), sessionOptions{audioStreamIndex: &audio[1]})

	var selected *apigen.StreamDecision

	for index := range session.Streams {
		if session.Streams[index].Kind == apigen.MediaStreamKindAudio {
			selected = &session.Streams[index]
		}
	}

	if selected == nil || int(selected.SourceIndex) != audio[1] || session.Mode == apigen.PlaybackModeDirectPlay {
		t.Fatalf("session = mode %s streams %+v, want audio stream %d through HLS", session.Mode, session.Streams, audio[1])
	}

	fetchBody(t, server.baseURL+session.StreamURL, http.StatusOK)
	fetchBody(t, segmentURL(server.baseURL+session.StreamURL), http.StatusOK)

	server.stop(t)
}

// TestStreamingBitRateCapProducesATranscode: (d) a 4 Mbit/s cap below the source forces a
// transcode with the reason bitrate_cap; the same file plays directly without the cap.
func TestStreamingBitRateCapProducesATranscode(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t, hiBitrateClipName), nil)
	clip := server.file(t, hiBitrateClipName)

	if clip.BitRate == nil || *clip.BitRate <= capBitRate {
		t.Fatalf("generated clip bit rate = %v, want above %d", clip.BitRate, capBitRate)
	}

	uncapped := server.mustStartSession(t, clip.ID.String(), sessionOptions{})
	if uncapped.Mode != apigen.PlaybackModeDirectPlay {
		t.Errorf("uncapped mode = %s, want direct_play", uncapped.Mode)
	}

	capped := server.mustStartSession(t, clip.ID.String(), sessionOptions{maxBitRate: capBitRate})
	if capped.Mode != apigen.PlaybackModeTranscode || !hasReason(capped.Reasons, "bitrate_cap") {
		t.Fatalf("capped session = mode %s reasons %+v, want a transcode with bitrate_cap", capped.Mode, capped.Reasons)
	}

	if capped.OutputBitRate == nil || *capped.OutputBitRate > capBitRate {
		t.Errorf("output bit rate = %v, want at most %d", capped.OutputBitRate, capBitRate)
	}

	fetchBody(t, server.baseURL+capped.StreamURL, http.StatusOK)

	server.stop(t)
}

// TestStreamingTranscoderStatusIsAdminOnlyAndConsistent: (e) the administrator sees an empty or
// consistent device list, a user without the admin role gets 403.
func TestStreamingTranscoderStatusIsAdminOnlyAndConsistent(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t), nil)

	var status apigen.TranscoderStatus

	getJSON(t, server.baseURL+"/api/v1/system/transcoder", &status)

	if status.Hwaccel != apigen.HardwareAccelerationSettingAuto {
		t.Errorf("hwaccel = %s, want auto", status.Hwaccel)
	}

	for _, device := range status.Devices {
		if device.Usable && (device.Message != nil || device.Encoder == "") {
			t.Errorf("inconsistent device %+v: a usable encoder has a name and no failure message", device)
		}

		if !device.Usable && device.Message == nil {
			t.Errorf("unusable device %+v carries no reason", device)
		}
	}

	if status.ThrottleSupported != (runtime.GOOS == "linux") {
		t.Errorf("throttleSupported = %t, want it only on Linux-like systems", status.ThrottleSupported)
	}

	userToken := createUserAndSignIn(t, server.baseURL)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.baseURL+"/api/v1/system/transcoder", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Authorization", "Bearer "+userToken)

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET transcoder as a user: %v", err)
	}

	_ = response.Body.Close()

	if response.StatusCode != http.StatusForbidden {
		t.Errorf("GET /system/transcoder as a user = %d, want 403", response.StatusCode)
	}

	server.stop(t)
}

// createUserAndSignIn creates an account without the admin role and returns its bearer token.
func createUserAndSignIn(t *testing.T, baseURL string) string {
	t.Helper()

	const name, password = "Viewer", "another long password"

	payload := fmt.Sprintf(`{"name":%q,"password":%q,"role":"user"}`, name, password)
	if status, body := postJSON(t, baseURL+"/api/v1/users", []byte(payload)); status != http.StatusCreated {
		t.Fatalf("create user = %d: %s", status, body)
	}

	client := newAnonymousClient(t)
	signIn := fmt.Sprintf(`{"name":%q,"password":%q,"client":"cli"}`, name, password)

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, baseURL+signInPath, strings.NewReader(signIn))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("sign in: %v", err)
	}

	defer func() { _ = response.Body.Close() }()

	var session apigen.Session
	if err := json.NewDecoder(response.Body).Decode(&session); err != nil || response.StatusCode != http.StatusOK || session.Token == nil {
		t.Fatalf("sign in = %d, %v, want a token", response.StatusCode, err)
	}

	return *session.Token
}

// TestStreamingTrickplayAndSessionListAnswer: the routes of M4-14 answer for real: the session
// list names the title, trickplay is requested and generated in the background.
func TestStreamingTrickplayAndSessionListAnswer(t *testing.T) {
	t.Parallel()

	server := startStreamingServer(t, streamingLibrary(t), nil)
	clip := server.file(t, filepath.Base(multiAudioClip))
	session := server.mustStartSession(t, clip.ID.String(), sessionOptions{})

	var list apigen.PlaybackSessionList

	getJSON(t, server.baseURL+"/api/v1/playback/sessions", &list)

	if len(list.Sessions) != 1 || list.Sessions[0].ID != session.ID || list.Sessions[0].Title == "" {
		t.Errorf("session list = %+v, want the started session with its title", list.Sessions)
	}

	manifestURL := server.baseURL + "/api/v1/media-files/" + clip.ID.String() + "/trickplay"
	if status, body := postJSON(t, manifestURL, nil); status != http.StatusAccepted {
		t.Fatalf("request trickplay = %d: %s", status, body)
	}

	deadline := time.Now().Add(scanTimeout)

	for {
		status := sendRequest(t, http.MethodGet, manifestURL)
		if status == http.StatusOK {
			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("trickplay manifest = %d after %s, want 200", status, scanTimeout)
		}

		time.Sleep(pollInterval)
	}

	fetchBody(t, manifestURL+"/0.jpg", http.StatusOK)

	server.stop(t)
}

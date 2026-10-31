package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

// fakeFile is one catalogued media file of the fake server and the session it gets.
type fakeFile struct {
	name   string
	fileID uuid.UUID
	itemID uuid.UUID
	// session is the template of the session the server creates for the file.
	session apigen.PlaybackSession
	// parts is the original file for direct play, or init.mp4 followed by the media
	// segments for HLS.
	parts []string
	// streamStatus, when set, answers every stream request with this status, like a
	// session whose FFmpeg failed.
	streamStatus int
	failureCode  string
	// seekStart and seekParts replace the start position and the parts of a session that
	// was requested with a start position.
	seekStart float64
	seekParts []string
}

// fakeSession is a running session of the fake server.
type fakeSession struct {
	file  *fakeFile
	parts []string
	// unfinishedPolls is how many playlist requests still get a playlist without
	// EXT-X-ENDLIST; notReady how many stream requests still get 503 stream_not_ready.
	unfinishedPolls int
	notReady        int
}

// fakeServer implements the part of the API contract the smoke test uses, in memory.
type fakeServer struct {
	t *testing.T

	mu        sync.Mutex
	libraryID uuid.UUID
	files     []*fakeFile
	// scanPolls is how many library reads still report the scan as running.
	scanPolls int
	scanState apigen.ScanState
	noLibrary bool
	// sessionLimitOnce makes the first session creation answer 503 session_limit_reached.
	sessionLimitOnce bool
	sessions         map[uuid.UUID]*fakeSession
	stopped          []uuid.UUID
}

func newFakeServer(t *testing.T, files ...*fakeFile) (*fakeServer, *httptest.Server) {
	t.Helper()

	fake := &fakeServer{
		t: t, libraryID: uuid.New(), files: files, scanPolls: 2, scanState: apigen.ScanStateIdle,
		sessions: map[uuid.UUID]*fakeSession{},
	}

	server := httptest.NewServer(fake.routes())
	t.Cleanup(server.Close)

	return fake, server
}

func (fake *fakeServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/system/info", fake.systemInfo)
	mux.HandleFunc("GET /api/v1/libraries", fake.libraries)
	mux.HandleFunc("GET /api/v1/libraries/{id}", fake.library)
	mux.HandleFunc("POST /api/v1/libraries/{id}/scan", fake.scan)
	mux.HandleFunc("GET /api/v1/libraries/{id}/items", fake.items)
	mux.HandleFunc("GET /api/v1/items/{id}", fake.item)
	mux.HandleFunc("POST /api/v1/playback/sessions", fake.createSession)
	mux.HandleFunc("GET /api/v1/playback/sessions/{id}", fake.getSession)
	mux.HandleFunc("DELETE /api/v1/playback/sessions/{id}", fake.deleteSession)
	mux.HandleFunc("GET /api/v1/playback/sessions/{id}/file", fake.streamFile)
	mux.HandleFunc("GET /api/v1/playback/sessions/{id}/hls/{name}", fake.hlsFile)

	return mux
}

func (fake *fakeServer) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", contentTypeJSON)
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		fake.t.Errorf("encode the fake response: %v", err)
	}
}

func (fake *fakeServer) problem(w http.ResponseWriter, status int, code string, retry int) {
	if retry > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retry))
	}

	fake.writeJSON(w, status, apigen.Problem{Status: int32(status), Code: code, Title: code, Type: "about:blank"}) //nolint:gosec // HTTP status codes fit
}

func (fake *fakeServer) systemInfo(w http.ResponseWriter, _ *http.Request) {
	fake.writeJSON(w, http.StatusOK, apigen.SystemInfo{ProductName: "Fake", Version: "1.2.3", APIVersion: "1"})
}

func (fake *fakeServer) libraryValue() apigen.Library {
	state := apigen.ScanStateRunning
	if fake.scanPolls <= 0 {
		state = fake.scanState
	}

	library := apigen.Library{ID: fake.libraryID, Name: "media", Available: true, Scan: apigen.ScanStatus{State: state}}
	if state == apigen.ScanStateFailed {
		code := "root_unavailable"
		library.Scan.ErrorCode = &code
	}

	return library
}

func (fake *fakeServer) libraries(w http.ResponseWriter, _ *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	list := apigen.LibraryList{Libraries: []apigen.Library{}}
	if !fake.noLibrary {
		list.Libraries = append(list.Libraries, fake.libraryValue())
	}

	fake.writeJSON(w, http.StatusOK, list)
}

func (fake *fakeServer) library(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if r.PathValue("id") != fake.libraryID.String() {
		fake.problem(w, http.StatusNotFound, "not_found", 0)

		return
	}

	fake.scanPolls--
	fake.writeJSON(w, http.StatusOK, fake.libraryValue())
}

func (fake *fakeServer) scan(w http.ResponseWriter, _ *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	fake.writeJSON(w, http.StatusAccepted, apigen.ScanStatus{State: apigen.ScanStateRunning})
}

// items pages one item at a time, so the cursor handling is exercised.
func (fake *fakeServer) items(w http.ResponseWriter, r *http.Request) {
	position := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		position, _ = strconv.Atoi(cursor)
	}

	page := apigen.ItemPage{Items: []apigen.ItemSummary{}}

	if position < len(fake.files) {
		file := fake.files[position]
		page.Items = append(page.Items, apigen.ItemSummary{ID: file.itemID, LibraryID: fake.libraryID, Title: file.name})
	}

	if position+1 < len(fake.files) {
		next := strconv.Itoa(position + 1)
		page.NextCursor = &next
	}

	fake.writeJSON(w, http.StatusOK, page)
}

func (fake *fakeServer) item(w http.ResponseWriter, r *http.Request) {
	for _, file := range fake.files {
		if file.itemID.String() == r.PathValue("id") {
			fake.writeJSON(w, http.StatusOK, apigen.ItemDetail{
				ID: file.itemID, LibraryID: fake.libraryID, Title: file.name,
				Files: []apigen.MediaFile{{ID: file.fileID, FileName: file.name, Available: true}},
			})

			return
		}
	}

	fake.problem(w, http.StatusNotFound, "not_found", 0)
}

func (fake *fakeServer) createSession(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	var request apigen.PlaybackRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.Profile.Name == "" {
		fake.problem(w, http.StatusBadRequest, "invalid", 0)

		return
	}

	if fake.sessionLimitOnce {
		fake.sessionLimitOnce = false
		fake.problem(w, http.StatusServiceUnavailable, codeSessionLimitReached, 1)

		return
	}

	for _, file := range fake.files {
		if file.fileID == request.FileID {
			fake.writeJSON(w, http.StatusCreated, fake.startSession(file, request))

			return
		}
	}

	fake.problem(w, http.StatusNotFound, "not_found", 0)
}

func (fake *fakeServer) startSession(file *fakeFile, request apigen.PlaybackRequest) apigen.PlaybackSession {
	session := file.session
	session.ID = uuid.New()
	session.FileID = file.fileID
	session.State = apigen.PlaybackSessionStateReady

	if session.Delivery == apigen.PlaybackDeliveryFile {
		session.StreamURL = "/api/v1/playback/sessions/" + session.ID.String() + "/file"
	} else {
		session.StreamURL = "/api/v1/playback/sessions/" + session.ID.String() + "/hls/index.m3u8"
	}

	parts := file.parts
	if request.StartPositionSeconds != nil {
		session.StartPositionSeconds, parts = file.seekStart, file.seekParts
	}

	fake.sessions[session.ID] = &fakeSession{file: file, parts: parts, unfinishedPolls: 1, notReady: 1}

	return session
}

func (fake *fakeServer) session(r *http.Request) (*fakeSession, uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return nil, uuid.UUID{}, false
	}

	session, found := fake.sessions[id]

	return session, id, found
}

func (fake *fakeServer) getSession(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	session, id, found := fake.session(r)
	if !found {
		fake.problem(w, http.StatusNotFound, "not_found", 0)

		return
	}

	value := session.file.session
	value.ID, value.State = id, apigen.PlaybackSessionStateEnded

	if session.file.failureCode != "" {
		value.State, value.FailureCode = apigen.PlaybackSessionStateFailed, &session.file.failureCode
	}

	fake.writeJSON(w, http.StatusOK, value)
}

func (fake *fakeServer) deleteSession(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	if _, id, found := fake.session(r); found {
		delete(fake.sessions, id)
		fake.stopped = append(fake.stopped, id)
	}

	w.WriteHeader(http.StatusNoContent)
}

func (fake *fakeServer) streamFile(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	session, _, found := fake.session(r)
	fake.mu.Unlock()

	if !found || session.file.session.Delivery != apigen.PlaybackDeliveryFile {
		fake.problem(w, http.StatusNotFound, "not_found", 0)

		return
	}

	if session.file.streamStatus != 0 {
		fake.problem(w, session.file.streamStatus, "internal", 0)

		return
	}

	http.ServeContent(w, r, session.file.name, time.Time{}, strings.NewReader(session.parts[0]))
}

func (fake *fakeServer) hlsFile(w http.ResponseWriter, r *http.Request) {
	fake.mu.Lock()
	defer fake.mu.Unlock()

	session, _, found := fake.session(r)

	switch {
	case !found:
		fake.problem(w, http.StatusNotFound, "not_found", 0)
	case session.file.streamStatus != 0:
		fake.problem(w, session.file.streamStatus, "internal", 0)
	case session.notReady > 0:
		session.notReady--
		fake.problem(w, http.StatusServiceUnavailable, codeStreamNotReady, 1)
	default:
		fake.serveHLS(w, r.PathValue("name"), session)
	}
}

func (fake *fakeServer) serveHLS(w http.ResponseWriter, name string, session *fakeSession) {
	parts := session.parts

	if name == "index.m3u8" {
		finished := session.unfinishedPolls <= 0
		session.unfinishedPolls--

		_, _ = fmt.Fprint(w, fakePlaylist(len(parts)-1, finished))

		return
	}

	if name == "init.mp4" {
		_, _ = fmt.Fprint(w, parts[0]) //nolint:gosec // fixed bytes of the test, no HTML

		return
	}

	var index int
	if _, err := fmt.Sscanf(name, "seg-%05d.m4s", &index); err != nil || index+1 >= len(parts) {
		fake.problem(w, http.StatusNotFound, "not_found", 0)

		return
	}

	_, _ = fmt.Fprint(w, parts[index+1]) //nolint:gosec // fixed bytes of the test, no HTML
}

func fakePlaylist(segments int, finished bool) string {
	var text strings.Builder

	text.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-PLAYLIST-TYPE:EVENT\n#EXT-X-MAP:URI=\"init.mp4\"\n")

	for index := range segments {
		fmt.Fprintf(&text, "#EXTINF:2.000000,\nseg-%05d.m4s\n", index)
	}

	if finished {
		text.WriteString(playlistEndList + "\n")
	}

	return text.String()
}

// fakeProber answers with the summary registered for the downloaded bytes.
type fakeProber struct {
	summaries map[string]outputSummary
}

func (p fakeProber) Probe(_ context.Context, path string) (outputSummary, error) {
	data, err := os.ReadFile(path) //nolint:gosec // temporary download of the test
	if err != nil {
		return outputSummary{}, fmt.Errorf("read the download: %w", err)
	}

	summary, found := p.summaries[string(data)]
	if !found {
		return outputSummary{}, fmt.Errorf("%w: no summary for %q", errNoDuration, data)
	}

	return summary, nil
}

// sessionFor builds the session template a correct server returns for the expectation.
func sessionFor(want corpustest.Expectation, delivery apigen.PlaybackDelivery) apigen.PlaybackSession {
	session := apigen.PlaybackSession{
		Mode: apigen.PlaybackMode(want.Mode), Delivery: delivery,
		Streams: []apigen.StreamDecision{
			{Kind: apigen.MediaStreamKindVideo, Action: apigen.StreamAction(want.Video)},
			{Kind: apigen.MediaStreamKindAudio, Action: apigen.StreamAction(want.Audio)},
		},
		Reasons: []apigen.PlaybackReason{}, Warnings: []apigen.PlaybackReason{},
	}

	for _, reason := range want.Reasons {
		session.Reasons = append(session.Reasons, apigen.PlaybackReason{
			Code: string(reason.Code), Scope: apigen.PlaybackReasonScope(reason.Scope),
		})
	}

	for _, warning := range want.Warnings {
		session.Warnings = append(session.Warnings, apigen.PlaybackReason{
			Code: string(warning.Code), Scope: apigen.PlaybackReasonScope(warning.Scope),
		})
	}

	return session
}

func expected(t *testing.T, caseName string) corpustest.Expectation {
	t.Helper()

	want, found := corpustest.Expected(caseName, corpustest.ChromiumDesktop)
	if !found {
		t.Fatalf("corpustest has no expectation for %s", caseName)
	}

	return want
}

func newFakeFile(name string, session apigen.PlaybackSession, parts ...string) *fakeFile {
	return &fakeFile{name: name, fileID: uuid.New(), itemID: uuid.New(), session: session, parts: parts}
}

// testManifest is a manifest entry of a generated test case.
type testManifest struct {
	name, category, file, videoCodec, audioCodec string
	durationSeconds                              float64
}

func writeManifest(t *testing.T, entries ...testManifest) string {
	t.Helper()

	manifest := corpusManifest{SchemaVersion: manifestSchemaVersion, FFmpegVersion: "test"}

	for _, entry := range entries {
		clip := corpusCase{
			Name: entry.name, Category: entry.category, File: entry.file, Status: statusGenerated,
			Expected: caseProperties{DurationSeconds: entry.durationSeconds, Audio: []streamProperties{{Codec: entry.audioCodec}}},
		}

		if entry.videoCodec != "" {
			clip.Expected.Video = &streamProperties{Codec: entry.videoCodec}
		}

		manifest.Cases = append(manifest.Cases, clip)
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode the manifest: %v", err)
	}

	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write the manifest: %v", err)
	}

	return path
}

func testDependencies(t *testing.T, summaries map[string]outputSummary) smokeDependencies {
	t.Helper()

	return smokeDependencies{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		prober:     fakeProber{summaries: summaries},
		poll:       pollSettings{interval: time.Millisecond, maxRetryAfter: time.Millisecond},
		workDir:    t.TempDir(),
	}
}

func testOptions(t *testing.T, server *httptest.Server, manifestPath string) options {
	t.Helper()

	base, err := resolve(mustParseURL(t, server.URL), "/")
	if err != nil {
		t.Fatalf("resolve the base: %v", err)
	}

	return options{
		base: base, manifest: manifestPath, profile: corpustest.ChromiumDesktop,
		caseTimeout: 10 * time.Second, scanTimeout: 10 * time.Second,
	}
}

func joined(parts ...string) string {
	var buffer bytes.Buffer
	for _, part := range parts {
		buffer.WriteString(part)
	}

	return buffer.String()
}

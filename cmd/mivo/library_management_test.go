package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// foreignOrigin is a page on another site, which the management guard must refuse.
	foreignOrigin = "http://evil.example"
	// managementPollInterval spaces the polls while a scan runs.
	managementPollInterval = 50 * time.Millisecond
	// testVideoSeconds, testVideoSize and testVideoRate describe the clips the test makes.
	testVideoSeconds = "1"
	testVideoSize    = "64x48"
	testVideoRate    = "5"
)

// managementFixture is a server whose browse root holds two movie folders next to the
// directories the browser must hide: its own data directory, a hidden folder and a Synology
// metadata folder.
type managementFixture struct {
	environment map[string]string
	browseRoot  string
	server      testServer
}

func newManagementEnvironment(t *testing.T, extraConfig func(browseRoot string) string) managementFixture {
	t.Helper()

	environment := isolatedEnvironment(t)
	browseRoot := t.TempDir()

	for _, name := range []string{"movies-a", "movies-b", "@eaDir", ".hidden"} {
		if err := os.Mkdir(filepath.Join(browseRoot, name), 0o750); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	// The data directory lies inside the browse root, as on a NAS share that holds both.
	environment[envPathsData] = filepath.Join(browseRoot, "mivo-data")

	configuration := fmt.Sprintf("scan:\n  watch: false\n  interval: 0\nbrowse:\n  roots:\n    - '%s'\n", browseRoot)
	if extraConfig != nil {
		configuration += extraConfig(browseRoot)
	}

	writeConfigFile(t, environment[paths.ConfigDirEnv()], configuration)

	return managementFixture{environment: environment, browseRoot: browseRoot}
}

// start runs the server on the fixture's directories; it may be called again after stop.
func (fixture *managementFixture) start(t *testing.T, hooks runHooks) {
	t.Helper()

	fixture.server = startTestServer(t, fixture.environment, hooks)
}

// call sends a JSON request and returns the status and the body.
func call(t *testing.T, method, address, body string) (int, []byte) {
	t.Helper()

	return send(t, newCall(t, method, address, body))
}

// callFromForeignOrigin sends the request as a page of another site would.
func callFromForeignOrigin(t *testing.T, method, address, body string) (int, []byte) {
	t.Helper()

	request := newCall(t, method, address, body)
	request.Header.Set("Origin", foreignOrigin)

	return send(t, request)
}

func newCall(t *testing.T, method, address, body string) *http.Request {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, address, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}

	return request
}

func send(t *testing.T, request *http.Request) (int, []byte) {
	t.Helper()

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", request.Method, request.URL, err)
	}

	defer func() { _ = response.Body.Close() }()

	content, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", request.Method, request.URL, err)
	}

	return response.StatusCode, content
}

func listAllLibraries(t *testing.T, baseURL string) []apigen.Library {
	t.Helper()

	var list struct {
		Libraries []apigen.Library `json:"libraries"`
	}

	getJSON(t, baseURL+"/api/v1/libraries", &list)

	return list.Libraries
}

// listRoots reads the folder roots and returns the only one.
func listRoots(t *testing.T, baseURL string) (apigen.FolderRoot, apigen.FolderRootList) {
	t.Helper()

	var roots apigen.FolderRootList

	getJSON(t, baseURL+"/api/v1/folder-roots", &roots)

	if len(roots.Roots) != 1 {
		t.Fatalf("folder roots = %+v, want exactly the configured one", roots.Roots)
	}

	return roots.Roots[0], roots
}

// TestLibraryManagementBrowsesFoldersSafely covers the folder browser through the real
// adapters: the root IDs are canonical UUIDs that list folders again, reserved and hidden
// folders are not offered and a path that leaves the root is refused.
func TestLibraryManagementBrowsesFoldersSafely(t *testing.T) {
	t.Parallel()

	fixture := newManagementEnvironment(t, nil)
	fixture.start(t, runHooks{})

	if libraries := listAllLibraries(t, fixture.server.baseURL); len(libraries) != 0 {
		t.Fatalf("libraries of a fresh server = %+v, want none", libraries)
	}

	root, all := listRoots(t, fixture.server.baseURL)
	if root.Path != fixture.browseRoot || !root.Available || all.InstallKind != apigen.InstallKind("standard") {
		t.Errorf("root = %+v, install kind %q; want the browse root, available, standard", root, all.InstallKind)
	}

	var listing apigen.FolderListing

	getJSON(t, fixture.server.baseURL+"/api/v1/folder-roots/"+root.ID.String()+"/folders", &listing)

	var names []string
	for _, folder := range listing.Folders {
		names = append(names, folder.Name)
	}

	if want := []string{"movies-a", "movies-b"}; !slices.Equal(names, want) {
		t.Errorf("folders = %v, want %v (@eaDir, .hidden and the data directory are hidden)", names, want)
	}

	traversal := fixture.server.baseURL + "/api/v1/folder-roots/" + root.ID.String() + "/folders?path=" + url.QueryEscape("../x")
	if status, _ := call(t, http.MethodGet, traversal, ""); status != http.StatusBadRequest {
		t.Errorf("GET folders with path=../x = %d, want 400", status)
	}

	if status, _ := callFromForeignOrigin(t, http.MethodGet, fixture.server.baseURL+"/api/v1/folder-roots", ""); status != http.StatusForbidden {
		t.Errorf("GET folder-roots with a foreign Origin = %d, want 403", status)
	}

	body := `{"kind":"movies","name":"Movies","folders":[{"rootId":"` + root.ID.String() + `","path":"movies-a"}],` +
		`"metadataLanguage":"en","options":{"scanExtras":true,"readNfo":true,"localArtwork":true}}`
	if status, _ := callFromForeignOrigin(t, http.MethodPost, fixture.server.baseURL+"/api/v1/libraries", body); status != http.StatusForbidden {
		t.Errorf("POST libraries with a foreign Origin = %d, want 403", status)
	}

	fixture.server.stop(t)
}

// TestLibraryManagementRefusesPublicPeers: a client outside the local network cannot manage
// libraries or browse folders, also when server.lan_only is off.
func TestLibraryManagementRefusesPublicPeers(t *testing.T) {
	t.Parallel()

	fixture := newManagementEnvironment(t, nil)
	fixture.environment[envServerLANOnly] = "false"
	fixture.start(t, runHooks{wrapListener: asPublicPeers})

	for _, target := range []string{"/api/v1/folder-roots"} {
		if status, _ := call(t, http.MethodGet, fixture.server.baseURL+target, ""); status != http.StatusForbidden {
			t.Errorf("GET %s from a public peer = %d, want 403", target, status)
		}
	}

	if status, _ := call(t, http.MethodPost, fixture.server.baseURL+"/api/v1/libraries", `{}`); status != http.StatusForbidden {
		t.Errorf("POST /api/v1/libraries from a public peer = %d, want 403", status)
	}

	fixture.server.stop(t)
}

// TestConfiguredLibrariesAreImportedOnce: a configured library appears once; after the user
// removed it in the app, the next start does not import it again (ADR 0028).
func TestConfiguredLibrariesAreImportedOnce(t *testing.T) {
	t.Parallel()

	fixture := newManagementEnvironment(t, func(browseRoot string) string {
		return fmt.Sprintf("libraries:\n  - name: Movies\n    kind: movies\n    path: '%s'\n",
			filepath.Join(browseRoot, "movies-a"))
	})
	fixture.start(t, runHooks{})

	libraries := listAllLibraries(t, fixture.server.baseURL)
	if len(libraries) != 1 || libraries[0].Name != "Movies" || !libraries[0].ImportedFromConfig {
		t.Fatalf("libraries after the first start = %+v, want the configured one, marked as imported", libraries)
	}

	status, _ := call(t, http.MethodDelete, fixture.server.baseURL+"/api/v1/libraries/"+libraries[0].ID.String(), "")
	if status != http.StatusNoContent {
		t.Fatalf("DELETE library = %d, want 204", status)
	}

	fixture.server.stop(t)
	fixture.start(t, runHooks{})

	if libraries := listAllLibraries(t, fixture.server.baseURL); len(libraries) != 0 {
		t.Errorf("libraries after the restart = %+v, want none: a removed library is not imported again", libraries)
	}

	fixture.server.stop(t)
}

// TestLibraryManagementCreatesEditsAndDeletesALibrary walks the whole lifecycle with real
// ffprobe: create with two folders (both are scanned), edit (the removed folder's items go
// after the rescan), delete (the library is gone).
func TestLibraryManagementCreatesEditsAndDeletesALibrary(t *testing.T) {
	t.Parallel()

	ffmpegPath := requireFFmpegForManagement(t)
	fixture := newManagementEnvironment(t, nil)
	writeTestMovie(t, ffmpegPath, filepath.Join(fixture.browseRoot, "movies-a", "Alpha (2020).mp4"))
	writeTestMovie(t, ffmpegPath, filepath.Join(fixture.browseRoot, "movies-b", "Beta (2021).mp4"))
	fixture.start(t, runHooks{})

	baseURL := fixture.server.baseURL
	root, _ := listRoots(t, baseURL)

	created := createMoviesLibrary(t, baseURL, root, "movies-a", "movies-b")
	if len(created.Folders) != 2 {
		t.Fatalf("created library has %d folders, want 2", len(created.Folders))
	}

	awaitLibraryItems(t, baseURL, created.ID.String(), []string{"Alpha", "Beta"})

	// Keep the first folder only: the items of the second leave with the rescan.
	patch := fmt.Sprintf(`{"folders":[{"folderId":%q}]}`, created.Folders[0].ID.String())
	libraryURL := baseURL + "/api/v1/libraries/" + created.ID.String()

	if status, body := call(t, http.MethodPatch, libraryURL, patch); status != http.StatusOK {
		t.Fatalf("PATCH library = %d (%s), want 200", status, body)
	}

	awaitLibraryItems(t, baseURL, created.ID.String(), []string{"Alpha"})

	if status, _ := call(t, http.MethodDelete, libraryURL, ""); status != http.StatusNoContent {
		t.Fatalf("DELETE library = %d, want 204", status)
	}

	if status, _ := call(t, http.MethodGet, libraryURL, ""); status != http.StatusNotFound {
		t.Errorf("GET deleted library = %d, want 404", status)
	}

	if status, _ := call(t, http.MethodGet, libraryURL+"/items", ""); status != http.StatusNotFound {
		t.Errorf("GET items of the deleted library = %d, want 404", status)
	}

	fixture.server.stop(t)
}

func createMoviesLibrary(t *testing.T, baseURL string, root apigen.FolderRoot, folders ...string) apigen.Library {
	t.Helper()

	refs := make([]map[string]string, 0, len(folders))
	for _, folder := range folders {
		refs = append(refs, map[string]string{"rootId": root.ID.String(), "path": folder})
	}

	body, err := json.Marshal(map[string]any{
		"kind": "movies", "name": "Movies", "folders": refs, "metadataLanguage": "en",
		"options": map[string]bool{"scanExtras": true, "readNfo": true, "localArtwork": true},
	})
	if err != nil {
		t.Fatalf("marshal create request: %v", err)
	}

	status, content := call(t, http.MethodPost, baseURL+"/api/v1/libraries", string(body))
	if status != http.StatusCreated {
		t.Fatalf("POST libraries = %d (%s), want 201", status, content)
	}

	var created apigen.Library
	if err := json.NewDecoder(bytes.NewReader(content)).Decode(&created); err != nil {
		t.Fatalf("decode created library: %v", err)
	}

	return created
}

// awaitLibraryItems polls until the scan is idle and the library lists exactly the titles.
func awaitLibraryItems(t *testing.T, baseURL, libraryID string, want []string) {
	t.Helper()

	deadline := time.Now().Add(scanTimeout)

	var titles []string

	for time.Now().Before(deadline) {
		var current apigen.Library

		getJSON(t, baseURL+"/api/v1/libraries/"+libraryID, &current)

		titles = itemTitles(listItems(t, baseURL, libraryID))
		slices.Sort(titles)

		if current.Scan.State != apigen.ScanStateRunning && slices.Equal(titles, want) {
			return
		}

		time.Sleep(managementPollInterval)
	}

	t.Fatalf("library items = %v within %s, want %v", titles, scanTimeout, want)
}

// requireFFmpegForManagement skips the test without FFmpeg and returns its path.
func requireFFmpegForManagement(t *testing.T) string {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping library management test: %s is not on PATH (see docs/development.md)", tool)
		}
	}

	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatalf("look up ffmpeg: %v", err)
	}

	return path
}

// writeTestMovie makes a one-second clip with the built-in mpeg4 encoder, which every FFmpeg
// build has.
func writeTestMovie(t *testing.T, ffmpegPath, target string) {
	t.Helper()

	source := fmt.Sprintf("testsrc=duration=%s:size=%s:rate=%s", testVideoSeconds, testVideoSize, testVideoRate)
	// ffmpegPath comes from exec.LookPath and the arguments are fixed.
	command := exec.CommandContext( //nolint:gosec // G204: a test helper with a looked-up tool
		t.Context(), ffmpegPath, "-v", "error", "-f", "lavfi", "-i", source,
		"-c:v", "mpeg4", "-an", "-y", target)

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("make test clip %s: %v: %s", target, err, output)
	}
}

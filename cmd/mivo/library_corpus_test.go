package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/jpeg"
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
	// libraryCorpusDirectory is the library tree of the test corpus seen from this package
	// (M3-17, ADR 0027).
	libraryCorpusDirectory = "../../testdata/library"
	// libraryManifestFile is the expected catalog the generator writes next to the tree.
	libraryManifestFile = "manifest.json"
	// libraryManifestVersion is the manifest schema this test understands.
	libraryManifestVersion = 1
	// listLimit is the largest page the API serves; the corpus fits in one page.
	listLimit = 500
	// posterWidth is the image width the grid requests (UX LB-40).
	posterWidth = 320
	// changedNFOTitle is what the touched NFO says after the test rewrote it.
	changedNFOTitle = "Cosmos Laundromat: Second Cycle"
	// dragonBallTitle is the show with absolutely numbered files (UX DT-24).
	dragonBallTitle = "Dragon Ball Super"
	// dragonBallEpisodes is its episode count in the corpus.
	dragonBallEpisodes = 131
	// specialsSeason is the season number of specials, which are listed last.
	specialsSeason = 0
)

// libraryManifest holds the fields of testdata/library/manifest.json this test compares.
// scripts/gen-testmedia is package main and cannot be imported, so the shape is repeated.
type libraryManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	Libraries     []manifestLibrary `json:"libraries"`
}

type manifestLibrary struct {
	Kind  string         `json:"kind"`
	Path  string         `json:"path"`
	Items []manifestItem `json:"items"`
}

type manifestItem struct {
	Kind     string            `json:"kind"`
	Title    string            `json:"title"`
	Year     int32             `json:"year"`
	Path     string            `json:"path"`
	Versions []manifestVersion `json:"versions"`
	Extras   []manifestExtra   `json:"extras"`
	Artwork  []manifestArtwork `json:"artwork"`
	Seasons  []manifestSeason  `json:"seasons"`
	NFO      *manifestNFO      `json:"nfo"`
}

type manifestVersion struct {
	Label string   `json:"label"`
	Parts []string `json:"parts"`
}

type manifestExtra struct {
	Type string `json:"type"`
	File string `json:"file"`
}

type manifestArtwork struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	File   string `json:"file"`
}

type manifestSeason struct {
	Number   int32             `json:"number"`
	Episodes []json.RawMessage `json:"episodes"`
}

type manifestNFO struct {
	File string `json:"file"`
}

// libraryCorpus is the tree copied to a temporary directory, so the test may change an NFO.
type libraryCorpus struct {
	root     string
	manifest libraryManifest
}

// copyLibraryCorpus copies the library tree or skips the test when FFmpeg, ffprobe or the
// tree is missing.
func copyLibraryCorpus(t *testing.T) libraryCorpus {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping FFmpeg integration test in -short mode")
	}

	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("skipping library corpus test: %s is not on PATH (see docs/development.md)", tool)
		}
	}

	content, err := os.ReadFile(filepath.Join(libraryCorpusDirectory, libraryManifestFile))
	if err != nil {
		t.Skipf("skipping library corpus test: %s is missing (run `go run ./scripts/gen-testmedia -only 'library-*'`)",
			libraryCorpusDirectory)
	}

	var manifest libraryManifest
	if err := json.Unmarshal(content, &manifest); err != nil || manifest.SchemaVersion != libraryManifestVersion {
		t.Fatalf("library manifest: schema version %d, error %v; want version %d", manifest.SchemaVersion, err, libraryManifestVersion)
	}

	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS(libraryCorpusDirectory)); err != nil {
		t.Fatalf("copy the library corpus: %v", err)
	}

	return libraryCorpus{root: root, manifest: manifest}
}

// configure writes a configuration with the movies and the shows library and without
// automatic scans, so the test decides when a scan runs.
func (corpus libraryCorpus) configure(t *testing.T, environment map[string]string) {
	t.Helper()

	var yaml strings.Builder

	yaml.WriteString("scan:\n  watch: false\n  interval: 0\nlibraries:\n")

	for _, configured := range corpus.manifest.Libraries {
		fmt.Fprintf(&yaml, "  - name: %s\n    kind: %s\n    path: '%s'\n",
			configured.Path, configured.Kind, filepath.Join(corpus.root, configured.Path))
	}

	writeConfigFile(t, environment[paths.ConfigDirEnv()], yaml.String())
}

// TestRunBuildsTheLibraryHierarchyFromTheCorpus is the walking skeleton of milestone 3: two
// libraries are scanned at startup, the catalog matches the generator's manifest, posters
// are served through the image endpoint, and a rescan changes only what changed on disk.
func TestRunBuildsTheLibraryHierarchyFromTheCorpus(t *testing.T) {
	t.Parallel()

	corpus := copyLibraryCorpus(t)
	environment := isolatedEnvironment(t)
	corpus.configure(t, environment)

	server := startTestServer(t, environment, runHooks{})
	libraries := awaitScannedLibraries(t, server.baseURL, len(corpus.manifest.Libraries), time.Time{})

	before := map[string]apigen.ItemSummary{}

	for _, configured := range corpus.manifest.Libraries {
		listed := listItems(t, server.baseURL, libraries[configured.Path].ID.String())
		assertLibraryItems(t, server.baseURL, configured, listed)

		for _, item := range listed {
			before[item.Title] = item
		}
	}

	assertRescanChangesNothing(t, server.baseURL, libraries, before)
	corpus.assertTouchedNFORefreshesOnlyItsItem(t, server.baseURL, libraries, before)

	server.stop(t)
}

func assertLibraryItems(t *testing.T, baseURL string, expected manifestLibrary, listed []apigen.ItemSummary) {
	t.Helper()

	if len(listed) != len(expected.Items) {
		t.Errorf("library %s lists %d items, want %d (samples and extras must not be listed): %v",
			expected.Path, len(listed), len(expected.Items), itemTitles(listed))
	}

	byTitle := map[string]apigen.ItemSummary{}
	for _, item := range listed {
		byTitle[item.Title] = item
	}

	for _, want := range expected.Items {
		got, found := byTitle[want.Title]
		if !found {
			t.Errorf("library %s: item %q (NFO or file name title) is not listed; got %v", expected.Path, want.Title, itemTitles(listed))

			continue
		}

		assertListedItem(t, baseURL, want, got)
	}
}

func assertListedItem(t *testing.T, baseURL string, want manifestItem, got apigen.ItemSummary) {
	t.Helper()

	if string(got.Kind) != want.Kind || (want.Year != 0 && (got.Year == nil || *got.Year != want.Year)) {
		t.Errorf("%q = kind %s year %v, want %s %d", want.Title, got.Kind, got.Year, want.Kind, want.Year)
	}

	var detail apigen.ItemDetail

	getJSON(t, baseURL+"/api/v1/items/"+got.ID.String(), &detail)

	if want.Kind == "movie" {
		assertMovieDetail(t, want, detail)
	} else {
		assertShowChildren(t, baseURL, want, got)
	}

	if hasLocalArtwork(want, "poster") {
		assertPosterServed(t, baseURL, want.Title, detail)
	}
}

func assertMovieDetail(t *testing.T, want manifestItem, detail apigen.ItemDetail) {
	t.Helper()

	var wantFiles int
	for _, version := range want.Versions {
		wantFiles += len(version.Parts)
	}

	if len(detail.Files) != wantFiles {
		t.Errorf("%q has %d files, want %d (%d versions)", want.Title, len(detail.Files), wantFiles, len(want.Versions))
	}

	var gotExtras, wantExtras []string
	for _, extra := range derefSlice(detail.Extras) {
		if extra.ExtraType != nil {
			gotExtras = append(gotExtras, string(*extra.ExtraType))
		}
	}

	for _, extra := range want.Extras {
		wantExtras = append(wantExtras, extra.Type)
	}

	slices.Sort(gotExtras)
	slices.Sort(wantExtras)

	if !slices.Equal(gotExtras, wantExtras) {
		t.Errorf("%q extras = %v, want %v", want.Title, gotExtras, wantExtras)
	}
}

func assertShowChildren(t *testing.T, baseURL string, want manifestItem, got apigen.ItemSummary) {
	t.Helper()

	seasons := listChildren(t, baseURL, got.ID.String())
	if len(seasons) != len(want.Seasons) {
		t.Fatalf("%q has %d seasons, want %d: %v", want.Title, len(seasons), len(want.Seasons), itemTitles(seasons))
	}

	for index, wantSeason := range specialsLast(want.Seasons) {
		season := seasons[index]
		if season.Number == nil || *season.Number != wantSeason.Number {
			t.Errorf("%q season at position %d is number %s, want %d (specials last)", want.Title, index, numberText(season.Number), wantSeason.Number)

			continue
		}

		episodes := listChildren(t, baseURL, season.ID.String())
		if len(episodes) != len(wantSeason.Episodes) {
			t.Errorf("%q season %d has %d episodes, want %d", want.Title, wantSeason.Number, len(episodes), len(wantSeason.Episodes))
		}
	}

	if want.Title == dragonBallTitle && (len(seasons) != 1 || len(listChildren(t, baseURL, seasons[0].ID.String())) != dragonBallEpisodes) {
		t.Errorf("%s: want 1 season with %d episodes (DT-24)", dragonBallTitle, dragonBallEpisodes)
	}
}

// specialsLast orders the manifest's ascending seasons like the API: the specials (season 0)
// come after the numbered seasons.
func specialsLast(seasons []manifestSeason) []manifestSeason {
	ordered := slices.Clone(seasons)
	slices.SortStableFunc(ordered, func(a, b manifestSeason) int {
		return specialsRank(a) - specialsRank(b)
	})

	return ordered
}

func specialsRank(season manifestSeason) int {
	if season.Number == specialsSeason {
		return 1
	}

	return 0
}

func numberText(number *int32) string {
	if number == nil {
		return "none"
	}

	return fmt.Sprint(*number)
}

// assertPosterServed requests the poster at the grid width and checks the image and the
// caching headers of the contract.
func assertPosterServed(t *testing.T, baseURL, title string, detail apigen.ItemDetail) {
	t.Helper()

	if detail.Artwork == nil || detail.Artwork.Poster == nil {
		t.Errorf("%q has a local poster file but no poster artwork", title)

		return
	}

	poster := detail.Artwork.Poster
	query := url.Values{"width": {fmt.Sprint(posterWidth)}, "tag": {poster.Tag}}
	address := baseURL + "/api/v1/images/" + poster.ID.String() + "?" + query.Encode()

	image := fetchImage(t, address)
	if image.status != http.StatusOK || image.header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("%q poster = %d %q, want 200 image/jpeg: %.200s", title, image.status, image.header.Get("Content-Type"), image.body)

		return
	}

	decoded, err := jpeg.DecodeConfig(bytes.NewReader(image.body))
	if err != nil || decoded.Width > posterWidth || decoded.Width == 0 {
		t.Errorf("%q poster width = %d (%v), want 1..%d", title, decoded.Width, err, posterWidth)
	}

	wantETag := fmt.Sprintf(`"%s-%d"`, poster.Tag, posterWidth)
	if etag := image.header.Get("ETag"); etag != wantETag {
		t.Errorf("%q poster ETag = %q, want %s", title, etag, wantETag)
	}

	if cache := image.header.Get("Cache-Control"); !strings.Contains(cache, "immutable") {
		t.Errorf("%q poster Cache-Control = %q, want immutable for the current tag", title, cache)
	}
}

// fetchedImage is what an image request answered.
type fetchedImage struct {
	status int
	header http.Header
	body   []byte
}

func fetchImage(t *testing.T, address string) fetchedImage {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, address, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET %s: %v", address, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxStreamBytes))
	if err != nil {
		t.Fatalf("read %s: %v", address, err)
	}

	return fetchedImage{status: response.StatusCode, header: response.Header, body: body}
}

// assertRescanChangesNothing scans every library again: no file is added, updated or
// removed, and every top-level item keeps its ID.
func assertRescanChangesNothing(t *testing.T, baseURL string, libraries map[string]apigen.Library, before map[string]apigen.ItemSummary) {
	t.Helper()

	scanned := rescanAll(t, baseURL, libraries)

	for name, library := range scanned {
		counts := library.Scan.Counts
		if counts.Added != 0 || counts.Updated != 0 || counts.Removed != 0 {
			t.Errorf("rescan of %s changed the catalog: %+v", name, counts)
		}
	}

	for title, item := range before {
		var detail apigen.ItemDetail

		getJSON(t, baseURL+"/api/v1/items/"+item.ID.String(), &detail)

		if detail.ID != item.ID || detail.Title != item.Title {
			t.Errorf("rescan changed %q: now %s %q", title, detail.ID, detail.Title)
		}
	}
}

// assertTouchedNFORefreshesOnlyItsItem rewrites one movie NFO and checks that after a
// rescan only this item changed its title.
func (corpus libraryCorpus) assertTouchedNFORefreshesOnlyItsItem(t *testing.T, baseURL string,
	libraries map[string]apigen.Library, before map[string]apigen.ItemSummary,
) {
	t.Helper()

	touched := corpus.firstNFOItem(t)
	nfoPath := filepath.Join(corpus.root, touched.library, filepath.FromSlash(touched.item.NFO.File))
	content := "<movie><title>" + changedNFOTitle + "</title></movie>"

	if err := os.WriteFile(nfoPath, []byte(content), 0o600); err != nil {
		t.Fatalf("rewrite %s: %v", nfoPath, err)
	}

	rescanAll(t, baseURL, libraries)

	after := listItems(t, baseURL, libraries[touched.library].ID.String())

	var titles []string

	for _, item := range after {
		titles = append(titles, item.Title)

		if original, known := before[item.Title]; known && original.ID != item.ID {
			t.Errorf("%q changed its ID after the NFO touch", item.Title)
		}
	}

	if !slices.Contains(titles, changedNFOTitle) || slices.Contains(titles, touched.item.Title) {
		t.Errorf("titles after touching the NFO of %q = %v, want %q instead", touched.item.Title, titles, changedNFOTitle)
	}
}

type touchedItem struct {
	library string
	item    manifestItem
}

// firstNFOItem returns the first movie with a well-formed XML NFO whose title differs from
// its folder name.
func (corpus libraryCorpus) firstNFOItem(t *testing.T) touchedItem {
	t.Helper()

	for _, configured := range corpus.manifest.Libraries {
		for _, item := range configured.Items {
			if item.Kind == "movie" && item.NFO != nil && strings.Contains(item.Title, ":") {
				return touchedItem{library: configured.Path, item: item}
			}
		}
	}

	t.Fatal("the library manifest lists no movie with an NFO title")

	return touchedItem{}
}

func hasLocalArtwork(item manifestItem, kind string) bool {
	return slices.ContainsFunc(item.Artwork, func(artwork manifestArtwork) bool {
		return artwork.Kind == kind && artwork.Source == "local"
	})
}

func itemTitles(items []apigen.ItemSummary) []string {
	titles := make([]string, 0, len(items))
	for _, item := range items {
		titles = append(titles, item.Title)
	}

	return titles
}

func derefSlice[T any](values *[]T) []T {
	if values == nil {
		return nil
	}

	return *values
}

func listItems(t *testing.T, baseURL, libraryID string) []apigen.ItemSummary {
	t.Helper()

	var page apigen.ItemPage

	getJSON(t, fmt.Sprintf("%s/api/v1/libraries/%s/items?limit=%d", baseURL, libraryID, listLimit), &page)

	if page.NextCursor != nil {
		t.Fatalf("library %s has more than %d items; the test expects one page", libraryID, listLimit)
	}

	return page.Items
}

func listChildren(t *testing.T, baseURL, itemID string) []apigen.ItemSummary {
	t.Helper()

	var page apigen.ItemPage

	getJSON(t, fmt.Sprintf("%s/api/v1/items/%s/children?limit=%d", baseURL, itemID, listLimit), &page)

	if page.NextCursor != nil {
		t.Fatalf("item %s has more than %d children; the test expects one page", itemID, listLimit)
	}

	return page.Items
}

// rescanAll starts a scan of every library and returns the finished libraries by name.
func rescanAll(t *testing.T, baseURL string, libraries map[string]apigen.Library) map[string]apigen.Library {
	t.Helper()

	started := time.Now()

	for name, library := range libraries {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
			baseURL+"/api/v1/libraries/"+library.ID.String()+"/scan", http.NoBody)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}

		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("start the scan of %s: %v", name, err)
		}

		_ = response.Body.Close()

		if response.StatusCode != http.StatusAccepted {
			t.Fatalf("start the scan of %s = %d, want 202", name, response.StatusCode)
		}
	}

	return awaitScannedLibraries(t, baseURL, len(libraries), started)
}

// awaitScannedLibraries waits until every library has a finished scan that ended after
// notBefore and returns the libraries by name.
func awaitScannedLibraries(t *testing.T, baseURL string, count int, notBefore time.Time) map[string]apigen.Library {
	t.Helper()

	deadline := time.Now().Add(scanTimeout)

	for {
		var list struct {
			Libraries []apigen.Library `json:"libraries"`
		}

		getJSON(t, baseURL+"/api/v1/libraries", &list)

		finished := map[string]apigen.Library{}

		for _, library := range list.Libraries {
			scan := library.Scan
			if scan.State != apigen.ScanStateRunning && scan.FinishedAt != nil && scan.FinishedAt.After(notBefore) {
				finished[library.Name] = library
			}
		}

		if len(finished) == count {
			return finished
		}

		if time.Now().After(deadline) {
			t.Fatalf("library scans did not finish within %s: %+v", scanTimeout, list.Libraries)
		}

		time.Sleep(pollInterval)
	}
}

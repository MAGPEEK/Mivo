package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/jpeg"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/brand"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// envTMDBKey and envMusicBrainz are the ADR 0018 settings the online metadata tests set.
	envTMDBKey     = brand.EnvPrefix + "_METADATA_TMDB_API_KEY"
	envMusicBrainz = brand.EnvPrefix + "_METADATA_MUSICBRAINZ_ENABLED"
	// fakePosterSize is the edge of the square JPEG the fake TMDB serves.
	fakePosterSize = 64
	// metadataPoll is how often the tests look at the API while the worker runs; the worker
	// itself polls every metadataWorkerPoll.
	metadataPoll       = 50 * time.Millisecond
	metadataWorkerPoll = 100 * time.Millisecond
	// enrichmentTimeout bounds waiting for the worker.
	enrichmentTimeout = 60 * time.Second
)

// fakeTMDB answers the TMDB v3 calls of the movies adapter: a search returns the queried
// title and year as the only candidate, details return it with an overview and a poster,
// and the image host serves a small JPEG. Show searches find nothing.
type fakeTMDB struct {
	server *httptest.Server
	mutex  sync.Mutex
	titles map[int64]fakeMovie
}

type fakeMovie struct {
	title string
	year  int
}

func newFakeTMDB(t *testing.T) *fakeTMDB {
	t.Helper()

	fake := &fakeTMDB{titles: map[int64]fakeMovie{}}
	poster := fakeJPEG(t)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /3/search/movie", fake.searchMovie)
	mux.HandleFunc("GET /3/movie/{id}", fake.movie)
	mux.HandleFunc("GET /3/search/tv", emptyResults)
	mux.HandleFunc("GET /3/find/{id}", emptyResults)
	mux.HandleFunc("GET /t/p/{size}/{file}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(poster)
	})

	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)

	return fake
}

func emptyResults(w http.ResponseWriter, _ *http.Request) {
	writeFakeJSON(w, map[string]any{"page": 1, "results": []any{}, "movie_results": []any{}, "tv_results": []any{}})
}

func (fake *fakeTMDB) searchMovie(w http.ResponseWriter, request *http.Request) {
	title := request.URL.Query().Get("query")
	year, _ := strconv.Atoi(request.URL.Query().Get("year"))

	hash := fnv.New32a()
	_, _ = hash.Write([]byte(title))
	id := int64(hash.Sum32()%1_000_000) + 1

	fake.mutex.Lock()
	fake.titles[id] = fakeMovie{title: title, year: year}
	fake.mutex.Unlock()

	writeFakeJSON(w, map[string]any{"page": 1, "results": []any{fakeMovieJSON(id, fakeMovie{title: title, year: year})}})
}

func (fake *fakeTMDB) movie(w http.ResponseWriter, request *http.Request) {
	id, err := strconv.ParseInt(request.PathValue("id"), 10, 64)

	fake.mutex.Lock()
	movie, known := fake.titles[id]
	fake.mutex.Unlock()

	if err != nil || !known {
		w.WriteHeader(http.StatusNotFound)

		return
	}

	writeFakeJSON(w, fakeMovieJSON(id, movie))
}

func fakeMovieJSON(id int64, movie fakeMovie) map[string]any {
	answer := map[string]any{
		"id": id, "title": movie.title, "original_title": movie.title, "popularity": 10.0,
		"overview": "Overview from the fake TMDB.", "poster_path": fmt.Sprintf("/poster%d.jpg", id),
	}

	if movie.year > 0 {
		answer["release_date"] = fmt.Sprintf("%04d-01-01", movie.year)
	}

	return answer
}

func writeFakeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func fakeJPEG(t *testing.T) []byte {
	t.Helper()

	picture := image.NewRGBA(image.Rect(0, 0, fakePosterSize, fakePosterSize))
	for x := range fakePosterSize {
		for y := range fakePosterSize {
			picture.Set(x, y, color.RGBA{R: uint8(x * 4), G: uint8(y * 4), B: 128, A: 255})
		}
	}

	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, picture, nil); err != nil {
		t.Fatalf("encode fake poster: %v", err)
	}

	return encoded.Bytes()
}

// tmdbHooks point the TMDB adapter at the fake and allow its loopback host.
func tmdbHooks(baseURL string) runHooks {
	return runHooks{metadata: metadataHooks{
		tmdbBaseURL: baseURL + "/3", tmdbImageBaseURL: baseURL + "/t/p",
		extraHosts: []string{"127.0.0.1"}, allowInsecureLoopback: true, pollInterval: metadataWorkerPoll,
	}}
}

// moviesCorpus copies the library corpus and configures only its movies library with
// online metadata on, an own TMDB key and MusicBrainz off.
func moviesCorpus(t *testing.T) (map[string]string, manifestLibrary) {
	t.Helper()

	corpus := copyLibraryCorpus(t)
	environment := isolatedEnvironment(t)
	environment[envMetadataOnline] = "true"
	environment[envTMDBKey] = "fake-key"
	environment[envMusicBrainz] = "false"

	for _, configured := range corpus.manifest.Libraries {
		if configured.Kind != "movies" {
			continue
		}

		writeConfigFile(t, environment[paths.ConfigDirEnv()], fmt.Sprintf(
			"scan:\n  watch: false\n  interval: 0\nlibraries:\n  - name: Movies\n    kind: movies\n    path: '%s'\n",
			filepath.Join(corpus.root, configured.Path)))

		return environment, configured
	}

	t.Fatal("the corpus has no movies library")

	return nil, manifestLibrary{}
}

// TestRunEnrichesScannedMoviesFromTheProvider is the M6 walking skeleton: the startup scan
// queues the movies, the worker matches them at the fake TMDB, and the API shows the match
// and serves the provider poster of a movie without a local one.
func TestRunEnrichesScannedMoviesFromTheProvider(t *testing.T) {
	t.Parallel()

	fake := newFakeTMDB(t)
	environment, movies := moviesCorpus(t)
	server := startTestServer(t, environment, tmdbHooks(fake.server.URL))
	library := awaitScannedLibrary(t, server.baseURL)

	withoutPoster := movieWithoutLocalPoster(t, movies)
	matched := awaitMatchedItem(t, server.baseURL, library.ID.String(), withoutPoster)

	var detail apigen.ItemDetail

	getJSON(t, server.baseURL+"/api/v1/items/"+matched.ID.String(), &detail)

	if detail.Summary == nil || *detail.Summary != "Overview from the fake TMDB." {
		t.Errorf("summary = %v, want the provider overview", detail.Summary)
	}

	assertProviderPicturesStored(t, environment[envPathsData])
	assertProviderPosterServed(t, server.baseURL, withoutPoster, detail)
	assertProvidersState(t, server.baseURL, apigen.ProviderStateOk)

	server.stop(t)
}

func movieWithoutLocalPoster(t *testing.T, movies manifestLibrary) string {
	t.Helper()

	for _, item := range movies.Items {
		if item.Kind == "movie" && item.Year > 0 && !hasLocalArtwork(item, "poster") && item.NFO == nil {
			return item.Title
		}
	}

	t.Fatal("the corpus has no movie with a year, without NFO and without a local poster")

	return ""
}

// awaitMatchedItem waits until the item titled title is matched and returns it.
func awaitMatchedItem(t *testing.T, baseURL, libraryID, title string) apigen.ItemSummary {
	t.Helper()

	deadline := time.Now().Add(enrichmentTimeout)

	for {
		for _, item := range listItems(t, baseURL, libraryID) {
			if item.Title == title && item.MatchState != nil && *item.MatchState == apigen.MatchStateMatched {
				return item
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("%q was not matched within %s", title, enrichmentTimeout)
		}

		time.Sleep(metadataPoll)
	}
}

func assertProviderPicturesStored(t *testing.T, dataDir string) {
	t.Helper()

	stored, err := filepath.Glob(filepath.Join(dataDir, "artwork", "*.jpg"))
	if err != nil || len(stored) == 0 {
		t.Errorf("downloaded provider pictures = %v (%v), want at least one below the data directory", stored, err)
	}
}

func assertProviderPosterServed(t *testing.T, baseURL, title string, detail apigen.ItemDetail) {
	t.Helper()

	if detail.Artwork == nil || detail.Artwork.Poster == nil {
		t.Fatalf("%q has no poster after enrichment", title)
	}

	poster := detail.Artwork.Poster
	query := url.Values{"width": {fmt.Sprint(posterWidth)}, "tag": {poster.Tag}}

	image := fetchImage(t, baseURL+"/api/v1/images/"+poster.ID.String()+"?"+query.Encode())
	if image.status != http.StatusOK || image.header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("%q provider poster = %d %q, want 200 image/jpeg: %.200s", title, image.status,
			image.header.Get("Content-Type"), image.body)
	}
}

// assertProvidersState waits until TMDB reports want in /metadata/status.
func assertProvidersState(t *testing.T, baseURL string, want apigen.ProviderState) {
	t.Helper()

	deadline := time.Now().Add(enrichmentTimeout)

	for {
		var status apigen.MetadataStatus

		getJSON(t, baseURL+"/api/v1/metadata/status", &status)

		for _, provider := range status.Providers {
			if provider.Provider == apigen.MetadataProviderTmdb && provider.State == want {
				return
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("metadata status = %+v, want tmdb %s", status, want)
		}

		time.Sleep(metadataPoll)
	}
}

// TestRunStaysUsableWhenTheProviderIsOffline scans with the fake TMDB stopped: the scan
// completes, the library lists its movies with their local data and TMDB reports offline.
func TestRunStaysUsableWhenTheProviderIsOffline(t *testing.T) {
	t.Parallel()

	environment, movies := moviesCorpus(t)
	server := startTestServer(t, environment, tmdbHooks(unreachableProvider(t)))
	library := awaitScannedLibrary(t, server.baseURL)

	if library.Scan.State != apigen.ScanStateIdle || library.Scan.ErrorCode != nil {
		t.Errorf("scan = %+v, want it completed without error", library.Scan)
	}

	assertProvidersState(t, server.baseURL, apigen.ProviderStateOffline)

	listed := listItems(t, server.baseURL, library.ID.String())
	if title := movieWithoutLocalPoster(t, movies); !containsTitle(listed, title) {
		t.Errorf("listed %v, want %q among the movies", itemTitles(listed), title)
	}

	for _, item := range listed {
		if item.MatchState != nil && *item.MatchState == apigen.MatchStateMatched {
			t.Errorf("%q is matched although the provider is offline", item.Title)
		}
	}

	server.stop(t)
}

// unreachableProvider is the URL of a stopped provider: its port stays bound (so no other
// test server can take it) and every connection is closed before an answer.
func unreachableProvider(t *testing.T) string {
	t.Helper()

	var listenConfig net.ListenConfig

	listener, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}

			_ = connection.Close()
		}
	}()

	return "http://" + listener.Addr().String()
}

func containsTitle(items []apigen.ItemSummary, title string) bool {
	for _, item := range items {
		if item.Title == title {
			return true
		}
	}

	return false
}

// recordingResolver records every name the outbound clients look up and resolves none, so
// even a wrongly wired call cannot leave the machine.
type recordingResolver struct {
	mutex sync.Mutex
	hosts []string
}

func (resolver *recordingResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()

	resolver.hosts = append(resolver.hosts, host)

	return nil, &net.DNSError{Err: "lookups are refused in tests", Name: host, IsNotFound: true}
}

func (resolver *recordingResolver) lookups() []string {
	resolver.mutex.Lock()
	defer resolver.mutex.Unlock()

	return append([]string(nil), resolver.hosts...)
}

// TestRunWithOnlineMetadataOffDialsNothing scans the movies with the real provider hosts
// configured but online metadata off: the outbound clients look up no host, nothing is
// queued and the identify search refuses with online_disabled.
func TestRunWithOnlineMetadataOffDialsNothing(t *testing.T) {
	t.Parallel()

	environment, _ := moviesCorpus(t)
	environment[envMetadataOnline] = "false"

	resolver := &recordingResolver{}
	server := startTestServer(t, environment, runHooks{metadata: metadataHooks{resolver: resolver, pollInterval: metadataWorkerPoll}})
	library := awaitScannedLibrary(t, server.baseURL)

	var status apigen.MetadataStatus

	getJSON(t, server.baseURL+"/api/v1/metadata/status", &status)

	if status.QueueLength != 0 {
		t.Errorf("queue length = %d, want nothing queued with online metadata off", status.QueueLength)
	}

	listed := listItems(t, server.baseURL, library.ID.String())
	if len(listed) == 0 {
		t.Fatal("the movies library lists nothing")
	}

	searchStatus := postJSONStatus(t, server.baseURL+"/api/v1/items/"+listed[0].ID.String()+"/identify/search",
		`{"query":"Heat"}`)
	if searchStatus != http.StatusConflict {
		t.Errorf("identify search = %d, want 409 online_disabled", searchStatus)
	}

	// Give a wrongly started worker the time to dial.
	time.Sleep(3 * metadataWorkerPoll)

	if lookups := resolver.lookups(); len(lookups) != 0 {
		t.Errorf("outbound lookups = %v, want none with online metadata off", lookups)
	}

	server.stop(t)
}

// postJSONStatus posts body as JSON from the loopback peer (the management guard accepts
// it) and returns the status.
func postJSONStatus(t *testing.T, address, body string) int {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, address, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", address, err)
	}

	_ = response.Body.Close()

	return response.StatusCode
}

// TestMetadataSettingsSurviveARestart changes the settings through the API, restarts on the
// same data directory and reads them back.
func TestMetadataSettingsSurviveARestart(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	server := startTestServer(t, environment, runHooks{})

	change := `{"language":"de","providers":[{"provider":"musicbrainz","enabled":false}]}`
	if status := putJSONStatus(t, server.baseURL+"/api/v1/settings/metadata", change); status != http.StatusOK {
		t.Fatalf("PUT settings/metadata = %d, want 200", status)
	}

	server.stop(t)

	restarted := startTestServer(t, environment, runHooks{})

	var settings apigen.MetadataSettings

	getJSON(t, restarted.baseURL+"/api/v1/settings/metadata", &settings)

	if settings.Language != "de" || providerEnabled(settings, apigen.MetadataProviderMusicbrainz) {
		t.Errorf("settings after restart = %+v, want musicbrainz off and language de", settings)
	}

	restarted.stop(t)
}

func providerEnabled(settings apigen.MetadataSettings, provider apigen.MetadataProvider) bool {
	for _, setting := range settings.Providers {
		if setting.Provider == provider {
			return setting.Enabled
		}
	}

	return false
}

func putJSONStatus(t *testing.T, address, body string) int {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, address, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("PUT %s: %v", address, err)
	}

	_ = response.Body.Close()

	return response.StatusCode
}

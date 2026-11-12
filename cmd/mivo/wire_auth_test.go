package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/platform/brand"
)

const (
	// publicClientAddress is a public address a reverse proxy would put into X-Forwarded-For.
	publicClientAddress = "203.0.113.9"
	// alphaSchemaVersion is the last migration the alpha releases shipped (00010, the search
	// index); the accounts start with 00011.
	alphaSchemaVersion = 10
	alphaMigrations    = "../../internal/adapters/sqlite/migrations"

	alphaLibraryID = "0f0f0f0f-0000-4000-8000-000000000001"
	alphaMovieID   = "0f0f0f0f-0000-4000-8000-000000000002"
	alphaFileID    = "0f0f0f0f-0000-4000-8000-000000000003"
	alphaRootID    = "0f0f0f0f-0000-4000-8000-0000000000aa"
	// alphaMovieSeconds is the catalog duration of the seeded movie.
	alphaMovieSeconds = 7200
)

// readBody sends the request with client and returns the status and body.
func readBody(t *testing.T, client *http.Client, method, url string) (int, string) {
	t.Helper()

	request, err := http.NewRequestWithContext(t.Context(), method, url, http.NoBody)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}

	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", url, err)
	}

	return response.StatusCode, string(body)
}

func problemCode(t *testing.T, body string) string {
	t.Helper()

	var problem struct {
		Code string `json:"code"`
	}

	if err := json.Unmarshal([]byte(body), &problem); err != nil {
		t.Fatalf("decode problem %q: %v", body, err)
	}

	return problem.Code
}

func TestFreshServerAsksForSetup(t *testing.T) {
	t.Parallel()

	server := startUnsetServer(t, isolatedEnvironment(t), runHooks{})
	client := newAnonymousClient(t)

	status, body := readBody(t, client, http.MethodGet, server.baseURL+setupPath)
	if status != http.StatusOK || !strings.Contains(body, `"required":true`) {
		t.Errorf("GET setup = %d %s, want 200 with required true", status, body)
	}

	status, body = readBody(t, client, http.MethodGet, server.baseURL+"/api/v1/libraries")
	if status != http.StatusUnauthorized || problemCode(t, body) != "setup_required" {
		t.Errorf("GET libraries = %d %s, want 401 setup_required", status, body)
	}

	server.stop(t)
}

func TestAfterSetupTheCookieSessionListsLibraries(t *testing.T) {
	t.Parallel()

	server := startUnsetServer(t, isolatedEnvironment(t), runHooks{})
	web := newAnonymousClient(t)

	setupStatus, session := postSetup(t, accountCall{client: web, baseURL: server.baseURL, clientKind: "web"})
	if setupStatus != http.StatusCreated || session.Token != nil {
		t.Fatalf("setup = %d (token in body: %v), want 201 with the cookie only", setupStatus, session.Token != nil)
	}

	if status, body := readBody(t, web, http.MethodGet, server.baseURL+"/api/v1/libraries"); status != http.StatusOK {
		t.Errorf("GET libraries with the session cookie = %d %s, want 200", status, body)
	}

	// The same server without the cookie: setup is done, so this is plain unauthenticated.
	status, body := readBody(t, newAnonymousClient(t), http.MethodGet, server.baseURL+"/api/v1/libraries")
	if status != http.StatusUnauthorized || problemCode(t, body) != "unauthenticated" {
		t.Errorf("GET libraries without a credential = %d %s, want 401 unauthenticated", status, body)
	}

	if status, _ := readBody(t, web, http.MethodGet, server.baseURL+setupPath); status != http.StatusOK {
		t.Errorf("GET setup = %d, want 200", status)
	}

	if again, _ := postSetup(t, anonymousCall(t, server.baseURL)); again != http.StatusConflict {
		t.Errorf("second setup = %d, want 409", again)
	}

	server.stop(t)
}

// TestTrustedProxiesDecideWhoIsLocal wires config.Server.TrustedProxies (ADR 0032 §3, §9).
// Behind a trusted proxy the client is the one X-Forwarded-For names, so a public address is
// refused the setup that a local peer may run; with nobody trusted the header is ignored.
func TestTrustedProxiesDecideWhoIsLocal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		trusted    string
		wantStatus int
	}{
		{name: "loopback trusted", trusted: "127.0.0.1", wantStatus: http.StatusForbidden},
		{name: "default trusts nobody", trusted: "", wantStatus: http.StatusCreated},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			environment := isolatedEnvironment(t)
			if test.trusted != "" {
				environment[brand.EnvPrefix+"_SERVER_TRUSTED_PROXIES"] = test.trusted
			}

			server := startUnsetServer(t, environment, runHooks{})
			call := anonymousCall(t, server.baseURL)
			call.headers = map[string]string{"X-Forwarded-For": publicClientAddress}

			if status, _ := postSetup(t, call); status != test.wantStatus {
				t.Errorf("POST setup with X-Forwarded-For %s = %d, want %d", publicClientAddress, status, test.wantStatus)
			}

			server.stop(t)
		})
	}
}

// seedAlphaDatabase creates <data>/server.db the way the alpha releases left it: migrated to
// version 10, with one library, one movie and its file, and no account.
func seedAlphaDatabase(t *testing.T, dataDir, libraryRoot string) {
	t.Helper()

	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatalf("create data directory: %v", err)
	}

	database, err := sql.Open("sqlite", filepath.Join(dataDir, databaseFileName))
	if err != nil {
		t.Fatalf("open alpha database: %v", err)
	}

	t.Cleanup(func() { _ = database.Close() })

	migrations, err := fs.Sub(os.DirFS(alphaMigrations), ".")
	if err != nil {
		t.Fatalf("migrations: %v", err)
	}

	provider, err := goose.NewProvider(goose.DialectSQLite3, database, migrations, goose.WithDisableGlobalRegistry(true))
	if err != nil {
		t.Fatalf("goose provider: %v", err)
	}

	if _, err := provider.UpTo(t.Context(), alphaSchemaVersion); err != nil {
		t.Fatalf("migrate to the alpha schema: %v", err)
	}

	seedAlphaCatalog(t, database, libraryRoot)
}

// offlineLibraryRoot is a library folder that does not exist (a share that is not mounted), so
// the startup scan cannot reach it and leaves the seeded catalog rows alone.
func offlineLibraryRoot(t *testing.T) string {
	t.Helper()

	return filepath.Join(t.TempDir(), "offline-share")
}

func seedAlphaCatalog(t *testing.T, database *sql.DB, libraryRoot string) {
	t.Helper()

	const now = 1_700_000_000_000

	statements := []struct {
		query string
		args  []any
	}{
		{
			`INSERT INTO libraries (id, name, kind, created_at, updated_at) VALUES (?, 'Alpha Movies', 'movies', ?, ?)`,
			[]any{alphaLibraryID, now, now},
		},
		{
			`INSERT INTO library_roots (id, library_id, path, path_key, position, created_at) VALUES (?, ?, ?, ?, 0, ?)`,
			[]any{alphaRootID, alphaLibraryID, libraryRoot, library.FolderKey(libraryRoot, true), now},
		},
		{
			`INSERT INTO items (id, library_id, kind, match_key, title, sort_title, content_rating, created_at, updated_at)
			 VALUES (?, ?, 'movie', 'alpha movie', 'Alpha Movie', 'alpha movie', 'R', ?, ?)`,
			[]any{alphaMovieID, alphaLibraryID, now, now},
		},
		{
			`INSERT INTO media_files (id, item_id, library_id, root_id, relative_path, size_bytes, modified_at, container, format_name,
			                         duration_ms, bit_rate, probed_at)
			 VALUES (?, ?, ?, ?, 'alpha.mkv', 1, ?, 'mkv', 'matroska,webm', ?, 1000, ?)`,
			[]any{alphaFileID, alphaMovieID, alphaLibraryID, alphaRootID, now, alphaMovieSeconds * 1000, now},
		},
	}

	for _, statement := range statements {
		if _, err := database.ExecContext(t.Context(), statement.query, statement.args...); err != nil {
			t.Fatalf("seed alpha catalog: %v\n%s", err, statement.query)
		}
	}
}

// startAlphaServer starts a server on a seeded alpha database. The tools are missing, so the
// startup scan reports prober_unavailable and leaves the seeded catalog alone.
func startAlphaServer(t *testing.T) testServer {
	t.Helper()

	environment := isolatedEnvironment(t)
	environment[envToolsFFmpeg] = missingToolPath(t, "ffmpeg")
	environment[envToolsFFprobe] = missingToolPath(t, "ffprobe")

	seedAlphaDatabase(t, environment[envPathsData], offlineLibraryRoot(t))

	return startTestServer(t, environment, runHooks{})
}

// TestAMigratedAlphaDatabaseKeepsItsLibrariesAndItems: the upgrade from an alpha build starts
// in setup state, keeps the catalog and shows it to the first admin.
func TestAMigratedAlphaDatabaseKeepsItsLibrariesAndItems(t *testing.T) {
	t.Parallel()

	server := startAlphaServer(t)

	var libraries struct {
		Libraries []apigen.Library `json:"libraries"`
	}

	getJSON(t, server.baseURL+"/api/v1/libraries", &libraries)

	if len(libraries.Libraries) != 1 || libraries.Libraries[0].Name != "Alpha Movies" {
		t.Fatalf("libraries = %+v, want the alpha library", libraries.Libraries)
	}

	status, body := readBody(t, http.DefaultClient, http.MethodGet, server.baseURL+"/api/v1/libraries/"+alphaLibraryID+"/items")
	if status != http.StatusOK || !strings.Contains(body, "Alpha Movie") {
		t.Errorf("items of the alpha library = %d %s, want the alpha movie", status, body)
	}

	server.stop(t)
}

// TestStartupBackfillsTheAgeBucketsOfAnAlphaCatalog: migration 00014 leaves the buckets stale;
// the server computes them before it serves.
func TestStartupBackfillsTheAgeBucketsOfAnAlphaCatalog(t *testing.T) {
	t.Parallel()

	environment := isolatedEnvironment(t)
	environment[envToolsFFmpeg] = missingToolPath(t, "ffmpeg")
	environment[envToolsFFprobe] = missingToolPath(t, "ffprobe")
	seedAlphaDatabase(t, environment[envPathsData], offlineLibraryRoot(t))

	startTestServer(t, environment, runHooks{}).stop(t)

	database, err := sql.Open("sqlite", filepath.Join(environment[envPathsData], databaseFileName))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	defer func() { _ = database.Close() }()

	var bucket sql.NullInt64

	err = database.QueryRowContext(t.Context(), `SELECT age_rating FROM items WHERE id = ?`, alphaMovieID).Scan(&bucket)
	if err != nil || !bucket.Valid || bucket.Int64 != 16 {
		t.Errorf("age bucket of the R rated movie = %+v, %v, want 16", bucket, err)
	}
}

// TestContinueWatchingRowWorksEndToEnd reports progress through the API and finds the movie in
// the Continue watching row of the home rows (M7-19, M7-23): the watch service, its catalog
// adapters, the sqlite watch rows and the rows service are wired together.
func TestContinueWatchingRowWorksEndToEnd(t *testing.T) {
	t.Parallel()

	server := startAlphaServer(t)

	progress := `{"positionSeconds":600}`
	if status := putJSONStatus(t, server.baseURL+"/api/v1/items/"+alphaMovieID+"/progress", progress); status != http.StatusNoContent {
		t.Fatalf("PUT progress = %d, want 204; log:\n%s", status, server.stderr.String())
	}

	var home apigen.HomeRows

	getJSON(t, server.baseURL+"/api/v1/home/rows", &home)

	if !rowHasItem(home.Rows, apigen.RowKindContinueWatching, alphaMovieID) {
		t.Errorf("home rows = %+v, want the movie in the continue watching row", home.Rows)
	}

	var item apigen.ItemDetail

	getJSON(t, server.baseURL+"/api/v1/items/"+alphaMovieID, &item)

	if item.ViewerState == nil || item.ViewerState.PositionSeconds != 600 {
		t.Errorf("viewerState of the item = %+v, want the reported position", item.ViewerState)
	}

	server.stop(t)
}

func rowHasItem(rows []apigen.Row, kind apigen.RowKind, itemID string) bool {
	for _, row := range rows {
		if row.Kind != kind {
			continue
		}

		for _, item := range row.Items {
			if item.ID.String() == itemID {
				return true
			}
		}
	}

	return false
}

func TestSignInAfterSetupWorksOnTheSameServer(t *testing.T) {
	t.Parallel()

	server := startUnsetServer(t, isolatedEnvironment(t), runHooks{})
	call := anonymousCall(t, server.baseURL)

	if status, _ := postSetup(t, call); status != http.StatusCreated {
		t.Fatalf("setup = %d, want 201", status)
	}

	if status, session := postSessions(t, call); status != http.StatusOK || session.Token == nil {
		t.Errorf("sign in = %d (token %v), want 200 with a token", status, session.Token != nil)
	}

	server.stop(t)
}

func TestSessionPurgerRunsAtOnceAndEveryInterval(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	purger := sessionPurger{
		purge:    func(context.Context) (int64, error) { calls.Add(1); return 0, nil },
		interval: 5 * time.Millisecond, logger: slog.New(slog.DiscardHandler),
	}

	ctx, cancel := context.WithCancel(t.Context())
	stop := startBackground(ctx, purger)

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	cancel()
	stop()

	if calls.Load() < 3 {
		t.Errorf("purge calls = %d, want at least 3 (at start and on every tick)", calls.Load())
	}

	settled := calls.Load()

	time.Sleep(20 * time.Millisecond)

	if calls.Load() != settled {
		t.Error("the purger kept running after its context ended")
	}
}

func TestSessionPurgerSurvivesAFailingPurge(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32

	purger := sessionPurger{
		purge:    func(context.Context) (int64, error) { calls.Add(1); return 0, errors.New("database locked") },
		interval: 5 * time.Millisecond, logger: slog.New(slog.DiscardHandler),
	}

	stop := startBackground(t.Context(), purger)

	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	stop()

	if calls.Load() < 2 {
		t.Errorf("purge calls = %d, want a retry after the failure", calls.Load())
	}
}

// Command record-provider-fixtures downloads the TMDB answers the adapter tests replay
// (ADR 0031 §12). It is a dev-only tool: it is never run in CI, needs network access and an
// own key in the TMDB_KEY environment variable, and writes indented JSON into
// testdata/providers/tmdb. Review and trim every file by hand before committing: fixtures
// keep only the fields the adapter reads and never contain a key. Errors 401, 404 and 429
// and the deliberately broken entries in the committed fixtures are written by hand.
//
//	go run ./scripts/record-provider-fixtures [-out testdata/providers/tmdb]
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/magpeek/mivo/internal/adapters/metadata/tmdb"
	"github.com/magpeek/mivo/internal/adapters/netaccess/outbound"
)

const (
	defaultOut    = "testdata/providers/tmdb"
	keyVariable   = "TMDB_KEY"
	fixtureMode   = 0o644
	directoryMode = 0o755
	exitFailure   = 1
)

var errNoKey = errors.New(keyVariable + " is not set")

// fixture is one request and the file its answer goes to.
type fixture struct {
	file   string
	path   string
	params url.Values
}

func fixtures() []fixture {
	details := func(extra, language string) url.Values {
		return url.Values{"language": {language}, "append_to_response": {"credits,external_ids," + extra + ",images"}}
	}

	return []fixture{
		{"search_movie.json", "/search/movie", url.Values{"query": {"The Matrix"}, "year": {"1999"}, "language": {"de"}, "include_adult": {"false"}}},
		{"search_tv.json", "/search/tv", url.Values{"query": {"Breaking Bad"}, "first_air_date_year": {"2008"}, "include_adult": {"false"}}},
		{"search_empty.json", "/search/movie", url.Values{"query": {"zzzz no such film zzzz"}, "include_adult": {"false"}}},
		{"find_movie.json", "/find/tt0133093", url.Values{"external_source": {"imdb_id"}}},
		{"find_show.json", "/find/tt0903747", url.Values{"external_source": {"imdb_id"}}},
		{"movie_603.json", "/movie/603", details("release_dates", "en-US")},
		{"movie_603_de.json", "/movie/603", details("release_dates", "de")},
		{"movie_550.json", "/movie/550", details("release_dates", "en-US")},
		{"tv_1396.json", "/tv/1396", details("content_ratings", "en-US")},
		{"tv_1396_season_1.json", "/tv/1396/season/1", url.Values{"language": {"en-US"}}},
		{"tv_1396_season_1_de.json", "/tv/1396/season/1", url.Values{"language": {"de"}}},
	}
}

func main() {
	out := flag.String("out", defaultOut, "directory the fixtures are written to")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := record(ctx, recorder{
		http:    outbound.New(outbound.Options{AllowedHosts: tmdb.AllowedHosts()}),
		baseURL: tmdb.DefaultBaseURL,
		key:     os.Getenv(keyVariable),
		out:     *out,
	})

	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, "record-provider-fixtures:", err)
		os.Exit(exitFailure)
	}
}

// recorder holds what one recording run needs.
type recorder struct {
	http    tmdb.HTTPGetter
	baseURL string
	key     string
	out     string
}

func record(ctx context.Context, run recorder) error {
	if run.key == "" {
		return errNoKey
	}

	if err := os.MkdirAll(run.out, directoryMode); err != nil {
		return fmt.Errorf("create %s: %w", run.out, err)
	}

	for _, item := range fixtures() {
		if err := run.fetch(ctx, item); err != nil {
			return err
		}
	}

	return nil
}

func (run recorder) fetch(ctx context.Context, item fixture) error {
	params := url.Values{}
	for name, values := range item.params {
		params[name] = values
	}

	header := http.Header{"Accept": {"application/json"}}
	// Same rule as the adapter: a v4 token travels as a bearer header, a v3 key as api_key.
	if tmdb.IsReadAccessToken(run.key) {
		header.Set("Authorization", "Bearer "+run.key)
	} else {
		params.Set("api_key", run.key)
	}

	response, err := run.http.Get(ctx, run.baseURL+item.path+"?"+params.Encode(), header)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", item.file, err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch %s: status %d", item.file, response.StatusCode)
	}

	var indented bytes.Buffer
	if err := json.Indent(&indented, response.Body, "", "  "); err != nil {
		return fmt.Errorf("indent %s: %w", item.file, err)
	}

	indented.WriteByte('\n')

	target := filepath.Join(run.out, item.file)
	if err := os.WriteFile(target, indented.Bytes(), fixtureMode); err != nil {
		return fmt.Errorf("write %s: %w", target, err)
	}

	return nil
}

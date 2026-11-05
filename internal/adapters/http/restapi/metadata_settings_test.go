package restapi_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/enrich"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	settingsPath = "/api/v1/settings/metadata"
	// secretKey is an own TMDB key; no answer may contain it.
	secretKey = "own-secret-tmdb-key-123" //nolint:gosec // a fake key the test looks for in answers
)

func newSettingsService() *metadatasettings.Service {
	return metadatasettings.NewService(metadatasettings.Options{
		Initial:         metadatasettings.State{Online: true, TMDB: true, MusicBrainz: true},
		DefaultLanguage: "en",
		BundledTMDBKey:  "bundled-secret",
	})
}

func TestMetadataSettingsKeepTheKeyWriteOnly(t *testing.T) {
	t.Parallel()

	settings := newSettingsService()
	handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Settings: settings})

	put := serveRequest(t, handler, jsonRequest(t, http.MethodPut, settingsPath,
		`{"tmdbApiKey":"`+secretKey+`","onlineEnabled":false,"language":"de-de","providers":[{"provider":"coverartarchive","enabled":false}]}`))
	get := serveRequest(t, handler, jsonRequest(t, http.MethodGet, settingsPath, ""))

	for name, got := range map[string]response{"PUT": put, "GET": get} {
		if got.status != http.StatusOK {
			t.Fatalf("%s status = %d, body %s", name, got.status, got.body)
		}

		if strings.Contains(string(got.body), secretKey) || strings.Contains(string(got.body), "bundled-secret") {
			t.Errorf("%s answer contains a key: %s", name, got.body)
		}

		body := decodeStrictly[apigen.MetadataSettings](t, got.body)
		assertSettings(t, body)
	}

	if key, _ := settings.APIKey(t.Context()); key != secretKey {
		t.Errorf("stored key = %q, want the own key", key)
	}

	online, _ := settings.OnlineSettings(t.Context())
	if online.Enabled || online.MusicBrainz || !online.TMDB || online.Language != "de-DE" {
		t.Errorf("OnlineSettings after the change = %+v; the next job must see it", online)
	}
}

func assertSettings(t *testing.T, body apigen.MetadataSettings) {
	t.Helper()

	if body.OnlineEnabled || body.Language != "de-DE" || len(body.Providers) != 3 {
		t.Fatalf("settings = %+v", body)
	}

	tmdb, musicbrainz, coverart := body.Providers[0], body.Providers[1], body.Providers[2]
	if tmdb.Provider != apigen.MetadataProviderTmdb || !tmdb.Enabled || tmdb.KeyStatus == nil || *tmdb.KeyStatus != apigen.ProviderKeyStatusOwn {
		t.Errorf("tmdb = %+v, want enabled with own key", tmdb)
	}

	if musicbrainz.Enabled || coverart.Enabled || coverart.Provider != apigen.MetadataProviderCoverartarchive {
		t.Errorf("musicbrainz = %+v, coverart = %+v, want both off", musicbrainz, coverart)
	}
}

func TestUpdateMetadataSettingsErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		body      string
		wantCode  string
		wantField string
	}{
		{"empty change", `{}`, restapi.CodeInvalid, ""},
		{"malformed language", `{"language":"german"}`, restapi.CodeLanguageInvalid, "language"},
		{"long language", `{"language":"` + strings.Repeat("a", 36) + `"}`, restapi.CodeLanguageInvalid, "language"},
		{"unknown provider", `{"providers":[{"provider":"tmdb","enabled":true},{"provider":"tvdb","enabled":true}]}`, restapi.CodeInvalid, "providers[1]"},
		{"contradicting shared switch", `{"providers":[{"provider":"musicbrainz","enabled":true},{"provider":"coverartarchive","enabled":false}]}`, restapi.CodeInvalid, "providers[1]"},
		{"contradicting duplicate", `{"providers":[{"provider":"tmdb","enabled":true},{"provider":"tmdb","enabled":false}]}`, restapi.CodeInvalid, "providers[1]"},
		{"key with a space", `{"tmdbApiKey":"abc def"}`, restapi.CodeInvalid, "tmdbApiKey"},
		{"long key", `{"tmdbApiKey":"` + strings.Repeat("k", 129) + `"}`, restapi.CodeInvalid, "tmdbApiKey"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Settings: newSettingsService()})

			got := serveRequest(t, handler, jsonRequest(t, http.MethodPut, settingsPath, test.body))
			assertProblemField(t, assertProblem(t, got, http.StatusBadRequest, test.wantCode), test.wantField)

			if strings.Contains(string(got.body), "abc") {
				t.Errorf("problem quotes the key: %s", got.body)
			}
		})
	}
}

func TestGetMetadataStatus(t *testing.T) {
	t.Parallel()

	status := fakeStatus{status: enrich.Status{QueueLength: 12, Providers: []enrich.ProviderStatus{
		{Provider: enrich.ProviderTMDB, State: enrich.ProviderStateRateLimited},
		{Provider: enrich.ProviderMusicBrainz, State: enrich.ProviderStateOK},
		{Provider: enrich.ProviderCoverArt, State: enrich.ProviderStateDisabled},
	}}}
	handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Status: status})

	got := serveRequest(t, handler, jsonRequest(t, http.MethodGet, "/api/v1/metadata/status", ""))
	body := decodeStrictly[apigen.MetadataStatus](t, got.body)

	want := apigen.MetadataStatus{QueueLength: 12, Providers: []apigen.MetadataProviderStatus{
		{Provider: apigen.MetadataProviderTmdb, State: apigen.ProviderStateRateLimited},
		{Provider: apigen.MetadataProviderMusicbrainz, State: apigen.ProviderStateOk},
		{Provider: apigen.MetadataProviderCoverartarchive, State: apigen.ProviderStateDisabled},
	}}
	assertSameJSON(t, body, want)

	failing, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Status: fakeStatus{err: errors.New("db")}})
	assertProblem(t, serveRequest(t, failing, jsonRequest(t, http.MethodGet, "/api/v1/metadata/status", "")),
		http.StatusInternalServerError, restapi.CodeInternal)
}

func TestListLibraryItemsFiltersAndShowsMatchState(t *testing.T) {
	t.Parallel()

	items := &fakeLibraryService{page: applibrary.ItemPage{Items: []applibrary.ItemSummary{
		{Item: testItem(testItemID, "Matched"), PrimaryFile: unprobedFile()},
		{Item: testItem(testSecondItemID, "Never looked up"), PrimaryFile: unprobedFile()},
	}}}
	matches := fakeMatches{statuses: map[string]match.ItemStatus{testItemID: {State: match.MatchStateMatched, Provider: match.RefTMDB}}}
	handler, _ := newMetadataHandler(t, items, restapi.MetadataServices{Matches: matches})

	got := serveRequest(t, handler, jsonRequest(t, http.MethodGet, itemsPath+"?matchState=unmatched", ""))
	page := decodeStrictly[apigen.ItemPage](t, got.body)

	if len(page.Items) != 2 || *page.Items[0].MatchState != apigen.MatchStateMatched ||
		strings.Join(*page.Items[0].MetadataSources, ",") != "tmdb" ||
		*page.Items[1].MatchState != apigen.MatchStateUnmatched || page.Items[1].MetadataSources != nil {
		t.Errorf("items = %+v", page.Items)
	}

	if queries := items.requestedQueries; len(queries) != 1 || queries[0].Filter.MatchState != library.MatchFilterUnmatched {
		t.Errorf("queries = %+v, want the unmatched filter", queries)
	}

	invalid := serveRequest(t, handler, jsonRequest(t, http.MethodGet, itemsPath+"?matchState=pending", ""))
	assertProblemField(t, assertProblem(t, invalid, http.StatusBadRequest, restapi.CodeInvalid), "matchState")
}

func TestItemAnswersOmitMatchStateWhenItCannotBeRead(t *testing.T) {
	t.Parallel()

	items := &fakeLibraryService{
		page:     applibrary.ItemPage{Items: []applibrary.ItemSummary{{Item: testItem(testItemID, "x"), PrimaryFile: unprobedFile()}}},
		children: applibrary.ItemPage{Items: []applibrary.ItemSummary{{Item: testItem(testItemID, "x"), PrimaryFile: unprobedFile()}}},
		detail:   applibrary.ItemDetail{Item: testItem(testItemID, "x"), Files: []media.File{unprobedFile()}},
	}

	for name, services := range map[string]restapi.MetadataServices{
		"without reader": {},
		"failing reader": {Matches: fakeMatches{err: errors.New("db")}},
	} {
		handler, logs := newMetadataHandler(t, items, services)

		for _, path := range []string{itemsPath, itemPath, itemPath + "/children"} {
			got := serveRequest(t, handler, jsonRequest(t, http.MethodGet, path, ""))
			if got.status != http.StatusOK || strings.Contains(string(got.body), "matchState") {
				t.Errorf("%s %s: status %d, body %s; want 200 without matchState", name, path, got.status, got.body)
			}
		}

		if name == "failing reader" && !strings.Contains(logs.String(), "item match states unavailable") {
			t.Errorf("failing reader was not logged: %s", logs)
		}
	}
}

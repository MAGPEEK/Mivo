package restapi_test

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/adapters/http/restapi"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	searchPath   = itemPath + "/identify/search"
	identifyPath = itemPath + "/identify"
	matchPath    = itemPath + "/match"
)

func TestMetadataOperationsWithoutServicesAnswerNotImplemented(t *testing.T) {
	t.Parallel()

	handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{})

	requests := []*http.Request{
		jsonRequest(t, http.MethodPost, searchPath, `{"query":"x"}`),
		jsonRequest(t, http.MethodPost, identifyPath, `{"provider":"tmdb","externalId":"603"}`),
		jsonRequest(t, http.MethodDelete, matchPath, ""),
		jsonRequest(t, http.MethodPost, itemPath+"/refresh", ""),
		jsonRequest(t, http.MethodPost, libraryPath+"/refresh", ""),
		jsonRequest(t, http.MethodGet, "/api/v1/settings/metadata", ""),
		jsonRequest(t, http.MethodPut, "/api/v1/settings/metadata", `{"onlineEnabled":false}`),
		jsonRequest(t, http.MethodGet, "/api/v1/metadata/status", ""),
	}

	for _, request := range requests {
		t.Run(request.Method+" "+request.URL.Path, func(t *testing.T) {
			assertProblem(t, serveRequest(t, handler, request), http.StatusNotImplemented, restapi.CodeNotImplemented)
		})
	}
}

func TestSearchIdentifyCandidates(t *testing.T) {
	t.Parallel()

	longOverview := strings.Repeat("word ", 200)
	identifier := &fakeIdentify{found: []match.Scored{
		{Candidate: match.Candidate{
			Ref: match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"}, Title: "The Matrix", Year: 1999,
			Overview: "A hacker.", Poster: "/matrix.jpg",
		}, Confidence: 0.97},
		{Candidate: match.Candidate{
			Ref: match.ProviderRef{Provider: match.RefIMDb, ID: "tt0133093"}, Title: "Matrix (IMDb)", Overview: longOverview,
		}, Confidence: 1.2},
		{Candidate: match.Candidate{
			Ref:   match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: "b84ee12a-09ef-421b-82de-0441a926375b"},
			Title: "Abbey Road", Poster: "https://coverartarchive.org/x.jpg",
		}, Confidence: 0.5},
		{Candidate: match.Candidate{Ref: match.ProviderRef{Provider: "tvdb", ID: "1"}, Title: "unknown provider"}},
	}}
	posters := &fakePosters{}
	handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Identify: identifier, Posters: posters})

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, searchPath,
		`{"query":"The Matrix","year":1999,"providerId":"https://www.themoviedb.org/movie/603"}`))
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	if strings.Contains(string(got.body), "tmdb.org") || strings.Contains(string(got.body), "coverartarchive") ||
		strings.Contains(string(got.body), "/matrix.jpg") {
		t.Errorf("body names a provider host or path: %s", got.body)
	}

	list := decodeStrictly[apigen.IdentifyCandidateList](t, got.body)
	assertCandidates(t, list.Candidates)

	wantQuery := identify.Query{Text: "The Matrix", Year: 1999, ProviderRef: "https://www.themoviedb.org/movie/603"}
	if len(identifier.queries) != 1 || identifier.queries[0] != wantQuery {
		t.Errorf("queries = %+v, want %+v", identifier.queries, wantQuery)
	}

	wantRef := enrich.ArtworkRef{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/matrix.jpg"}
	if len(posters.registered) != 1 || posters.registered[0] != wantRef {
		t.Errorf("registered posters = %+v, want only %+v", posters.registered, wantRef)
	}
}

func assertCandidates(t *testing.T, candidates []apigen.IdentifyCandidate) {
	t.Helper()

	if len(candidates) != 3 {
		t.Fatalf("candidates = %+v, want 3 (the unknown provider left out)", candidates)
	}

	matrix := candidates[0]
	if matrix.Provider != apigen.MetadataProviderTmdb || matrix.ExternalID != "603" || matrix.Title != "The Matrix" ||
		matrix.Year == nil || *matrix.Year != 1999 || matrix.Overview == nil || *matrix.Overview != "A hacker." ||
		matrix.PosterURL == nil || !strings.HasPrefix(*matrix.PosterURL, "/api/v1/metadata/posters/") || matrix.Confidence != 0.97 {
		t.Errorf("first candidate = %+v", matrix)
	}

	imdb := candidates[1]
	if imdb.Provider != apigen.MetadataProviderTmdb || imdb.ExternalID != "tt0133093" || imdb.Confidence != 1 ||
		imdb.Year != nil || imdb.PosterURL != nil || imdb.Overview == nil {
		t.Errorf("imdb candidate = %+v", imdb)
	}

	if overview := *imdb.Overview; utf8.RuneCountInString(overview) > 501 || !strings.HasSuffix(overview, "…") {
		t.Errorf("long overview not shortened: %d characters", utf8.RuneCountInString(overview))
	}

	if album := candidates[2]; album.Provider != apigen.MetadataProviderMusicbrainz || album.PosterURL != nil {
		t.Errorf("music candidate = %+v, want musicbrainz without poster", album)
	}
}

func TestSearchIdentifyCandidatesWithoutPosterProxyOmitsPosters(t *testing.T) {
	t.Parallel()

	identifier := &fakeIdentify{found: []match.Scored{{Candidate: match.Candidate{
		Ref: match.ProviderRef{Provider: match.RefTMDB, ID: "603"}, Title: "The Matrix", Poster: "/matrix.jpg",
	}}}}
	handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Identify: identifier})

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, searchPath, `{"query":"The Matrix"}`))

	list := decodeStrictly[apigen.IdentifyCandidateList](t, got.body)
	if len(list.Candidates) != 1 || list.Candidates[0].PosterURL != nil {
		t.Errorf("candidates = %+v, want one without posterUrl", list.Candidates)
	}
}

func TestSearchIdentifyCandidatesErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		body           string
		err            error
		wantStatus     int
		wantCode       string
		wantField      string
		wantRetryAfter string
	}{
		{name: "online off", body: `{"query":"x"}`, err: fmt.Errorf("search: %w", domain.ErrConflict), wantStatus: http.StatusConflict, wantCode: restapi.CodeOnlineDisabled},
		{name: "rate limited with pause", body: `{"query":"x"}`, err: providerError(rateLimited{pause: 11500 * time.Millisecond}), wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeProviderUnavailable, wantRetryAfter: "12"},
		{name: "rate limited without pause", body: `{"query":"x"}`, err: providerError(domain.ErrRateLimited), wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeProviderUnavailable, wantRetryAfter: "30"},
		{name: "offline", body: `{"query":"x"}`, err: providerError(domain.ErrUnavailable), wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeProviderUnavailable, wantRetryAfter: "30"},
		{name: "shutdown is no provider failure", body: `{"query":"x"}`, err: domain.ErrUnavailable, wantStatus: http.StatusServiceUnavailable, wantCode: restapi.CodeUnavailable},
		{name: "unknown item", body: `{"query":"x"}`, err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{name: "kind without provider", body: `{"query":"x"}`, err: domain.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "store failure", body: `{"query":"x"}`, err: errors.New("disk"), wantStatus: http.StatusInternalServerError, wantCode: restapi.CodeInternal},
		{name: "empty query", body: `{"query":""}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "query"},
		{name: "long query", body: `{"query":"` + strings.Repeat("q", 257) + `"}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "query"},
		{name: "year too early", body: `{"query":"x","year":1799}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "year"},
		{name: "long provider id", body: `{"query":"x","providerId":"` + strings.Repeat("p", 513) + `"}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "providerId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Identify: &fakeIdentify{err: test.err}})

			got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, searchPath, test.body))
			problem := assertProblem(t, got, test.wantStatus, test.wantCode)

			assertProblemField(t, problem, test.wantField)

			if retryAfter := got.header.Get("Retry-After"); retryAfter != test.wantRetryAfter {
				t.Errorf("Retry-After = %q, want %q", retryAfter, test.wantRetryAfter)
			}

			if test.wantRetryAfter != "" && (problem.RetryAfterSeconds == nil || fmt.Sprint(*problem.RetryAfterSeconds) != test.wantRetryAfter) {
				t.Errorf("retryAfterSeconds = %v, want %s", problem.RetryAfterSeconds, test.wantRetryAfter)
			}
		})
	}
}

func assertProblemField(t *testing.T, problem apigen.Problem, want string) {
	t.Helper()

	got := ""
	if problem.Field != nil {
		got = *problem.Field
	}

	if got != want {
		t.Errorf("problem field = %q, want %q", got, want)
	}
}

func detailService() *fakeLibraryService {
	return &fakeLibraryService{detail: applibrary.ItemDetail{Item: testItem(testItemID, "Matrix"), Files: []media.File{unprobedFile()}}}
}

func TestIdentifyItemAnswersTheMatchedItem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want appliedMatch
	}{
		{"tmdb", `{"provider":"tmdb","externalId":"603"}`, appliedMatch{testItemID, match.RefTMDB, "603"}},
		{"imdb through tmdb", `{"provider":"tmdb","externalId":"tt0133093"}`, appliedMatch{testItemID, match.RefIMDb, "tt0133093"}},
		{"musicbrainz", `{"provider":"musicbrainz","externalId":"b84ee12a-09ef-421b-82de-0441a926375b"}`, appliedMatch{testItemID, match.RefMusicBrainz, "b84ee12a-09ef-421b-82de-0441a926375b"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			identifier := &fakeIdentify{}
			matches := fakeMatches{statuses: map[string]match.ItemStatus{testItemID: {State: match.MatchStateMatched, Provider: match.RefTMDB}}}
			handler, _ := newMetadataHandler(t, detailService(), restapi.MetadataServices{Identify: identifier, Matches: matches})

			got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, identifyPath, test.body))
			if got.status != http.StatusOK {
				t.Fatalf("status = %d, body %s", got.status, got.body)
			}

			detail := decodeStrictly[apigen.ItemDetail](t, got.body)
			if detail.MatchState == nil || *detail.MatchState != apigen.MatchStateMatched ||
				detail.MetadataSources == nil || strings.Join(*detail.MetadataSources, ",") != "tmdb" {
				t.Errorf("detail match = %v / %v, want matched from tmdb", detail.MatchState, detail.MetadataSources)
			}

			if len(identifier.applied) != 1 || identifier.applied[0] != test.want {
				t.Errorf("applied = %+v, want %+v", identifier.applied, test.want)
			}
		})
	}
}

func TestIdentifyItemErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		err        error
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{name: "online off", body: `{"provider":"tmdb","externalId":"603"}`, err: domain.ErrConflict, wantStatus: http.StatusConflict, wantCode: restapi.CodeOnlineDisabled},
		{name: "unknown item", body: `{"provider":"tmdb","externalId":"603"}`, err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: restapi.CodeNotFound},
		{name: "wrong kind", body: `{"provider":"tmdb","externalId":"603"}`, err: domain.ErrInvalid, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid},
		{name: "cover art archive", body: `{"provider":"coverartarchive","externalId":"x"}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "provider"},
		{name: "unknown provider", body: `{"provider":"tvdb","externalId":"x"}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "provider"},
		{name: "empty id", body: `{"provider":"tmdb","externalId":" "}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "externalId"},
		{name: "long id", body: `{"provider":"tmdb","externalId":"` + strings.Repeat("1", 129) + `"}`, wantStatus: http.StatusBadRequest, wantCode: restapi.CodeInvalid, wantField: "externalId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler, _ := newMetadataHandler(t, detailService(), restapi.MetadataServices{Identify: &fakeIdentify{err: test.err}})

			got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, identifyPath, test.body))
			assertProblemField(t, assertProblem(t, got, test.wantStatus, test.wantCode), test.wantField)

			if got.header.Get("Retry-After") != "" {
				t.Error("identify answered Retry-After although it never calls the provider")
			}
		})
	}
}

func TestIdentifyItemDetailFailure(t *testing.T) {
	t.Parallel()

	items := &fakeLibraryService{itemErr: domain.ErrNotFound}
	handler, _ := newMetadataHandler(t, items, restapi.MetadataServices{Identify: &fakeIdentify{}})

	got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, identifyPath, `{"provider":"tmdb","externalId":"603"}`))
	assertProblem(t, got, http.StatusNotFound, restapi.CodeNotFound)
}

func TestRemoveItemMatch(t *testing.T) {
	t.Parallel()

	identifier := &fakeIdentify{}
	matches := fakeMatches{statuses: map[string]match.ItemStatus{testItemID: {State: match.MatchStateDisabled}}}
	handler, _ := newMetadataHandler(t, detailService(), restapi.MetadataServices{Identify: identifier, Matches: matches})

	got := serveRequest(t, handler, jsonRequest(t, http.MethodDelete, matchPath, ""))
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body %s", got.status, got.body)
	}

	detail := decodeStrictly[apigen.ItemDetail](t, got.body)
	if detail.MatchState == nil || *detail.MatchState != apigen.MatchStateDisabled || detail.MetadataSources != nil {
		t.Errorf("detail match = %v / %v, want disabled without sources", detail.MatchState, detail.MetadataSources)
	}

	if len(identifier.unmatched) != 1 || identifier.unmatched[0] != testItemID {
		t.Errorf("unmatched = %v", identifier.unmatched)
	}

	unknown, _ := newMetadataHandler(t, detailService(), restapi.MetadataServices{Identify: &fakeIdentify{err: domain.ErrNotFound}})
	assertProblem(t, serveRequest(t, unknown, jsonRequest(t, http.MethodDelete, matchPath, "")), http.StatusNotFound, restapi.CodeNotFound)
}

func TestRefreshMetadata(t *testing.T) {
	t.Parallel()

	paths := map[string]string{
		"item:" + testItemID:       itemPath + "/refresh",
		"library:" + testLibraryID: libraryPath + "/refresh",
	}

	for want, path := range paths {
		t.Run(path, func(t *testing.T) {
			t.Parallel()

			refresher := &fakeIdentify{}
			handler, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Refresh: refresher})

			got := serveRequest(t, handler, jsonRequest(t, http.MethodPost, path, ""))
			if got.status != http.StatusAccepted || len(refresher.refreshed) != 1 || refresher.refreshed[0] != want {
				t.Errorf("status = %d, refreshed = %v, want 202 and %s", got.status, refresher.refreshed, want)
			}

			for err, wantStatus := range map[error]int{
				domain.ErrConflict: http.StatusConflict, domain.ErrNotFound: http.StatusNotFound, errors.New("disk"): http.StatusInternalServerError,
			} {
				failing, _ := newMetadataHandler(t, nil, restapi.MetadataServices{Refresh: &fakeIdentify{err: err}})
				if got := serveRequest(t, failing, jsonRequest(t, http.MethodPost, path, "")); got.status != wantStatus {
					t.Errorf("%v: status = %d, want %d", err, got.status, wantStatus)
				}
			}
		})
	}
}

package tmdb_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/tmdb"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

func TestShowSearchUsesFirstAirYear(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/search/tv", fixture: "search_tv.json"})

	found, err := newClient(fake, testKey).Shows().Search(t.Context(), enrich.SearchQuery{Title: "Breaking Bad", Year: 2008, Language: "pt-BR"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := []match.Candidate{{Ref: tmdbRef(match.RefKindShow, "1396"), Title: "Breaking Bad", OriginalTitle: "Breaking Bad", Year: 2008, Popularity: 300.2}}
	if !slices.Equal(found, want) {
		t.Errorf("candidates = %+v", found)
	}

	query := fake.recorded()[0].query
	if query.Get("first_air_date_year") != "2008" || query.Has("year") || query.Get("language") != "pt-BR" {
		t.Errorf("query = %v", query)
	}

	fake.assertPrivacy()
}

func TestShowDetails(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/tv/1396", fixture: "tv_1396.json"})

	details, err := newClient(fake, testKey).Shows().Details(t.Context(), enrich.DetailsRequest{Ref: tmdbRef(match.RefKindShow, "1396"), Language: "de"})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	record := details.Record
	assertText(t, "title", record.Title, "Breaking Bad")
	assertText(t, "content rating", record.ContentRating, "16")
	assertText(t, "first air date", record.ReleaseDate.String(), "2008-01-20")
	assertList(t, "studios", record.Studios, []string{"AMC"})

	wantIDs := []metadata.ProviderID{
		{Provider: metadata.ProviderTMDB, Value: "1396"},
		{Provider: metadata.ProviderIMDb, Value: "tt0903747"},
		{Provider: metadata.ProviderTVDB, Value: "81189"},
	}
	if !slices.Equal(record.ProviderIDs, wantIDs) {
		t.Errorf("provider IDs = %v", record.ProviderIDs)
	}

	if len(record.People) != 2 {
		t.Errorf("people = %+v, want the two actors (executive producers are not producers)", record.People)
	}

	kinds := make([]artwork.Kind, 0, len(details.Artwork))
	for _, ref := range details.Artwork {
		kinds = append(kinds, ref.Kind)
	}

	if !slices.Equal(kinds, []artwork.Kind{artwork.KindPoster, artwork.KindPoster, artwork.KindBackdrop, artwork.KindLogo}) {
		t.Errorf("artwork kinds = %v", kinds)
	}

	assertText(t, "append", fake.recorded()[0].query.Get("append_to_response"), "credits,external_ids,content_ratings,images")
}

func TestShowContentRatingFallsBackToUS(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/tv/1396", fixture: "tv_1396.json"})

	details, err := newClient(fake, testKey).Shows().Details(t.Context(), enrich.DetailsRequest{Ref: tmdbRef(match.RefKindShow, "1396"), Language: "ja"})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	assertText(t, "content rating", details.Record.ContentRating, "TV-MA")
}

func TestSeasonDetailsResolvesIMDbAndFillsFromEnglish(t *testing.T) {
	fake := newFakeTMDB(t,
		route{path: "/3/find/tt0903747", fixture: "find_show.json"},
		route{path: "/3/tv/1396/season/1", language: "de", fixture: "tv_1396_season_1_de.json"},
		route{path: "/3/tv/1396/season/1", language: "en-US", fixture: "tv_1396_season_1.json"},
	)

	season, err := newClient(fake, testKey).Shows().SeasonDetails(t.Context(), enrich.SeasonRequest{
		Show: match.ProviderRef{Provider: match.RefIMDb, ID: "tt0903747"}, Season: 1, Language: "de",
	})
	if err != nil {
		t.Fatalf("SeasonDetails: %v", err)
	}

	assertText(t, "season title", season.Record.Title, "Staffel 1")
	assertText(t, "season summary from English", season.Record.Summary,
		"High school chemistry teacher Walter White's life is suddenly transformed by a dire medical diagnosis.")

	if len(season.Episodes) != 2 {
		t.Fatalf("episodes = %d, want 2", len(season.Episodes))
	}

	first, second := season.Episodes[0], season.Episodes[1]
	assertText(t, "translated episode title", first.Record.Title, "Der Einstieg")
	assertText(t, "English episode title", second.Record.Title, "Cat's in the Bag...")
	assertText(t, "English episode summary", second.Record.Summary, "Walt and Jesse attempt to tie up loose ends.")

	if first.Number != 1 || second.Number != 2 || len(second.Artwork) != 0 || first.Artwork[0].Kind != artwork.KindThumb {
		t.Errorf("episodes = %+v", season.Episodes)
	}

	if len(fake.recorded()) != 3 {
		t.Errorf("requests = %d, want find, German season, English season", len(fake.recorded()))
	}

	fake.assertPrivacy()
}

func TestSeasonDetailsInEnglishNeedsOneRequest(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/tv/1396/season/1", fixture: "tv_1396_season_1.json"})

	season, err := newClient(fake, testKey).Shows().SeasonDetails(t.Context(), enrich.SeasonRequest{Show: tmdbRef(match.RefKindShow, "1396"), Season: 1, Language: "en"})
	if err != nil {
		t.Fatalf("SeasonDetails: %v", err)
	}

	pilot := season.Episodes[0].Record
	wantPeople := []metadata.Credit{
		{Name: "Vince Gilligan", Role: metadata.RoleDirector, Order: 0},
		{Name: "Vince Gilligan", Role: metadata.RoleWriter, Order: 1},
		{Name: "Max Arciniega", Role: metadata.RoleGuestStar, Character: "Krazy-8", Order: 0},
	}

	if !slices.Equal(pilot.People, wantPeople) || pilot.Year != 2008 {
		t.Errorf("pilot = %+v", pilot)
	}

	if season.Artwork[0].Kind != artwork.KindPoster || len(fake.recorded()) != 1 {
		t.Errorf("season artwork %+v after %d requests", season.Artwork, len(fake.recorded()))
	}
}

func TestSeasonDetailsKeepsTranslationWhenEnglishFails(t *testing.T) {
	fake := newFakeTMDB(t,
		route{path: "/3/tv/1396/season/1", language: "de", fixture: "tv_1396_season_1_de.json"},
		route{path: "/3/tv/1396/season/1", language: "en-US", status: 404, fixture: "error_404.json"},
	)

	season, err := newClient(fake, testKey).Shows().SeasonDetails(t.Context(), enrich.SeasonRequest{Show: tmdbRef(match.RefKindShow, "1396"), Season: 1, Language: "de"})
	if err != nil {
		t.Fatalf("SeasonDetails: %v", err)
	}

	assertText(t, "season title", season.Record.Title, "Staffel 1")
	assertText(t, "summary without fallback", season.Record.Summary, "")
}

func TestSeasonDetailsStopsOnCancellationDuringFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	fake := newFakeTMDB(t, route{path: "/3/tv/1396/season/1", language: "de", fixture: "tv_1396_season_1_de.json"})
	// The key is read before every request; the second read cancels, so the English
	// request starts with a cancelled context.
	client := tmdb.New(tmdb.Options{HTTP: newOutbound(), Keys: &cancellingKey{cancel: cancel, after: 1}, BaseURL: fake.server.URL + "/3"})

	_, err := client.Shows().SeasonDetails(ctx, enrich.SeasonRequest{Show: tmdbRef(match.RefKindShow, "1396"), Season: 1, Language: "de"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestSeasonDetailsRejectsNegativeSeasonAndMovieRefs(t *testing.T) {
	fake := newFakeTMDB(t)
	shows := newClient(fake, testKey).Shows()

	for _, request := range []enrich.SeasonRequest{
		{Show: tmdbRef(match.RefKindShow, "1396"), Season: -1},
		{Show: tmdbRef(match.RefKindMovie, "603"), Season: 1},
	} {
		if _, err := shows.SeasonDetails(t.Context(), request); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("SeasonDetails(%+v) err = %v, want ErrInvalid", request, err)
		}
	}

	if len(fake.recorded()) != 0 {
		t.Errorf("requests = %d, want none", len(fake.recorded()))
	}
}

func TestFindByExternalID(t *testing.T) {
	fake := newFakeTMDB(t,
		route{path: "/3/find/81189", fixture: "find_show.json"},
		route{path: "/3/find/tt0000001", fixture: "find_empty.json"},
	)
	client := newClient(fake, testKey)

	ref, err := client.FindByExternalID(t.Context(), metadata.ProviderID{Provider: metadata.ProviderTVDB, Value: "81189"}, match.RefKindShow)
	if err != nil || ref != tmdbRef(match.RefKindShow, "1396") {
		t.Errorf("TVDB find = %+v, %v", ref, err)
	}

	assertText(t, "external source", fake.recorded()[0].query.Get("external_source"), "tvdb_id")

	_, err = client.FindByExternalID(t.Context(), metadata.ProviderID{Provider: metadata.ProviderTVDB, Value: "81189"}, match.RefKindMovie)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("show ID as movie: err = %v, want ErrNotFound", err)
	}

	_, err = client.FindByExternalID(t.Context(), metadata.ProviderID{Provider: metadata.ProviderIMDb, Value: "tt0000001"}, match.RefKindMovie)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("unknown ID: err = %v, want ErrNotFound", err)
	}

	for _, id := range []metadata.ProviderID{
		{Provider: metadata.ProviderTMDB, Value: "603"},
		{Provider: metadata.ProviderIMDb, Value: "../x"},
		{Provider: "tvmaze", Value: "1"},
	} {
		if _, err := client.FindByExternalID(t.Context(), id, match.RefKindMovie); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("FindByExternalID(%v) err = %v, want ErrInvalid", id, err)
		}
	}

	fake.assertPrivacy()
}

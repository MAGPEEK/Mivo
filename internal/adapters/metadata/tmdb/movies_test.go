package tmdb_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

func tmdbRef(kind match.RefKind, id string) match.ProviderRef {
	return match.ProviderRef{Provider: match.RefTMDB, Kind: kind, ID: id}
}

func TestMovieSearchSendsOnlyTitleYearLanguage(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/search/movie", fixture: "search_movie.json"})

	found, err := newClient(fake, testKey).Movies().Search(t.Context(), enrich.SearchQuery{Title: " The Matrix ", Year: 1999, Language: "de"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	want := []match.Candidate{
		{
			Ref: tmdbRef(match.RefKindMovie, "603"), Title: "The Matrix", OriginalTitle: "The Matrix", Year: 1999, Popularity: 85.3,
			Overview: "A hacker learns the truth about his reality.", Poster: "/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
		},
		// The poster path that would leave the image base is dropped.
		{Ref: tmdbRef(match.RefKindMovie, "604"), Title: "The Matrix Reloaded", OriginalTitle: "The Matrix Reloaded", Year: 2003, Popularity: 40.1},
		{Ref: tmdbRef(match.RefKindMovie, "1000001"), Title: "The Matrix Revisited", OriginalTitle: "The Matrix Revisited", Popularity: 2.5},
	}
	if !slices.Equal(found, want) {
		t.Errorf("candidates = %+v\nwant %+v", found, want)
	}

	query := fake.recorded()[0].query
	if query.Get("query") != "The Matrix" || query.Get("year") != "1999" || query.Get("language") != "de" || query.Get("api_key") != testKey {
		t.Errorf("query = %v", query)
	}

	fake.assertPrivacy()
}

func TestMovieSearchWithoutYearAndEmptyResult(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/search/movie", fixture: "search_empty.json"})

	found, err := newClient(fake, testKey).Movies().Search(t.Context(), enrich.SearchQuery{Title: "Nothing"})
	if err != nil || len(found) != 0 {
		t.Fatalf("Search = %v, %v; want empty", found, err)
	}

	query := fake.recorded()[0].query
	if query.Has("year") || query.Get("language") != "en-US" {
		t.Errorf("query = %v, want no year and the English default", query)
	}
}

func TestMovieSearchRejectsEmptyTitle(t *testing.T) {
	fake := newFakeTMDB(t)

	_, err := newClient(fake, testKey).Movies().Search(t.Context(), enrich.SearchQuery{Title: "  "})
	if !errors.Is(err, domain.ErrInvalid) || len(fake.recorded()) != 0 {
		t.Errorf("err = %v after %d requests, want ErrInvalid without a request", err, len(fake.recorded()))
	}
}

func TestMovieDetailsWithCollection(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/movie/603", fixture: "movie_603.json"})

	details, err := newClient(fake, testKey).Movies().Details(t.Context(), enrich.DetailsRequest{Ref: tmdbRef(match.RefKindMovie, "603"), Language: "en"})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	record := details.Record
	assertText(t, "title", record.Title, "The Matrix")
	assertText(t, "tagline", record.Tagline, "Welcome to the Real World.")
	assertText(t, "content rating", record.ContentRating, "R")
	assertText(t, "release date", record.ReleaseDate.String(), "1999-03-31")
	assertList(t, "genres", record.Genres, []string{"Action", "Science Fiction"})
	assertList(t, "studios", record.Studios, []string{"Village Roadshow Pictures", "Warner Bros. Pictures"})
	assertList(t, "countries", record.Countries, []string{"United States of America"})
	assertList(t, "collections", record.Collections, []string{"The Matrix Collection"})

	if record.Year != 1999 || record.Summary == "" {
		t.Errorf("year %d, summary %q", record.Year, record.Summary)
	}

	wantIDs := []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: "603"}, {Provider: metadata.ProviderIMDb, Value: "tt0133093"}}
	if !slices.Equal(record.ProviderIDs, wantIDs) {
		t.Errorf("provider IDs = %v", record.ProviderIDs)
	}

	wantPeople := []metadata.Credit{
		{Name: "Keanu Reeves", Role: metadata.RoleActor, Character: "Neo", Order: 0},
		{Name: "Laurence Fishburne", Role: metadata.RoleActor, Character: "Morpheus", Order: 1},
		{Name: "Lana Wachowski", Role: metadata.RoleDirector, Order: 0},
		{Name: "Lilly Wachowski", Role: metadata.RoleDirector, Order: 1},
		{Name: "Lana Wachowski", Role: metadata.RoleWriter, Order: 2},
		{Name: "Joel Silver", Role: metadata.RoleProducer, Order: 3},
	}
	if !slices.Equal(record.People, wantPeople) {
		t.Errorf("people = %+v", record.People)
	}

	wantArtwork := []enrich.ArtworkRef{
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindPoster, Path: "/aOIuZAjPaRIE6CMzbazvcHuHXDc.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindBackdrop, Path: "/fNG7i7RqMErkcqhohV2a6cV1Ehy.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindBackdrop, Path: "/l4QHerTSbMI7qgvasqxP36pqjN6.jpg"},
		{Provider: enrich.ArtworkProviderTMDB, Kind: artwork.KindLogo, Path: "/hXbnbLpTBGVAdAnyMJdvn8rpTvz.png"},
	}
	if !slices.Equal(details.Artwork, wantArtwork) {
		t.Errorf("artwork = %+v", details.Artwork)
	}

	query := fake.recorded()[0].query
	assertText(t, "append", query.Get("append_to_response"), "credits,external_ids,release_dates,images")
	assertText(t, "image languages", query.Get("include_image_language"), "en,null")
	fake.assertPrivacy()
}

func TestMovieDetailsWithoutCollection(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/movie/550", fixture: "movie_550.json"})

	details, err := newClient(fake, testKey).Movies().Details(t.Context(), enrich.DetailsRequest{Ref: match.ProviderRef{Provider: match.RefTMDB, ID: "550"}})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	if len(details.Record.Collections) != 0 {
		t.Errorf("collections = %v, want none", details.Record.Collections)
	}

	if id, _ := details.Record.ProviderID(metadata.ProviderIMDb); id != "tt0137523" {
		t.Errorf("IMDb ID = %q, want the top-level imdb_id", id)
	}

	if len(details.Artwork) != 1 || details.Artwork[0].Kind != artwork.KindPoster {
		t.Errorf("artwork = %+v, want the poster only", details.Artwork)
	}
}

func TestMovieDetailsInGerman(t *testing.T) {
	fake := newFakeTMDB(t, route{path: "/3/movie/603", fixture: "movie_603_de.json"})

	details, err := newClient(fake, testKey).Movies().Details(t.Context(), enrich.DetailsRequest{Ref: tmdbRef(match.RefKindMovie, "603"), Language: "de-de"})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	assertText(t, "title", details.Record.Title, "Matrix")
	assertText(t, "original title", details.Record.OriginalTitle, "The Matrix")
	assertText(t, "content rating", details.Record.ContentRating, "16")

	query := fake.recorded()[0].query
	assertText(t, "language", query.Get("language"), "de-DE")
	assertText(t, "image languages", query.Get("include_image_language"), "de,null,en")
}

func TestMovieDetailsResolvesIMDbThroughFind(t *testing.T) {
	fake := newFakeTMDB(t,
		route{path: "/3/find/tt0133093", fixture: "find_movie.json"},
		route{path: "/3/movie/603", fixture: "movie_603.json"},
	)

	details, err := newClient(fake, testKey).Movies().Details(t.Context(), enrich.DetailsRequest{Ref: match.ProviderRef{Provider: match.RefIMDb, ID: "tt0133093"}})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	assertText(t, "title", details.Record.Title, "The Matrix")

	requests := fake.recorded()
	if len(requests) != 2 || requests[0].query.Get("external_source") != "imdb_id" {
		t.Errorf("requests = %+v, want find then details", requests)
	}

	fake.assertPrivacy()
}

func TestMovieDetailsRejectsForeignReferences(t *testing.T) {
	tests := []struct {
		name string
		ref  match.ProviderRef
	}{
		{"show reference", tmdbRef(match.RefKindShow, "1396")},
		{"malformed TMDB ID", tmdbRef(match.RefKindMovie, "abc")},
		{"malformed IMDb ID", match.ProviderRef{Provider: match.RefIMDb, ID: "603"}},
		{"MusicBrainz", match.ProviderRef{Provider: match.RefMusicBrainz, ID: "5b11f4ce-a62d-471e-81fc-a69a8278c7da"}},
		{"unknown provider", match.ProviderRef{Provider: "tvmaze", ID: "1"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFakeTMDB(t)

			_, err := newClient(fake, testKey).Movies().Details(t.Context(), enrich.DetailsRequest{Ref: test.ref})
			if !errors.Is(err, domain.ErrInvalid) || len(fake.recorded()) != 0 {
				t.Errorf("err = %v after %d requests, want ErrInvalid without a request", err, len(fake.recorded()))
			}
		})
	}
}

func assertText(t *testing.T, name, got, want string) {
	t.Helper()

	if got != want {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

func assertList(t *testing.T, name string, got, want []string) {
	t.Helper()

	if !slices.Equal(got, want) {
		t.Errorf("%s = %q, want %q", name, got, want)
	}
}

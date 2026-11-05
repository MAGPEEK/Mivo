package musicbrainz_test

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

func notFound(path string) route {
	return route{path: path, status: http.StatusNotFound, fixture: "error_404.json"}
}

func TestDetailsReleaseGroup(t *testing.T) {
	fake := newFake(t, route{path: "/release-group/" + okComputerGroup, fixture: "release_group_" + okComputerGroup + ".json"})

	details, err := newClient(fake, newFakeClock()).Details(t.Context(),
		enrich.DetailsRequest{Ref: mbRef(match.RefKindRelease, strings.ToUpper(okComputerGroup)), Language: "de"})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	record := details.Record
	if record.Title != "OK Computer" || record.Year != 1997 || record.ReleaseDate.String() != "1997-05-21" {
		t.Errorf("record = %+v, want title, year and date of the release group", record)
	}

	if want := []string{"art rock", "alternative rock"}; !slices.Equal(record.Genres, want) {
		t.Errorf("genres = %v, want %v (by votes, case-insensitive duplicates dropped)", record.Genres, want)
	}

	if len(record.ProviderIDs) != 0 || len(record.People) != 0 {
		t.Errorf("record = %+v, want no provider IDs or people (no matching metadata field)", record)
	}

	wantArt := []enrich.ArtworkRef{{
		Provider: enrich.ArtworkProviderCoverArt, Kind: artwork.KindPoster,
		Path: "https://coverartarchive.org/release-group/" + okComputerGroup + "/front-1200",
	}}
	if !slices.Equal(details.Artwork, wantArt) {
		t.Errorf("artwork = %+v, want %+v", details.Artwork, wantArt)
	}

	requests := fake.recorded()
	if len(requests) != 1 || !strings.Contains(requests[0].query.Get("inc"), "genres") {
		t.Errorf("requests = %+v, want one lookup including genres", requests)
	}

	fake.assertPrivacy()
}

func TestDetailsReleaseFallsBackFromGroup(t *testing.T) {
	fake := newFake(t,
		notFound("/release-group/"+okComputerRelease),
		route{path: "/release/" + okComputerRelease, fixture: "release_" + okComputerRelease + ".json"},
	)

	details, err := newClient(fake, newFakeClock()).Details(t.Context(), enrich.DetailsRequest{Ref: mbRef(match.RefKindRelease, okComputerRelease)})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	record := details.Record
	if record.Title != "OK Computer" || record.Year != 1997 || record.ReleaseDate.String() != "1997-05-21" {
		t.Errorf("record = %+v, want the first release of the release group, not the 2009 edition", record)
	}

	if !slices.Equal(record.Studios, []string{"Parlophone"}) {
		t.Errorf("studios = %v, want the label once", record.Studios)
	}

	if !slices.Equal(record.Genres, []string{"rock", "remaster"}) {
		t.Errorf("genres = %v, want tags when no genres are voted", record.Genres)
	}

	wantPath := "https://coverartarchive.org/release/" + okComputerRelease + "/front-1200"
	if len(details.Artwork) != 1 || details.Artwork[0].Path != wantPath {
		t.Errorf("artwork = %+v, want %s", details.Artwork, wantPath)
	}

	fake.assertPrivacy()
}

func TestDetailsArtist(t *testing.T) {
	fake := newFake(t, route{path: "/artist/" + radioheadArtist, fixture: "artist_" + radioheadArtist + ".json"})

	details, err := newClient(fake, newFakeClock()).Details(t.Context(), enrich.DetailsRequest{Ref: mbRef(match.RefKindArtist, radioheadArtist)})
	if err != nil {
		t.Fatalf("Details: %v", err)
	}

	want := metadata.Record{
		Title: "Radiohead", SortTitle: "Radiohead",
		Genres: []string{"alternative rock", "art rock"}, Countries: []string{"United Kingdom"},
	}
	if got := details.Record; got.Title != want.Title || got.SortTitle != want.SortTitle || got.Year != 0 ||
		!slices.Equal(got.Genres, want.Genres) || !slices.Equal(got.Countries, want.Countries) {
		t.Errorf("record = %+v, want %+v", got, want)
	}

	if len(details.Artwork) != 0 {
		t.Errorf("artwork = %+v, want none (Cover Art Archive has no artist pictures)", details.Artwork)
	}
}

func TestDetailsUnknownKindTriesEveryEntity(t *testing.T) {
	fake := newFake(t,
		notFound("/release-group/"+radioheadArtist),
		notFound("/release/"+radioheadArtist),
		route{path: "/artist/" + radioheadArtist, fixture: "artist_" + radioheadArtist + ".json"},
	)

	details, err := newClient(fake, newFakeClock()).Details(t.Context(), enrich.DetailsRequest{Ref: mbRef(match.RefKindUnknown, radioheadArtist)})
	if err != nil || details.Record.Title != "Radiohead" {
		t.Fatalf("Details = %+v, %v; want the artist", details.Record, err)
	}

	if got := len(fake.recorded()); got != 3 {
		t.Errorf("requests = %d, want 3", got)
	}
}

func TestDetailsNotFound(t *testing.T) {
	fake := newFake(t, notFound("/release-group/"+okComputerGroup), notFound("/release/"+okComputerGroup))

	_, err := newClient(fake, newFakeClock()).Details(t.Context(), enrich.DetailsRequest{Ref: mbRef(match.RefKindRelease, okComputerGroup)})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDetailsRejectsInvalidReferences(t *testing.T) {
	tests := []struct {
		name string
		ref  match.ProviderRef
	}{
		{"tmdb reference", match.ProviderRef{Provider: match.RefTMDB, Kind: match.RefKindMovie, ID: "603"}},
		{"not a uuid", mbRef(match.RefKindRelease, "../../admin")},
		{"show kind", mbRef(match.RefKindShow, okComputerGroup)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fake := newFake(t)

			_, err := newClient(fake, newFakeClock()).Details(t.Context(), enrich.DetailsRequest{Ref: test.ref})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}

			if len(fake.recorded()) != 0 {
				t.Error("an invalid reference must not reach MusicBrainz")
			}
		})
	}
}

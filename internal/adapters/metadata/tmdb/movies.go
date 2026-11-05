package tmdb

import (
	"context"
	"fmt"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

var _ enrich.MovieLookup = Movies{}

const appendReleaseDates = "release_dates"

// Movies implements enrich.MovieLookup.
type Movies struct {
	client *Client
}

// Movies returns the movie lookup of the client.
func (c *Client) Movies() Movies {
	return Movies{client: c}
}

// Search returns TMDB's movie candidates for a title and year, in TMDB's order.
func (movies Movies) Search(ctx context.Context, query enrich.SearchQuery) ([]match.Candidate, error) {
	return movies.client.search(ctx, match.RefKindMovie, searchParams{
		title: query.Title, year: query.Year, language: parseLanguage(query.Language),
	})
}

// Details returns the movie a TMDB or IMDb reference points to.
func (movies Movies) Details(ctx context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	id, err := movies.client.tmdbID(ctx, request.Ref, match.RefKindMovie)
	if err != nil {
		return enrich.Details{}, fmt.Errorf("movie details: %w", err)
	}

	lang := parseLanguage(request.Language)

	var details movieDetails
	if err := movies.client.getJSON(ctx, "/movie/"+id, detailsParams(lang, appendReleaseDates), &details); err != nil {
		return enrich.Details{}, fmt.Errorf("movie details: %w", err)
	}

	return enrich.Details{
		Record:  movieRecord(details, lang),
		Artwork: artworkRefs(details.PosterPath, details.BackdropPath, details.Images),
	}, nil
}

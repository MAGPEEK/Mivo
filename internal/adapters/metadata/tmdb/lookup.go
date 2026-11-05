package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	paramQuery           = "query"
	paramLanguage        = "language"
	paramAppend          = "append_to_response"
	paramImageLanguage   = "include_image_language"
	paramExternalSource  = "external_source"
	paramIncludeAdult    = "include_adult"
	paramMovieYear       = "year"
	paramFirstAirYear    = "first_air_date_year"
	sourceIMDb           = "imdb_id"
	sourceTVDB           = "tvdb_id"
	includeAdultDisabled = "false"
)

// kindPath is the TMDB path segment of a reference kind ("movie" or "tv").
func kindPath(kind match.RefKind) string {
	return string(kind)
}

// search asks TMDB for movies or shows by title and, when known, year.
func (c *Client) search(ctx context.Context, kind match.RefKind, query searchParams) ([]match.Candidate, error) {
	title := strings.TrimSpace(query.title)
	if title == "" {
		return nil, fmt.Errorf("tmdb search %s: empty title: %w", kind, domain.ErrInvalid)
	}

	params := url.Values{paramQuery: {title}, paramLanguage: {query.language.tag}, paramIncludeAdult: {includeAdultDisabled}}
	if query.year > 0 {
		params.Set(yearParam(kind), strconv.Itoa(query.year))
	}

	var response searchResponse
	if err := c.getJSON(ctx, "/search/"+kindPath(kind), params, &response); err != nil {
		return nil, err
	}

	return candidates(kind, response.Results), nil
}

// searchParams is the validated form of enrich.SearchQuery.
type searchParams struct {
	title    string
	year     int
	language language
}

func yearParam(kind match.RefKind) string {
	if kind == match.RefKindShow {
		return paramFirstAirYear
	}

	return paramMovieYear
}

func candidates(kind match.RefKind, results []searchResult) []match.Candidate {
	found := make([]match.Candidate, 0, len(results))

	for _, result := range results {
		if result.ID <= 0 {
			continue
		}

		candidate := match.Candidate{
			Ref:           match.ProviderRef{Provider: match.RefTMDB, Kind: kind, ID: formatID(result.ID)},
			Title:         result.Title,
			OriginalTitle: result.OriginalTitle,
			Popularity:    result.Popularity,
			Overview:      result.Overview,
		}
		// Only a plain TMDB file name may become a poster reference; ImageURL and the
		// artwork fetcher check it again before any request.
		if isFilePath(result.PosterPath) {
			candidate.Poster = result.PosterPath
		}
		date := result.ReleaseDate

		if kind == match.RefKindShow {
			candidate.Title, candidate.OriginalTitle, date = result.Name, result.OriginalName, result.FirstAirDate
		}

		_, candidate.Year = parseDate(date)
		found = append(found, candidate)
	}

	return found
}

// FindByExternalID resolves an IMDb or TVDB ID to the TMDB movie or show it belongs to
// (GET /find). An ID TMDB does not know, or knows only for the other kind, wraps
// domain.ErrNotFound.
func (c *Client) FindByExternalID(ctx context.Context, id metadata.ProviderID, kind match.RefKind) (match.ProviderRef, error) {
	source, err := externalSource(id)
	if err != nil {
		return match.ProviderRef{}, err
	}

	var response findResponse
	if err := c.getJSON(ctx, "/find/"+url.PathEscape(id.Value), url.Values{paramExternalSource: {source}}, &response); err != nil {
		return match.ProviderRef{}, err
	}

	results := response.MovieResults
	if kind == match.RefKindShow {
		results = response.TVResults
	}

	for _, result := range results {
		if result.ID > 0 {
			return match.ProviderRef{Provider: match.RefTMDB, Kind: kind, ID: formatID(result.ID)}, nil
		}
	}

	return match.ProviderRef{}, fmt.Errorf("tmdb find %s %s %s: %w", kind, id.Provider, id.Value, domain.ErrNotFound)
}

func externalSource(id metadata.ProviderID) (string, error) {
	if err := metadata.ValidateProviderID(id.Provider, id.Value); err != nil {
		return "", fmt.Errorf("tmdb find: %w", err)
	}

	switch id.Provider {
	case metadata.ProviderIMDb:
		return sourceIMDb, nil
	case metadata.ProviderTVDB:
		return sourceTVDB, nil
	case metadata.ProviderTMDB:
		return "", fmt.Errorf("tmdb find: %s is not an external ID: %w", id.Provider, domain.ErrInvalid)
	default:
		return "", fmt.Errorf("tmdb find: unknown provider: %w", domain.ErrInvalid)
	}
}

// tmdbID returns the TMDB ID of kind a reference points to, asking /find for an IMDb ID.
func (c *Client) tmdbID(ctx context.Context, ref match.ProviderRef, kind match.RefKind) (string, error) {
	switch ref.Provider {
	case match.RefTMDB:
		if ref.Kind != match.RefKindUnknown && ref.Kind != kind {
			return "", fmt.Errorf("tmdb %s lookup of a %s reference: %w", kind, ref.Kind, domain.ErrInvalid)
		}

		if err := metadata.ValidateProviderID(metadata.ProviderTMDB, ref.ID); err != nil {
			return "", fmt.Errorf("tmdb %s lookup: %w", kind, err)
		}

		return ref.ID, nil
	case match.RefIMDb:
		found, err := c.FindByExternalID(ctx, metadata.ProviderID{Provider: metadata.ProviderIMDb, Value: ref.ID}, kind)

		return found.ID, err
	case match.RefMusicBrainz:
		return "", fmt.Errorf("tmdb %s lookup of a %s reference: %w", kind, ref.Provider, domain.ErrInvalid)
	default:
		return "", fmt.Errorf("tmdb %s lookup: unknown provider: %w", kind, domain.ErrInvalid)
	}
}

// detailsParams asks for details with everything the record needs in one request.
func detailsParams(lang language, extra string) url.Values {
	return url.Values{
		paramLanguage:      {lang.tag},
		paramAppend:        {"credits,external_ids," + extra + ",images"},
		paramImageLanguage: {lang.imageLanguages()},
	}
}

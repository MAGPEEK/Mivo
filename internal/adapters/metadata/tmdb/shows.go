package tmdb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

var _ enrich.ShowLookup = Shows{}

const appendContentRatings = "content_ratings"

// Shows implements enrich.ShowLookup.
type Shows struct {
	client *Client
}

// Shows returns the show lookup of the client.
func (c *Client) Shows() Shows {
	return Shows{client: c}
}

// Search returns TMDB's show candidates for a title and first-air year, in TMDB's order.
func (shows Shows) Search(ctx context.Context, query enrich.SearchQuery) ([]match.Candidate, error) {
	return shows.client.search(ctx, match.RefKindShow, searchParams{
		title: query.Title, year: query.Year, language: parseLanguage(query.Language),
	})
}

// Details returns the show a TMDB or IMDb reference points to.
func (shows Shows) Details(ctx context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	id, err := shows.client.tmdbID(ctx, request.Ref, match.RefKindShow)
	if err != nil {
		return enrich.Details{}, fmt.Errorf("show details: %w", err)
	}

	lang := parseLanguage(request.Language)

	var details showDetails
	if err := shows.client.getJSON(ctx, "/tv/"+id, detailsParams(lang, appendContentRatings), &details); err != nil {
		return enrich.Details{}, fmt.Errorf("show details: %w", err)
	}

	return enrich.Details{
		Record:  showRecord(details, lang),
		Artwork: artworkRefs(details.PosterPath, details.BackdropPath, details.Images),
	}, nil
}

// SeasonDetails returns one season with its episodes. An IMDb show reference is resolved to
// the TMDB show first. Fields TMDB has not translated are filled from English.
func (shows Shows) SeasonDetails(ctx context.Context, request enrich.SeasonRequest) (enrich.SeasonDetails, error) {
	if request.Season < 0 {
		return enrich.SeasonDetails{}, fmt.Errorf("season details: season %d: %w", request.Season, domain.ErrInvalid)
	}

	id, err := shows.client.tmdbID(ctx, request.Show, match.RefKindShow)
	if err != nil {
		return enrich.SeasonDetails{}, fmt.Errorf("season %d details: %w", request.Season, err)
	}

	path := "/tv/" + id + "/season/" + strconv.Itoa(request.Season)
	lang := parseLanguage(request.Language)

	var season seasonDetails
	if err := shows.client.getJSON(ctx, path, url.Values{paramLanguage: {lang.tag}}, &season); err != nil {
		return enrich.SeasonDetails{}, fmt.Errorf("season %d details: %w", request.Season, err)
	}

	result := seasonResult(season)
	if lang.isEnglish() || !seasonNeedsFallback(result) {
		return result, nil
	}

	return shows.withEnglish(ctx, path, result)
}

// withEnglish fills untranslated fields from the English season. The fallback is best
// effort: when it fails for any reason but cancellation, the translated season is still
// the better answer than none.
func (shows Shows) withEnglish(ctx context.Context, path string, translated enrich.SeasonDetails) (enrich.SeasonDetails, error) {
	var english seasonDetails

	err := shows.client.getJSON(ctx, path, url.Values{paramLanguage: {fallbackLanguage}}, &english)
	if err != nil && ctx.Err() != nil {
		return enrich.SeasonDetails{}, fmt.Errorf("season details in English: %w", err)
	}

	if err != nil {
		return translated, nil //nolint:nilerr // best-effort fallback, see doc comment
	}

	return mergeEnglish(translated, seasonResult(english)), nil
}

func mergeEnglish(translated, english enrich.SeasonDetails) enrich.SeasonDetails {
	translated.Record = fillEmpty(translated.Record, english.Record)

	englishByNumber := make(map[int]enrich.EpisodeDetails, len(english.Episodes))
	for _, episode := range english.Episodes {
		englishByNumber[episode.Number] = episode
	}

	for index, episode := range translated.Episodes {
		if fallback, ok := englishByNumber[episode.Number]; ok {
			translated.Episodes[index].Record = fillEmpty(episode.Record, fallback.Record)
		}
	}

	return translated
}

func seasonNeedsFallback(season enrich.SeasonDetails) bool {
	if needsFallback(season.Record) {
		return true
	}

	for _, episode := range season.Episodes {
		if needsFallback(episode.Record) {
			return true
		}
	}

	return false
}

func seasonResult(season seasonDetails) enrich.SeasonDetails {
	result := enrich.SeasonDetails{
		Details: enrich.Details{Record: seasonRecord(season), Artwork: singleRef(artwork.KindPoster, season.PosterPath)},
	}

	for _, episode := range season.Episodes {
		result.Episodes = append(result.Episodes, enrich.EpisodeDetails{
			Number:  episode.EpisodeNumber,
			Details: enrich.Details{Record: episodeRecord(episode), Artwork: singleRef(artwork.KindThumb, episode.StillPath)},
		})
	}

	return result
}

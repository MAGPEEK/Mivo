package musicbrainz

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	pathReleaseGroup = "/release-group"
	pathRelease      = "/release"
	pathArtist       = "/artist"

	paramQuery = "query"
	paramLimit = "limit"

	// searchLimit bounds the candidates per search; the scorer only needs the top few.
	searchLimit = "10"

	fieldReleaseGroup = "releasegroup"
	fieldArtist       = "artist"
	fieldFirstRelease = "firstreleasedate"
	luceneAnd         = " AND "
)

// Search returns MusicBrainz candidates in MusicBrainz's order: release groups for
// match.RefKindRelease (an album), artists for match.RefKindArtist. An album search with a
// year that finds nothing is repeated without the year, because tags often carry the year
// of a reissue instead of the first release.
func (c *Client) Search(ctx context.Context, query enrich.MusicQuery) ([]match.Candidate, error) {
	title := normalizeName(query.Title)
	if title == "" {
		return nil, fmt.Errorf("musicbrainz search %s: empty title: %w", query.Kind, domain.ErrInvalid)
	}

	switch query.Kind {
	case match.RefKindRelease:
		return c.searchAlbums(ctx, title, normalizeName(query.Artist), query.Year)
	case match.RefKindArtist:
		return c.searchArtists(ctx, title)
	case match.RefKindUnknown, match.RefKindMovie, match.RefKindShow:
		return nil, fmt.Errorf("musicbrainz search: unsupported kind %q: %w", query.Kind, domain.ErrInvalid)
	default:
		return nil, fmt.Errorf("musicbrainz search: unknown kind %q: %w", query.Kind, domain.ErrInvalid)
	}
}

func (c *Client) searchAlbums(ctx context.Context, title, artistName string, year int) ([]match.Candidate, error) {
	if year > 0 {
		found, err := c.searchAlbumsOnce(ctx, albumQuery(title, artistName, year))
		if err != nil || len(found) > 0 {
			return found, err
		}
	}

	return c.searchAlbumsOnce(ctx, albumQuery(title, artistName, 0))
}

func (c *Client) searchAlbumsOnce(ctx context.Context, lucene string) ([]match.Candidate, error) {
	var response releaseGroupSearchResponse
	if err := c.getJSON(ctx, pathReleaseGroup, searchParams(lucene), &response); err != nil {
		return nil, fmt.Errorf("musicbrainz search albums: %w", err)
	}

	found := make([]match.Candidate, 0, len(response.ReleaseGroups))

	for _, group := range response.ReleaseGroups {
		id, ok := normalizeMBID(group.ID)
		if !ok {
			continue
		}

		year, _ := parsePartialDate(group.FirstReleaseDate)
		found = append(found, match.Candidate{
			Ref:        match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindRelease, ID: id},
			Title:      group.Title,
			Year:       year,
			Popularity: group.Score,
		})
	}

	return found, nil
}

func (c *Client) searchArtists(ctx context.Context, name string) ([]match.Candidate, error) {
	var response artistSearchResponse
	if err := c.getJSON(ctx, pathArtist, searchParams(phrase(fieldArtist, name)), &response); err != nil {
		return nil, fmt.Errorf("musicbrainz search artists: %w", err)
	}

	found := make([]match.Candidate, 0, len(response.Artists))

	for _, entry := range response.Artists {
		id, ok := normalizeMBID(entry.ID)
		if !ok {
			continue
		}

		found = append(found, match.Candidate{
			Ref:        match.ProviderRef{Provider: match.RefMusicBrainz, Kind: match.RefKindArtist, ID: id},
			Title:      entry.Name,
			Popularity: entry.Score,
		})
	}

	return found, nil
}

func searchParams(lucene string) url.Values {
	return url.Values{paramQuery: {lucene}, paramLimit: {searchLimit}}
}

// albumQuery builds the Lucene query of a release group search; artist and year are added
// only when known.
func albumQuery(title, artistName string, year int) string {
	clauses := []string{phrase(fieldReleaseGroup, title)}

	if artistName != "" {
		clauses = append(clauses, phrase(fieldArtist, artistName))
	}

	if year > 0 {
		clauses = append(clauses, fieldFirstRelease+":"+strconv.Itoa(year))
	}

	return strings.Join(clauses, luceneAnd)
}

// phrase quotes value as a Lucene phrase for field; inside quotes only the backslash and the
// quote itself need escaping.
func phrase(field, value string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value)

	return field + `:"` + escaped + `"`
}

// normalizeName trims and collapses whitespace of a tag-derived name.
func normalizeName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}

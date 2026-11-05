package musicbrainz

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

const (
	paramInclude = "inc"

	// Includes per entity; url.Values encodes the spaces as the '+' MusicBrainz expects.
	includeReleaseGroup = "genres tags"
	includeRelease      = "release-groups labels genres tags"
	includeArtist       = "genres tags"

	// coverArtBaseURL is always HTTPS: Cover Art Archive answers may name http:// image URLs,
	// but the artwork fetcher only accepts HTTPS (BACKLOG m6-08), so refs are built here.
	coverArtBaseURL = "https://coverartarchive.org"
	coverArtFront   = "front-1200"
)

// Details returns the entry a MusicBrainz reference points to. An album reference
// (match.RefKindRelease) may hold a release group ID (search results) or a release ID
// (embedded MUSICBRAINZ_ALBUMID tags, pasted release URLs): the release group is tried
// first, then the release. A reference without a kind (a pasted bare ID) also tries the
// artist. The language is ignored; MusicBrainz data is not localised.
func (c *Client) Details(ctx context.Context, request enrich.DetailsRequest) (enrich.Details, error) {
	ref := request.Ref
	if ref.Provider != match.RefMusicBrainz {
		return enrich.Details{}, fmt.Errorf("musicbrainz details of a %s reference: %w", ref.Provider, domain.ErrInvalid)
	}

	id, ok := normalizeMBID(ref.ID)
	if !ok {
		return enrich.Details{}, fmt.Errorf("musicbrainz details: malformed id: %w", domain.ErrInvalid)
	}

	switch ref.Kind {
	case match.RefKindRelease:
		return c.albumDetails(ctx, id)
	case match.RefKindArtist:
		return c.artistDetails(ctx, id)
	case match.RefKindUnknown:
		details, err := c.albumDetails(ctx, id)
		if !errors.Is(err, domain.ErrNotFound) {
			return details, err
		}

		return c.artistDetails(ctx, id)
	case match.RefKindMovie, match.RefKindShow:
		return enrich.Details{}, fmt.Errorf("musicbrainz details of a %s reference: %w", ref.Kind, domain.ErrInvalid)
	default:
		return enrich.Details{}, fmt.Errorf("musicbrainz details: unknown kind %q: %w", ref.Kind, domain.ErrInvalid)
	}
}

func (c *Client) albumDetails(ctx context.Context, id string) (enrich.Details, error) {
	details, err := c.releaseGroupDetails(ctx, id)
	if !errors.Is(err, domain.ErrNotFound) {
		return details, err
	}

	return c.releaseDetails(ctx, id)
}

func (c *Client) releaseGroupDetails(ctx context.Context, id string) (enrich.Details, error) {
	var group releaseGroup
	if err := c.getJSON(ctx, pathReleaseGroup+"/"+id, includes(includeReleaseGroup), &group); err != nil {
		return enrich.Details{}, fmt.Errorf("musicbrainz release group details: %w", err)
	}

	return enrich.Details{
		Record:  releaseGroupRecord(group),
		Artwork: []enrich.ArtworkRef{frontCover(pathReleaseGroup, id)},
	}, nil
}

func (c *Client) releaseDetails(ctx context.Context, id string) (enrich.Details, error) {
	var edition release
	if err := c.getJSON(ctx, pathRelease+"/"+id, includes(includeRelease), &edition); err != nil {
		return enrich.Details{}, fmt.Errorf("musicbrainz release details: %w", err)
	}

	details := enrich.Details{Record: releaseRecord(edition)}
	if edition.CoverArtArchive.Front {
		details.Artwork = []enrich.ArtworkRef{frontCover(pathRelease, id)}
	}

	return details, nil
}

func (c *Client) artistDetails(ctx context.Context, id string) (enrich.Details, error) {
	var entry artist
	if err := c.getJSON(ctx, pathArtist+"/"+id, includes(includeArtist), &entry); err != nil {
		return enrich.Details{}, fmt.Errorf("musicbrainz artist details: %w", err)
	}

	// Cover Art Archive holds album covers only; artist pictures need another source.
	return enrich.Details{Record: artistRecord(entry)}, nil
}

func includes(value string) url.Values {
	return url.Values{paramInclude: {value}}
}

// frontCover points at the Cover Art Archive front image of a release or release group. A
// group without art answers 404; the enricher skips pictures that fail to download.
func frontCover(entityPath, id string) enrich.ArtworkRef {
	return enrich.ArtworkRef{
		Provider: enrich.ArtworkProviderCoverArt,
		Kind:     artwork.KindPoster,
		Path:     coverArtBaseURL + entityPath + "/" + id + "/" + coverArtFront,
	}
}

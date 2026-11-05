package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Providers of the MusicBrainz IDs read from audio tags (M3-13). They share
// musicProviderPrefix, which marks them as owned by the music writer: the metadata writer
// neither replaces nor reads them (see queries/catalog_metadata.sql).
const (
	musicProviderPrefix       = "musicbrainz_"
	providerMusicBrainzArtist = musicProviderPrefix + "artist"
	providerMusicBrainzAlbum  = musicProviderPrefix + "album"
	providerMusicBrainzTrack  = musicProviderPrefix + "track"
)

// A MusicBrainz ID is a lower-case UUID: groups of these digits joined by the separator.
const (
	uuidGroupSeparator = "-"
	lowerHexDigits     = "0123456789abcdef"
)

// firstDisc is the disc of a track whose disc number is not stored.
const firstDisc = 1

// isMusicProvider reports whether a stored provider belongs to the music writer.
func isMusicProvider(provider string) bool {
	return strings.HasPrefix(provider, musicProviderPrefix)
}

// SaveTrackDetails stores what a placement level cannot carry for a track saved by
// SavePlacedFile: its disc number and the MusicBrainz IDs of its tags as provider IDs of the
// track, its album and its artist. The artist ID of the tags names the track artist, so it is
// stored on the artist item only when the track artist is the album artist (never on
// media.VariousArtists). An empty album or artist ID leaves the stored one alone, because
// another track of the album may carry it; an empty track ID removes the stored one. Errors
// wrap domain.ErrNotFound for an unknown item and domain.ErrInvalid for an item that is no
// track, a disc below 1 or a malformed ID.
func (catalog *LibraryCatalog) SaveTrackDetails(ctx context.Context, trackID string, disc int, tags media.AudioTags) error {
	if err := checkTrackDetails(disc, tags); err != nil {
		return fmt.Errorf("save details of track %q: %w", trackID, err)
	}

	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		return trackDetailsWrite{queries: queries, disc: disc, tags: tags, now: catalog.now().UnixMilli()}.apply(ctx, trackID)
	})
	if err != nil {
		return fmt.Errorf("save details of track %q: %w", trackID, err)
	}

	return nil
}

func checkTrackDetails(disc int, tags media.AudioTags) error {
	if disc < firstDisc {
		return fmt.Errorf("disc %d: %w", disc, domain.ErrInvalid)
	}

	for _, id := range []string{tags.MusicBrainzArtistID, tags.MusicBrainzAlbumID, tags.MusicBrainzTrackID} {
		if id != "" && !isMusicBrainzID(id) {
			return fmt.Errorf("MusicBrainz ID %q: %w", id, domain.ErrInvalid)
		}
	}

	return nil
}

// isMusicBrainzID checks the lower-case UUID shape the tag reader normalises IDs to.
func isMusicBrainzID(id string) bool {
	groups := strings.Split(id, uuidGroupSeparator)
	lengths := uuidGroupLengths()

	if len(groups) != len(lengths) {
		return false
	}

	for index, group := range groups {
		if len(group) != lengths[index] || strings.Trim(group, lowerHexDigits) != "" {
			return false
		}
	}

	return true
}

// uuidGroupLengths returns the lengths of the groups of the UUID text form (8-4-4-4-12).
func uuidGroupLengths() []int {
	return []int{8, 4, 4, 4, 12}
}

// trackDetailsWrite is one SaveTrackDetails run inside its write transaction.
type trackDetailsWrite struct {
	queries *sqlcgen.Queries
	disc    int
	tags    media.AudioTags
	now     int64
}

func (run trackDetailsWrite) apply(ctx context.Context, trackID string) error {
	track, err := itemOfKind(ctx, run.queries, trackID, library.ItemKindTrack)
	if err != nil {
		return err
	}

	err = run.queries.SetTrackDisc(ctx, sqlcgen.SetTrackDiscParams{
		ID: track.ID, DiscNumber: sql.NullInt64{Int64: int64(run.disc), Valid: true}, UpdatedAt: run.now,
	})
	if err != nil {
		return fmt.Errorf("set disc: %w", err)
	}

	if err := run.replaceTrackID(ctx, track.ID); err != nil {
		return err
	}

	album, err := itemOfKind(ctx, run.queries, track.ParentID.String, library.ItemKindAlbum)
	if err != nil {
		return fmt.Errorf("album of the track: %w", err)
	}

	if err := run.upsertID(ctx, album.ID, providerMusicBrainzAlbum, run.tags.MusicBrainzAlbumID); err != nil {
		return err
	}

	if !artistIDNamesAlbumArtist(run.tags) {
		return nil
	}

	return run.upsertID(ctx, album.ParentID.String, providerMusicBrainzArtist, run.tags.MusicBrainzArtistID)
}

func (run trackDetailsWrite) replaceTrackID(ctx context.Context, trackID string) error {
	if run.tags.MusicBrainzTrackID != "" {
		return run.upsertID(ctx, trackID, providerMusicBrainzTrack, run.tags.MusicBrainzTrackID)
	}

	err := run.queries.DeleteMusicProviderID(ctx, sqlcgen.DeleteMusicProviderIDParams{ItemID: trackID, Provider: providerMusicBrainzTrack})
	if err != nil {
		return fmt.Errorf("delete %s: %w", providerMusicBrainzTrack, err)
	}

	return nil
}

// upsertID stores a non-empty ID; an empty one changes nothing.
func (run trackDetailsWrite) upsertID(ctx context.Context, itemID, provider, externalID string) error {
	if externalID == "" {
		return nil
	}

	err := run.queries.UpsertMusicProviderID(ctx, sqlcgen.UpsertMusicProviderIDParams{
		ItemID: itemID, Provider: provider, ExternalID: externalID,
	})
	if err != nil {
		return fmt.Errorf("store %s: %w", provider, err)
	}

	return nil
}

// artistIDNamesAlbumArtist reports whether the artist ID of the tags (the track artist's)
// also identifies the artist the album is filed under.
func artistIDNamesAlbumArtist(tags media.AudioTags) bool {
	placement := tags.Placement()

	return placement.ArtistName != "" && placement.ArtistName != media.VariousArtists &&
		library.ArtistKey(placement.TrackArtist) == library.ArtistKey(placement.ArtistName)
}

// itemOfKind reads an item and checks its kind: an unknown ID wraps domain.ErrNotFound, an
// item of another kind domain.ErrInvalid.
func itemOfKind(ctx context.Context, queries *sqlcgen.Queries, itemID string, kind library.ItemKind) (sqlcgen.Item, error) {
	item, err := queries.GetItem(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlcgen.Item{}, fmt.Errorf("item %q: %w", itemID, domain.ErrNotFound)
	}

	if err != nil {
		return sqlcgen.Item{}, fmt.Errorf("read item %q: %w", itemID, err)
	}

	if item.Kind != kind.String() {
		return sqlcgen.Item{}, fmt.Errorf("item %q is a %s, not a %s: %w", itemID, item.Kind, kind, domain.ErrInvalid)
	}

	return item, nil
}

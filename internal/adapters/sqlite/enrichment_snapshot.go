package sqlite

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/app/identify"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
	"github.com/magpeek/mivo/internal/domain/metadata/match"
)

// Compile-time checks: the enrichment and identify use cases read and write items here.
var (
	_ enrich.MetadataStore = (*EnrichmentStore)(nil)
	_ identify.ItemReader  = (*EnrichmentStore)(nil)
)

// ItemSnapshot returns what enrichment needs to know about an item (enrich.MetadataStore):
// its kind and switches, match, stored record and locks, local records, placement numbers,
// the show's provider IDs, the album artist, the linked reference and the artwork. A music
// item without a stored reference is linked by the MusicBrainz ID of its tags. The error
// wraps domain.ErrNotFound for an unknown item.
func (store *EnrichmentStore) ItemSnapshot(ctx context.Context, itemID string) (enrich.ItemSnapshot, error) {
	var snapshot enrich.ItemSnapshot

	err := inReadTransaction(ctx, store.reader, func(queries *sqlcgen.Queries) error {
		var err error

		snapshot, err = readSnapshot(ctx, queries, itemID)

		return err
	})
	if err != nil {
		return enrich.ItemSnapshot{}, fmt.Errorf("snapshot of item %q: %w", itemID, err)
	}

	return snapshot, nil
}

func readSnapshot(ctx context.Context, queries *sqlcgen.Queries, itemID string) (enrich.ItemSnapshot, error) {
	row, kind, err := readEnrichmentItem(ctx, queries, itemID)
	if err != nil {
		return enrich.ItemSnapshot{}, err
	}

	snapshot, err := readItemState(ctx, queries, itemID)
	if err != nil {
		return enrich.ItemSnapshot{}, err
	}

	snapshot.Kind = kind
	snapshot.OnlineEnabled = row.OnlineMetadata == storedTrue
	snapshot.Language = cmp.Or(row.MetadataLanguage, row.LibraryLanguage)

	if err := readPlacement(ctx, queries, row, &snapshot); err != nil {
		return enrich.ItemSnapshot{}, err
	}

	if err := linkMusicTags(ctx, queries, itemID, &snapshot); err != nil {
		return enrich.ItemSnapshot{}, err
	}

	snapshot.Artwork, err = readArtworkState(ctx, queries, itemID)

	return snapshot, err
}

// readItemState reads the match, record, locks and local records of an item.
func readItemState(ctx context.Context, queries *sqlcgen.Queries, itemID string) (enrich.ItemSnapshot, error) {
	itemMatch, err := readItemMatch(ctx, queries, itemID)
	if err != nil {
		return enrich.ItemSnapshot{}, err
	}

	record, err := readStoredRecord(ctx, queries, itemID)
	if err != nil {
		return enrich.ItemSnapshot{}, err
	}

	locks, err := readFieldLocks(ctx, queries, itemID)
	if err != nil {
		return enrich.ItemSnapshot{}, err
	}

	local, err := localRecordsOf(ctx, queries, itemID, record)
	if err != nil {
		return enrich.ItemSnapshot{}, err
	}

	return enrich.ItemSnapshot{
		MatchState: itemMatch.State, Record: record, Locks: locks, Local: local, LinkedRef: itemMatch.Ref,
	}, nil
}

func readEnrichmentItem(ctx context.Context, queries *sqlcgen.Queries, itemID string) (sqlcgen.GetEnrichmentItemRow, library.ItemKind, error) {
	row, err := queries.GetEnrichmentItem(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return sqlcgen.GetEnrichmentItemRow{}, "", domain.ErrNotFound
	}

	if err != nil {
		return sqlcgen.GetEnrichmentItemRow{}, "", fmt.Errorf("read item: %w", err)
	}

	kind, err := library.ParseItemKind(row.Kind)
	if err != nil {
		return sqlcgen.GetEnrichmentItemRow{}, "", undecodable(fmt.Sprintf("item %q: kind", itemID), err)
	}

	return row, kind, nil
}

func readItemMatch(ctx context.Context, queries *sqlcgen.Queries, itemID string) (ItemMatch, error) {
	row, err := queries.GetItemMatch(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return ItemMatch{}, domain.ErrNotFound
	}

	if err != nil {
		return ItemMatch{}, fmt.Errorf("read match: %w", err)
	}

	return storedItemMatch(row)
}

// readPlacement fills the numbers of a season or episode (snapshot.Kind) with the provider IDs
// of its show, and the artist name of an album.
func readPlacement(ctx context.Context, queries *sqlcgen.Queries, row sqlcgen.GetEnrichmentItemRow,
	snapshot *enrich.ItemSnapshot,
) error {
	switch snapshot.Kind {
	case library.ItemKindSeason:
		snapshot.SeasonNumber = int(row.Number.Int64)

		return readShowIDs(ctx, queries, row.ParentID, snapshot)
	case library.ItemKindEpisode:
		snapshot.EpisodeNumber = int(row.Number.Int64)

		return readEpisodeParents(ctx, queries, row.ParentID, snapshot)
	case library.ItemKindAlbum:
		parent, parentKind, found, err := readParent(ctx, queries, row.ParentID)
		if err == nil && found && parentKind == library.ItemKindArtist {
			snapshot.ArtistName = parent.Title
		}

		return err
	case library.ItemKindMovie, library.ItemKindExtra, library.ItemKindShow, library.ItemKindArtist,
		library.ItemKindTrack, library.ItemKindPhotoAlbum, library.ItemKindPhoto, library.ItemKindAudiobook:
		return nil
	default:
		return nil
	}
}

// readEpisodeParents takes the season number from the episode's season and the provider IDs
// from the show above it (or directly above the episode).
func readEpisodeParents(ctx context.Context, queries *sqlcgen.Queries, parentID sql.NullString, snapshot *enrich.ItemSnapshot) error {
	parent, parentKind, found, err := readParent(ctx, queries, parentID)
	if err != nil || !found {
		return err
	}

	if parentKind != library.ItemKindSeason {
		return readShowIDs(ctx, queries, parentID, snapshot)
	}

	snapshot.SeasonNumber = int(parent.Number.Int64)

	return readShowIDs(ctx, queries, parent.ParentID, snapshot)
}

func readShowIDs(ctx context.Context, queries *sqlcgen.Queries, showID sql.NullString, snapshot *enrich.ItemSnapshot) error {
	if !showID.Valid {
		return nil
	}

	var show metadata.Record
	if err := readRecordProviderIDs(ctx, queries, showID.String, &show); err != nil {
		return err
	}

	snapshot.ParentProviderIDs = show.ProviderIDs

	return nil
}

func readParent(ctx context.Context, queries *sqlcgen.Queries, parentID sql.NullString) (
	sqlcgen.GetEnrichmentItemRow, library.ItemKind, bool, error,
) {
	if !parentID.Valid {
		return sqlcgen.GetEnrichmentItemRow{}, "", false, nil
	}

	parent, kind, err := readEnrichmentItem(ctx, queries, parentID.String)
	if errors.Is(err, domain.ErrNotFound) {
		return sqlcgen.GetEnrichmentItemRow{}, "", false, nil
	}

	return parent, kind, err == nil, err
}

// linkMusicTags links an album or artist without a stored reference by the MusicBrainz ID of
// its tags (M3-13d), so enrichment looks it up directly instead of searching. The album tag
// is a release ID.
func linkMusicTags(ctx context.Context, queries *sqlcgen.Queries, itemID string, snapshot *enrich.ItemSnapshot) error {
	if !snapshot.LinkedRef.IsZero() {
		return nil
	}

	provider, refKind := providerMusicBrainzAlbum, match.RefKindRelease

	if snapshot.Kind == library.ItemKindArtist {
		provider, refKind = providerMusicBrainzArtist, match.RefKindArtist
	} else if snapshot.Kind != library.ItemKindAlbum {
		return nil
	}

	found, err := queries.FindMusicBrainzID(ctx, sqlcgen.FindMusicBrainzIDParams{Provider: provider, ItemID: itemID})
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("read musicbrainz id: %w", err)
	}

	snapshot.LinkedRef = match.ProviderRef{Provider: match.RefMusicBrainz, Kind: refKind, ID: found.ExternalID}

	return nil
}

// readArtworkState returns every artwork candidate of the item and its selection per kind.
func readArtworkState(ctx context.Context, queries *sqlcgen.Queries, itemID string) (enrich.ArtworkState, error) {
	rows, err := queries.ListItemArtwork(ctx, itemID)
	if err != nil {
		return enrich.ArtworkState{}, fmt.Errorf("read artwork: %w", err)
	}

	state := enrich.ArtworkState{Current: map[artwork.Kind]artwork.Candidate{}}

	for _, row := range rows {
		key, known := storedArtworkKey(row)
		if !known {
			continue
		}

		candidate := artwork.Candidate{
			Kind: key.kind, Source: key.source,
			Location: artwork.Location{
				RootID: key.rootID, RelativePath: key.path, StreamIndex: key.stream, DataPath: key.data,
			},
			Size: row.SizeBytes, ModifiedAt: time.UnixMilli(row.ModifiedAt).UTC(),
		}

		state.Candidates = append(state.Candidates, candidate)
		if row.Selected == storedTrue {
			state.Current[key.kind] = candidate
		}
	}

	return state, nil
}

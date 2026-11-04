package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

// artworkKey identifies a picture of an item by what it is read from, so a rescan finds the
// stored row of an unchanged candidate and keeps its ID (image URLs stay valid).
type artworkKey struct {
	kind   artwork.Kind
	source artwork.SourceKind
	path   string
	stream int
	data   string
}

func candidateKey(candidate artwork.Candidate) artworkKey {
	return artworkKey{
		kind: candidate.Kind, source: candidate.Source, path: candidate.Location.RelativePath,
		stream: candidate.Location.StreamIndex, data: candidate.Location.DataPath,
	}
}

// storedArtworkKey returns the key of a stored row; ok is false for a row of an unknown kind
// or source, which is left alone.
func storedArtworkKey(row sqlcgen.ListItemArtworkRow) (artworkKey, bool) {
	kind, kindErr := artwork.ParseKind(row.Kind)
	source, sourceErr := artwork.ParseSourceKind(row.Source)

	if kindErr != nil || sourceErr != nil {
		return artworkKey{}, false
	}

	key := artworkKey{kind: kind, source: source, stream: artwork.WholeFile, data: row.DataPath.String}

	switch source {
	case artwork.SourceLocal:
		key.path = row.RelativePath.String
	case artwork.SourceEmbedded:
		key.path, key.stream = row.MediaRelativePath.String, int(row.StreamIndex.Int64)
	case artwork.SourceProvider, artwork.SourceUpload:
	}

	return key, true
}

// readSelectedArtwork fills the selection per kind of a metadata state.
func readSelectedArtwork(ctx context.Context, queries *sqlcgen.Queries, itemID string, state *scan.MetadataState) error {
	rows, err := queries.ListItemArtwork(ctx, itemID)
	if err != nil {
		return fmt.Errorf("read artwork: %w", err)
	}

	for _, row := range rows {
		key, known := storedArtworkKey(row)
		if row.Selected == 0 || !known {
			continue
		}

		state.CurrentArtwork[key.kind] = artwork.Candidate{
			Kind: key.kind, Source: key.source,
			Location: artwork.Location{RelativePath: key.path, StreamIndex: key.stream, DataPath: key.data},
			Size:     row.SizeBytes, ModifiedAt: time.UnixMilli(row.ModifiedAt).UTC(),
		}
	}

	return nil
}

// SaveArtwork replaces the local and embedded pictures of the item with candidates and
// stores the selection per kind. Provider and upload pictures stay; a kind whose artwork
// field is locked is not touched at all, so its selection survives. A candidate keeps the
// ID of the row with the same source location. Errors wrap domain.ErrNotFound for an
// unknown item and domain.ErrInvalid for a malformed candidate, an embedded picture of a
// media file outside the item's library or a selection that is no candidate.
func (catalog *LibraryCatalog) SaveArtwork(ctx context.Context, itemID string, candidates []artwork.Candidate,
	selection map[artwork.Kind]artwork.Candidate,
) error {
	err := inWriteTransaction(ctx, catalog.writer, func(queries *sqlcgen.Queries) error {
		return saveArtwork(ctx, queries, artworkRequest{itemID: itemID, candidates: candidates, selection: selection})
	})
	if err != nil {
		return fmt.Errorf("save artwork of item %q: %w", itemID, err)
	}

	return nil
}

// artworkSave is one SaveArtwork run inside its transaction.
type artworkSave struct {
	queries   *sqlcgen.Queries
	itemID    string
	libraryID string
	now       time.Time
	locked    map[artwork.Kind]bool
	// ids maps the key of every stored or inserted row to its ID.
	ids map[artworkKey]string
}

// artworkRequest is the input of SaveArtwork.
type artworkRequest struct {
	itemID     string
	candidates []artwork.Candidate
	selection  map[artwork.Kind]artwork.Candidate
}

func saveArtwork(ctx context.Context, queries *sqlcgen.Queries, request artworkRequest) error {
	itemID := request.itemID

	libraryID, err := queries.GetItemLibrary(ctx, itemID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}

	if err != nil {
		return fmt.Errorf("read item library: %w", err)
	}

	locks, err := readFieldLocks(ctx, queries, itemID)
	if err != nil {
		return err
	}

	locked, err := lockedArtworkKinds(locks)
	if err != nil {
		return err
	}

	run := artworkSave{
		queries: queries, itemID: itemID, libraryID: libraryID, now: time.Now(),
		locked: locked, ids: map[artworkKey]string{},
	}

	wanted, err := run.validate(request.candidates, request.selection)
	if err != nil {
		return err
	}

	if err := run.replacePictures(ctx, wanted); err != nil {
		return err
	}

	return run.applySelection(ctx, request.selection)
}

// validate returns the candidates of unlocked kinds without repeats. Locked kinds are
// ignored with their candidates and selection.
func (run artworkSave) validate(candidates []artwork.Candidate,
	selection map[artwork.Kind]artwork.Candidate,
) (map[artworkKey]artwork.Candidate, error) {
	wanted := map[artworkKey]artwork.Candidate{}

	for _, candidate := range candidates {
		if err := checkCandidate(candidate); err != nil {
			return nil, err
		}

		if !run.locked[candidate.Kind] {
			wanted[candidateKey(candidate)] = candidate
		}
	}

	for kind := range selection {
		if !kind.IsValid() {
			return nil, fmt.Errorf("selection for artwork kind %q: %w", kind, domain.ErrInvalid)
		}
	}

	return wanted, nil
}

func checkCandidate(candidate artwork.Candidate) error {
	location := candidate.Location
	valid := candidate.Kind.IsValid() && candidate.Size >= 0 && location.RelativePath != ""

	switch candidate.Source {
	case artwork.SourceLocal:
		valid = valid && location.StreamIndex == artwork.WholeFile
	case artwork.SourceEmbedded:
		valid = valid && location.StreamIndex >= 0
	case artwork.SourceProvider, artwork.SourceUpload:
		valid = false
	}

	if !valid {
		return fmt.Errorf("artwork candidate (kind %q, source %q): %w", candidate.Kind, candidate.Source, domain.ErrInvalid)
	}

	return nil
}

// replacePictures deletes the stored local and embedded rows that are no candidate any more,
// refreshes the file data of the ones that stay and inserts the new ones.
func (run artworkSave) replacePictures(ctx context.Context, wanted map[artworkKey]artwork.Candidate) error {
	rows, err := run.queries.ListItemArtwork(ctx, run.itemID)
	if err != nil {
		return fmt.Errorf("read artwork: %w", err)
	}

	for _, row := range rows {
		key, known := storedArtworkKey(row)
		if !known {
			continue
		}

		run.ids[key] = row.ID

		if err := run.reconcileRow(ctx, row, key, wanted); err != nil {
			return err
		}
	}

	for key, candidate := range wanted {
		if _, stored := run.ids[key]; stored {
			continue
		}

		if err := run.insert(ctx, key, candidate); err != nil {
			return err
		}
	}

	return nil
}

func (run artworkSave) reconcileRow(ctx context.Context, row sqlcgen.ListItemArtworkRow, key artworkKey,
	wanted map[artworkKey]artwork.Candidate,
) error {
	picture := key.source == artwork.SourceLocal || key.source == artwork.SourceEmbedded
	if !picture || run.locked[key.kind] {
		return nil
	}

	candidate, keep := wanted[key]
	if !keep {
		delete(run.ids, key)

		if err := run.queries.DeleteArtwork(ctx, row.ID); err != nil {
			return fmt.Errorf("delete artwork: %w", err)
		}

		return nil
	}

	err := run.queries.UpdateArtworkFile(ctx, sqlcgen.UpdateArtworkFileParams{
		SizeBytes: candidate.Size, ModifiedAt: candidate.ModifiedAt.UnixMilli(), Tag: candidate.Tag(), ID: row.ID,
	})
	if err != nil {
		return fmt.Errorf("update artwork: %w", err)
	}

	return nil
}

func (run artworkSave) insert(ctx context.Context, key artworkKey, candidate artwork.Candidate) error {
	params := sqlcgen.InsertArtworkParams{
		ID: uuid.NewV4().String(), ItemID: run.itemID, Kind: candidate.Kind.String(),
		Source: candidate.Source.String(), SizeBytes: candidate.Size,
		ModifiedAt: candidate.ModifiedAt.UnixMilli(), Tag: candidate.Tag(), CreatedAt: run.now.UnixMilli(),
	}

	if candidate.Source == artwork.SourceLocal {
		params.LibraryID = sql.NullString{String: run.libraryID, Valid: true}
		params.RelativePath = sql.NullString{String: candidate.Location.RelativePath, Valid: true}
	} else {
		fileID, err := run.ownedMediaFile(ctx, candidate.Location.RelativePath)
		if err != nil {
			return err
		}

		params.MediaFileID = sql.NullString{String: fileID, Valid: true}
		params.StreamIndex = sql.NullInt64{Int64: int64(candidate.Location.StreamIndex), Valid: true}
	}

	if err := run.queries.InsertArtwork(ctx, params); err != nil {
		return fmt.Errorf("insert artwork: %w", uniqueAsConflict(err))
	}

	run.ids[key] = params.ID

	return nil
}

// ownedMediaFile finds the media file an embedded picture comes from. It must be stored in
// the item's own library: a file of another library would leak its pictures into this one.
func (run artworkSave) ownedMediaFile(ctx context.Context, relativePath string) (string, error) {
	id, err := run.queries.FindMediaFileID(ctx, sqlcgen.FindMediaFileIDParams{
		LibraryID: run.libraryID, RelativePath: relativePath,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("embedded artwork: no media file in the item's library: %w", domain.ErrInvalid)
	}

	if err != nil {
		return "", fmt.Errorf("find media file: %w", err)
	}

	return id, nil
}

// applySelection stores at most one selected row per unlocked kind: the one the scan chose,
// none when it chose nothing.
func (run artworkSave) applySelection(ctx context.Context, selection map[artwork.Kind]artwork.Candidate) error {
	for _, kind := range artwork.Kinds() {
		if run.locked[kind] {
			continue
		}

		if err := run.queries.ClearSelectedArtwork(ctx, sqlcgen.ClearSelectedArtworkParams{
			ItemID: run.itemID, Kind: kind.String(),
		}); err != nil {
			return fmt.Errorf("clear artwork selection: %w", err)
		}

		chosen, has := selection[kind]
		if !has {
			continue
		}

		chosen.Kind = kind

		id, stored := run.ids[candidateKey(chosen)]
		if !stored {
			return fmt.Errorf("selected %s artwork is not a candidate of the item: %w", kind, domain.ErrInvalid)
		}

		if err := run.queries.SelectArtwork(ctx, id); err != nil {
			return fmt.Errorf("select artwork: %w", err)
		}
	}

	return nil
}

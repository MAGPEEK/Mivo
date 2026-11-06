package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite/sqlcgen"
	"github.com/magpeek/mivo/internal/app/metadatasettings"
)

// MetadataSettingsStore persists the online metadata settings changed in Settings › Metadata
// (metadatasettings.Store), so a restart keeps them. The own TMDB key is stored like the
// rest of the configuration in the data directory and never logged.
type MetadataSettingsStore struct {
	writer *sql.DB
	reader *sql.DB
	clock  func() time.Time
}

// Compile-time check: the settings service saves its state here.
var _ metadatasettings.Store = (*MetadataSettingsStore)(nil)

// NewMetadataSettingsStore returns a store on db, which must be migrated; clock supplies
// updated_at (nil means time.Now).
func NewMetadataSettingsStore(db *DB, clock func() time.Time) *MetadataSettingsStore {
	if clock == nil {
		clock = time.Now
	}

	return &MetadataSettingsStore{writer: db.writer, reader: db.reader, clock: clock}
}

// LoadMetadataSettings returns the stored state; found is false when the settings were never
// changed, so the configuration applies.
func (store *MetadataSettingsStore) LoadMetadataSettings(ctx context.Context) (state metadatasettings.State, found bool, err error) {
	row, err := sqlcgen.New(store.reader).GetMetadataSettings(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return metadatasettings.State{}, false, nil
	}

	if err != nil {
		return metadatasettings.State{}, false, fmt.Errorf("read metadata settings: %w", err)
	}

	return metadatasettings.State{
		Online:      row.Online == storedTrue,
		TMDB:        row.Tmdb == storedTrue,
		MusicBrainz: row.Musicbrainz == storedTrue,
		Language:    row.Language,
		OwnTMDBKey:  row.TmdbApiKey,
	}, true, nil
}

// SaveMetadataSettings replaces the stored state. The error never contains the key.
func (store *MetadataSettingsStore) SaveMetadataSettings(ctx context.Context, state metadatasettings.State) error {
	err := sqlcgen.New(store.writer).UpsertMetadataSettings(ctx, sqlcgen.UpsertMetadataSettingsParams{
		Online:      storedFlag(state.Online),
		Tmdb:        storedFlag(state.TMDB),
		Musicbrainz: storedFlag(state.MusicBrainz),
		Language:    state.Language,
		TmdbApiKey:  state.OwnTMDBKey,
		UpdatedAt:   store.clock().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf("save metadata settings: %w", err)
	}

	return nil
}

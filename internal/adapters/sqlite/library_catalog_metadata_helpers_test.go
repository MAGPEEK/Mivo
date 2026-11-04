package sqlite_test

import (
	"database/sql"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/sqlite"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// metadataFixture is a catalog fixture with the v2 catalog on top and one movie.
type metadataFixture struct {
	catalogFixture
	catalog *sqlite.LibraryCatalog
	movieID string
}

func newMetadataFixture(t *testing.T) metadataFixture {
	t.Helper()

	fixture := metadataFixture{catalogFixture: newCatalogFixture(t), movieID: testID('m', 1)}
	fixture.catalog = sqlite.NewLibraryCatalog(fixture.db, time.Now)
	insertItem(t, fixture.inspection, newItemSpec(fixture.movieID, moviesLibraryID, "movie", "Stored Title"))

	return fixture
}

// addMovie inserts another movie of the movies library.
func (fixture metadataFixture) addMovie(t *testing.T, number int) string {
	t.Helper()

	id := testID('m', number)
	insertItem(t, fixture.inspection, newItemSpec(id, moviesLibraryID, "movie", "Movie "+id[:8]))

	return id
}

func (fixture metadataFixture) lock(t *testing.T, itemID string, field metadata.Field) {
	t.Helper()

	execute(t, fixture.inspection, `INSERT INTO item_field_locks (item_id, field) VALUES (?, ?)`, itemID, field.String())
}

func countRows(t *testing.T, inspection *sql.DB, query string, args ...any) int {
	t.Helper()

	var count int
	if err := inspection.QueryRowContext(t.Context(), query, args...).Scan(&count); err != nil {
		t.Fatalf("%s: %v", query, err)
	}

	return count
}

// fullRecord sets every field of a record.
func fullRecord(t *testing.T) metadata.Record {
	return metadata.Record{
		Title: "The Film", SortTitle: "film, the", OriginalTitle: "Le Film", Year: 2024,
		ReleaseDate: mustDate(t, "2024-05-17"), Summary: "A summary.", Tagline: "A tagline.",
		ContentRating: "PG-13", Genres: []string{"Drama", "Crime"}, Studios: []string{"Studio A"},
		Countries: []string{"FR"}, Collections: []string{"The Collection"},
		People: []metadata.Credit{
			{Name: "Ann Actor", Role: metadata.RoleActor, Character: "Hero", Order: 0},
			{Name: "Dirk Director", Role: metadata.RoleDirector, Order: 1},
			{Name: "ann actor", Role: metadata.RoleActor, Character: "Villain", Order: 2},
		},
		ProviderIDs: []metadata.ProviderID{
			{Provider: metadata.ProviderIMDb, Value: "tt1234567"}, {Provider: metadata.ProviderTMDB, Value: "603"},
		},
	}
}

// storedFullRecord is fullRecord as the catalog reads it back: both parts of the actor are
// one credit with joined characters, provider IDs sorted by provider.
func storedFullRecord(t *testing.T) metadata.Record {
	record := fullRecord(t)
	record.People = []metadata.Credit{
		{Name: "Ann Actor", Role: metadata.RoleActor, Character: "Hero / Villain", Order: 0},
		{Name: "Dirk Director", Role: metadata.RoleDirector, Order: 1},
	}
	record.ProviderIDs = []metadata.ProviderID{
		{Provider: metadata.ProviderIMDb, Value: "tt1234567"}, {Provider: metadata.ProviderTMDB, Value: "603"},
	}

	return record
}

func mustDate(t *testing.T, text string) metadata.Date {
	t.Helper()

	date, err := metadata.ParseDate(text)
	if err != nil {
		t.Fatalf("ParseDate(%q) error = %v", text, err)
	}

	return date
}

// otherRecord differs from fullRecord in every field.
func otherRecord(t *testing.T) metadata.Record {
	return metadata.Record{
		Title: "Other Title", SortTitle: "other sort", OriginalTitle: "Other Original", Year: 1999,
		ReleaseDate: mustDate(t, "1999-01-02"), Summary: "Other summary.", Tagline: "Other tagline.",
		ContentRating: "R", Genres: []string{"Comedy"}, Studios: []string{"Studio B"},
		Countries: []string{"US"}, Collections: []string{"Other Collection"},
		People:      []metadata.Credit{{Name: "Bob Writer", Role: metadata.RoleWriter, Order: 0}},
		ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTVDB, Value: "81189"}},
	}
}

func localCandidate(kind artwork.Kind, path string, size int64) artwork.Candidate {
	return artwork.Candidate{
		Kind: kind, Source: artwork.SourceLocal, Location: artwork.LocalLocation(path), Size: size,
		ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
	}
}

func embeddedCandidate(kind artwork.Kind, mediaPath string, stream int) artwork.Candidate {
	return artwork.Candidate{
		Kind: kind, Source: artwork.SourceEmbedded, Location: artwork.EmbeddedLocation(mediaPath, stream), Size: 10,
		ModifiedAt: time.UnixMilli(fileModifiedMilli).UTC(),
	}
}

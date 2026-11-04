package sqlite_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func TestSaveMetadataRoundTripsAFullRecord(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)

	if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, fullRecord(t)); err != nil {
		t.Fatalf("SaveMetadata error = %v", err)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
	if err != nil {
		t.Fatalf("ItemMetadataState error = %v", err)
	}

	assertDeepEqual(t, "record", state.Record, storedFullRecord(t))

	if len(state.Locks.Fields()) != 0 || len(state.CurrentArtwork) != 0 {
		t.Errorf("state = %+v, want no locks and no artwork", state)
	}
}

func TestSaveMetadataComputesTheSortTitleWhenTheRecordHasNone(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	record := metadata.Record{Title: "The Matrix"}

	if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, record); err != nil {
		t.Fatalf("SaveMetadata error = %v", err)
	}

	state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
	if err != nil || state.Record.SortTitle != "matrix" {
		t.Errorf("sort title = %q, %v, want %q", state.Record.SortTitle, err, "matrix")
	}
}

// ED-02: one test case per lockable field group; the locked field keeps the stored value.
func TestSaveMetadataKeepsLockedFields(t *testing.T) {
	t.Parallel()

	keep := map[metadata.Field]func(target *metadata.Record, stored metadata.Record){
		metadata.FieldTitle:         func(target *metadata.Record, stored metadata.Record) { target.Title = stored.Title },
		metadata.FieldSortTitle:     func(target *metadata.Record, stored metadata.Record) { target.SortTitle = stored.SortTitle },
		metadata.FieldOriginalTitle: func(target *metadata.Record, stored metadata.Record) { target.OriginalTitle = stored.OriginalTitle },
		metadata.FieldYear:          func(target *metadata.Record, stored metadata.Record) { target.Year = stored.Year },
		metadata.FieldReleaseDate:   func(target *metadata.Record, stored metadata.Record) { target.ReleaseDate = stored.ReleaseDate },
		metadata.FieldSummary:       func(target *metadata.Record, stored metadata.Record) { target.Summary = stored.Summary },
		metadata.FieldTagline:       func(target *metadata.Record, stored metadata.Record) { target.Tagline = stored.Tagline },
		metadata.FieldContentRating: func(target *metadata.Record, stored metadata.Record) { target.ContentRating = stored.ContentRating },
		metadata.FieldGenres:        func(target *metadata.Record, stored metadata.Record) { target.Genres = stored.Genres },
		metadata.FieldStudios:       func(target *metadata.Record, stored metadata.Record) { target.Studios = stored.Studios },
		metadata.FieldCountries:     func(target *metadata.Record, stored metadata.Record) { target.Countries = stored.Countries },
		metadata.FieldCollections:   func(target *metadata.Record, stored metadata.Record) { target.Collections = stored.Collections },
		metadata.FieldPeople:        func(target *metadata.Record, stored metadata.Record) { target.People = stored.People },
		metadata.FieldProviderIDs:   func(target *metadata.Record, stored metadata.Record) { target.ProviderIDs = stored.ProviderIDs },
	}

	for field, restore := range keep {
		t.Run(field.String(), func(t *testing.T) {
			t.Parallel()

			fixture := newMetadataFixture(t)
			if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, fullRecord(t)); err != nil {
				t.Fatalf("first SaveMetadata error = %v", err)
			}

			fixture.lock(t, fixture.movieID, field)

			if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, otherRecord(t)); err != nil {
				t.Fatalf("second SaveMetadata error = %v", err)
			}

			state, err := fixture.catalog.ItemMetadataState(t.Context(), fixture.movieID)
			if err != nil {
				t.Fatalf("ItemMetadataState error = %v", err)
			}

			want := otherRecord(t)
			restore(&want, storedFullRecord(t))
			want = normalizedStored(want)
			assertDeepEqual(t, "record with "+field.String()+" locked", state.Record, want)

			if !state.Locks.Has(field) {
				t.Errorf("state locks = %v, want %s", state.Locks.Fields(), field)
			}
		})
	}
}

// normalizedStored applies what the writer does to a record on top of the locks: the sort
// title is folded.
func normalizedStored(record metadata.Record) metadata.Record {
	record.SortTitle = metadata.SortTitleFor(record)

	return record
}

func TestSaveMetadataRejectsWhatItCannotStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		itemID string
		change func(*metadata.Record)
		want   error
	}{
		{"unknown item", unknownID, func(*metadata.Record) {}, domain.ErrNotFound},
		{"empty title", "", func(record *metadata.Record) { record.Title = " " }, domain.ErrInvalid},
		{"negative year", "", func(record *metadata.Record) { record.Year = -1 }, domain.ErrInvalid},
		{"unknown role", "", func(record *metadata.Record) { record.People[0].Role = "gaffer" }, domain.ErrInvalid},
		{"negative credit order", "", func(record *metadata.Record) { record.People[0].Order = -1 }, domain.ErrInvalid},
		{"blank person", "", func(record *metadata.Record) { record.People[0].Name = " " }, domain.ErrInvalid},
		{"malformed provider ID", "", func(record *metadata.Record) { record.ProviderIDs[0].Value = "x" }, domain.ErrInvalid},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fixture := newMetadataFixture(t)
			itemID := test.itemID
			if itemID == "" {
				itemID = fixture.movieID
			}

			record := fullRecord(t)
			test.change(&record)

			err := fixture.catalog.SaveMetadata(t.Context(), itemID, record)
			if !errors.Is(err, test.want) {
				t.Errorf("SaveMetadata error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestItemMetadataStateOfAnUnknownItem(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)

	if _, err := fixture.catalog.ItemMetadataState(t.Context(), unknownID); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("ItemMetadataState error = %v, want ErrNotFound", err)
	}
}

func TestSaveMetadataSharesTagsAndPeopleAndDropsUnusedOnes(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	second := fixture.addMovie(t, 2)

	for _, id := range []string{fixture.movieID, second} {
		record := metadata.Record{
			Title: "Film " + id[:8], Genres: []string{"Drama", "drama", " "},
			People: []metadata.Credit{{Name: "Ann Actor", Role: metadata.RoleActor}},
		}
		if id == second {
			record.People[0].Name = "ANN ACTOR"
		}

		if err := fixture.catalog.SaveMetadata(t.Context(), id, record); err != nil {
			t.Fatalf("SaveMetadata(%s) error = %v", id, err)
		}
	}

	if tags, people := countRows(t, fixture.inspection, `SELECT count(*) FROM tags`),
		countRows(t, fixture.inspection, `SELECT count(*) FROM people`); tags != 1 || people != 1 {
		t.Errorf("tags = %d, people = %d, want 1 and 1 (shared, case-insensitive)", tags, people)
	}

	for _, id := range []string{fixture.movieID, second} {
		if err := fixture.catalog.SaveMetadata(t.Context(), id, metadata.Record{Title: "Plain"}); err != nil {
			t.Fatalf("SaveMetadata(%s) error = %v", id, err)
		}
	}

	if tags, people := countRows(t, fixture.inspection, `SELECT count(*) FROM tags`),
		countRows(t, fixture.inspection, `SELECT count(*) FROM people`); tags != 0 || people != 0 {
		t.Errorf("tags = %d, people = %d, want unused rows deleted", tags, people)
	}
}

func TestDeletingAnItemRemovesItsMetadataRows(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	if err := fixture.catalog.SaveMetadata(t.Context(), fixture.movieID, fullRecord(t)); err != nil {
		t.Fatalf("SaveMetadata error = %v", err)
	}

	fixture.lock(t, fixture.movieID, metadata.FieldTitle)
	execute(t, fixture.inspection, `DELETE FROM items WHERE id = ?`, fixture.movieID)

	for _, table := range []string{"item_tags", "item_people", "item_provider_ids", "item_field_locks"} {
		if count := countRows(t, fixture.inspection, `SELECT count(*) FROM `+table); count != 0 {
			t.Errorf("%s has %d rows after the item was deleted, want 0", table, count)
		}
	}
}

func TestSaveMetadataErrorsHaveNoSQLDetails(t *testing.T) {
	t.Parallel()

	fixture := newMetadataFixture(t)
	err := fixture.catalog.SaveMetadata(t.Context(), unknownID, fullRecord(t))

	if err == nil || strings.Contains(err.Error(), "UNIQUE") {
		t.Errorf("SaveMetadata error = %v, want a neutral error", err)
	}
}

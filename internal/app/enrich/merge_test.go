package enrich_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/app/enrich"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// fieldValue reads one lockable record field, so the lock test covers every field.
func fieldValue(record metadata.Record, field metadata.Field) any {
	values := map[metadata.Field]any{
		metadata.FieldTitle:         record.Title,
		metadata.FieldSortTitle:     record.SortTitle,
		metadata.FieldOriginalTitle: record.OriginalTitle,
		metadata.FieldYear:          record.Year,
		metadata.FieldReleaseDate:   record.ReleaseDate,
		metadata.FieldSummary:       record.Summary,
		metadata.FieldTagline:       record.Tagline,
		metadata.FieldContentRating: record.ContentRating,
		metadata.FieldGenres:        record.Genres,
		metadata.FieldStudios:       record.Studios,
		metadata.FieldCountries:     record.Countries,
		metadata.FieldCollections:   record.Collections,
		metadata.FieldPeople:        record.People,
		metadata.FieldProviderIDs:   record.ProviderIDs,
	}

	return values[field]
}

func userEditedRecord(t *testing.T) metadata.Record {
	t.Helper()

	return metadata.Record{
		Title: "My Matrix", SortTitle: "matrix mine", OriginalTitle: "Mine", Year: 2001,
		ReleaseDate: mustDate(t, "2001-01-01"), Summary: "Mine.", Tagline: "Mine!", ContentRating: "FSK 6",
		Genres: []string{"Mine"}, Studios: []string{"Home"}, Countries: []string{"AT"},
		Collections: []string{"Favourites"}, People: []metadata.Credit{{Name: "Me", Role: metadata.RoleActor}},
		ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderIMDb, Value: "tt0000001"}},
	}
}

func fullProviderRecord(t *testing.T) metadata.Record {
	t.Helper()

	return metadata.Record{
		Title: "The Matrix", SortTitle: "matrix", OriginalTitle: "The Matrix", Year: 1999,
		ReleaseDate: mustDate(t, "1999-03-31"), Summary: "A hacker learns the truth.", Tagline: "Free your mind.",
		ContentRating: "R", Genres: []string{"Action"}, Studios: []string{"Warner Bros."},
		Countries: []string{"US"}, Collections: []string{"The Matrix Collection"},
		People:      []metadata.Credit{{Name: "Keanu Reeves", Role: metadata.RoleActor}},
		ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: matrixTMDBID}},
	}
}

func mustDate(t *testing.T, text string) metadata.Date {
	t.Helper()

	date, err := metadata.ParseDate(text)
	if err != nil {
		t.Fatalf("parse date %q: %v", text, err)
	}

	return date
}

func TestEnrichItemKeepsEveryLockedField(t *testing.T) {
	for _, field := range metadata.Fields() {
		if strings.HasPrefix(field.String(), "artwork.") {
			continue // artwork locks are covered by the artwork tests
		}

		t.Run(field.String(), func(t *testing.T) {
			h := newHarness()
			snapshot := movieSnapshot()
			snapshot.Record = userEditedRecord(t)
			snapshot.Locks = metadata.NewLockSet([]metadata.Field{field})
			h.setSnapshot(snapshot)
			provider := fullProviderRecord(t)
			// The user's record names the movie by its IMDb ID, which is looked up directly.
			h.movies.details = map[string]enrich.Details{"tt0000001": {Record: provider}}

			h.mustEnrich(t, enrich.OutcomeMatched)

			saved := h.onlyEnrichment(t).Record
			if got, want := fieldValue(saved, field), fieldValue(snapshot.Record, field); !reflect.DeepEqual(got, want) {
				t.Errorf("locked %s = %v, want %v", field, got, want)
			}

			if field != metadata.FieldSummary && saved.Summary != provider.Summary {
				t.Errorf("unlocked summary = %q, want the provider's", saved.Summary)
			}
		})
	}
}

func TestEnrichItemMergeOrder(t *testing.T) {
	h := newHarness()
	snapshot := movieSnapshot()
	snapshot.Local.NFO = metadata.Record{Title: "The Matrix (NFO)", Year: 1999}
	snapshot.Local.Filename = metadata.Record{Title: "The Matrix", Year: 1999, Tagline: "from the file name"}
	snapshot.LinkedRef = matrixCandidate().Ref
	h.setSnapshot(snapshot)

	h.mustEnrich(t, enrich.OutcomeMatched)

	saved := h.onlyEnrichment(t).Record
	if saved.Title != "The Matrix (NFO)" {
		t.Errorf("title = %q, want the NFO's over the provider's", saved.Title)
	}

	if saved.Summary != matrixDetails().Record.Summary {
		t.Errorf("summary = %q, want the provider's", saved.Summary)
	}

	if saved.Tagline != "from the file name" {
		t.Errorf("tagline = %q, want the file name's where the provider has none", saved.Tagline)
	}
}

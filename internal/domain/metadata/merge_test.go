package metadata_test

import (
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/domain/metadata"
)

func mustDate(t *testing.T, text string) metadata.Date {
	t.Helper()

	date, err := metadata.ParseDate(text)
	if err != nil {
		t.Fatalf("ParseDate(%q) error = %v", text, err)
	}

	return date
}

// storedRecord is what the catalog holds before the merge, for example values a user set.
func storedRecord(t *testing.T) metadata.Record {
	t.Helper()

	return metadata.Record{
		Title: "Stored Title", SortTitle: "stored sort", OriginalTitle: "Stored Original",
		Year: 1990, ReleaseDate: mustDate(t, "1990-06-01"), Summary: "Stored summary.",
		Tagline: "Stored tagline", ContentRating: "FSK 12",
		Genres: []string{"Drama"}, Studios: []string{"Stored Studio"},
		Countries: []string{"DE"}, Collections: []string{"Stored Collection"},
		People:      []metadata.Credit{{Name: "Stored Person", Role: metadata.RoleDirector}},
		ProviderIDs: []metadata.ProviderID{{Provider: metadata.ProviderTMDB, Value: "1"}},
	}
}

// nfoRecord is a complete record of a higher-priority source.
func nfoRecord(t *testing.T) metadata.Record {
	t.Helper()

	return metadata.Record{
		Title: "The Matrix", SortTitle: "matrix", OriginalTitle: "The Matrix",
		Year: 1999, ReleaseDate: mustDate(t, "1999-03-31"), Summary: "A hacker learns the truth.",
		Tagline: "Welcome to the Real World", ContentRating: "R",
		Genres: []string{"Action", "Science Fiction"}, Studios: []string{"Warner Bros."},
		Countries: []string{"US"}, Collections: []string{"The Matrix Collection"},
		People: []metadata.Credit{
			{Name: "Keanu Reeves", Role: metadata.RoleActor, Character: "Neo", Order: 0},
			{Name: "Lana Wachowski", Role: metadata.RoleDirector},
		},
		ProviderIDs: []metadata.ProviderID{
			{Provider: metadata.ProviderTMDB, Value: "603"},
			{Provider: metadata.ProviderIMDb, Value: "tt0133093"},
		},
	}
}

// fileNameRecord is what the name parser derives: title and year only.
func fileNameRecord() metadata.Record {
	return metadata.Record{Title: "Matrix", Year: 1998}
}

// fieldCopier copies one record field from source to target.
type fieldCopier func(target *metadata.Record, source metadata.Record)

// recordFieldCopiers returns a copier for every lockable record field.
func recordFieldCopiers() map[metadata.Field]fieldCopier {
	type record = metadata.Record

	return map[metadata.Field]fieldCopier{
		metadata.FieldTitle:         func(target *record, source record) { target.Title = source.Title },
		metadata.FieldSortTitle:     func(target *record, source record) { target.SortTitle = source.SortTitle },
		metadata.FieldOriginalTitle: func(target *record, source record) { target.OriginalTitle = source.OriginalTitle },
		metadata.FieldYear:          func(target *record, source record) { target.Year = source.Year },
		metadata.FieldReleaseDate:   func(target *record, source record) { target.ReleaseDate = source.ReleaseDate },
		metadata.FieldSummary:       func(target *record, source record) { target.Summary = source.Summary },
		metadata.FieldTagline:       func(target *record, source record) { target.Tagline = source.Tagline },
		metadata.FieldContentRating: func(target *record, source record) { target.ContentRating = source.ContentRating },
		metadata.FieldGenres:        func(target *record, source record) { target.Genres = source.Genres },
		metadata.FieldStudios:       func(target *record, source record) { target.Studios = source.Studios },
		metadata.FieldCountries:     func(target *record, source record) { target.Countries = source.Countries },
		metadata.FieldCollections:   func(target *record, source record) { target.Collections = source.Collections },
		metadata.FieldPeople:        func(target *record, source record) { target.People = source.People },
		metadata.FieldProviderIDs:   func(target *record, source record) { target.ProviderIDs = source.ProviderIDs },
	}
}

func TestMergeKeepsEveryLockedField(t *testing.T) {
	t.Parallel()

	for _, field := range metadata.Fields() {
		t.Run(field.String(), func(t *testing.T) {
			t.Parallel()

			current := storedRecord(t)
			sources := []metadata.SourcedRecord{
				{Source: metadata.SourceNFO, Record: nfoRecord(t)},
				{Source: metadata.SourceFilename, Record: fileNameRecord()},
			}

			want := nfoRecord(t)
			if copyField, isRecordField := recordFieldCopiers()[field]; isRecordField {
				copyField(&want, current)
			}

			got := metadata.Merge(current, metadata.NewLockSet([]metadata.Field{field}), sources)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Merge with %q locked =\n%+v\nwant\n%+v", field, got, want)
			}
		})
	}
}

func TestMergeCoversEveryRecordField(t *testing.T) {
	t.Parallel()

	// The lock test is only complete if it knows how to copy every field of Record.
	if got, want := len(recordFieldCopiers()), reflect.TypeFor[metadata.Record]().NumField(); got != want {
		t.Errorf("recordFieldCopiers has %d fields, Record has %d", got, want)
	}
}

func TestMerge(t *testing.T) {
	t.Parallel()

	matrix := nfoRecord(t)
	userRecord := metadata.Record{Title: "My Matrix", Genres: []string{"Favourites"}}
	partialNFO := metadata.Record{Summary: "Only a plot.", Year: 1999}
	listOnlySecond := metadata.Record{Title: "Second", Genres: []string{"Action"}, People: matrix.People}
	allLocked := metadata.NewLockSet(metadata.Fields())

	tests := []struct {
		name    string
		current metadata.Record
		locks   metadata.LockSet
		sources []metadata.SourcedRecord
		want    metadata.Record
	}{
		{name: "no sources and no locks", current: storedRecord(t), want: metadata.Record{}},
		{name: "no sources keeps locked fields", current: storedRecord(t), locks: allLocked, want: storedRecord(t)},
		{
			name:    "nfo before filename",
			current: storedRecord(t),
			sources: []metadata.SourcedRecord{
				{Source: metadata.SourceNFO, Record: matrix},
				{Source: metadata.SourceFilename, Record: fileNameRecord()},
			},
			want: matrix,
		},
		{
			name: "scalars fall through to the next source with a value",
			sources: []metadata.SourcedRecord{
				{Source: metadata.SourceNFO, Record: partialNFO},
				{Source: metadata.SourceFilename, Record: fileNameRecord()},
			},
			want: metadata.Record{Title: "Matrix", Year: 1999, Summary: "Only a plot."},
		},
		{
			name: "list from the second source",
			sources: []metadata.SourcedRecord{
				{Source: metadata.SourceNFO, Record: metadata.Record{Title: "First", Genres: []string{}}},
				{Source: metadata.SourceFilename, Record: listOnlySecond},
			},
			want: metadata.Record{Title: "First", Genres: []string{"Action"}, People: matrix.People},
		},
		{
			name: "lists are taken whole, never mixed",
			sources: []metadata.SourcedRecord{
				{Source: metadata.SourceNFO, Record: metadata.Record{Genres: []string{"Drama"}}},
				{Source: metadata.SourceFilename, Record: metadata.Record{Genres: []string{"Action", "Comedy"}}},
			},
			want: metadata.Record{Genres: []string{"Drama"}},
		},
		{
			name: "user source first",
			sources: []metadata.SourcedRecord{
				{Source: metadata.SourceUser, Record: userRecord},
				{Source: metadata.SourceNFO, Record: matrix},
			},
			want: withTitleAndGenres(matrix, "My Matrix", []string{"Favourites"}),
		},
		{
			name:    "locked empty field stays empty",
			current: metadata.Record{Title: "Stored"},
			locks:   metadata.NewLockSet([]metadata.Field{metadata.FieldTagline, metadata.FieldGenres}),
			sources: []metadata.SourcedRecord{{Source: metadata.SourceNFO, Record: matrix}},
			want:    withTaglineAndGenres(matrix, "", nil),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := metadata.Merge(test.current, test.locks, test.sources)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("Merge() =\n%+v\nwant\n%+v", got, test.want)
			}
		})
	}
}

func withTitleAndGenres(record metadata.Record, title string, genres []string) metadata.Record {
	record.Title = title
	record.Genres = genres

	return record
}

func withTaglineAndGenres(record metadata.Record, tagline string, genres []string) metadata.Record {
	record.Tagline = tagline
	record.Genres = genres

	return record
}

func TestMergeSharesNoListStorage(t *testing.T) {
	t.Parallel()

	current := storedRecord(t)
	source := nfoRecord(t)
	locks := metadata.NewLockSet([]metadata.Field{metadata.FieldStudios})

	merged := metadata.Merge(current, locks, []metadata.SourcedRecord{{Source: metadata.SourceNFO, Record: source}})
	merged.Genres[0] = "changed"
	merged.Studios[0] = "changed"

	if source.Genres[0] != "Action" || current.Studios[0] != "Stored Studio" {
		t.Errorf("changing the merge result changed its inputs: genres %v, studios %v", source.Genres, current.Studios)
	}
}

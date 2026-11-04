package metadata

import "slices"

// Merge combines the records of ordered sources (highest priority first) into the record an
// item stores (ADR 0027 §6):
//   - a scalar field takes the value of the first source that has one;
//   - a list field is taken whole from the first source with a non-empty list, so genres of
//     two sources are never mixed;
//   - a locked field keeps its value from current, whatever the sources say.
//
// Unlocked fields come from the sources only: a value that disappears from every source
// (for example a tagline deleted from the NFO) disappears from the result. The result never
// shares list storage with current or the sources.
func Merge(current Record, locks LockSet, sources []SourcedRecord) Record {
	merger := recordMerger{current: current, locks: locks, sources: sources}

	merged := merger.scalars()
	merger.lists(&merged)

	return merged
}

// recordMerger bundles the merge inputs so each field needs one short line.
type recordMerger struct {
	current Record
	locks   LockSet
	sources []SourcedRecord
}

func (merger recordMerger) scalars() Record {
	return Record{
		Title:         mergeScalar(merger, FieldTitle, func(record Record) string { return record.Title }),
		SortTitle:     mergeScalar(merger, FieldSortTitle, func(record Record) string { return record.SortTitle }),
		OriginalTitle: mergeScalar(merger, FieldOriginalTitle, func(record Record) string { return record.OriginalTitle }),
		Year:          mergeScalar(merger, FieldYear, func(record Record) int { return record.Year }),
		ReleaseDate:   mergeScalar(merger, FieldReleaseDate, func(record Record) Date { return record.ReleaseDate }),
		Summary:       mergeScalar(merger, FieldSummary, func(record Record) string { return record.Summary }),
		Tagline:       mergeScalar(merger, FieldTagline, func(record Record) string { return record.Tagline }),
		ContentRating: mergeScalar(merger, FieldContentRating, func(record Record) string { return record.ContentRating }),
	}
}

func (merger recordMerger) lists(merged *Record) {
	merged.Genres = mergeList(merger, FieldGenres, func(record Record) []string { return record.Genres })
	merged.Studios = mergeList(merger, FieldStudios, func(record Record) []string { return record.Studios })
	merged.Countries = mergeList(merger, FieldCountries, func(record Record) []string { return record.Countries })
	merged.Collections = mergeList(merger, FieldCollections, func(record Record) []string { return record.Collections })
	merged.People = mergeList(merger, FieldPeople, func(record Record) []Credit { return record.People })
	merged.ProviderIDs = mergeList(merger, FieldProviderIDs, func(record Record) []ProviderID { return record.ProviderIDs })
}

func mergeScalar[Value comparable](merger recordMerger, field Field, valueOf func(Record) Value) Value {
	if merger.locks.Has(field) {
		return valueOf(merger.current)
	}

	var empty Value

	for _, source := range merger.sources {
		if value := valueOf(source.Record); value != empty {
			return value
		}
	}

	return empty
}

func mergeList[Element any](merger recordMerger, field Field, listOf func(Record) []Element) []Element {
	if merger.locks.Has(field) {
		return slices.Clone(listOf(merger.current))
	}

	for _, source := range merger.sources {
		if list := listOf(source.Record); len(list) > 0 {
			return slices.Clone(list)
		}
	}

	return nil
}

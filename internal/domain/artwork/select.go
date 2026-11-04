package artwork

import (
	"cmp"
	"slices"
	"strings"
)

// Select chooses the artwork of one kind for an item (ADR 0027 §8, UX ED-05) from the
// candidates of that kind. A locked kind keeps current, even when its file is gone, and has
// no selection without one. Otherwise local pictures win by priority, then embedded ones;
// provider and upload pictures (milestone 6) follow until milestone 6 gives them their
// place in the ordered source list. Ties are broken by location, so the result never
// depends on the order of candidates. The second result is false when nothing is selected.
func Select(candidates []Candidate, locked bool, current *Candidate) (Candidate, bool) {
	if locked {
		if current == nil {
			return Candidate{}, false
		}

		return *current, true
	}

	if len(candidates) == 0 {
		return Candidate{}, false
	}

	return slices.MinFunc(candidates, compareCandidates), true
}

func compareCandidates(first, second Candidate) int {
	return cmp.Or(
		cmp.Compare(sourceRank(first.Source), sourceRank(second.Source)),
		cmp.Compare(first.Priority, second.Priority),
		strings.Compare(first.Location.RelativePath, second.Location.RelativePath),
		cmp.Compare(first.Location.StreamIndex, second.Location.StreamIndex),
		strings.Compare(first.Location.DataPath, second.Location.DataPath),
	)
}

// sourceRank orders sources by selection preference; an unknown source ranks last.
func sourceRank(source SourceKind) int {
	preference := []SourceKind{SourceLocal, SourceEmbedded, SourceProvider, SourceUpload}
	if rank := slices.Index(preference, source); rank >= 0 {
		return rank
	}

	return len(preference)
}

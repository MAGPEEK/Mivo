package match

import (
	"cmp"
	"slices"
)

// Query is what the scan or the identify dialog knows about an item before matching. Year is
// 0 when unknown. Ref is set when the ID is known (NFO tag, pasted ID); the zero value means
// "search by title".
type Query struct {
	Title string
	Year  int
	Ref   ProviderRef
}

// Candidate is one provider entry that might be the item. It is provider-neutral: adapters
// translate their JSON into it.
type Candidate struct {
	Ref           ProviderRef
	Title         string
	OriginalTitle string
	// Year is the release or first-air year, 0 when unknown.
	Year int
	// Popularity is the provider's own popularity number; it only breaks ties between
	// equal confidences and never raises one.
	Popularity float64
}

// Scored is a candidate with its confidence in [0, 1].
type Scored struct {
	Candidate  Candidate
	Confidence float64
	// ExactID is true when the candidate is the entry the query named by ID.
	ExactID bool
}

// Rank scores all candidates against the query and returns them best first: by confidence,
// then by popularity, then in the provider's order. The input is not modified.
func Rank(query Query, candidates []Candidate) []Scored {
	ranked := make([]Scored, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, Scored{
			Candidate:  candidate,
			Confidence: Score(query, candidate),
			ExactID:    isExactID(query, candidate),
		})
	}

	slices.SortStableFunc(ranked, func(left, right Scored) int {
		if byConfidence := cmp.Compare(right.Confidence, left.Confidence); byConfidence != 0 {
			return byConfidence
		}

		return cmp.Compare(right.Candidate.Popularity, left.Candidate.Popularity)
	})

	return ranked
}

func isExactID(query Query, candidate Candidate) bool {
	return !query.Ref.IsZero() && query.Ref.ID == candidate.Ref.ID && query.Ref.Provider == candidate.Ref.Provider
}

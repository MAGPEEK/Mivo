package match

// Auto-match rule (ADR 0031 §9): the best candidate must reach the threshold and lead the
// runner-up by a clear margin, so two near-identical titles go to the user instead.
const (
	AutoMatchThreshold = 0.85
	AutoMatchLead      = 0.10

	// confidenceEpsilon absorbs float rounding at the two boundaries above: 0.95 - 0.85 is
	// not exactly 0.10 in binary floating point.
	confidenceEpsilon = 1e-9
)

// Decision is the outcome of matching one item.
type Decision struct {
	State MatchState
	// Chosen is the candidate to link; set only when a new automatic match was made.
	Chosen *Candidate
}

// Decide returns the best candidate and whether it may be linked automatically: its
// confidence reaches AutoMatchThreshold and leads the second best by AutoMatchLead. A
// candidate named by an exact provider ID is always automatic. The input need not be sorted.
func Decide(scored []Scored) (best *Scored, auto bool) {
	bestIndex := bestIndexOf(scored)
	if bestIndex < 0 {
		return nil, false
	}

	chosen := scored[bestIndex]
	if chosen.ExactID {
		return &chosen, true
	}

	if chosen.Confidence < AutoMatchThreshold-confidenceEpsilon {
		return &chosen, false
	}

	return &chosen, chosen.Confidence-runnerUpConfidence(scored, bestIndex) >= AutoMatchLead-confidenceEpsilon
}

// bestIndexOf returns the index of the highest confidence (exact ID first), -1 when empty.
func bestIndexOf(scored []Scored) int {
	bestIndex := -1

	for index, candidate := range scored {
		if bestIndex < 0 || outranks(candidate, scored[bestIndex]) {
			bestIndex = index
		}
	}

	return bestIndex
}

func outranks(candidate, other Scored) bool {
	if candidate.ExactID != other.ExactID {
		return candidate.ExactID
	}

	return candidate.Confidence > other.Confidence
}

// runnerUpConfidence is the highest confidence among all but the best candidate, 0 if alone.
func runnerUpConfidence(scored []Scored, bestIndex int) float64 {
	var runnerUp float64

	for index, candidate := range scored {
		if index != bestIndex {
			runnerUp = max(runnerUp, candidate.Confidence)
		}
	}

	return runnerUp
}

// Resolve is the scan's entry point. A disabled item stays disabled. An item whose
// provider_ids field is locked (the user identified it) keeps its state. Otherwise a clear
// winner links the item; without one the item keeps a match it already has and stays
// unmatched if it had none, so an empty or failed lookup never drops a match.
func Resolve(current MatchState, providerIDsLocked bool, scored []Scored) Decision {
	if current == MatchStateDisabled || providerIDsLocked {
		return Decision{State: current}
	}

	if best, auto := Decide(scored); auto {
		return Decision{State: MatchStateMatched, Chosen: &best.Candidate}
	}

	if current == MatchStateMatched {
		return Decision{State: MatchStateMatched}
	}

	return Decision{State: MatchStateUnmatched}
}

package match

// Scoring constants. A perfect title match with the same year scores 1.
const (
	yearFactorExact    = 1.0
	yearFactorOffByOne = 0.92
	// yearFactorUnknown keeps a perfect title without a year agreement below certainty, yet
	// above the auto-match threshold when it is the only good candidate.
	yearFactorUnknown = 0.9
	yearFactorFar     = 0.6
	yearToleranceOne  = 1

	// subtitlePenalty scales the similarity when only the part before a colon matches:
	// "Star Wars" is not "Star Wars: A New Hope". It stays below AutoMatchThreshold so a
	// subtitle match is never automatic.
	subtitlePenalty = 0.8
	// numberMismatchCap bounds the confidence when the sequel numbers differ ("Toy Story 2"
	// against "Toy Story 3"); it stays below AutoMatchThreshold.
	numberMismatchCap = 0.7
)

// Score returns the confidence in [0, 1] that the candidate is the queried item. An exact
// provider ID match is 1. Otherwise it is the best title similarity over title and original
// title, scaled by the year agreement and capped when sequel numbers differ.
func Score(query Query, candidate Candidate) float64 {
	if isExactID(query, candidate) {
		return 1
	}

	factor := yearFactor(query.Year, candidate.Year)

	return max(
		titleConfidence(query.Title, candidate.Title, factor),
		titleConfidence(query.Title, candidate.OriginalTitle, factor),
	)
}

func yearFactor(queryYear, candidateYear int) float64 {
	if queryYear <= 0 || candidateYear <= 0 {
		return yearFactorUnknown
	}

	switch difference := absolute(queryYear - candidateYear); {
	case difference == 0:
		return yearFactorExact
	case difference <= yearToleranceOne:
		return yearFactorOffByOne
	default:
		return yearFactorFar
	}
}

func absolute(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

// titleConfidence compares the full titles and, with the subtitle penalty, the parts before
// a colon, and returns the best result.
func titleConfidence(queryTitle, candidateTitle string, yearScale float64) float64 {
	queryFull, candidateFull := NormalizeTitle(queryTitle), NormalizeTitle(candidateTitle)
	best := pairConfidence(queryFull, candidateFull, yearScale)

	queryMain, candidateMain := colonMainTitle(queryTitle), colonMainTitle(candidateTitle)
	if queryMain != "" {
		best = max(best, subtitlePenalty*pairConfidence(queryMain, candidateFull, yearScale))
	}

	if candidateMain != "" {
		best = max(best, subtitlePenalty*pairConfidence(queryFull, candidateMain, yearScale))
	}

	if queryMain != "" && candidateMain != "" {
		best = max(best, subtitlePenalty*pairConfidence(queryMain, candidateMain, yearScale))
	}

	return best
}

// pairConfidence is the similarity of two normalised titles scaled by the year factor and
// capped when their sequel numbers differ.
func pairConfidence(left, right string, yearScale float64) float64 {
	left, right = canonicalNumbers(left), canonicalNumbers(right)
	confidence := similarity(left, right) * yearScale
	if numberSignature(left) != numberSignature(right) {
		return min(confidence, numberMismatchCap)
	}

	return confidence
}

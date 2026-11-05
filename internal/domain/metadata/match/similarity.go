package match

// maxComparedRunes bounds the edit distance work: titles are compared on their first 200
// characters, so a hostile provider response cannot make a comparison quadratic in a huge
// string. Real titles are far shorter.
const maxComparedRunes = 200

// similarity is 1 minus the normalised edit distance of two normalised titles. An empty
// title is similar to nothing: nothing says it is the same.
func similarity(left, right string) float64 {
	leftRunes, rightRunes := cappedRunes(left), cappedRunes(right)
	if len(leftRunes) == 0 || len(rightRunes) == 0 {
		return 0
	}

	longest := max(len(leftRunes), len(rightRunes))

	return 1 - float64(editDistance(leftRunes, rightRunes))/float64(longest)
}

func cappedRunes(text string) []rune {
	runes := []rune(text)
	if len(runes) > maxComparedRunes {
		return runes[:maxComparedRunes]
	}

	return runes
}

// editDistance is the Levenshtein distance, computed with two reused rows.
func editDistance(left, right []rune) int {
	previous := make([]int, len(right)+1)
	current := make([]int, len(right)+1)

	for column := range previous {
		previous[column] = column
	}

	for row := 1; row <= len(left); row++ {
		current[0] = row

		for column := 1; column <= len(right); column++ {
			substitution := previous[column-1]
			if left[row-1] != right[column-1] {
				substitution++
			}

			current[column] = min(substitution, previous[column]+1, current[column-1]+1)
		}

		previous, current = current, previous
	}

	return previous[len(right)]
}

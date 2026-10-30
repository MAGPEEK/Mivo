package main

import (
	"fmt"
	"path"
	"strings"
)

// action says what the generator does with a planned case.
type action int

const (
	// actionBuild builds the file, or keeps an existing one unless -force is given.
	actionBuild action = iota
	// actionSkip records the case as skipped because the local FFmpeg cannot build it.
	actionSkip
	// actionKeepOnly leaves a case excluded by -only untouched: an existing file from an
	// earlier run is listed as generated, so a filtered run never shrinks the manifest.
	actionKeepOnly
	// actionGap records a known gap (ADR 0021): a format FFmpeg cannot produce at all.
	actionGap
)

// plannedCase is one case with the decision of what to do with it.
type plannedCase struct {
	Case   mediaCase
	Action action
	// Reason explains actionSkip, actionKeepOnly and actionGap.
	Reason string
}

// planCases decides for every case name what to do. onlyGlob filters by name with
// path.Match syntax; "" selects every case. Alternatives sharing a name collapse into one
// plan: the first buildable alternative, or a skip that names what each one lacks.
func planCases(cases []mediaCase, caps capabilities, onlyGlob string) []plannedCase {
	var plans []plannedCase

	for _, alternatives := range groupByName(cases) {
		plans = append(plans, planAlternatives(alternatives, caps, onlyGlob))
	}

	return plans
}

func planAlternatives(alternatives []mediaCase, caps capabilities, onlyGlob string) plannedCase {
	first := alternatives[0]
	// Gaps are listed in every run, filtered or not, so the manifest always shows them.
	if first.isGap() {
		return plannedCase{Case: first, Action: actionGap, Reason: first.Gap}
	}

	if !matchesFilter(first.Name, onlyGlob) {
		return plannedCase{
			Case: first, Action: actionKeepOnly,
			Reason: fmt.Sprintf("not selected by -only %q", onlyGlob),
		}
	}

	reasons := make([]string, 0, len(alternatives))

	for _, alternative := range alternatives {
		missing := caps.missingRequirements(alternative)
		if missing == "" {
			return plannedCase{Case: alternative, Action: actionBuild}
		}

		reasons = append(reasons, missing)
	}

	return plannedCase{Case: first, Action: actionSkip, Reason: strings.Join(reasons, "; ")}
}

// groupByName groups cases by name, keeping the registry order of names and alternatives.
func groupByName(cases []mediaCase) [][]mediaCase {
	var groups [][]mediaCase

	indexByName := make(map[string]int)

	for _, c := range cases {
		index, seen := indexByName[c.Name]
		if !seen {
			indexByName[c.Name] = len(groups)
			groups = append(groups, []mediaCase{c})

			continue
		}

		groups[index] = append(groups[index], c)
	}

	return groups
}

// matchesFilter reports whether a case name matches the -only glob. The glob was validated
// when the flags were parsed, so a match error cannot occur here and counts as no match.
func matchesFilter(name, onlyGlob string) bool {
	if onlyGlob == "" {
		return true
	}

	matched, err := path.Match(onlyGlob, name)

	return err == nil && matched
}

// validateFilter rejects malformed -only globs before any work starts.
func validateFilter(onlyGlob string) error {
	if _, err := path.Match(onlyGlob, ""); err != nil {
		return fmt.Errorf("invalid -only glob %q: %w", onlyGlob, err)
	}

	return nil
}

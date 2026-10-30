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
	// earlier run is listed as a kept, generated file, so a filtered run never shrinks the
	// manifest.
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

// planCases decides for every case name what to do; filter selects the cases to build.
// Alternatives sharing a name collapse into one plan: the first buildable alternative, or a
// skip that names what each one lacks.
func planCases(cases []mediaCase, caps capabilities, filter caseFilter) []plannedCase {
	var plans []plannedCase

	for _, alternatives := range groupByName(cases) {
		plans = append(plans, planAlternatives(alternatives, caps, filter))
	}

	return plans
}

func planAlternatives(alternatives []mediaCase, caps capabilities, filter caseFilter) plannedCase {
	first := alternatives[0]
	// Gaps are listed in every run, filtered or not, so the manifest always shows them.
	if first.isGap() {
		return plannedCase{Case: first, Action: actionGap, Reason: first.Gap}
	}

	if !filter.matches(first.Name) {
		return plannedCase{
			Case: first, Action: actionKeepOnly,
			Reason: fmt.Sprintf("not selected by -only %q", filter),
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

// filterSeparator separates the globs of one -only value.
const filterSeparator = ","

// caseFilter is the parsed -only flag: a comma-separated list of path.Match globs. A case is
// selected when one of them matches its name. The zero value selects every case.
type caseFilter struct {
	// text is the flag value as given, for messages.
	text  string
	globs []string
}

// parseCaseFilter splits and validates an -only value, so a typo fails before any work
// starts instead of silently selecting nothing. Spaces around the globs are ignored.
func parseCaseFilter(text string) (caseFilter, error) {
	if text == "" {
		return caseFilter{}, nil
	}

	var globs []string

	for glob := range strings.SplitSeq(text, filterSeparator) {
		glob = strings.TrimSpace(glob)
		if glob == "" {
			return caseFilter{}, fmt.Errorf("empty glob in -only %q", text)
		}

		if _, err := path.Match(glob, ""); err != nil {
			return caseFilter{}, fmt.Errorf("invalid -only glob %q: %w", glob, err)
		}

		globs = append(globs, glob)
	}

	return caseFilter{text: text, globs: globs}, nil
}

// matches reports whether the filter selects the case name. The globs were validated by
// parseCaseFilter, so a match error cannot occur here and counts as no match.
func (f caseFilter) matches(name string) bool {
	if len(f.globs) == 0 {
		return true
	}

	for _, glob := range f.globs {
		if matched, err := path.Match(glob, name); err == nil && matched {
			return true
		}
	}

	return false
}

// String returns the flag value as given.
func (f caseFilter) String() string {
	return f.text
}

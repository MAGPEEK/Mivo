package corpustest_test

import (
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/corpustest"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/domain/playback/playbacktest"
)

// referenceProfileNames are the profile keys of the table; they must be the names the
// reference profiles carry, or a caller looking up profile.Name finds nothing.
func referenceProfileNames() []string {
	return []string{playbacktest.ChromiumDesktop().Name, playbacktest.FirefoxDesktop().Name}
}

func TestProfileKeysAreTheReferenceProfileNames(t *testing.T) {
	t.Parallel()

	if got, want := []string{corpustest.ChromiumDesktop, corpustest.FirefoxDesktop}, referenceProfileNames(); !slices.Equal(got, want) {
		t.Errorf("profile keys = %q, want the reference profile names %q", got, want)
	}
}

func TestCaseNamesAreSortedAndUnique(t *testing.T) {
	t.Parallel()

	names := corpustest.CaseNames()
	if len(names) == 0 {
		t.Fatal("CaseNames is empty")
	}

	if !slices.IsSorted(names) {
		t.Errorf("CaseNames is not sorted: %q", names)
	}

	if compacted := slices.Compact(slices.Clone(names)); len(compacted) != len(names) {
		t.Errorf("CaseNames has duplicates: %q", names)
	}
}

// TestEveryExpectationIsConsistent checks the rules of ADR 0023 every entry must follow, so
// a typo in the table is caught without FFmpeg: direct play copies everything and has no
// reasons, remux copies everything with at least one reason, transcode re-encodes at least
// one stream with at least one reason.
func TestEveryExpectationIsConsistent(t *testing.T) {
	t.Parallel()

	for _, name := range corpustest.CaseNames() {
		for _, profile := range referenceProfileNames() {
			expected, found := corpustest.Expected(name, profile)
			if !found {
				t.Errorf("%s has no expectation for %s", name, profile)

				continue
			}

			if problem := inconsistency(expected); problem != "" {
				t.Errorf("%s/%s: %s (%+v)", name, profile, problem, expected)
			}
		}
	}
}

func inconsistency(expected corpustest.Expectation) string {
	copiesAll := expected.Video != playback.ActionTranscode && expected.Audio != playback.ActionTranscode

	switch expected.Mode {
	case playback.ModeDirectPlay:
		if !copiesAll || len(expected.Reasons) > 0 {
			return "direct play must copy every stream and have no reasons"
		}
	case playback.ModeRemux:
		if !copiesAll || len(expected.Reasons) == 0 {
			return "remux must copy every stream and say why the file is not played directly"
		}
	case playback.ModeTranscode:
		if copiesAll || len(expected.Reasons) == 0 {
			return "transcode must re-encode a stream and say why"
		}
	default:
		return "unknown mode"
	}

	return ""
}

func TestExpectedRejectsUnknownCasesAndProfiles(t *testing.T) {
	t.Parallel()

	known := corpustest.CaseNames()[0]

	lookups := []struct{ caseName, profile string }{
		{caseName: "video-unknown-case", profile: corpustest.ChromiumDesktop},
		{caseName: known, profile: "safari-ios"},
		{caseName: known, profile: ""},
	}

	for _, lookup := range lookups {
		if expected, found := corpustest.Expected(lookup.caseName, lookup.profile); found {
			t.Errorf("Expected(%q, %q) = %+v, true; want not found", lookup.caseName, lookup.profile, expected)
		}
	}
}

// TestExpectedReturnsIndependentValues guards the promise that callers own the result: a
// test that appends to the reasons of one lookup must not change the next.
func TestExpectedReturnsIndependentValues(t *testing.T) {
	t.Parallel()

	const name = "video-mkv-hevc10-hdr10-ac3"

	first, found := corpustest.Expected(name, corpustest.ChromiumDesktop)
	if !found || len(first.Reasons) == 0 || len(first.Warnings) == 0 {
		t.Fatalf("Expected(%s) = %+v, %t; want reasons and warnings", name, first, found)
	}

	first.Reasons[0] = playback.Reason{Code: "changed"}
	first.Warnings[0] = playback.Warning{Code: "changed"}

	second, _ := corpustest.Expected(name, corpustest.ChromiumDesktop)
	if second.Reasons[0].Code == "changed" || second.Warnings[0].Code == "changed" {
		t.Errorf("a change to one result leaked into the next: %+v", second)
	}
}

// TestSeekExpectationsMatchTheirMode: a copied video starts at a keyframe at or before the
// requested position, a re-encoded one exactly there (ADR 0024). The two seek clips cover
// both paths.
func TestSeekExpectationsMatchTheirMode(t *testing.T) {
	t.Parallel()

	modes := map[playback.Mode]bool{}

	for _, seek := range corpustest.SeekExpectations() {
		expected, found := corpustest.Expected(seek.Case, seek.Profile)
		if !found {
			t.Errorf("seek case %s/%s has no expectation", seek.Case, seek.Profile)

			continue
		}

		modes[expected.Mode] = true

		switch {
		case seek.Requested <= 0:
			t.Errorf("%s: requested position %s, want a position inside the clip", seek.Case, seek.Requested)
		case expected.Video == playback.ActionTranscode && seek.ActualStart != seek.Requested:
			t.Errorf("%s re-encodes video, actual start %s, want the requested %s", seek.Case, seek.ActualStart, seek.Requested)
		case expected.Video == playback.ActionCopy && (seek.ActualStart <= 0 || seek.ActualStart > seek.Requested):
			t.Errorf("%s copies video, actual start %s, want a keyframe in (0, %s]", seek.Case, seek.ActualStart, seek.Requested)
		}
	}

	if !modes[playback.ModeRemux] || !modes[playback.ModeTranscode] {
		t.Errorf("seek expectations cover modes %v, want remux and transcode", modes)
	}
}

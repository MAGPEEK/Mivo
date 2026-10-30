package main

import (
	"strings"
	"testing"
)

func testCases() []mediaCase {
	return []mediaCase{
		{Name: "video-mkv-h264-aac", Output: "video/a.mkv", Muxer: "matroska", Encoders: []string{"libx264", "aac"}},
		{Name: "video-av1", Output: "video/av1.webm", Muxer: "webm", Encoders: []string{"libsvtav1"}},
		{Name: "video-mxf", Output: "video/b.mxf", Muxer: "mxf", Encoders: []string{"mpeg2video"}},
		{Name: "video-av1", Output: "video/av1.webm", Muxer: "webm", Encoders: []string{"libaom-av1"}},
	}
}

func mustParseFilter(t *testing.T, text string) caseFilter {
	t.Helper()

	filter, err := parseCaseFilter(text)
	if err != nil {
		t.Fatalf("parseCaseFilter(%q): %v", text, err)
	}

	return filter
}

func TestPlanCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		encoders []string
		muxers   []string
		filter   string
		// want lists "<name> <action> <chosen encoder>" per plan in order.
		want []string
	}{
		{
			name:     "everything available picks the preferred alternative",
			encoders: []string{"libx264", "aac", "libsvtav1", "libaom-av1", "mpeg2video"},
			muxers:   []string{"matroska", "webm", "mxf"},
			want:     []string{"video-mkv-h264-aac build libx264", "video-av1 build libsvtav1", "video-mxf build mpeg2video"},
		},
		{
			name:     "missing capabilities skip and fall back",
			encoders: []string{"libx264", "aac", "libaom-av1", "mpeg2video"},
			muxers:   []string{"matroska", "webm"},
			want:     []string{"video-mkv-h264-aac build libx264", "video-av1 build libaom-av1", "video-mxf skip mpeg2video"},
		},
		{
			name:     "glob limits the built cases, the rest is keep-only",
			encoders: []string{"libx264", "aac", "libsvtav1", "mpeg2video"},
			muxers:   []string{"matroska", "webm", "mxf"},
			filter:   "video-m*",
			want:     []string{"video-mkv-h264-aac build libx264", "video-av1 keep libsvtav1", "video-mxf build mpeg2video"},
		},
		{
			name:     "a comma-separated list selects every case one of its globs matches",
			encoders: []string{"libx264", "aac", "libsvtav1", "mpeg2video"},
			muxers:   []string{"matroska", "webm", "mxf"},
			filter:   "video-av1,video-mx*",
			want:     []string{"video-mkv-h264-aac keep libx264", "video-av1 build libsvtav1", "video-mxf build mpeg2video"},
		},
		{
			name:     "excluded cases are keep-only even without capabilities",
			encoders: nil,
			muxers:   nil,
			filter:   "nothing-*",
			want:     []string{"video-mkv-h264-aac keep libx264", "video-av1 keep libsvtav1", "video-mxf keep mpeg2video"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plans := planCases(testCases(), capabilitiesOf(test.encoders, test.muxers), mustParseFilter(t, test.filter))
			if len(plans) != len(test.want) {
				t.Fatalf("got %d plans, want %d", len(plans), len(test.want))
			}

			for index, plan := range plans {
				if got := describePlan(plan); got != test.want[index] {
					t.Errorf("plan %d = %q, want %q", index, got, test.want[index])
				}
			}
		})
	}
}

func describePlan(plan plannedCase) string {
	names := map[action]string{actionBuild: "build", actionSkip: "skip", actionKeepOnly: "keep"}

	return plan.Case.Name + " " + names[plan.Action] + " " + plan.Case.Encoders[0]
}

func TestPlanCasesExplainsSkips(t *testing.T) {
	t.Parallel()

	plans := planCases(testCases(), capabilitiesOf([]string{"libx264"}, []string{"matroska"}), mustParseFilter(t, "video-*"))

	tests := []struct {
		index int
		want  string
	}{
		{index: 0, want: "ffmpeg lacks encoder aac"},
		{index: 1, want: "ffmpeg lacks encoder libsvtav1, muxer webm; ffmpeg lacks encoder libaom-av1, muxer webm"},
		{index: 2, want: "ffmpeg lacks encoder mpeg2video, muxer mxf"},
	}

	for _, test := range tests {
		if got := plans[test.index].Reason; got != test.want {
			t.Errorf("reason %d = %q, want %q", test.index, got, test.want)
		}
	}
}

func TestPlanCasesNamesTheFilterForExcludedCases(t *testing.T) {
	t.Parallel()

	plans := planCases(testCases(), capabilitiesOf(nil, nil), mustParseFilter(t, "video-mxf,audio-*"))

	if !strings.Contains(plans[0].Reason, `not selected by -only "video-mxf,audio-*"`) {
		t.Errorf("reason = %q", plans[0].Reason)
	}
}

func TestParseCaseFilterAcceptsGlobLists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		// matching and other are case names the filter must and must not select.
		matching []string
		other    []string
	}{
		{text: "", matching: []string{"video-mkv-h264-aac", "audio-mp3"}},
		{text: "*", matching: []string{"video-mkv-h264-aac", "audio-mp3"}},
		{text: "video-*", matching: []string{"video-mkv-h264-aac"}, other: []string{"audio-mp3"}},
		{text: "video-mkv-h264-aac", matching: []string{"video-mkv-h264-aac"}, other: []string{"video-mkv-h264-aac-long"}},
		{
			text:     "video-[mw]*",
			matching: []string{"video-mkv-h264-aac", "video-webm-vp9-opus"},
			other:    []string{"video-avi-xvid-mp3"},
		},
		{
			text:     "video-mp4-*,video-webm-vp9-opus",
			matching: []string{"video-mp4-h264-aac-faststart", "video-webm-vp9-opus"},
			other:    []string{"video-webm-av1-opus", "video-mkv-h264-aac"},
		},
		{
			// Spaces around the commas are ignored; case names never contain spaces.
			text:     " audio-mp3 , image-* ",
			matching: []string{"audio-mp3", "image-png"},
			other:    []string{"audio-flac-16bit-44k"},
		},
	}

	for _, test := range tests {
		filter := mustParseFilter(t, test.text)

		for _, name := range test.matching {
			if !filter.matches(name) {
				t.Errorf("filter %q does not select %s", test.text, name)
			}
		}

		for _, name := range test.other {
			if filter.matches(name) {
				t.Errorf("filter %q selects %s", test.text, name)
			}
		}

		if filter.String() != test.text {
			t.Errorf("filter %q prints as %q", test.text, filter.String())
		}
	}
}

func TestParseCaseFilterValidatesEveryGlob(t *testing.T) {
	t.Parallel()

	tests := []struct {
		text string
		want string
	}{
		{text: "video-[", want: `invalid -only glob "video-["`},
		{text: "video-*,audio-[", want: `invalid -only glob "audio-["`},
		{text: "video-*,", want: `empty glob in -only "video-*,"`},
		{text: ",video-*", want: `empty glob in -only ",video-*"`},
		{text: "video-*, ,audio-*", want: `empty glob in -only "video-*, ,audio-*"`},
	}

	for _, test := range tests {
		_, err := parseCaseFilter(test.text)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("parseCaseFilter(%q) = %v, want an error containing %q", test.text, err, test.want)
		}
	}
}

func TestPlanCasesListsKnownGapsRegardlessOfFilter(t *testing.T) {
	t.Parallel()

	gap := knownGap("audio-ape", "audio/a.ape", expectation{}, "no APE encoder")

	for _, text := range []string{"", "video-*"} {
		plans := planCases([]mediaCase{gap}, capabilitiesOf(nil, nil), mustParseFilter(t, text))
		if len(plans) != 1 || plans[0].Action != actionGap || plans[0].Reason != gapReasonPrefix+"no APE encoder" {
			t.Errorf("filter %q: plans = %+v", text, plans)
		}
	}
}

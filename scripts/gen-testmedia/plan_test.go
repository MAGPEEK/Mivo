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

func TestPlanCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		encoders []string
		muxers   []string
		glob     string
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
			glob:     "video-m*",
			want:     []string{"video-mkv-h264-aac build libx264", "video-av1 keep libsvtav1", "video-mxf build mpeg2video"},
		},
		{
			name:     "excluded cases are keep-only even without capabilities",
			encoders: nil,
			muxers:   nil,
			glob:     "nothing-*",
			want:     []string{"video-mkv-h264-aac keep libx264", "video-av1 keep libsvtav1", "video-mxf keep mpeg2video"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			plans := planCases(testCases(), capabilitiesOf(test.encoders, test.muxers), test.glob)
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

	plans := planCases(testCases(), capabilitiesOf([]string{"libx264"}, []string{"matroska"}), "video-*")

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

	plans := planCases(testCases(), capabilitiesOf(nil, nil), "video-mxf")

	if !strings.Contains(plans[0].Reason, `not selected by -only "video-mxf"`) {
		t.Errorf("reason = %q", plans[0].Reason)
	}
}

func TestValidateFilter(t *testing.T) {
	t.Parallel()

	for _, valid := range []string{"", "*", "video-*", "video-mkv-h264-aac", "video-[mw]*"} {
		if err := validateFilter(valid); err != nil {
			t.Errorf("validateFilter(%q) = %v, want nil", valid, err)
		}
	}

	if err := validateFilter("video-["); err == nil {
		t.Error("validateFilter accepted a malformed glob")
	}
}

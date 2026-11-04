package naming

import (
	"slices"
	"testing"
)

func TestTokenize(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want []string
	}{
		{"The.Matrix.1999", []string{"The", "Matrix", "1999"}},
		{"Movie_Name 2010", []string{"Movie", "Name", "2010"}},
		{"Movie.H.264.DDP5.1", []string{"Movie", "H.264", "DDP5.1"}},
		{"[Group] Show - 130 [1080p] [ABCD1234]", []string{"Show", "-", "130", "[1080p]"}},
		{"Dr.Strangelove", []string{"Dr.", "Strangelove"}},
		{"L.A. Confidential", []string{"L.A.", "Confidential"}},
	}

	for _, testCase := range cases {
		if got := Tokenize(testCase.name); !slices.Equal(got, testCase.want) {
			t.Errorf("Tokenize(%q) = %q, want %q", testCase.name, got, testCase.want)
		}
	}
}

func TestIsReleaseTag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		token string
		want  bool
	}{
		{"480p", true},
		{"4320p", true},
		{"360p", false},
		{"8640p", false},
		{"1080P", true},
		{"4K", true},
		{"UHD", true},
		{"x265-GRP", true},
		{"web-dl", true},
		{"WEB-DL-GRP", true},
		{"[1080p]", true},
		{"HEVC", true},
		{"10bit", true},
		{"7.1", true},
		{"dv", true},
		{"Matrix", false},
		{"Spider-Man", false},
		{"-x264", false},
		{"1999", false},
	}

	for _, testCase := range cases {
		if got := IsReleaseTag(testCase.token); got != testCase.want {
			t.Errorf("IsReleaseTag(%q) = %v, want %v", testCase.token, got, testCase.want)
		}
	}
}

func TestYearOf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		token    string
		wantYear int
		wantBare bool
		wantOK   bool
	}{
		{"(1999)", 1999, false, true},
		{"[2010]", 2010, false, true},
		{"1880", 1880, true, true},
		{"2100", 2100, true, true},
		{"1879", 0, false, false},
		{"2101", 0, false, false},
		{"(2010]", 0, false, false},
		{"20101", 0, false, false},
	}

	for _, testCase := range cases {
		year, bare, ok := YearOf(testCase.token)
		if year != testCase.wantYear || bare != testCase.wantBare || ok != testCase.wantOK {
			t.Errorf("YearOf(%q) = %d, %v, %v; want %d, %v, %v", testCase.token, year, bare, ok,
				testCase.wantYear, testCase.wantBare, testCase.wantOK)
		}
	}
}

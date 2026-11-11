package library_test

import (
	"reflect"
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestItemSortIsValid(t *testing.T) {
	t.Parallel()

	valid := []library.ItemSort{
		library.SortByTitle, library.SortByYear, library.SortByReleaseDate, library.SortByContentRating,
		library.SortByDuration, library.SortByDateAdded, library.SortByResolution, library.SortByBitrate,
		library.SortByFileSize, library.SortByRandom,
	}
	for _, sort := range valid {
		if !sort.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", sort)
		}
	}

	for _, sort := range []library.ItemSort{"", "Title", "rating"} {
		if sort.IsValid() {
			t.Errorf("%q.IsValid() = true, want false", sort)
		}
	}
}

func TestQuickFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		quick      library.QuickFilter
		wantValid  bool
		wantActive bool
	}{
		{quick: "", wantValid: false, wantActive: false},
		{quick: library.QuickAll, wantValid: true, wantActive: false},
		{quick: library.QuickHDR, wantValid: true, wantActive: true},
		{quick: library.QuickDolbyVision, wantValid: true, wantActive: true},
		{quick: library.QuickHDR10Plus, wantValid: true, wantActive: true},
		{quick: library.QuickAtmos, wantValid: true, wantActive: true},
		{quick: library.QuickUHD, wantValid: true, wantActive: true},
		{quick: library.QuickUnavailable, wantValid: true, wantActive: true},
		{quick: library.QuickDuplicates, wantValid: true, wantActive: true},
		{quick: "HDR", wantValid: false, wantActive: true},
	}
	for _, test := range tests {
		if got := test.quick.IsValid(); got != test.wantValid {
			t.Errorf("%q.IsValid() = %v, want %v", test.quick, got, test.wantValid)
		}

		if got := test.quick.Active(); got != test.wantActive {
			t.Errorf("%q.Active() = %v, want %v", test.quick, got, test.wantActive)
		}
	}
}

func TestFacetFields(t *testing.T) {
	t.Parallel()

	fields := library.FacetFields()
	if len(fields) != 10 || fields[0] != library.FacetGenre || fields[9] != library.FacetSubtitleLanguage {
		t.Errorf("FacetFields() = %v, want the ten fields in contract order", fields)
	}

	for _, field := range fields {
		if !field.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", field)
		}
	}

	if library.FacetField("quick").IsValid() {
		t.Error(`"quick".IsValid() = true, want false`)
	}
}

func fullFilter() library.ListingFilter {
	return library.ListingFilter{
		Kind: library.ItemKindMovie, Quick: library.QuickHDR, Genres: []string{"Drama"}, Years: []int{2008},
		Decades: []int{2000}, ContentRatings: []string{"PG"}, Resolutions: []library.Resolution{library.Resolution4K},
		VideoCodecs: []string{"hevc"}, AudioCodecs: []string{"aac"}, Containers: []string{"mkv"},
		AudioLanguages: []string{"eng"}, SubtitleLanguages: []string{"ger"}, OfflineRootIDs: []string{"root"},
		MatchState: library.MatchFilterUnmatched,
	}
}

func TestListingFilterWithoutClearsOnlyItsField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		field library.FacetField
		clear func(*library.ListingFilter)
	}{
		{library.FacetGenre, func(filter *library.ListingFilter) { filter.Genres = nil }},
		{library.FacetYear, func(filter *library.ListingFilter) { filter.Years = nil }},
		{library.FacetDecade, func(filter *library.ListingFilter) { filter.Decades = nil }},
		{library.FacetContentRating, func(filter *library.ListingFilter) { filter.ContentRatings = nil }},
		{library.FacetResolution, func(filter *library.ListingFilter) { filter.Resolutions = nil }},
		{library.FacetVideoCodec, func(filter *library.ListingFilter) { filter.VideoCodecs = nil }},
		{library.FacetAudioCodec, func(filter *library.ListingFilter) { filter.AudioCodecs = nil }},
		{library.FacetContainer, func(filter *library.ListingFilter) { filter.Containers = nil }},
		{library.FacetAudioLanguage, func(filter *library.ListingFilter) { filter.AudioLanguages = nil }},
		{library.FacetSubtitleLanguage, func(filter *library.ListingFilter) { filter.SubtitleLanguages = nil }},
		{"unknown", func(*library.ListingFilter) {}},
	}
	for _, test := range tests {
		t.Run(string(test.field), func(t *testing.T) {
			t.Parallel()

			want := fullFilter()
			test.clear(&want)

			if got := fullFilter().Without(test.field); !reflect.DeepEqual(got, want) {
				t.Errorf("Without(%q) = %+v, want %+v", test.field, got, want)
			}
		})
	}
}

func TestMatchStateFilterIsValid(t *testing.T) {
	t.Parallel()

	tests := map[library.MatchStateFilter]bool{
		library.MatchFilterAll:       true,
		library.MatchFilterMatched:   true,
		library.MatchFilterUnmatched: true,
		library.MatchFilterDisabled:  true,
		"Matched":                    false,
		"pending":                    false,
	}
	for filter, want := range tests {
		if got := filter.IsValid(); got != want {
			t.Errorf("MatchStateFilter(%q).IsValid() = %v, want %v", filter, got, want)
		}
	}
}

func TestListingOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		order          library.ListingOrder
		wantKey        library.ItemSort
		wantDescending bool
	}{
		{name: "zero value", order: library.ListingOrder{}, wantKey: library.SortByTitle},
		{name: "descending year", order: library.ListingOrder{Sort: library.SortByYear, Descending: true}, wantKey: library.SortByYear, wantDescending: true},
		{name: "random ignores the direction", order: library.ListingOrder{Sort: library.SortByRandom, Descending: true}, wantKey: library.SortByRandom},
	}
	for _, test := range tests {
		if got := test.order.Key(); got != test.wantKey {
			t.Errorf("%s: Key() = %q, want %q", test.name, got, test.wantKey)
		}

		if got := test.order.IsDescending(); got != test.wantDescending {
			t.Errorf("%s: IsDescending() = %v, want %v", test.name, got, test.wantDescending)
		}
	}
}

func TestResolutionOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		width, height int
		want          library.Resolution
	}{
		{0, 0, library.ResolutionSD},
		{720, 576, library.ResolutionSD},
		{1099, 649, library.ResolutionSD},
		{1100, 0, library.Resolution720p},
		{0, 650, library.Resolution720p},
		{1280, 720, library.Resolution720p},
		{1920, 800, library.Resolution1080p},
		{608, 1080, library.Resolution1080p},
		{3840, 1600, library.Resolution4K},
		{3200, 0, library.Resolution4K},
		{0, 1800, library.Resolution4K},
		{7680, 4320, library.Resolution8K},
		{0, 4000, library.Resolution8K},
	}
	for _, test := range tests {
		if got := library.ResolutionOf(test.width, test.height); got != test.want {
			t.Errorf("ResolutionOf(%d, %d) = %q, want %q", test.width, test.height, got, test.want)
		}
	}
}

func TestResolutionClasses(t *testing.T) {
	t.Parallel()

	for _, resolution := range library.Resolutions() {
		if !resolution.IsValid() {
			t.Errorf("%q.IsValid() = false, want true", resolution)
		}

		wantUHD := resolution == library.Resolution4K || resolution == library.Resolution8K
		if got := resolution.IsUHD(); got != wantUHD {
			t.Errorf("%q.IsUHD() = %v, want %v", resolution, got, wantUHD)
		}
	}

	if library.Resolution("2k").IsValid() {
		t.Error(`"2k".IsValid() = true, want false`)
	}
}

func TestLetterOf(t *testing.T) {
	t.Parallel()

	tests := []struct {
		sortTitle string
		want      string
	}{
		{"avatar", "A"},
		{"Zorro", "Z"},
		{"über", "U"},
		{"Ärger", "A"},
		{"ßtraße", "S"},
		{"œuvre", "O"},
		{"çava", "C"},
		{"12 monkeys", "#"},
		{"!bang", "#"},
		{"東京", "#"},
		{"ωmega", "#"},
		{"", "#"},
		{"\xffbroken", "#"},
	}
	for _, test := range tests {
		if got := library.LetterOf(test.sortTitle); got != test.want {
			t.Errorf("LetterOf(%q) = %q, want %q", test.sortTitle, got, test.want)
		}
	}
}

// A caller that forgets the restriction must get nothing: the zero query denies everything.
func TestListingQueryZeroRestrictionDeniesEverything(t *testing.T) {
	t.Parallel()

	restriction := library.ListingQuery{}.Restriction
	if !restriction.DeniesAll() || restriction.AllLibraries() || restriction.Allows("library", 0, true) {
		t.Errorf("zero ListingQuery restriction = %+v, want one that denies everything", restriction)
	}
}

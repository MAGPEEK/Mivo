package naming

import (
	"errors"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

const noYear = 0

func movieLevel(key, title string, year int) library.Level {
	level := library.Level{Kind: library.ItemKindMovie, MatchKey: key, Title: title}
	if year != noYear {
		level.Year = library.NumberOf(year)
	}

	return level
}

func moviePlacement(level library.Level, label string, part int) library.Placement {
	placement := library.Placement{Levels: []library.Level{level}, VersionLabel: label}
	if part != 0 {
		placement.PartNumber = library.NumberOf(part)
	}

	return placement
}

func extraOf(parent library.Level, key, title string, extraType library.ExtraType) library.Placement {
	return library.Placement{Levels: []library.Level{parent, {
		Kind: library.ItemKindExtra, MatchKey: key, Title: title, ExtraType: extraType,
	}}}
}

type placeCase struct {
	name  string
	path  string
	count int
	want  library.Placement
}

func runPlaceCases(t *testing.T, cases []placeCase) {
	t.Helper()

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := Place(library.KindMovies, FileContext{Path: testCase.path, DirectoryVideoCount: testCase.count})
			if err != nil {
				t.Fatalf("Place(%q) error = %v", testCase.path, err)
			}

			if result.Outcome != OutcomePlaced {
				t.Fatalf("Place(%q) outcome = %q, want placed", testCase.path, result.Outcome)
			}

			assertPlacement(t, result.Placement, testCase.want)
		})
	}
}

func assertPlacement(t *testing.T, got, want library.Placement) {
	t.Helper()

	if got.Key() != want.Key() {
		t.Errorf("key = %s\nwant  %s", got.Key(), want.Key())
	}

	if len(got.Levels) != len(want.Levels) {
		t.Fatalf("levels = %+v, want %+v", got.Levels, want.Levels)
	}

	for index := range want.Levels {
		if got.Levels[index] != want.Levels[index] {
			t.Errorf("level %d = %+v\nwant      %+v", index, got.Levels[index], want.Levels[index])
		}
	}
}

func TestPlaceMovies(t *testing.T) {
	t.Parallel()

	bigBuck := movieLevel("dir:big buck bunny 2008", "Big Buck Bunny", 2008)
	bladeRunner := movieLevel("dir:blade runner 1982", "Blade Runner", 1982)
	killBill := movieLevel("dir:kill bill 2003", "Kill Bill", 2003)
	matrixFolder := movieLevel("dir:the matrix 1999", "The Matrix", 1999)

	runPlaceCases(t, []placeCase{
		// Folder movies and versions.
		{"folder version 1080p", "Big Buck Bunny (2008)/Big Buck Bunny (2008) - 1080p.mkv", 3, moviePlacement(bigBuck, "1080p", 0)},
		{"folder version 4K", "Big Buck Bunny (2008)/Big Buck Bunny (2008) - 4K.mkv", 3, moviePlacement(bigBuck, "4K", 0)},
		{"folder plain", "Blade Runner (1982)/Blade Runner (1982).mkv", 2, moviePlacement(bladeRunner, "", 0)},
		{"folder edition", "Blade Runner (1982)/Blade Runner (1982) {edition-Director's Cut}.mkv", 2, moviePlacement(bladeRunner, "Director's Cut", 0)},
		{"folder edition upper case", "Blade Runner (1982)/Blade Runner (1982) {EDITION-Final Cut}.mp4", 2, moviePlacement(bladeRunner, "Final Cut", 0)},
		{"folder label and edition", "Blade Runner (1982)/Blade Runner (1982) - 4K {edition-Final Cut}.mkv", 2, moviePlacement(bladeRunner, "4K Final Cut", 0)},
		{"folder bracket label", "Blade Runner (1982)/Blade Runner (1982) [Remastered].mkv", 2, moviePlacement(bladeRunner, "Remastered", 0)},
		{"folder name case-insensitive", "Blade Runner (1982)/blade.runner.1982.remux.mkv", 2, moviePlacement(bladeRunner, "remux", 0)},
		{"folder release name", "The Matrix (1999)/The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv", 2, moviePlacement(matrixFolder, "1080p.BluRay.x264-GROUP", 0)},
		{"folder only video other name", "The Matrix (1999)/movie.mkv", 1, moviePlacement(matrixFolder, "", 0)},
		{"folder only video with edition", "The Matrix (1999)/whatever {edition-IMAX}.mkv", 1, moviePlacement(matrixFolder, "IMAX", 0)},
		{"folder without year", "Elephants Dream/Elephants Dream.mkv", 1, moviePlacement(movieLevel("dir:elephants dream", "Elephants Dream", noYear), "", 0)},
		{"nested folder key", "Sci-Fi/Spring (2019)/Spring (2019).mkv", 1, moviePlacement(movieLevel("dir:sci fi spring 2019", "Spring", 2019), "", 0)},
		{"folder NFC title", "Amélie (2001)/Amélie (2001).mkv", 1, moviePlacement(movieLevel("dir:amélie 2001", "Amélie", 2001), "", 0)},
		{"folder square year", "Das Boot [1981]/Das Boot [1981].mkv", 1, moviePlacement(movieLevel("dir:das boot 1981", "Das Boot", 1981), "", 0)},
		{"word boundary prefix", "Movie (2010)/Movie (2010)2.mkv", 2, moviePlacement(movieLevel("dir:movie 2010", "Movie", 2010), "2", 0)},
		// Parts.
		{"part cd1", "Kill Bill (2003)/Kill Bill (2003) - cd1.mkv", 2, moviePlacement(killBill, "", 1)},
		{"part cd2", "Kill Bill (2003)/Kill Bill (2003) - cd2.mkv", 2, moviePlacement(killBill, "", 2)},
		{"part disc", "Kill Bill (2003)/Kill Bill (2003).disc1.mkv", 2, moviePlacement(killBill, "", 1)},
		{"part disk", "Kill Bill (2003)/Kill Bill (2003)_disk2.mkv", 2, moviePlacement(killBill, "", 2)},
		{"part word", "Kill Bill (2003)/Kill Bill (2003) part3.mkv", 2, moviePlacement(killBill, "", 3)},
		{"part pt", "Kill Bill (2003)/Kill Bill (2003)-pt2.mkv", 2, moviePlacement(killBill, "", 2)},
		{"part with label", "Kill Bill (2003)/Kill Bill (2003) - 4K - cd1.mkv", 2, moviePlacement(killBill, "4K", 1)},
		{"part upper case", "Kill Bill (2003)/Kill Bill (2003) - CD2.mkv", 2, moviePlacement(killBill, "", 2)},
		{"part loose", "Kill Bill (2003) - cd1.mkv", 0, moviePlacement(movieLevel("|kill bill|2003", "Kill Bill", 2003), "", 1)},
		{"title Part 1 is no part", "Deathly Hallows Part 1 (2010).mkv", 0, moviePlacement(movieLevel("|deathly hallows part 1|2010", "Deathly Hallows Part 1", 2010), "", 0)},
		{"part 0 is no part", "Movie (2010) cd0.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		// Loose movies and years.
		{"root file never folder movie", "Night of the Living Dead (1968).mp4", 1, moviePlacement(movieLevel("|night of the living dead|1968", "Night of the Living Dead", 1968), "", 0)},
		{"loose in shared folder", "Blender Open Movies/Sintel (2010).mkv", 3, moviePlacement(movieLevel("blender open movies|sintel|2010", "Sintel", 2010), "", 0)},
		{"loose second movie", "Blender Open Movies/Tears of Steel (2012).mkv", 3, moviePlacement(movieLevel("blender open movies|tears of steel|2012", "Tears of Steel", 2012), "", 0)},
		{"year in title", "Blade Runner 2049 (2017).mkv", 0, moviePlacement(movieLevel("|blade runner 2049|2017", "Blade Runner 2049", 2017), "", 0)},
		{"title starts with number", "2001 A Space Odyssey (1968).mkv", 0, moviePlacement(movieLevel("|2001 a space odyssey|1968", "2001 A Space Odyssey", 1968), "", 0)},
		{"title is a year", "1917 (2019).mkv", 0, moviePlacement(movieLevel("|1917|2019", "1917", 2019), "", 0)},
		{"two bare years", "2012.2009.1080p.mkv", 0, moviePlacement(movieLevel("|2012|2009", "2012", 2009), "", 0)},
		{"scene release", "The.Matrix.1999.1080p.BluRay.x264-GROUP.mkv", 0, moviePlacement(movieLevel("|the matrix|1999", "The Matrix", 1999), "", 0)},
		{"no year", "Elephants Dream.mkv", 0, moviePlacement(movieLevel("|elephants dream|", "Elephants Dream", noYear), "", 0)},
		{"bare year at the end", "Movie 2010.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		{"bare year at the end with spaces", "The Matrix 1999.mkv", 0, moviePlacement(movieLevel("|the matrix|1999", "The Matrix", 1999), "", 0)},
		{"bare year at the end with dots", "The.Matrix.1999.mkv", 0, moviePlacement(movieLevel("|the matrix|1999", "The Matrix", 1999), "", 0)},
		{"year-like title without other year", "Blade Runner 2049.mkv", 0, moviePlacement(movieLevel("|blade runner|2049", "Blade Runner", 2049), "", 0)},
		{"bare year not at the end", "Movie 2010 Extended.mkv", 0, moviePlacement(movieLevel("|movie 2010 extended|", "Movie 2010 Extended", noYear), "", 0)},
		{"lone bare year is the title", "1917.mkv", 0, moviePlacement(movieLevel("|1917|", "1917", noYear), "", 0)},
		{"underscores", "Tears_of_Steel_(2012).mkv", 0, moviePlacement(movieLevel("|tears of steel|2012", "Tears of Steel", 2012), "", 0)},
		{"square year", "Sintel [2010].mkv", 0, moviePlacement(movieLevel("|sintel|2010", "Sintel", 2010), "", 0)},
		{"year out of range", "Movie (1700).mkv", 0, moviePlacement(movieLevel("|movie 1700|", "Movie (1700)", noYear), "", 0)},
		{"abbreviation kept", "Mr.Nobody.2009.720p.mkv", 0, moviePlacement(movieLevel("|mr nobody|2009", "Mr. Nobody", 2009), "", 0)},
		{"acronym kept", "S.H.I.E.L.D. Files (2015).mkv", 0, moviePlacement(movieLevel("|s h i e l d files|2015", "S.H.I.E.L.D. Files", 2015), "", 0)},
		{"many release tags", "Movie.2016.2160p.UHD.HDR10.DV.HEVC.TrueHD.Atmos.7.1-GRP.mkv", 0, moviePlacement(movieLevel("|movie|2016", "Movie", 2016), "", 0)},
		{"audio tag with dot", "Movie.2016.WEB-DL.DDP5.1.H.264-GRP.mkv", 0, moviePlacement(movieLevel("|movie|2016", "Movie", 2016), "", 0)},
		{"group and checksum", "[Group] Movie (2016) [1080p] [ABCD1234].mkv", 0, moviePlacement(movieLevel("|movie|2016", "Movie", 2016), "", 0)},
		{"loose edition", "Movie (2010) {edition-Extended}.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "Extended", 0)},
		{"only year and tags", "2019.1080p.mkv", 0, moviePlacement(movieLevel("|2019|", "2019", noYear), "", 0)},
		{"only tags", "1080p.mkv", 0, moviePlacement(movieLevel("|1080p|", "1080p", noYear), "", 0)},
		{"no extension", "Movie (2010)", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		{"root trailer is a movie", "trailer.mkv", 0, moviePlacement(movieLevel("|trailer|", "trailer", noYear), "", 0)},
		{"extras folder in root", "Trailers/Movie (2010).mkv", 2, moviePlacement(movieLevel("trailers|movie|2010", "Movie", 2010), "", 0)},
		{"extras folder in collection folder", "Blender Open Movies/Extras/Making Of.mkv", 2, moviePlacement(movieLevel("blender open movies extras|making of|", "Making Of", noYear), "", 0)},
		{"trailers folder below folder without year", "Elephants Dream/Trailers/Teaser.mkv", 2, moviePlacement(movieLevel("elephants dream trailers|teaser|", "Teaser", noYear), "", 0)},
		// Bare suffixes and release groups that look like extra suffixes.
		{"root bare suffix is a movie", "-trailer.mkv", 0, moviePlacement(movieLevel("|trailer|", "trailer", noYear), "", 0)},
		{"release group SCENE", "Movie.2010.1080p.x264-SCENE.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		{"release group SHORT", "Movie.2010.1080p.BluRay.x264-SHORT.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		{"release group OTHER", "Movie.2010.720p-OTHER.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		{"release group TRAILER in folder", "Big Buck Bunny (2008)/Big.Buck.Bunny.2008.2160p.HEVC-TRAILER.mkv", 3, moviePlacement(bigBuck, "2160p.HEVC-TRAILER", 0)},
		{"leading group named like suffix", "[TRAILER] Movie (2010).mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 0)},
		// Parts with one separator at the end.
		{"spaced part at the end", "Movie (2010) - part 1.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 1)},
		{"spaced cd at the end", "Kill Bill (2003)/Kill Bill (2003) - cd 2.mkv", 2, moviePlacement(killBill, "", 2)},
		{"dotted disc at the end", "Movie.2010.disc.2.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "", 2)},
		{"spaced part before edition", "Movie (2010) Part 2 {edition-Extended}.mkv", 0, moviePlacement(movieLevel("|movie|2010", "Movie", 2010), "Extended", 2)},
		// Robustness.
		{"very long name", strings.Repeat("Long ", 300) + "(2010).mkv", 0, moviePlacement(movieLevel("|"+strings.TrimSpace(strings.Repeat("long ", 300))+"|2010", strings.TrimSpace(strings.Repeat("Long ", 300)), 2010), "", 0)},
		{"punctuation only", "!!!.mkv", 0, moviePlacement(movieLevel("||", "!!!", noYear), "", 0)},
		{"CJK title", "千と千尋の神隠し (2001).mkv", 0, moviePlacement(movieLevel("|千と千尋の神隠し|2001", "千と千尋の神隠し", 2001), "", 0)},
		{"CJK folder movie", "君の名は。 (2016)/君の名は。 (2016) - 4K.mkv", 2, moviePlacement(movieLevel("dir:君の名は 2016", "君の名は。", 2016), "4K", 0)},
	})
}

func TestPlaceExtras(t *testing.T) {
	t.Parallel()

	bigBuck := movieLevel("dir:big buck bunny 2008", "Big Buck Bunny", 2008)
	sintel := movieLevel("blender open movies|sintel|2010", "Sintel", 2010)

	runPlaceCases(t, []placeCase{
		{"trailers folder", "Big Buck Bunny (2008)/Trailers/Big Buck Bunny Trailer.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 trailers big buck bunny trailer mkv", "Big Buck Bunny Trailer", library.ExtraTypeTrailer)},
		{"featurettes folder", "Big Buck Bunny (2008)/Featurettes/Making Of.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 featurettes making of mkv", "Making Of", library.ExtraTypeFeaturette)},
		{"behind the scenes folder", "Big Buck Bunny (2008)/behind the scenes/Set.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 behind the scenes set mkv", "Set", library.ExtraTypeBehindTheScenes)},
		{"deleted scenes folder", "Big Buck Bunny (2008)/Deleted Scenes/Cut.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 deleted scenes cut mkv", "Cut", library.ExtraTypeDeletedScene)},
		{"interviews folder", "Big Buck Bunny (2008)/INTERVIEWS/Director.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 interviews director mkv", "Director", library.ExtraTypeInterview)},
		{"scenes folder", "Big Buck Bunny (2008)/Scenes/Chase.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 scenes chase mkv", "Chase", library.ExtraTypeScene)},
		{"shorts folder", "Big Buck Bunny (2008)/Shorts/Bonus.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 shorts bonus mkv", "Bonus", library.ExtraTypeShort)},
		{"other folder", "Big Buck Bunny (2008)/Other/Thing.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 other thing mkv", "Thing", library.ExtraTypeOther)},
		{"extras folder nested", "Big Buck Bunny (2008)/Extras/Disc 2/Thing.mkv", 1, extraOf(bigBuck, "big buck bunny 2008 extras disc 2 thing mkv", "Thing", library.ExtraTypeOther)},
		{"suffix featurette", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-featurette.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 featurette mkv", "Big Buck Bunny", library.ExtraTypeFeaturette)},
		{"suffix trailer loose", "Blender Open Movies/Sintel (2010)-trailer.mkv", 3, extraOf(sintel, "blender open movies sintel 2010 trailer mkv", "Sintel", library.ExtraTypeTrailer)},
		{"suffix behind the scenes", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-BehindTheScenes.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 behindthescenes mkv", "Big Buck Bunny", library.ExtraTypeBehindTheScenes)},
		{"suffix deleted", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-deleted.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 deleted mkv", "Big Buck Bunny", library.ExtraTypeDeletedScene)},
		{"suffix interview", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-interview.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 interview mkv", "Big Buck Bunny", library.ExtraTypeInterview)},
		{"suffix scene", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-scene.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 scene mkv", "Big Buck Bunny", library.ExtraTypeScene)},
		{"suffix short", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-short.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 short mkv", "Big Buck Bunny", library.ExtraTypeShort)},
		{"suffix other", "Big Buck Bunny (2008)/Big Buck Bunny (2008)-other.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 big buck bunny 2008 other mkv", "Big Buck Bunny", library.ExtraTypeOther)},
		{"suffix only", "Big Buck Bunny (2008)/-trailer.mkv", 3, extraOf(bigBuck, "big buck bunny 2008 trailer mkv", "trailer", library.ExtraTypeTrailer)},
		{"trailer file in movie folder", "Big Buck Bunny (2008)/trailer.mp4", 3, extraOf(bigBuck, "big buck bunny 2008 trailer mp4", "trailer", library.ExtraTypeTrailer)},
		{"root suffix extra", "Movie (2010)-trailer.mkv", 0, extraOf(movieLevel("|movie|2010", "Movie", 2010), "movie 2010 trailer mkv", "Movie", library.ExtraTypeTrailer)},
	})
}

func TestPlaceIgnoresSamples(t *testing.T) {
	t.Parallel()

	paths := []string{
		"Big Buck Bunny (2008)/sample.mkv",
		"Big Buck Bunny (2008)/SAMPLE.mkv",
		"Big Buck Bunny (2008)/Big Buck Bunny (2008)-sample.mkv",
		"The.Matrix.1999.1080p.sample.mkv",
		"Big Buck Bunny (2008)/Sample/anything.mkv",
		"sample.mkv",
	}

	for _, filePath := range paths {
		t.Run(filePath, func(t *testing.T) {
			t.Parallel()

			result, err := Place(library.KindMovies, FileContext{Path: filePath, DirectoryVideoCount: 1})
			if err != nil {
				t.Fatalf("Place(%q) error = %v", filePath, err)
			}

			if result.Outcome != OutcomeIgnored || len(result.Placement.Levels) != 0 {
				t.Errorf("Place(%q) = %+v, want ignored", filePath, result)
			}
		})
	}
}

func TestPlaceErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		kind library.Kind
		path string
		want error
	}{
		{"shows unsupported", library.KindShows, "Show/Show - S01E01.mkv", domain.ErrUnsupported},
		{"unknown kind", library.Kind("music"), "a.mkv", domain.ErrInvalid},
		{"absolute path", library.KindMovies, "/a.mkv", domain.ErrInvalid},
		{"parent segment", library.KindMovies, "../a.mkv", domain.ErrInvalid},
		{"blank folder title", library.KindMovies, " /a.mkv", domain.ErrInvalid},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			_, err := Place(testCase.kind, FileContext{Path: testCase.path, DirectoryVideoCount: 1})
			if !errors.Is(err, testCase.want) {
				t.Errorf("Place error = %v, want %v", err, testCase.want)
			}
		})
	}
}

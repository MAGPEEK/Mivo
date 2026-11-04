package artwork_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/artwork"
)

type classifyCase struct {
	file  string
	want  artwork.LocalMatch
	found bool
}

func poster(priority int) artwork.LocalMatch {
	return artwork.LocalMatch{Kind: artwork.KindPoster, Priority: priority}
}

func match(kind artwork.Kind, priority int) artwork.LocalMatch {
	return artwork.LocalMatch{Kind: kind, Priority: priority}
}

func seasonPoster(priority, season int) artwork.LocalMatch {
	return artwork.LocalMatch{Kind: artwork.KindPoster, Priority: priority, ForSeason: true, Season: season}
}

func runClassifyCases(t *testing.T, context artwork.LocalContext, tests []classifyCase) {
	t.Helper()

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			t.Parallel()

			got, found := artwork.ClassifyLocal(context, test.file)
			if got != test.want || found != test.found {
				t.Errorf("ClassifyLocal(%+v, %q) = %+v, %t, want %+v, %t",
					context, test.file, got, found, test.want, test.found)
			}
		})
	}
}

func TestClassifyLocalFolderMovie(t *testing.T) {
	t.Parallel()

	context := artwork.LocalContext{Kind: artwork.ContextFolderMovie, BaseName: "The Matrix (1999)"}

	runClassifyCases(t, context, []classifyCase{
		{file: "poster.jpg", want: poster(0), found: true},
		{file: "folder.jpeg", want: poster(1), found: true},
		{file: "cover.png", want: poster(2), found: true},
		{file: "movie.webp", want: poster(3), found: true},
		{file: "The Matrix (1999)-poster.jpg", want: poster(4), found: true},
		{file: "the matrix (1999).tbn", want: poster(5), found: true},
		{file: "fanart.jpg", want: match(artwork.KindBackdrop, 0), found: true},
		{file: "backdrop.jpg", want: match(artwork.KindBackdrop, 1), found: true},
		{file: "background.jpg", want: match(artwork.KindBackdrop, 2), found: true},
		{file: "art.jpg", want: match(artwork.KindBackdrop, 3), found: true},
		{file: "The Matrix (1999)-fanart.jpg", want: match(artwork.KindBackdrop, 4), found: true},
		{file: "landscape.jpg", want: match(artwork.KindThumb, 0), found: true},
		{file: "thumb.jpg", want: match(artwork.KindThumb, 1), found: true},
		{file: "The Matrix (1999)-thumb.jpg", want: match(artwork.KindThumb, 2), found: true},
		{file: "clearlogo.png", want: match(artwork.KindLogo, 0), found: true},
		{file: "logo.png", want: match(artwork.KindLogo, 1), found: true},
		{file: "banner.jpg", want: match(artwork.KindBanner, 0), found: true},
		{file: "POSTER.JPG", want: poster(0), found: true},
		{file: "poster.gif"},
		{file: "poster"},
		{file: ".jpg"},
		{file: "poster.jpg.bak"},
		{file: "sub/poster.jpg"},
		{file: `sub\poster.jpg`},
		{file: "Other Movie-poster.jpg"},
		{file: "season01-poster.jpg"},
		{file: "-poster.jpg"},
	})
}

func TestClassifyLocalFolderMovieWithoutBaseName(t *testing.T) {
	t.Parallel()

	context := artwork.LocalContext{Kind: artwork.ContextFolderMovie}

	runClassifyCases(t, context, []classifyCase{
		{file: "poster.jpg", want: poster(0), found: true},
		{file: "-poster.jpg"},
		{file: "-thumb.jpg"},
	})
}

func TestClassifyLocalPrefersFixedNamesOverTheBaseName(t *testing.T) {
	t.Parallel()

	// A video called "fanart.mkv" must not turn the folder's fanart.jpg into a poster.
	context := artwork.LocalContext{Kind: artwork.ContextFolderMovie, BaseName: "fanart"}

	runClassifyCases(t, context, []classifyCase{
		{file: "fanart.jpg", want: match(artwork.KindBackdrop, 0), found: true},
		{file: "fanart-poster.jpg", want: poster(4), found: true},
	})
}

func TestClassifyLocalLooseMovie(t *testing.T) {
	t.Parallel()

	context := artwork.LocalContext{Kind: artwork.ContextLooseMovie, BaseName: "Heat (1995)"}

	runClassifyCases(t, context, []classifyCase{
		{file: "Heat (1995)-poster.jpg", want: poster(0), found: true},
		{file: "Heat (1995).jpg", want: poster(1), found: true},
		{file: "Heat (1995)-fanart.png", want: match(artwork.KindBackdrop, 0), found: true},
		{file: "Heat (1995)-thumb.jpg", want: match(artwork.KindThumb, 0), found: true},
		{file: "poster.jpg"},
		{file: "folder.jpg"},
		{file: "fanart.jpg"},
		{file: "logo.png"},
		{file: "banner.jpg"},
		{file: "Ronin (1998)-poster.jpg"},
	})
}

func TestClassifyLocalShowFolder(t *testing.T) {
	t.Parallel()

	context := artwork.LocalContext{Kind: artwork.ContextShowFolder}

	runClassifyCases(t, context, []classifyCase{
		{file: "poster.jpg", want: poster(0), found: true},
		{file: "folder.jpg", want: poster(1), found: true},
		{file: "fanart.jpg", want: match(artwork.KindBackdrop, 0), found: true},
		{file: "backdrop.jpg", want: match(artwork.KindBackdrop, 1), found: true},
		{file: "clearlogo.png", want: match(artwork.KindLogo, 0), found: true},
		{file: "logo.png", want: match(artwork.KindLogo, 1), found: true},
		{file: "banner.jpg", want: match(artwork.KindBanner, 0), found: true},
		{file: "season01-poster.jpg", want: seasonPoster(2, 1), found: true},
		{file: "Season12-Poster.PNG", want: seasonPoster(2, 12), found: true},
		{file: "season1-poster.jpg", want: seasonPoster(2, 1), found: true},
		{file: "season00-poster.jpg", want: seasonPoster(2, 0), found: true},
		{file: "season2024-poster.jpg", want: seasonPoster(2, 2024), found: true},
		{file: "season-specials-poster.jpg", want: seasonPoster(2, 0), found: true},
		{file: "season12345-poster.jpg"},
		{file: "season-all-poster.jpg"},
		{file: "season-poster.jpg"},
		{file: "season01-fanart.jpg"},
		{file: "season01.jpg"},
		{file: "season+1-poster.jpg"},
		{file: "cover.jpg"},
		{file: "thumb.jpg"},
		{file: "season01-poster.txt"},
	})
}

func TestClassifyLocalSeasonFolder(t *testing.T) {
	t.Parallel()

	context := artwork.LocalContext{Kind: artwork.ContextSeasonFolder, Season: 3}

	runClassifyCases(t, context, []classifyCase{
		{file: "poster.jpg", want: seasonPoster(0, 3), found: true},
		{file: "folder.jpg", want: seasonPoster(1, 3), found: true},
		{file: "season03-poster.jpg"},
		{file: "fanart.jpg"},
		{file: "banner.jpg"},
	})
}

func TestClassifyLocalEpisode(t *testing.T) {
	t.Parallel()

	context := artwork.LocalContext{Kind: artwork.ContextEpisode, BaseName: "Show S01E02"}

	runClassifyCases(t, context, []classifyCase{
		{file: "Show S01E02-thumb.jpg", want: match(artwork.KindThumb, 0), found: true},
		{file: "Show S01E02.jpg", want: match(artwork.KindThumb, 1), found: true},
		{file: "Show S01E02-poster.jpg"},
		{file: "Show S01E03-thumb.jpg"},
		{file: "poster.jpg"},
		{file: "thumb.jpg"},
	})
}

func TestClassifyLocalUnknownContext(t *testing.T) {
	t.Parallel()

	runClassifyCases(t, artwork.LocalContext{Kind: "album", BaseName: "x"}, []classifyCase{
		{file: "poster.jpg"},
		{file: "x.jpg"},
	})
}

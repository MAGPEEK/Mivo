package main

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
)

// The library tree (task M3-17) is declared as its expected catalog: what a scan following
// ADR 0027 must make of the files. library.go derives the FFmpeg cases and the NFO text from
// it, so the manifest and the files on disk cannot diverge. Real movie and show names are
// used only as file and folder names; every file content is generated.

// Library directories below the -library directory, one per library kind.
const (
	moviesLibraryPath = "movies"
	showsLibraryPath  = "shows"
)

// The absolute-numbering regression (UX DT-24): one show folder with this many episode files
// numbered 001 … 131 must become one show with one season and 131 episodes.
const (
	dragonBallSuperFolder   = "Dragon Ball Super"
	dragonBallSuperEpisodes = 131
	// animeEpisodeNumber is the number in the anime-style release name of the second folder.
	animeEpisodeNumber = 130
)

// Episode numbers of the show with season folders and of the show in the library root.
const (
	firstSeason = 1
	// secondSeason has its own folder; season 0 (specials) has the "Specials" folder.
	secondSeason = 2
	// multiEpisodeEnd is the last episode of the "S01E01-E02" file.
	multiEpisodeEnd = 2
	// metadataEpisode has an episode NFO and a thumbnail.
	metadataEpisode = 3
	// rootShowEpisodes is the number of episode files directly in the library root.
	rootShowEpisodes = 2
)

// yearDigits is the length of the year in a "Title (YYYY)" name.
const yearDigits = 4

// libraryCatalog returns the expected catalog of the generated library tree.
func libraryCatalog() []expectedLibrary {
	return []expectedLibrary{
		{Kind: libraryKindMovies, Path: moviesLibraryPath, Items: movieItems()},
		{Kind: libraryKindShows, Path: showsLibraryPath, Items: showItems()},
	}
}

func movieItems() []expectedItem {
	return slices.Concat(
		[]expectedItem{bigBuckBunny(), bladeRunner(), killBill()},
		nfoMovies(),
		[]expectedItem{amelie()},
		looseMovies(),
		embeddedArtworkMovies(),
	)
}

// bigBuckBunny is a folder movie with two versions, both extras styles, a sample and every
// local artwork kind the M3 slice reads for movies.
func bigBuckBunny() expectedItem {
	const folder = "Big Buck Bunny (2008)"

	return expectedItem{
		Kind: itemKindMovie, Title: "Big Buck Bunny", Year: yearOf(folder), Path: folder,
		Versions: []expectedVersion{
			{Label: "1080p", Parts: inFolder(folder, "Big Buck Bunny (2008) - 1080p.mkv")},
			{Label: "4K", Parts: inFolder(folder, "Big Buck Bunny (2008) - 4K.mkv")},
		},
		Extras: []expectedExtra{
			{Type: extraTrailer, File: path.Join(folder, "Trailers", "Big Buck Bunny Trailer.mkv")},
			{Type: extraFeaturette, File: path.Join(folder, "Big Buck Bunny (2008)-featurette.mkv")},
		},
		Ignored: inFolder(folder, "sample.mkv"),
		Artwork: []expectedArtwork{
			localArtwork(artworkPoster, path.Join(folder, "poster.jpg")),
			localArtwork(artworkBackdrop, path.Join(folder, "fanart.jpg")),
			localArtwork(artworkLogo, path.Join(folder, "clearlogo.png")),
		},
	}
}

// bladeRunner has a plain version and a Plex-style edition.
func bladeRunner() expectedItem {
	const folder = "Blade Runner (1982)"

	return expectedItem{
		Kind: itemKindMovie, Title: "Blade Runner", Year: yearOf(folder), Path: folder,
		Versions: []expectedVersion{
			{Label: "", Parts: inFolder(folder, "Blade Runner (1982).mkv")},
			{Label: "Director's Cut", Parts: inFolder(folder, "Blade Runner (1982) {edition-Director's Cut}.mkv")},
		},
	}
}

// killBill is one version split into two parts.
func killBill() expectedItem {
	const folder = "Kill Bill (2003)"

	return expectedItem{
		Kind: itemKindMovie, Title: "Kill Bill", Year: yearOf(folder), Path: folder,
		Versions: []expectedVersion{
			{Label: "", Parts: inFolder(folder, "Kill Bill (2003) - cd1.mkv", "Kill Bill (2003) - cd2.mkv")},
		},
	}
}

// nfoMovies cover every NFO variant: a title that differs from the folder, a URL-only file,
// ISO-8859-1 text and a malformed file that must not fail the scan.
func nfoMovies() []expectedItem {
	const (
		cosmos     = "Cosmos Laundromat (2015)"
		das        = "Das Boot (1981)"
		cosmosNFO  = "Cosmos Laundromat: First Cycle"
		dasBootNFO = "Das Boot: Größenwahn unter Wasser"
	)

	return []expectedItem{
		singleFileMovie(cosmosNFO, cosmos, &expectedNFO{
			File: path.Join(cosmos, "movie.nfo"), Format: nfoFormatXML, Encoding: encodingUTF8,
			Fields: nfoFields{
				Title: cosmosNFO, OriginalTitle: "Cosmos Laundromat", Year: yearOf(cosmos),
				Plot:   "A suicidal sheep meets a salesman who offers him lives in other worlds.",
				Genres: []string{"Animation", "Fantasy"},
				ProviderIDs: map[string]string{
					providerTMDB: "1000001", providerIMDb: "tt1000001",
				},
			},
		}),
		singleFileMovie("Spring", "Spring (2019)", &expectedNFO{
			File: "Spring (2019)/Spring (2019).nfo", Format: nfoFormatURL,
			Fields: nfoFields{ProviderIDs: map[string]string{providerTMDB: "1000002"}},
		}),
		singleFileMovie(dasBootNFO, das, &expectedNFO{
			File: path.Join(das, "movie.nfo"), Format: nfoFormatXML, Encoding: encodingLatin1,
			Fields: nfoFields{
				Title: dasBootNFO, OriginalTitle: "Das Boot", Year: yearOf(das),
				// Only ISO-8859-1 characters: the file is written in that encoding.
				Plot:   "Ein U-Boot läuft aus. Übermüdete Männer, Öl und Lärm.",
				Genres: []string{"Drama", "Kriegsfilm"},
			},
		}),
		singleFileMovie("Caminandes", "Caminandes (2013)", &expectedNFO{
			File: "Caminandes (2013)/movie.nfo", Format: nfoFormatMalformed,
		}),
	}
}

// amelie lives in a folder whose name is in Unicode NFD (e + combining acute accent), as
// macOS writes it. The scanner normalises to NFC, so the title is the precomposed "Amélie".
func amelie() expectedItem {
	return singleFileMovie("Amélie", "Amélie (2001)", nil)
}

// looseMovies share one folder that is no movie folder, so each file is its own movie.
func looseMovies() []expectedItem {
	const folder = "Blender Open Movies"

	sintel := looseMovie(folder, "Sintel (2010)")
	sintel.Artwork = []expectedArtwork{localArtwork(artworkPoster, path.Join(folder, "Sintel (2010)-poster.jpg"))}

	return []expectedItem{sintel, looseMovie(folder, "Tears of Steel (2012)"), looseMovie(folder, "Elephants Dream (2006)")}
}

// embeddedArtworkMovies have no local poster: a Matroska cover attachment and an MP4 covr
// atom, the latter as a loose movie directly in the library root.
func embeddedArtworkMovies() []expectedItem {
	glassHalf := singleFileMovie("Glass Half", "Glass Half (2015)", nil)
	glassHalf.Artwork = []expectedArtwork{embeddedArtwork(artworkPoster, glassHalf.Versions[0].Parts[0])}

	const nightFile = "Night of the Living Dead (1968).mp4"

	night := expectedItem{
		Kind: itemKindMovie, Title: "Night of the Living Dead", Year: yearOf(nightFile), Path: nightFile,
		Versions: []expectedVersion{{Label: "", Parts: []string{nightFile}}},
		Artwork:  []expectedArtwork{embeddedArtwork(artworkPoster, nightFile)},
	}

	return []expectedItem{glassHalf, night}
}

func showItems() []expectedItem {
	return []expectedItem{pioneerOne(), starWreck(), dragonBallSuper(), dragonBallSuperFansub()}
}

// pioneerOne has season folders, specials, a multi-episode file, a tvshow.nfo with provider
// IDs, a season poster in the show folder and an episode with its own NFO and thumbnail.
func pioneerOne() expectedItem {
	const folder = "Pioneer One (2010)"

	specials := path.Join(folder, "Specials")
	season1 := path.Join(folder, "Season 01")
	season2 := path.Join(folder, "Season 02")

	return expectedItem{
		Kind: itemKindShow, Title: "Pioneer One", Year: yearOf(folder), Path: folder,
		NFO: &expectedNFO{
			File: path.Join(folder, "tvshow.nfo"), Format: nfoFormatXML, Encoding: encodingUTF8,
			Fields: nfoFields{
				Title: "Pioneer One", Year: yearOf(folder), Plot: "An object falls from the sky over Canada.",
				Genres: []string{"Drama", "Science Fiction"},
				ProviderIDs: map[string]string{
					providerTMDB: "1000010", providerTVDB: "1000011", providerIMDb: "tt1000012",
				},
			},
		},
		Artwork: []expectedArtwork{
			localArtwork(artworkPoster, path.Join(folder, "poster.jpg")),
			localArtwork(artworkBackdrop, path.Join(folder, "fanart.jpg")),
		},
		Seasons: []expectedSeason{
			{Number: 0, Path: specials, Episodes: []expectedEpisode{
				{Number: 1, Files: inFolder(specials, "Pioneer One - S00E01.mkv")},
			}},
			{
				Number: firstSeason, Path: season1,
				Artwork: []expectedArtwork{localArtwork(artworkPoster, path.Join(folder, "season01-poster.jpg"))},
				Episodes: []expectedEpisode{
					{Number: 1, NumberEnd: multiEpisodeEnd, Files: inFolder(season1, "Pioneer One - S01E01-E02.mkv")},
					pioneerOneEpisodeWithMetadata(season1),
				},
			},
			{Number: secondSeason, Path: season2, Episodes: []expectedEpisode{
				{Number: 1, Files: inFolder(season2, "Pioneer One - S02E01.mkv")},
			}},
		},
	}
}

func pioneerOneEpisodeWithMetadata(seasonFolder string) expectedEpisode {
	return expectedEpisode{
		Number: metadataEpisode, Files: inFolder(seasonFolder, "Pioneer One - S01E03.mkv"),
		NFO: &expectedNFO{
			File: path.Join(seasonFolder, "Pioneer One - S01E03.nfo"), Format: nfoFormatXML, Encoding: encodingUTF8,
			Fields: nfoFields{
				Title: "The Man from Mars", Season: firstSeason, Episode: metadataEpisode, Aired: "2010-06-15",
				Plot: "The agents question the pilot.",
			},
		},
		Artwork: []expectedArtwork{localArtwork(artworkThumb, path.Join(seasonFolder, "Pioneer One - S01E03-thumb.jpg"))},
	}
}

// starWreck has its episode files directly in the library root, without a show folder.
func starWreck() expectedItem {
	episodes := make([]expectedEpisode, 0, rootShowEpisodes)
	for number := 1; number <= rootShowEpisodes; number++ {
		episodes = append(episodes, expectedEpisode{
			Number: number, Files: []string{fmt.Sprintf("Star Wreck - S01E%02d.mkv", number)},
		})
	}

	return expectedItem{
		Kind: itemKindShow, Title: "Star Wreck", Path: "",
		Seasons: []expectedSeason{{Number: firstSeason, Path: "", Episodes: episodes}},
	}
}

// dragonBallSuper is the DT-24 regression: 131 absolutely numbered files in one folder are
// one show with one season and 131 episodes, never 130 seasons.
func dragonBallSuper() expectedItem {
	episodes := make([]expectedEpisode, 0, dragonBallSuperEpisodes)

	for number := 1; number <= dragonBallSuperEpisodes; number++ {
		name := fmt.Sprintf("%s - %03d.mkv", dragonBallSuperFolder, number)
		episodes = append(episodes, expectedEpisode{
			Number: number, AbsoluteNumber: number, Files: inFolder(dragonBallSuperFolder, name),
		})
	}

	return expectedItem{
		Kind: itemKindShow, Title: dragonBallSuperFolder, Path: dragonBallSuperFolder,
		Seasons: []expectedSeason{{Number: firstSeason, Path: "", Episodes: episodes}},
	}
}

// dragonBallSuperFansub holds one anime-style release name. It lives in its own folder so its
// show key never collides with the 131-episode show.
func dragonBallSuperFansub() expectedItem {
	const folder = "Dragon Ball Super Fansub"

	name := fmt.Sprintf("[Group] Dragon Ball Super - %d [1080p] [ABCD1234].mkv", animeEpisodeNumber)

	return expectedItem{
		Kind: itemKindShow, Title: folder, Path: folder,
		Seasons: []expectedSeason{{Number: firstSeason, Path: "", Episodes: []expectedEpisode{
			{Number: animeEpisodeNumber, AbsoluteNumber: animeEpisodeNumber, Files: inFolder(folder, name)},
		}}},
	}
}

// singleFileMovie is a folder movie "<folder>/<folder>.mkv" with one version; the year comes
// from the folder name.
func singleFileMovie(title, folder string, nfo *expectedNFO) expectedItem {
	return expectedItem{
		Kind: itemKindMovie, Title: title, Year: yearOf(folder), Path: folder,
		Versions: []expectedVersion{{Label: "", Parts: inFolder(folder, folder+".mkv")}},
		NFO:      nfo,
	}
}

// looseMovie is "<folder>/<name>.mkv" in a folder holding several movies; name is
// "Title (YYYY)".
func looseMovie(folder, name string) expectedItem {
	file := path.Join(folder, name+".mkv")
	title, _, _ := strings.Cut(name, " (")

	return expectedItem{
		Kind: itemKindMovie, Title: title, Year: yearOf(name), Path: file,
		Versions: []expectedVersion{{Label: "", Parts: []string{file}}},
	}
}

// yearOf returns the year of the last "(YYYY)" in name, or 0 when there is none. The catalog
// states each year once, in the name a scan has to read it from.
func yearOf(name string) int {
	start := strings.LastIndex(name, "(") + len("(")
	closing := start + yearDigits

	if start == 0 || closing >= len(name) || name[closing] != ')' {
		return 0
	}

	year, err := strconv.Atoi(name[start:closing])
	if err != nil {
		return 0
	}

	return year
}

func localArtwork(kind, file string) expectedArtwork {
	return expectedArtwork{Kind: kind, Source: artworkSourceLocal, File: file}
}

func embeddedArtwork(kind, file string) expectedArtwork {
	return expectedArtwork{Kind: kind, Source: artworkSourceEmbedded, File: file}
}

// inFolder joins every name to folder with forward slashes.
func inFolder(folder string, names ...string) []string {
	files := make([]string, 0, len(names))
	for _, name := range names {
		files = append(files, path.Join(folder, name))
	}

	return files
}

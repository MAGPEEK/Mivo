package sqlite_test

import (
	"database/sql"
	"fmt"
	"testing"
)

// listingItem is one items row of the listing fixtures with the columns the listing sorts
// and filters by.
type listingItem struct {
	id, libraryID, parentID, kind string
	title, sortTitle              string
	year                          int
	releaseDate, contentRating    string
	createdAt                     int64
	number                        int
	genres                        []string
}

func insertListingItem(t *testing.T, inspection *sql.DB, item listingItem) {
	t.Helper()

	extraType := ""
	if item.kind == "extra" {
		extraType = "trailer"
	}

	execute(t, inspection, `INSERT INTO items (id, library_id, parent_id, kind, match_key, title, sort_title, year,
		release_date, content_rating, number, extra_type, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.id, item.libraryID, nullable(item.parentID, ""), item.kind, item.id, item.title, item.sortTitle,
		item.year, item.releaseDate, item.contentRating, nullable(item.number, 0), nullable(extraType, ""),
		item.createdAt, item.createdAt)

	for position, genre := range item.genres {
		tagID := "genre-" + genre
		execute(t, inspection, `INSERT OR IGNORE INTO tags (id, kind, name) VALUES (?, 'genre', ?)`, tagID, genre)
		execute(t, inspection, `INSERT INTO item_tags (item_id, tag_id, position) VALUES (?, ?, ?)`, item.id, tagID, position)
	}
}

// listingStream is one media_streams row: video streams carry codec, size and HDR, audio and
// subtitle streams codec and language.
type listingStream struct {
	kind, codec   string
	width, height int
	hdr, language string
	isDefault     bool
	isPicture     bool
	index         int
}

// listingFile is one media_files row with its streams; an empty probeError means probed.
type listingFile struct {
	id, itemID, libraryID, rootID, relativePath string
	container                                   string
	sizeBytes, durationMillis, bitRate          int64
	partNumber                                  int
	probeError                                  string
	streams                                     []listingStream
}

func insertListingFile(t *testing.T, inspection *sql.DB, file listingFile) {
	t.Helper()

	rootID := file.rootID
	if rootID == "" {
		rootID = file.libraryID
	}

	execute(t, inspection, `INSERT INTO media_files (id, item_id, library_id, root_id, relative_path, size_bytes,
		modified_at, container, format_name, duration_ms, bit_rate, probed_at, probe_error, part_number)
		VALUES (?, ?, ?, ?, ?, ?, 0, ?, 'format', ?, ?, 0, ?, ?)`,
		file.id, file.itemID, file.libraryID, rootID, file.relativePath, file.sizeBytes, file.container,
		file.durationMillis, file.bitRate, nullable(file.probeError, ""), file.partNumber)

	for index, stream := range file.streams {
		stream.index = index
		insertListingStream(t, inspection, file.id, stream)
	}
}

func insertListingStream(t *testing.T, inspection *sql.DB, fileID string, stream listingStream) {
	t.Helper()

	execute(t, inspection, `INSERT INTO media_streams (media_file_id, stream_index, kind, codec, profile, level,
		pixel_format, bit_depth, width, height, frame_rate, color_primaries, color_transfer, color_space, hdr,
		channels, channel_layout, sample_rate, language, title, is_default, is_forced, is_attached_picture, bit_rate)
		VALUES (?, ?, ?, ?, '', 0, '', 0, ?, ?, '', '', '', '', ?, 0, '', 0, ?, '', ?, 0, ?, 0)`,
		fileID, stream.index, stream.kind, stream.codec, stream.width, stream.height, stream.hdr, stream.language,
		stream.isDefault, stream.isPicture)
}

func video(codec string, width, height int, hdr string) listingStream {
	return listingStream{kind: "video", codec: codec, width: width, height: height, hdr: hdr}
}

func audio(codec, language string) listingStream {
	return listingStream{kind: "audio", codec: codec, language: language}
}

func subtitle(language string) listingStream {
	return listingStream{kind: "subtitle", codec: "subrip", language: language}
}

// IDs of the listing fixture: five movies, a trailer, three shows.
const (
	movieAlpha   = "m0000001-0000-4000-8000-000000000000"
	movieBravo   = "m0000002-0000-4000-8000-000000000000"
	movieCharlie = "m0000003-0000-4000-8000-000000000000"
	movieAerger  = "m0000004-0000-4000-8000-000000000000"
	movie1917    = "m0000005-0000-4000-8000-000000000000"
	trailerAlpha = "x0000001-0000-4000-8000-000000000000"
	showA        = "s0000001-0000-4000-8000-000000000000"
	showB        = "s0000002-0000-4000-8000-000000000000"
	showC        = "s0000003-0000-4000-8000-000000000000"
	// listingSecondRoot is a second folder of the movies library, holding 1917.
	listingSecondRoot = "r0000002-0000-4000-8000-000000000000"
)

// newListingFixture seeds the movies and shows libraries of a catalog fixture. In title order
// the movies are 1917, Alpha, Bravo, charlie, Ärger (sort titles compare as bytes, so "ä"
// comes after "c").
func newListingFixture(t *testing.T) catalogFixture {
	t.Helper()

	fixture := newCatalogFixture(t)
	execute(t, fixture.inspection, `INSERT INTO library_roots (id, library_id, path, path_key, position, created_at)
		VALUES (?, ?, '/media/movies-2', '/media/movies-2', 1, 0)`, listingSecondRoot, moviesLibraryID)

	seedListingMovies(t, fixture.inspection)
	seedListingShows(t, fixture.inspection)

	return fixture
}

func seedListingMovies(t *testing.T, inspection *sql.DB) {
	t.Helper()

	movies := []struct {
		item  listingItem
		files []listingFile
	}{
		{
			item: listingItem{id: movieAlpha, title: "Alpha", sortTitle: "alpha", year: 2008, releaseDate: "2008-05-01", contentRating: "PG", createdAt: 1000, genres: []string{"Drama"}},
			files: []listingFile{{
				relativePath: "Alpha (2008)/Alpha.mkv", container: "mkv", sizeBytes: 100, durationMillis: 5000, bitRate: 900,
				streams: []listingStream{{kind: "video", codec: "mjpeg", width: 600, height: 900, isPicture: true}, video("h264", 1920, 1080, ""), audio("aac", "eng"), subtitle("ger")},
			}},
		},
		{
			item: listingItem{id: movieBravo, title: "Bravo", sortTitle: "bravo", year: 1999, releaseDate: "1999-01-01", contentRating: "R", createdAt: 3000, genres: []string{"Action"}},
			files: []listingFile{
				{relativePath: "Bravo/Bravo.mp4", container: "mp4", sizeBytes: 300, durationMillis: 7000, bitRate: 500, streams: []listingStream{video("hevc", 3840, 2160, "hdr10"), audio("eac3", "ger")}},
				{relativePath: "Bravo/Bravo.part2.mp4", container: "mp4", sizeBytes: 999, durationMillis: 1, bitRate: 1, partNumber: 2, streams: []listingStream{video("vp9", 640, 360, "")}},
			},
		},
		{
			item:  listingItem{id: movieCharlie, title: "charlie", sortTitle: "charlie", createdAt: 2000},
			files: []listingFile{{relativePath: "charlie.mkv", container: "mkv", sizeBytes: 200, probeError: "probe_failed"}},
		},
		{
			item:  listingItem{id: movieAerger, title: "Ärger", sortTitle: "ärger", year: 2001, releaseDate: "2001-03-03", contentRating: "PG", createdAt: 4000, genres: []string{"Drama", "Comedy"}},
			files: []listingFile{{relativePath: "Ärger.avi", container: "avi", sizeBytes: 50, durationMillis: 3000, bitRate: 700, streams: []listingStream{video("mpeg4", 720, 576, ""), audio("mp3", "eng")}}},
		},
		{
			item: listingItem{id: movie1917, title: "1917", sortTitle: "1917", year: 2019, releaseDate: "2019-12-25", contentRating: "R", createdAt: 5000, genres: []string{"War"}},
			files: []listingFile{{
				rootID: listingSecondRoot, relativePath: "1917.mkv", container: "mkv", sizeBytes: 400, durationMillis: 9000, bitRate: 1100,
				streams: []listingStream{video("hevc", 1280, 720, ""), audio("aac", "eng"), {kind: "audio", codec: "truehd", language: "eng", isDefault: true}},
			}},
		},
	}

	for index, movie := range movies {
		movie.item.libraryID, movie.item.kind = moviesLibraryID, "movie"
		insertListingItem(t, inspection, movie.item)

		for fileIndex, file := range movie.files {
			file.id, file.itemID, file.libraryID = fmt.Sprintf("f%07d-%d000-4000-8000-000000000000", index+1, fileIndex), movie.item.id, moviesLibraryID
			insertListingFile(t, inspection, file)
		}
	}

	// A trailer of Alpha: never listed, counted or found in a folder.
	insertListingItem(t, inspection, listingItem{
		id: trailerAlpha, libraryID: moviesLibraryID, parentID: movieAlpha, kind: "extra",
		title: "Trailer", sortTitle: "a trailer", year: 1950, createdAt: 1, genres: []string{"Drama"},
	})
	insertListingFile(t, inspection, listingFile{
		id: "fx000001-0000-4000-8000-000000000000", itemID: trailerAlpha, libraryID: moviesLibraryID,
		relativePath: "Alpha (2008)/trailer.mkv", container: "webm", sizeBytes: 1, streams: []listingStream{video("av1", 7680, 4320, "hlg"), audio("opus", "fre")},
	})
}

// seedListingShows adds Show A (an HDR 4K and a 1080p episode), Show B (a 720p episode) and
// Show C (no episodes).
func seedListingShows(t *testing.T, inspection *sql.DB) {
	t.Helper()

	shows := []struct {
		id, title string
		year      int
		episodes  []listingFile
	}{
		{id: showA, title: "Show A", year: 2010, episodes: []listingFile{
			{container: "mkv", sizeBytes: 10, streams: []listingStream{video("hevc", 3840, 2160, "hdr10")}},
			{container: "mp4", sizeBytes: 20, streams: []listingStream{video("h264", 1920, 1080, "")}},
		}},
		{id: showB, title: "Show B", year: 2015, episodes: []listingFile{
			{container: "mkv", sizeBytes: 5, streams: []listingStream{video("h264", 1280, 720, "")}},
		}},
		{id: showC, title: "Show C", year: 2020},
	}

	for showIndex, show := range shows {
		insertListingItem(t, inspection, listingItem{
			id: show.id, libraryID: showsLibraryID, kind: "show", title: show.title,
			sortTitle: show.title, year: show.year, createdAt: int64(showIndex),
		})

		seasonID := fmt.Sprintf("t%07d-0000-4000-8000-000000000000", showIndex+1)
		insertListingItem(t, inspection, listingItem{
			id: seasonID, libraryID: showsLibraryID, parentID: show.id, kind: "season",
			title: "Season 1", sortTitle: "season 1", number: 1,
		})

		for episodeIndex, file := range show.episodes {
			episodeID := fmt.Sprintf("e%07d-%d000-4000-8000-000000000000", showIndex+1, episodeIndex)
			insertListingItem(t, inspection, listingItem{
				id: episodeID, libraryID: showsLibraryID, parentID: seasonID, kind: "episode",
				title: "Episode", sortTitle: "episode", number: episodeIndex + 1,
			})

			file.id, file.itemID, file.libraryID = "g"+episodeID[1:], episodeID, showsLibraryID
			file.relativePath = fmt.Sprintf("%s/Season 1/e%d.mkv", show.title, episodeIndex+1)
			insertListingFile(t, inspection, file)
		}
	}
}

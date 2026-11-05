package main

import (
	"fmt"
	"path"
)

// The music tree (task M3-13f) is declared as its expected catalog like the movie and show
// trees: library_music.go derives one FFmpeg case per track from it, so the manifest and the
// files cannot diverge. Artist, album and track names are invented; every file is a short
// sine tone.

// musicLibraryPath is the music library root below the -library directory.
const musicLibraryPath = "music"

// Short container names of the music albums; each is also the file extension.
const (
	musicContainerFLAC = "flac"
	musicContainerMP3  = "mp3"
	musicContainerM4A  = "m4a"
)

const (
	// folderCoverName is the cover image a scan finds in an album folder.
	folderCoverName = "cover.jpg"
	// variousArtists is the album artist of compilations, as the domain spells it.
	variousArtists = "Various Artists"
	// singleDisc is the disc of every track of a one-disc album; firstDisc and secondDisc
	// are the discs of the two-disc album.
	singleDisc = 1
	firstDisc  = 1
	secondDisc = 2
	// discFolderFormat names the disc folders of a multi-disc album ("CD 1").
	discFolderFormat = "CD %d"
	// trackFileFormat names a track file "NN Title.ext".
	trackFileFormat = "%02d %s.%s"
)

// trackSpec is one track of an album before its file name and numbers are derived.
type trackSpec struct {
	disc  int
	title string
	// artist is the track artist; "" means the album artist.
	artist string
}

// albumLayout says how the tracks of an album are stored.
type albumLayout struct {
	tracks      []trackSpec
	tagged      bool
	coverSource string
}

// musicCatalog returns the expected catalog of the generated music tree.
func musicCatalog() expectedMusic {
	return expectedMusic{Path: musicLibraryPath, Albums: []expectedAlbum{
		multiDiscAlbum(), mp3Album(), folderCoverAlbum(), compilationAlbum(), untaggedAlbum(),
	}}
}

// multiDiscAlbum is a tagged FLAC album on two discs in "CD n" folders with an embedded
// cover in every track.
func multiDiscAlbum() expectedAlbum {
	const year = 2019

	album := expectedAlbum{
		AlbumArtist: "Generated Orchestra", Title: "Two Disc Symphony", Year: year,
		Path: "Generated Orchestra/Two Disc Symphony (2019)", Container: musicContainerFLAC,
	}

	return withTracks(album, albumLayout{tagged: true, coverSource: coverSourceEmbedded, tracks: []trackSpec{
		{disc: firstDisc, title: "Overture"},
		{disc: firstDisc, title: "Allegro"},
		{disc: secondDisc, title: "Adagio"},
		{disc: secondDisc, title: "Finale"},
	}})
}

// mp3Album is a tagged MP3 album without cover; one track has a guest artist, so track and
// album artist differ.
func mp3Album() expectedAlbum {
	const year = 2021

	album := expectedAlbum{
		AlbumArtist: "Sine Wave Band", Title: "Square Roots", Year: year,
		Path: "Sine Wave Band/Square Roots", Container: musicContainerMP3,
	}

	return withTracks(album, albumLayout{tagged: true, coverSource: coverSourceNone, tracks: []trackSpec{
		{disc: singleDisc, title: "First Tone"},
		{disc: singleDisc, title: "Second Tone", artist: "Sine Wave Band feat. White Noise"},
		{disc: singleDisc, title: "Third Tone"},
	}})
}

// folderCoverAlbum is a tagged M4A (AAC) album whose cover is cover.jpg in its folder.
func folderCoverAlbum() expectedAlbum {
	const year = 2023

	album := expectedAlbum{
		AlbumArtist: "Lavfi Quartet", Title: "Test Pattern", Year: year,
		Path: "Lavfi Quartet/Test Pattern", Container: musicContainerM4A,
	}

	return withTracks(album, albumLayout{tagged: true, coverSource: coverSourceFolder, tracks: []trackSpec{
		{disc: singleDisc, title: "Colour Bars"}, {disc: singleDisc, title: "Moving Gradient"},
	}})
}

// compilationAlbum is a tagged "Various Artists" compilation: album artist Various Artists,
// the compilation flag and a different artist per track.
func compilationAlbum() expectedAlbum {
	const year = 2024

	album := expectedAlbum{
		AlbumArtist: variousArtists, Title: "Generated Hits", Year: year, Compilation: true,
		Path: "Various Artists/Generated Hits", Container: musicContainerMP3,
	}

	return withTracks(album, albumLayout{tagged: true, coverSource: coverSourceNone, tracks: []trackSpec{
		{disc: singleDisc, title: "Alpha Song", artist: "Alpha Artist"},
		{disc: singleDisc, title: "Beta Song", artist: "Beta Artist"},
		{disc: singleDisc, title: "Gamma Song", artist: "Gamma Artist"},
	}})
}

// untaggedAlbum carries no tags at all: a scan takes artist, album, track and title from the
// layout "Artist/Album/NN Title.flac".
func untaggedAlbum() expectedAlbum {
	album := expectedAlbum{
		AlbumArtist: "Folder Artist", Title: "Folder Album",
		Path: "Folder Artist/Folder Album", Container: musicContainerFLAC,
	}

	return withTracks(album, albumLayout{tagged: false, coverSource: coverSourceNone, tracks: []trackSpec{
		{disc: singleDisc, title: "Untagged One"}, {disc: singleDisc, title: "Untagged Two"},
	}})
}

// withTracks derives the track files "<album>/[CD n/]NN Title.<container>", numbering the
// tracks per disc.
func withTracks(album expectedAlbum, layout albumLayout) expectedAlbum {
	tracksOnDisc := map[int]int{}
	for _, spec := range layout.tracks {
		tracksOnDisc[spec.disc]++
	}

	numberOnDisc := map[int]int{}

	for _, spec := range layout.tracks {
		numberOnDisc[spec.disc]++
		number := numberOnDisc[spec.disc]

		folder := album.Path
		if len(tracksOnDisc) > 1 {
			folder = path.Join(folder, fmt.Sprintf(discFolderFormat, spec.disc))
		}

		track := expectedTrack{
			File:   path.Join(folder, fmt.Sprintf(trackFileFormat, number, spec.title, album.Container)),
			Artist: firstNonEmpty(spec.artist, album.AlbumArtist), Album: album.Title,
			Disc: spec.disc, Track: number, Title: spec.title, Tagged: layout.tagged,
			CoverSource: layout.coverSource,
		}

		if layout.tagged {
			track.DiscTotal = len(tracksOnDisc)
			track.TrackTotal = tracksOnDisc[spec.disc]
		}

		track.CoverFile = coverFileOf(album, track)
		album.Tracks = append(album.Tracks, track)
	}

	return album
}

// coverFileOf is the folder image, the track itself for an embedded cover, or "".
func coverFileOf(album expectedAlbum, track expectedTrack) string {
	switch track.CoverSource {
	case coverSourceFolder:
		return path.Join(album.Path, folderCoverName)
	case coverSourceEmbedded:
		return track.File
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

package main

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"
)

const (
	// categoryMusic groups the track cases in progress lines and the summary.
	categoryMusic = "music"
	// artworkCover is the square album cover shape; every folder cover is a copy of it and
	// the embedded covers are byte copies of its JPEG.
	artworkCover = "cover"
	coverEdge    = 500
	// compilationFlag is the value of the compilation tag (ID3 TCMP, MP4 cpil).
	compilationFlag = "1"
)

// musicFormat is how the tracks of one container are encoded.
type musicFormat struct {
	muxer       string
	encoder     string
	codec       string
	probeFormat string
}

// musicFormatOf returns the format of an album container; the ipod muxer writes the MP4
// flavour Apple uses for .m4a.
func musicFormatOf(container string) musicFormat {
	switch container {
	case musicContainerMP3:
		return musicFormat{muxer: "mp3", encoder: "libmp3lame", codec: "mp3", probeFormat: "mp3"}
	case musicContainerM4A:
		return musicFormat{muxer: "ipod", encoder: "aac", codec: "aac", probeFormat: probeISOBMFF}
	default:
		return musicFormat{muxer: "flac", encoder: "flac", codec: "flac", probeFormat: "flac"}
	}
}

// musicCases returns the cover image case and one case per track. The cover image is built
// as the first folder cover; tracks with an embedded cover take that file in the second
// phase of generateLibrary.
func musicCases() []mediaCase {
	catalog := musicCatalog()
	folderCovers := folderCoverFiles(catalog)

	var (
		cases []mediaCase
		cover string
	)

	if len(folderCovers) > 0 {
		cover = folderCovers[0]
		shape := imageShape{name: artworkCover, width: coverEdge, height: coverEdge, extension: extensionJPEG}
		cases = append(cases, imageCase(shape, cover, folderCovers[1:]))
	}

	for _, album := range catalog.Albums {
		for _, track := range album.Tracks {
			cases = append(cases, trackCase(catalog.Path, album, track, cover))
		}
	}

	return cases
}

// folderCoverFiles lists the folder cover images, relative to the -library directory.
func folderCoverFiles(catalog expectedMusic) []string {
	var files []string

	for _, album := range catalog.Albums {
		for _, track := range album.Tracks {
			file := path.Join(catalog.Path, track.CoverFile)
			if track.CoverSource == coverSourceFolder && !slices.Contains(files, file) {
				files = append(files, file)
			}
		}
	}

	return files
}

// trackCase builds one 2 s stereo sine track carrying the tags of the manifest; untagged
// tracks carry none, because buildArgs strips all metadata.
func trackCase(root string, album expectedAlbum, track expectedTrack, cover string) mediaCase {
	format := musicFormatOf(album.Container)
	output := path.Join(root, track.File)

	c := mediaCase{
		Name:     libraryCasePrefix + root + "-" + slug(strings.TrimSuffix(track.File, path.Ext(track.File))),
		Category: categoryMusic, Output: output,
		Source: sourceSpec{SampleRate: cdSampleRate, Channels: stereoChannels, Duration: clipDuration},
		Muxer:  format.muxer, Encoders: []string{format.encoder},
		Args:   slices.Concat(audioArgs(format.encoder), metadataArgs(trackTags(album, track))),
		Expect: expect(album.Container, format.probeFormat, nil, audioOf(format.codec)),
	}

	if track.CoverSource == coverSourceEmbedded {
		c.Cover = cover
	}

	return c
}

// writeMusicSummary adds one line per album, e.g.
// "album Sine Wave Band - Square Roots (2021): 3 mp3 tracks, cover none".
func writeMusicSummary(text *strings.Builder, music expectedMusic) {
	if len(music.Albums) == 0 {
		return
	}

	fmt.Fprintf(text, "  %s/ (%d albums)\n", music.Path, len(music.Albums))

	for _, album := range music.Albums {
		title := album.AlbumArtist + " - " + album.Title
		if album.Year > 0 {
			title += fmt.Sprintf(" (%d)", album.Year)
		}

		fmt.Fprintf(text, "    album %s: %s\n", title, albumDetails(album))
	}
}

// albumDetails describes the tracks of an album. Disc count, tagging and cover are the same
// for every track of an album, so the first track speaks for all of them.
func albumDetails(album expectedAlbum) string {
	details := fmt.Sprintf("%d %s tracks", len(album.Tracks), album.Container)
	if len(album.Tracks) == 0 {
		return details
	}

	first := album.Tracks[0]
	if first.DiscTotal > 1 {
		details += fmt.Sprintf(" on %d discs", first.DiscTotal)
	}

	if !first.Tagged {
		details += ", untagged"
	}

	if album.Compilation {
		details += ", compilation"
	}

	return details + ", cover " + first.CoverSource
}

// trackTags are the FFmpeg metadata keys of a tagged track; FFmpeg maps them to ID3, MP4
// and Vorbis comment fields.
func trackTags(album expectedAlbum, track expectedTrack) map[string]string {
	if !track.Tagged {
		return nil
	}

	tags := map[string]string{
		"title": track.Title, "artist": track.Artist, "album_artist": album.AlbumArtist, "album": track.Album,
		"track": fmt.Sprintf("%d/%d", track.Track, track.TrackTotal),
		"disc":  fmt.Sprintf("%d/%d", track.Disc, track.DiscTotal),
	}

	if album.Year > 0 {
		tags["date"] = strconv.Itoa(album.Year)
	}

	if album.Compilation {
		tags["compilation"] = compilationFlag
	}

	return tags
}

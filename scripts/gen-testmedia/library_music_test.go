package main

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func musicAlbumNamed(t *testing.T, title string) expectedAlbum {
	t.Helper()

	for _, album := range musicCatalog().Albums {
		if album.Title == title {
			return album
		}
	}

	t.Fatalf("no music album %q", title)

	return expectedAlbum{}
}

// TestMusicCatalogCoversTheRequiredAlbums pins the albums task M3-13f asks for.
func TestMusicCatalogCoversTheRequiredAlbums(t *testing.T) {
	t.Parallel()

	tests := []struct {
		title       string
		container   string
		tagged      bool
		coverSource string
		discs       int
		compilation bool
	}{
		{"Two Disc Symphony", musicContainerFLAC, true, coverSourceEmbedded, 2, false},
		{"Square Roots", musicContainerMP3, true, coverSourceNone, 1, false},
		{"Test Pattern", musicContainerM4A, true, coverSourceFolder, 1, false},
		{"Generated Hits", musicContainerMP3, true, coverSourceNone, 1, true},
		{"Folder Album", musicContainerFLAC, false, coverSourceNone, 1, false},
	}

	for _, test := range tests {
		album := musicAlbumNamed(t, test.title)
		discs := map[int]bool{}

		for _, track := range album.Tracks {
			discs[track.Disc] = true

			if track.Tagged != test.tagged || track.CoverSource != test.coverSource || !strings.HasSuffix(track.File, "."+test.container) {
				t.Errorf("%s: track %+v, want tagged %t, cover %s, extension %s", test.title, track, test.tagged, test.coverSource, test.container)
			}
		}

		if album.Container != test.container || len(discs) != test.discs || album.Compilation != test.compilation {
			t.Errorf("%s: container %s, %d discs, compilation %t", test.title, album.Container, len(discs), album.Compilation)
		}
	}

	if compilation := musicAlbumNamed(t, "Generated Hits"); compilation.AlbumArtist != variousArtists {
		t.Errorf("compilation album artist = %q, want %q", compilation.AlbumArtist, variousArtists)
	}
}

func TestWithTracksNumbersPerDiscAndUsesDiscFolders(t *testing.T) {
	t.Parallel()

	album := musicAlbumNamed(t, "Two Disc Symphony")
	want := []expectedTrack{
		{File: "Generated Orchestra/Two Disc Symphony (2019)/CD 1/01 Overture.flac", Disc: 1, DiscTotal: 2, Track: 1, TrackTotal: 2},
		{File: "Generated Orchestra/Two Disc Symphony (2019)/CD 1/02 Allegro.flac", Disc: 1, DiscTotal: 2, Track: 2, TrackTotal: 2},
		{File: "Generated Orchestra/Two Disc Symphony (2019)/CD 2/01 Adagio.flac", Disc: 2, DiscTotal: 2, Track: 1, TrackTotal: 2},
		{File: "Generated Orchestra/Two Disc Symphony (2019)/CD 2/02 Finale.flac", Disc: 2, DiscTotal: 2, Track: 2, TrackTotal: 2},
	}

	for index, track := range album.Tracks {
		got := expectedTrack{File: track.File, Disc: track.Disc, DiscTotal: track.DiscTotal, Track: track.Track, TrackTotal: track.TrackTotal}
		if got != want[index] {
			t.Errorf("track %d = %+v, want %+v", index, got, want[index])
		}

		if track.CoverFile != track.File || track.Artist != album.AlbumArtist {
			t.Errorf("track %d: cover file %q, artist %q", index, track.CoverFile, track.Artist)
		}
	}

	untagged := musicAlbumNamed(t, "Folder Album").Tracks[0]
	if untagged.File != "Folder Artist/Folder Album/01 Untagged One.flac" || untagged.DiscTotal != 0 || untagged.TrackTotal != 0 {
		t.Errorf("untagged track = %+v", untagged)
	}
}

func TestTrackTags(t *testing.T) {
	t.Parallel()

	compilation := musicAlbumNamed(t, "Generated Hits")
	got := trackTags(compilation, compilation.Tracks[1])
	want := map[string]string{
		"title": "Beta Song", "artist": "Beta Artist", "album_artist": variousArtists, "album": "Generated Hits",
		"track": "2/3", "disc": "1/1", "date": "2024", "compilation": compilationFlag,
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("trackTags = %v, want %v", got, want)
	}

	untagged := musicAlbumNamed(t, "Folder Album")
	if tags := trackTags(untagged, untagged.Tracks[0]); tags != nil {
		t.Errorf("untagged track tags = %v, want none", tags)
	}
}

func TestMusicCasesEmbedTheFolderCoverImage(t *testing.T) {
	t.Parallel()

	cases := musicCases()
	cover := cases[0]

	if cover.Name != artworkCaseName(artworkCover) || cover.Output != "music/Lavfi Quartet/Test Pattern/cover.jpg" || len(cover.Copies) != 0 {
		t.Fatalf("first music case = %s writing %s, want the album cover image", cover.Name, cover.Output)
	}

	for _, c := range cases[1:] {
		embeds := strings.HasSuffix(c.Output, ".flac") && strings.Contains(c.Output, "Two Disc Symphony")
		if (c.Cover == cover.Output) != embeds || (c.Cover != "" && !embeds) {
			t.Errorf("%s: cover %q, want the album cover only for the embedded-cover album", c.Name, c.Cover)
		}

		if c.Category != categoryMusic || c.Source.hasVideo() || !c.Source.hasAudio() {
			t.Errorf("%s: category %s, source %+v, want audio-only music", c.Name, c.Category, c.Source)
		}
	}
}

func TestMusicFormatOf(t *testing.T) {
	t.Parallel()

	for container, muxer := range map[string]string{
		musicContainerFLAC: "flac", musicContainerMP3: "mp3", musicContainerM4A: "ipod",
	} {
		if got := musicFormatOf(container).muxer; got != muxer {
			t.Errorf("musicFormatOf(%s).muxer = %s, want %s", container, got, muxer)
		}
	}
}

// TestCoverArgsForAudioOnlySources: without a source video the cover is the first video
// stream, so its codec and disposition options name v:0.
func TestCoverArgsForAudioOnlySources(t *testing.T) {
	t.Parallel()

	c := mediaCase{Muxer: "flac", Cover: "cover.jpg", Source: sourceSpec{SampleRate: cdSampleRate, Channels: stereoChannels}}

	inputs, maps, options := coverArgs(c, 1)
	if !slices.Equal(inputs, []string{"-i", "cover.jpg"}) || !slices.Equal(maps, []string{"-map", "1:v:0"}) ||
		!slices.Equal(options, []string{"-c:v:0", "copy", "-disposition:v:0", "attached_pic"}) {
		t.Errorf("coverArgs = %v %v %v", inputs, maps, options)
	}
}

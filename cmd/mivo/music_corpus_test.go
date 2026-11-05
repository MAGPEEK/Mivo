package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/paths"
)

const (
	// musicLibraryName is the name the test gives the music library.
	musicLibraryName = "Music"
	// coverSourceNone marks an album of the manifest without cover.
	coverSourceNone = "none"
	// directPlayMusicContainer is the album container the test profile plays directly (AAC in
	// MP4); the FLAC and MP3 albums need a transcode to AAC over HLS.
	directPlayMusicContainer = "m4a"
	// transcodedMusicContainer is the album container the test profile cannot play directly.
	transcodedMusicContainer = "flac"
	// firstDiscNumber is the disc of every track of a one-disc album.
	firstDiscNumber = 1
)

// musicCorpusManifest holds the music part of testdata/library/manifest.json (M3-13f).
type musicCorpusManifest struct {
	Music struct {
		Path   string       `json:"path"`
		Albums []musicAlbum `json:"albums"`
	} `json:"music"`
}

type musicAlbum struct {
	AlbumArtist string       `json:"albumArtist"`
	Title       string       `json:"title"`
	Year        int32        `json:"year"`
	Container   string       `json:"container"`
	Tracks      []musicTrack `json:"tracks"`
}

type musicTrack struct {
	Title       string `json:"title"`
	Disc        int32  `json:"disc"`
	Track       int32  `json:"track"`
	Tagged      bool   `json:"tagged"`
	CoverSource string `json:"coverSource"`
}

// TestRunServesTheMusicCorpus is the music walking skeleton (M3-13g): a configured music
// library is scanned at startup, artists, albums and tracks match the generator's manifest
// in contract order, covers are served through the image endpoint, and tracks play directly
// or transcoded to AAC.
func TestRunServesTheMusicCorpus(t *testing.T) {
	t.Parallel()

	corpus := copyLibraryCorpus(t)
	music := readMusicManifest(t)
	environment := isolatedEnvironment(t)
	writeConfigFile(t, environment[paths.ConfigDirEnv()], fmt.Sprintf(
		"scan:\n  watch: false\n  interval: 0\nlibraries:\n  - name: %s\n    kind: music\n    path: '%s'\n",
		musicLibraryName, filepath.Join(corpus.root, music.Music.Path)))

	server := startTestServer(t, environment, runHooks{})
	assertMusicKindAvailable(t, server.baseURL)

	libraryID := awaitScannedLibraries(t, server.baseURL, 1, time.Time{})[musicLibraryName].ID.String()
	artists := assertArtists(t, server.baseURL, libraryID, music.Music.Albums)

	albums := listItemsOfKind(t, server.baseURL, libraryID, apigen.ItemKindAlbum)
	if len(albums) != len(music.Music.Albums) {
		t.Fatalf("music library lists %d albums, want %d: %v", len(albums), len(music.Music.Albums), itemTitles(albums))
	}

	tracksByContainer := map[string]apigen.ItemSummary{}

	for _, want := range music.Music.Albums {
		album := findByTitle(t, albums, want.Title)
		if album.ParentID == nil || *album.ParentID != artists[want.AlbumArtist].ID {
			t.Errorf("album %q parent = %v, want the artist %q", want.Title, album.ParentID, want.AlbumArtist)
		}

		tracks := assertAlbum(t, server.baseURL, want, album)
		tracksByContainer[want.Container] = tracks[0]
	}

	assertTrackPlays(t, server.baseURL, tracksByContainer[directPlayMusicContainer], apigen.PlaybackModeDirectPlay)
	assertTrackPlays(t, server.baseURL, tracksByContainer[transcodedMusicContainer], apigen.PlaybackModeTranscode)

	server.stop(t)
	assertNoSessionDirectories(t, environment[envPathsTranscode])
}

// readMusicManifest reads the music part of the manifest the corpus copy came from.
func readMusicManifest(t *testing.T) musicCorpusManifest {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(libraryCorpusDirectory, libraryManifestFile))
	if err != nil {
		t.Fatalf("read the library manifest: %v", err)
	}

	var manifest musicCorpusManifest
	if err := json.Unmarshal(content, &manifest); err != nil || len(manifest.Music.Albums) == 0 {
		t.Fatalf("library manifest has no music albums (error %v); regenerate it with `go run ./scripts/gen-testmedia`", err)
	}

	return manifest
}

func assertMusicKindAvailable(t *testing.T, baseURL string) {
	t.Helper()

	var info apigen.SystemInfo

	getJSON(t, baseURL+"/api/v1/system/info", &info)

	if !slices.Contains(info.LibraryKinds, apigen.LibraryKindMusic) {
		t.Errorf("libraryKinds = %v, want music", info.LibraryKinds)
	}
}

// assertArtists checks that the default listing of a music library holds one artist per
// album artist in title order and returns them by title.
func assertArtists(t *testing.T, baseURL, libraryID string, albums []musicAlbum) map[string]apigen.ItemSummary {
	t.Helper()

	var want []string
	for _, album := range albums {
		if !slices.Contains(want, album.AlbumArtist) {
			want = append(want, album.AlbumArtist)
		}
	}

	slices.Sort(want)

	listed := listItems(t, baseURL, libraryID)
	if got := itemTitles(listed); !slices.Equal(got, want) {
		t.Errorf("artists = %v, want %v", got, want)
	}

	artists := map[string]apigen.ItemSummary{}

	for _, artist := range listed {
		if artist.Kind != apigen.ItemKindArtist {
			t.Errorf("%q kind = %s, want artist", artist.Title, artist.Kind)
		}

		artists[artist.Title] = artist
	}

	return artists
}

// assertAlbum checks year, track count, track order, numbers and the cover of an album and
// returns its tracks.
func assertAlbum(t *testing.T, baseURL string, want musicAlbum, album apigen.ItemSummary) []apigen.ItemSummary {
	t.Helper()

	if want.Year != 0 && (album.Year == nil || *album.Year != want.Year) {
		t.Errorf("album %q year = %v, want %d", want.Title, album.Year, want.Year)
	}

	if album.ChildCount == nil || int(*album.ChildCount) != len(want.Tracks) {
		t.Errorf("album %q childCount = %v, want %d", want.Title, album.ChildCount, len(want.Tracks))
	}

	tracks := listChildren(t, baseURL, album.ID.String())
	if len(tracks) != len(want.Tracks) {
		t.Fatalf("album %q has %d tracks, want %d: %v", want.Title, len(tracks), len(want.Tracks), itemTitles(tracks))
	}

	for index, wantTrack := range want.Tracks {
		assertTrack(t, want.Title, wantTrack, tracks[index])
	}

	if want.Tracks[0].CoverSource != coverSourceNone {
		assertCoverServed(t, baseURL, want.Title, album)
	} else if album.Square != nil {
		t.Errorf("album %q has no cover but lists square %+v", want.Title, album.Square)
	}

	return tracks
}

// assertTrack compares a track at its position in album order (disc, then track number).
func assertTrack(t *testing.T, album string, want musicTrack, got apigen.ItemSummary) {
	t.Helper()

	if got.Kind != apigen.ItemKindTrack || got.Title != want.Title || got.Number == nil || *got.Number != want.Track {
		t.Errorf("album %q: track = %s %q number %s, want %q number %d", album, got.Kind, got.Title, numberText(got.Number), want.Title, want.Track)
	}

	// An untagged file has no disc tag: its disc is unknown (omitted) or the first.
	untaggedFirstDisc := !want.Tagged && want.Disc == firstDiscNumber && got.DiscNumber == nil
	if !untaggedFirstDisc && (got.DiscNumber == nil || *got.DiscNumber != want.Disc) {
		t.Errorf("album %q: track %q discNumber = %s, want %d", album, want.Title, numberText(got.DiscNumber), want.Disc)
	}

	if got.Media == nil || !got.Media.Available || got.Media.AudioCodec == nil {
		t.Errorf("album %q: track %q media = %+v, want a probed audio file", album, want.Title, got.Media)
	}
}

// assertCoverServed requests the square cover of an album at the grid width.
func assertCoverServed(t *testing.T, baseURL, title string, album apigen.ItemSummary) {
	t.Helper()

	if album.Square == nil || album.Poster != nil {
		t.Errorf("album %q has a cover but square = %+v, poster = %+v", title, album.Square, album.Poster)

		return
	}

	query := url.Values{"width": {fmt.Sprint(posterWidth)}, "tag": {album.Square.Tag}}

	image := fetchImage(t, baseURL+"/api/v1/images/"+album.Square.ID.String()+"?"+query.Encode())
	if image.status != http.StatusOK || !strings.HasPrefix(image.header.Get("Content-Type"), "image/") {
		t.Errorf("album %q cover = %d %s, want 200 with an image", title, image.status, image.header.Get("Content-Type"))
	}
}

// assertTrackPlays starts a session for the track's file and fetches the start of the stream.
func assertTrackPlays(t *testing.T, baseURL string, track apigen.ItemSummary, wantMode apigen.PlaybackMode) {
	t.Helper()

	var detail apigen.ItemDetail

	getJSON(t, baseURL+"/api/v1/items/"+track.ID.String(), &detail)

	if len(detail.Files) != 1 {
		t.Fatalf("track %q has %d files, want 1", track.Title, len(detail.Files))
	}

	session := createSession(t, baseURL, detail.Files[0].ID.String(), http.StatusCreated)
	if session.Mode != wantMode {
		t.Fatalf("track %q session mode = %s, want %s", track.Title, session.Mode, wantMode)
	}

	if wantMode == apigen.PlaybackModeDirectPlay {
		assertRangeServed(t, baseURL+session.StreamURL)

		return
	}

	playlist := fetchBody(t, baseURL+session.StreamURL, http.StatusOK)
	if !strings.Contains(playlist, "seg-00000.m4s") {
		t.Fatalf("track %q playlist lacks the first segment:\n%s", track.Title, playlist)
	}

	fetchBody(t, strings.TrimSuffix(baseURL+session.StreamURL, "index.m3u8")+"seg-00000.m4s", http.StatusOK)
}

func listItemsOfKind(t *testing.T, baseURL, libraryID string, kind apigen.ItemKind) []apigen.ItemSummary {
	t.Helper()

	var page apigen.ItemPage

	getJSON(t, fmt.Sprintf("%s/api/v1/libraries/%s/items?limit=%d&type=%s", baseURL, libraryID, listLimit, kind), &page)

	if page.NextCursor != nil {
		t.Fatalf("library %s has more than %d %s items; the test expects one page", libraryID, listLimit, kind)
	}

	return page.Items
}

func findByTitle(t *testing.T, items []apigen.ItemSummary, title string) apigen.ItemSummary {
	t.Helper()

	for _, item := range items {
		if item.Title == title {
			return item
		}
	}

	t.Fatalf("%q is not listed; got %v", title, itemTitles(items))

	return apigen.ItemSummary{}
}

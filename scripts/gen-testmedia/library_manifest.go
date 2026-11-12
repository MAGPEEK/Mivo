package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// libraryManifestSchemaVersion changes when consumers of <library>/manifest.json (M3-08,
// M3-11b, M3-18) have to adapt to the JSON shape. The schema is documented in the package doc.
const libraryManifestSchemaVersion = 1

// Library and item kinds as ADR 0027 names them.
const (
	libraryKindMovies = "movies"
	libraryKindShows  = "shows"
	itemKindMovie     = "movie"
	itemKindShow      = "show"
)

// Extra types (ADR 0027 section 4).
const (
	extraTrailer    = "trailer"
	extraFeaturette = "featurette"
)

// Artwork kinds and sources (ADR 0027 section 8).
const (
	artworkPoster         = "poster"
	artworkBackdrop       = "backdrop"
	artworkThumb          = "thumb"
	artworkLogo           = "logo"
	artworkSourceLocal    = "local"
	artworkSourceEmbedded = "embedded"
)

// NFO formats: Kodi XML, a file holding only a provider URL, and a file that is no
// well-formed XML (the scan must ignore it and keep the file-name metadata).
const (
	nfoFormatXML       = "xml"
	nfoFormatURL       = "url"
	nfoFormatMalformed = "malformed"
	encodingUTF8       = "UTF-8"
	encodingLatin1     = "ISO-8859-1"
)

// Provider names of NFO provider IDs (ADR 0027 section 5).
const (
	providerTMDB = "tmdb"
	providerIMDb = "imdb"
	providerTVDB = "tvdb"
)

// libraryManifest is <library>/manifest.json: the expected catalog and the build result of
// every library case.
type libraryManifest struct {
	SchemaVersion int               `json:"schemaVersion"`
	FFmpegVersion string            `json:"ffmpegVersion"`
	Libraries     []expectedLibrary `json:"libraries"`
	// Music is the music library (task M3-13f). It is kept out of Libraries, whose consumers
	// compare movie and show items, so adding it changed no existing consumer.
	Music expectedMusic `json:"music"`
	// Discs is the disc library (task M3-16i), kept apart for the same reason.
	Discs expectedDiscs `json:"discs"`
	// Audiobooks is the audiobook library (task M3-14f), kept apart for the same reason.
	Audiobooks expectedAudiobooks `json:"audiobooks"`
	// Photos is the photo library (task M3-15g), kept apart for the same reason.
	Photos expectedPhotos `json:"photos"`
	// Subtitles is the sidecar subtitle library (task M4-03), kept apart for the same reason.
	Subtitles expectedSubtitles  `json:"subtitles"`
	Cases     []libraryCaseEntry `json:"cases"`
}

// Music cover sources: an attached picture in the file, an image in the album folder, none.
const (
	coverSourceEmbedded = "embedded"
	coverSourceFolder   = "folder"
	coverSourceNone     = "none"
)

// expectedMusic is the music library root below the -library directory.
type expectedMusic struct {
	// Path is the library root relative to the -library directory; the paths of its albums
	// and tracks are relative to this root.
	Path   string          `json:"path"`
	Albums []expectedAlbum `json:"albums"`
}

// expectedAlbum is one album a scan must produce.
type expectedAlbum struct {
	AlbumArtist string `json:"albumArtist"`
	Title       string `json:"title"`
	Year        int    `json:"year,omitempty"`
	Compilation bool   `json:"compilation,omitempty"`
	// Path is the album folder.
	Path string `json:"path"`
	// Container is the short container name of every track: "flac", "mp3" or "m4a".
	Container string          `json:"container"`
	Tracks    []expectedTrack `json:"tracks"`
}

// expectedTrack is one audio file. Artist, Album, Disc, Track and Title are the expected
// catalog values; Tagged says whether the file carries them as tags (and Year, AlbumArtist
// and Compilation of its album) or a scan has to take them from the path.
type expectedTrack struct {
	File   string `json:"file"`
	Artist string `json:"artist"`
	Album  string `json:"album"`
	Disc   int    `json:"disc"`
	// DiscTotal and TrackTotal are written as "n/total" tags; 0 when untagged.
	DiscTotal  int    `json:"discTotal,omitempty"`
	Track      int    `json:"track"`
	TrackTotal int    `json:"trackTotal,omitempty"`
	Title      string `json:"title"`
	Tagged     bool   `json:"tagged"`
	// CoverSource is "embedded", "folder" or "none"; CoverFile is the folder image or, for
	// an embedded cover, the track file itself.
	CoverSource string `json:"coverSource"`
	CoverFile   string `json:"coverFile,omitempty"`
}

// expectedLibrary is one library root below the -library directory.
type expectedLibrary struct {
	Kind string `json:"kind"`
	// Path is the library root relative to the -library directory; every other path in the
	// catalog is relative to this root, with forward slashes and in the raw (unnormalised)
	// form the file system holds.
	Path  string         `json:"path"`
	Items []expectedItem `json:"items"`
}

// expectedItem is one top-level item (movie or show) a scan must produce.
type expectedItem struct {
	Kind string `json:"kind"`
	// Title is the expected catalog title after the metadata merge (NFO before file name),
	// in Unicode NFC.
	Title string `json:"title"`
	Year  int    `json:"year,omitempty"`
	// Path is the movie or show folder, the file of a loose movie, or "" for a show whose
	// files lie directly in the library root.
	Path     string            `json:"path"`
	Versions []expectedVersion `json:"versions,omitempty"`
	Extras   []expectedExtra   `json:"extras,omitempty"`
	// Ignored are files in the item's folder the scan must not list (samples).
	Ignored []string          `json:"ignored,omitempty"`
	NFO     *expectedNFO      `json:"nfo,omitempty"`
	Artwork []expectedArtwork `json:"artwork,omitempty"`
	Seasons []expectedSeason  `json:"seasons,omitempty"`
}

// expectedVersion is one version of a movie; Parts lists its files in part order.
type expectedVersion struct {
	Label string   `json:"label"`
	Parts []string `json:"parts"`
}

// expectedExtra is one extra attached to its item.
type expectedExtra struct {
	Type string `json:"type"`
	File string `json:"file"`
}

// expectedNFO is an NFO file and the fields a reader must take from it.
type expectedNFO struct {
	File   string `json:"file"`
	Format string `json:"format"`
	// Encoding is the declared character encoding of XML files.
	Encoding string `json:"encoding,omitempty"`
	// Fields are empty for a malformed file: nothing may be read from it.
	Fields nfoFields `json:"fields"`
}

// nfoFields are the values an NFO provides, in Unicode NFC.
type nfoFields struct {
	Title         string   `json:"title,omitempty"`
	OriginalTitle string   `json:"originalTitle,omitempty"`
	Year          int      `json:"year,omitempty"`
	Plot          string   `json:"plot,omitempty"`
	Aired         string   `json:"aired,omitempty"`
	Season        int      `json:"season,omitempty"`
	Episode       int      `json:"episode,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	// ProviderIDs maps tmdb, imdb and tvdb to their IDs.
	ProviderIDs map[string]string `json:"providerIds,omitempty"`
}

// expectedArtwork is one artwork candidate. File is the image for local artwork and the
// video file that embeds the picture for embedded artwork.
type expectedArtwork struct {
	Kind   string `json:"kind"`
	Source string `json:"source"`
	File   string `json:"file"`
}

// expectedSeason is one season of a show, in ascending number order (specials are 0).
type expectedSeason struct {
	Number int `json:"number"`
	// Path is the season folder, or "" when the episodes lie in the show folder or root.
	Path     string            `json:"path"`
	Artwork  []expectedArtwork `json:"artwork,omitempty"`
	Episodes []expectedEpisode `json:"episodes"`
}

// expectedEpisode is one episode; a multi-episode file sets NumberEnd.
type expectedEpisode struct {
	Number         int               `json:"number"`
	NumberEnd      int               `json:"numberEnd,omitempty"`
	AbsoluteNumber int               `json:"absoluteNumber,omitempty"`
	Files          []string          `json:"files"`
	NFO            *expectedNFO      `json:"nfo,omitempty"`
	Artwork        []expectedArtwork `json:"artwork,omitempty"`
}

// libraryCaseEntry is the build result of one library case and every file it writes.
type libraryCaseEntry struct {
	Name     string      `json:"name"`
	Category string      `json:"category"`
	Status   entryStatus `json:"status"`
	SkipKind skipKind    `json:"skipKind,omitempty"`
	Reason   string      `json:"reason,omitempty"`
	// Files are relative to the -library directory: the built file, its copies and sidecars.
	Files []string `json:"files"`

	// kept labels the summary like manifestEntry.kept.
	kept bool
}

// newLibraryManifest pairs the run's entries with the files of their cases.
func newLibraryManifest(ffmpegVersion string, cases []mediaCase, entries []manifestEntry) libraryManifest {
	filesByName := make(map[string][]string, len(cases))
	for _, c := range cases {
		filesByName[c.Name] = c.files()
	}

	result := libraryManifest{
		SchemaVersion: libraryManifestSchemaVersion, FFmpegVersion: ffmpegVersion,
		Libraries: libraryCatalog(), Music: musicCatalog(), Discs: discCatalog(), Audiobooks: audiobookCatalog(), Photos: photoCatalog(),
		Subtitles: subtitleCatalog(),
		Cases:     make([]libraryCaseEntry, 0, len(entries)),
	}

	for _, entry := range newManifest(ffmpegVersion, entries).Cases {
		result.Cases = append(result.Cases, libraryCaseEntry{
			Name: entry.Name, Category: entry.Category, Status: entry.Status, SkipKind: entry.SkipKind,
			Reason: entry.Reason, Files: filesByName[entry.Name], kept: entry.kept,
		})
	}

	return result
}

// count returns how many cases have the given status.
func (m libraryManifest) count(status entryStatus) int {
	total := 0

	for _, entry := range m.Cases {
		if entry.Status == status {
			total++
		}
	}

	return total
}

// writeLibraryManifest replaces <dir>/manifest.json atomically.
func writeLibraryManifest(dir string, m libraryManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode library manifest: %w", err)
	}

	if err := os.MkdirAll(dir, outputDirPermissions); err != nil {
		return fmt.Errorf("create library directory: %w", err)
	}

	return writeFileAtomically(filepath.Join(dir, manifestFileName), append(data, '\n'))
}

package main

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

const (
	// categoryAudiobooks groups the audiobook cases (cover image and parts) in progress lines
	// and the summary; it is also the case name prefix after "library-".
	categoryAudiobooks = "audiobooks"
	// audiobookCoverCaseName builds the shared 2:3 book cover; the audiobook cover is a
	// poster (task M3-14b), not a square album cover.
	audiobookCoverCaseName = libraryCasePrefix + categoryAudiobooks + "-cover"
)

// expectedAudiobooks is the audiobook library root below the -library directory.
type expectedAudiobooks struct {
	// Path is the library root relative to the -library directory; the paths of its books,
	// parts and chapters are relative to this root.
	Path  string              `json:"path"`
	Books []expectedAudiobook `json:"books"`
}

// expectedAudiobook is one book a scan must produce (naming.GroupAudiobooks).
type expectedAudiobook struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	// AuthorTag is "album_artist" (written with an equal artist tag) or "artist".
	AuthorTag string `json:"authorTag"`
	// Composer is the raw composer tag of every part. Narrator is the narrator a scan must
	// produce, written as an explicit narrator tag (ID3 TXXX); a composer alone is no
	// narrator (media.AudioTags.ComposerIsNarrator). Both are omitted when empty.
	Composer string `json:"composer,omitempty"`
	Narrator string `json:"narrator,omitempty"`
	// Path is the folder of the book; StandsAlone marks a book that is one file sharing its
	// folder with other books (an M4B), so only images named after it are its cover.
	Path        string `json:"path"`
	StandsAlone bool   `json:"standsAlone,omitempty"`
	Container   string `json:"container"`
	// CoverSource is "embedded", "folder" (an image file, also for a stand-alone book) or
	// "none"; CoverFile is the image file, or the first part for an embedded cover.
	CoverSource string                  `json:"coverSource"`
	CoverFile   string                  `json:"coverFile,omitempty"`
	Parts       []expectedAudiobookPart `json:"parts"`
	Chapters    []expectedChapter       `json:"chapters"`
}

// expectedAudiobookPart is one part file in play order. Disc and track numbers are written
// as "n/total" tags and omitted when the part has none.
type expectedAudiobookPart struct {
	File       string `json:"file"`
	Title      string `json:"title"`
	Disc       int    `json:"disc,omitempty"`
	DiscTotal  int    `json:"discTotal,omitempty"`
	Track      int    `json:"track,omitempty"`
	TrackTotal int    `json:"trackTotal,omitempty"`
	// DurationSeconds is the nominal length; the encoded file may differ by encoder padding.
	DurationSeconds float64 `json:"durationSeconds"`
}

// expectedChapter is one chapter of a book in play order, with offsets relative to its file.
// Embedded chapters are in the file; the others are the one chapter a scan derives over a
// whole part without chapters (title tag, end at the probed length).
type expectedChapter struct {
	File         string  `json:"file"`
	Title        string  `json:"title"`
	StartSeconds float64 `json:"startSeconds"`
	EndSeconds   float64 `json:"endSeconds"`
	Embedded     bool    `json:"embedded"`
}

// audiobookFormatOf returns the format of an audiobook container; the ipod muxer writes the
// MP4 flavour Apple uses for .m4b.
func audiobookFormatOf(container string) musicFormat {
	if container == audiobookContainerM4B {
		return musicFormat{muxer: "ipod", encoder: "aac", codec: "aac", probeFormat: probeISOBMFF}
	}

	return musicFormatOf(musicContainerMP3)
}

// libraryAudiobookCases returns the cover image case and one case per part file. The cover image is
// the first image cover of the catalog; the other image covers are byte copies and embedded
// covers take it as input in the second phase of generateLibrary.
func libraryAudiobookCases() []mediaCase {
	catalog := audiobookCatalog()

	var (
		cases  []mediaCase
		covers []string
	)

	for _, book := range catalog.Books {
		if book.CoverSource == coverSourceFolder {
			covers = append(covers, path.Join(catalog.Path, book.CoverFile))
		}
	}

	cover := ""

	if len(covers) > 0 {
		cover = covers[0]
		shape := imageShape{name: categoryAudiobooks, width: posterWidth, height: posterHeight, extension: extensionJPEG}
		image := imageCase(shape, cover, covers[1:])
		image.Name, image.Category = audiobookCoverCaseName, categoryAudiobooks
		cases = append(cases, image)
	}

	for _, spec := range audiobookSpecs() {
		for _, part := range spec.parts {
			cases = append(cases, audiobookPartCase(catalog.Path, spec.book, part, cover))
		}
	}

	return cases
}

// audiobookPartCase builds one stereo sine part carrying the book's tags and, for a file
// with chapters, its chapter list.
func audiobookPartCase(root string, book expectedAudiobook, part audiobookPartSpec, cover string) mediaCase {
	format := audiobookFormatOf(book.Container)
	expected := expect(book.Container, format.probeFormat, nil, audioOf(format.codec))

	c := mediaCase{
		Name:     libraryCasePrefix + categoryAudiobooks + "-" + slug(strings.TrimSuffix(part.file, path.Ext(part.file))),
		Category: categoryAudiobooks, Output: path.Join(root, part.file),
		Source: sourceSpec{SampleRate: cdSampleRate, Channels: stereoChannels, Duration: part.length()},
		Muxer:  format.muxer, Encoders: []string{format.encoder},
		Args: slices.Concat(audioArgs(format.encoder), metadataArgs(audiobookPartTags(book, part))),
	}

	if len(part.chapters) > 0 {
		c.Inputs = []textInput{{
			FileName: "chapters.ffmeta", Format: "ffmetadata", Role: roleChapters,
			Content: formatFFMetadata(part.chapters),
		}}

		for _, entry := range part.chapters {
			expected.Chapters = append(expected.Chapters, chapterExpectation{
				Title: entry.Title, StartSeconds: entry.Start.Seconds(), EndSeconds: entry.End.Seconds(),
			})
		}
	}

	c.Expect = expected

	if book.CoverSource == coverSourceEmbedded {
		c.Cover = cover
	}

	return c
}

// audiobookPartTags are the FFmpeg metadata keys of a part; FFmpeg maps them to MP4 atoms and
// ID3 frames (an unknown key such as "narrator" becomes a TXXX frame in ID3 only).
func audiobookPartTags(book expectedAudiobook, part audiobookPartSpec) map[string]string {
	tags := map[string]string{"title": part.title, "album": book.Title, "artist": book.Author}

	if book.AuthorTag == authorTagAlbumArtist {
		tags["album_artist"] = book.Author
	}

	if book.Composer != "" {
		tags[composerTag] = book.Composer
	}

	if book.Narrator != "" {
		tags[narratorTag] = book.Narrator
	}

	if part.track > 0 {
		tags["track"] = fmt.Sprintf("%d/%d", part.track, part.trackTotal)
	}

	if part.disc > 0 {
		tags["disc"] = fmt.Sprintf("%d/%d", part.disc, part.discTotal)
	}

	return tags
}

// writeAudiobookSummary adds one line per book, e.g.
// "book Ada Inkwell - The Clockwork Orchard: 1 m4b part, 3 chapters, stands alone, composer
// Nora Vale, cover embedded".
func writeAudiobookSummary(text *strings.Builder, audiobooks expectedAudiobooks) {
	if len(audiobooks.Books) == 0 {
		return
	}

	fmt.Fprintf(text, "  %s/ (%d books)\n", audiobooks.Path, len(audiobooks.Books))

	for _, book := range audiobooks.Books {
		details := fmt.Sprintf("%d %s %s", len(book.Parts), book.Container, plural(len(book.Parts), "part"))

		if embedded := slices.ContainsFunc(book.Chapters, func(entry expectedChapter) bool { return entry.Embedded }); embedded {
			details += fmt.Sprintf(", %d chapters", len(book.Chapters))
		}

		if book.StandsAlone {
			details += ", stands alone"
		}

		if book.Composer != "" {
			details += ", composer " + book.Composer
		}

		if book.Narrator != "" {
			details += ", narrator " + book.Narrator
		}

		fmt.Fprintf(text, "    book %s - %s: %s, cover %s\n", book.Author, book.Title, details, book.CoverSource)
	}
}

// plural returns noun with an "s" unless count is one.
func plural(count int, noun string) string {
	if count == 1 {
		return noun
	}

	return noun + "s"
}

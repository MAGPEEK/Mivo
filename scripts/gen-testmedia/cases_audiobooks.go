package main

import (
	"path"
	"time"
)

// The audiobook tree (task M3-14f) is declared as its expected catalog like the music tree:
// library_audiobooks.go derives one FFmpeg case per part file from it, so the manifest and
// the files cannot diverge. Authors, narrators and titles are invented; every file is a
// sine tone.

// audiobooksLibraryPath is the audiobook library root below the -library directory.
const audiobooksLibraryPath = "audiobooks"

// Short container names of the audiobook files; each is also the file extension.
const (
	audiobookContainerM4B = "m4b"
	audiobookContainerMP3 = "mp3"
)

// Tags that carry the author: the album artist (written together with the artist) or the
// artist alone. naming.AudiobookFile takes the album artist, else the artist.
const (
	authorTagAlbumArtist = "album_artist"
	authorTagArtist      = "artist"
)

// FFmpeg metadata keys of the people besides the author. MP4 has no narrator atom (the ipod
// muxer drops the key), so only the MP3 book carries an explicit narrator, as an ID3 TXXX
// "narrator" frame. The M4B books carry a composer, which the tag reader takes as the
// narrator only when it is labelled so (narratedByLabel, media.AudioTags.ComposerIsNarrator,
// task M3-14g): the labelled one expects its narrator, the plain one none.
const (
	composerTag = "composer"
	narratorTag = "narrator"
	// narratedByLabel starts a composer tag that names the narrator.
	narratedByLabel = "Narrated by "
)

const (
	// audiobookChapterLength is the length of every chapter of the M4B with chapters; the
	// part files and the other books last clipDuration.
	audiobookChapterLength = 2 * time.Second
	// secondChapterStart and thirdChapterStart split the chaptered M4B; it ends at
	// chapteredBookLength.
	secondChapterStart  = audiobookChapterLength
	thirdChapterStart   = 2 * audiobookChapterLength
	chapteredBookLength = 3 * audiobookChapterLength
	// lighthouseDiscs is the disc count of the folder book; firstDiscParts the number of
	// its parts on the first disc.
	lighthouseDiscs = 2
	firstDiscParts  = 2
)

// audiobookPartSpec is one part file of a book before its seconds are written out.
type audiobookPartSpec struct {
	file       string
	title      string
	disc       int
	discTotal  int
	track      int
	trackTotal int
	// chapters are the chapters the file carries; none means a scan derives one chapter
	// over the whole file from its title tag.
	chapters []chapter
	duration time.Duration
}

// audiobookSpec is one book before its parts and chapters are written out.
type audiobookSpec struct {
	book  expectedAudiobook
	parts []audiobookPartSpec
}

// audiobookSpecs returns the books of the tree in catalog order.
func audiobookSpecs() []audiobookSpec {
	return []audiobookSpec{clockworkOrchard(), paperLighthouse(), gardenNotes(), midnightRecipes()}
}

// audiobookCatalog returns the expected catalog of the generated audiobook tree.
func audiobookCatalog() expectedAudiobooks {
	specs := audiobookSpecs()
	books := make([]expectedAudiobook, 0, len(specs))

	for _, spec := range specs {
		books = append(books, spec.expected())
	}

	return expectedAudiobooks{Path: audiobooksLibraryPath, Books: books}
}

// clockworkOrchard is a single-file M4B with three chapters and an embedded cover; its
// composer tag names the narrator with a label, the only way an M4B can.
func clockworkOrchard() audiobookSpec {
	const (
		title = "The Clockwork Orchard"
		file  = "Ada Inkwell/" + title + ".m4b"
	)

	chapters := []chapter{
		{Title: "The Seed Vault", Start: 0, End: secondChapterStart},
		{Title: "Brass Branches", Start: secondChapterStart, End: thirdChapterStart},
		{Title: "Harvest Moon", Start: thirdChapterStart, End: chapteredBookLength},
	}

	return audiobookSpec{
		book: expectedAudiobook{
			Title: title, Author: "Ada Inkwell", AuthorTag: authorTagAlbumArtist,
			Composer: narratedByLabel + "Nora Vale", Narrator: "Nora Vale", Path: "Ada Inkwell", StandsAlone: true, Container: audiobookContainerM4B,
			CoverSource: coverSourceEmbedded, CoverFile: file,
		},
		parts: []audiobookPartSpec{{
			file: file, title: title, chapters: chapters,
			duration: chapteredBookLength,
		}},
	}
}

// paperLighthouse is a folder book of three MP3 parts on two discs with cover.jpg in its
// folder and an explicit narrator tag. The file names sort "Dawn, Keeper, Storm", but the
// disc and track tags say "Storm, Keeper, Dawn", and track 1 occurs on both discs, so only
// ordering by (disc, track) gives the play order.
func paperLighthouse() audiobookSpec {
	const folder = "Bram Quill/The Paper Lighthouse"

	return audiobookSpec{
		book: expectedAudiobook{
			Title: "The Paper Lighthouse", Author: "Bram Quill", AuthorTag: authorTagAlbumArtist,
			Narrator: "Iris Fenn", Path: folder, Container: audiobookContainerMP3,
			CoverSource: coverSourceFolder, CoverFile: path.Join(folder, folderCoverName),
		},
		parts: []audiobookPartSpec{
			{file: folder + "/Storm.mp3", title: "Storm", disc: firstDisc, discTotal: lighthouseDiscs, track: 1, trackTotal: firstDiscParts},
			{file: folder + "/Keeper.mp3", title: "Keeper", disc: firstDisc, discTotal: lighthouseDiscs, track: firstDiscParts, trackTotal: firstDiscParts},
			{file: folder + "/Dawn.mp3", title: "Dawn", disc: secondDisc, discTotal: lighthouseDiscs, track: 1, trackTotal: 1},
		},
	}
}

// gardenNotes is a single MP3 book in the shared folder "Short Reads": the M4B next to it
// is a book of its own, so the MP3 is the folder's book. Only the artist tag names the
// author; it has no narrator, no disc or track tag and no cover.
func gardenNotes() audiobookSpec {
	const title = "Garden Notes"

	return audiobookSpec{
		book: expectedAudiobook{
			Title: title, Author: "Cleo Marsh", AuthorTag: authorTagArtist,
			Path: sharedAudiobookFolder, Container: audiobookContainerMP3, CoverSource: coverSourceNone,
		},
		parts: []audiobookPartSpec{{file: sharedAudiobookFolder + "/" + title + ".mp3", title: title}},
	}
}

// midnightRecipes is an M4B without chapters in the shared folder; it stands alone, so its
// cover is the image named after the file, not a folder image. Its composer has no
// narrator label, so it names no narrator.
func midnightRecipes() audiobookSpec {
	const title = "Midnight Recipes"

	return audiobookSpec{
		book: expectedAudiobook{
			Title: title, Author: "Cleo Marsh", AuthorTag: authorTagAlbumArtist,
			Composer: "Otto Brine", Path: sharedAudiobookFolder, StandsAlone: true, Container: audiobookContainerM4B,
			CoverSource: coverSourceFolder, CoverFile: sharedAudiobookFolder + "/" + title + ".jpg",
		},
		parts: []audiobookPartSpec{{file: sharedAudiobookFolder + "/" + title + ".m4b", title: title}},
	}
}

// sharedAudiobookFolder holds an MP3 book and an M4B book side by side.
const sharedAudiobookFolder = "Short Reads"

// expected writes out the parts and chapters of the book: the file's own chapters, else one
// chapter over the whole part titled like the part.
func (spec audiobookSpec) expected() expectedAudiobook {
	book := spec.book

	for _, part := range spec.parts {
		duration := part.length()
		book.Parts = append(book.Parts, expectedAudiobookPart{
			File: part.file, Title: part.title, Disc: part.disc, DiscTotal: part.discTotal,
			Track: part.track, TrackTotal: part.trackTotal, DurationSeconds: duration.Seconds(),
		})

		chapters := part.chapters
		embedded := len(chapters) > 0

		if !embedded {
			chapters = []chapter{{Title: part.title, End: duration}}
		}

		for _, entry := range chapters {
			book.Chapters = append(book.Chapters, expectedChapter{
				File: part.file, Title: entry.Title, Embedded: embedded,
				StartSeconds: entry.Start.Seconds(), EndSeconds: entry.End.Seconds(),
			})
		}
	}

	return book
}

// length is the nominal length of the part file.
func (part audiobookPartSpec) length() time.Duration {
	if part.duration > 0 {
		return part.duration
	}

	return clipDuration
}

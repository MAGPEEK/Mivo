package naming

import (
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Parts of the audiobook match key. The prefixes keep the keys of the two book shapes apart:
// without them a folder book titled "x.m4b" would share the key of the M4B "x.m4b" beside it.
const (
	matchKeySeparator   = "/"
	standAloneKeyPrefix = "file:"
	folderBookKeyPrefix = "folder:"
)

// NewAudiobookFile is the grouping input of the audio file at the library-relative path
// with its tags. The author is the album artist, else the artist.
func NewAudiobookFile(relativePath string, tags media.AudioTags) AudiobookFile {
	author := strings.TrimSpace(tags.AlbumArtist)
	if author == "" {
		author = strings.TrimSpace(tags.Artist)
	}

	return AudiobookFile{
		Path: relativePath, Author: author, Album: tags.Album,
		Composer: tags.Composer, Narrator: tags.Narrator, ComposerIsNarrator: tags.ComposerIsNarrator,
		Disc: tags.Disc, Track: tags.Track,
	}
}

// AudiobookStandsAlone reports whether the file at the library-relative path is a book of
// its own whatever its folder holds: an M4B file or a file in the library root. Its
// artwork comes from images named after it, not from the folder it shares.
func AudiobookStandsAlone(relativePath string) bool {
	return folderKey(AudiobookFile{Path: relativePath}) == relativePath
}

// MatchKey is the key of the book among the books of its library (ADR 0027 §3): the
// lower-cased path of a book that stands alone ("file:" prefix), else the lower-cased folder
// and title ("folder:" prefix), so two albums in one folder stay two books and a retitled
// folder book becomes a new one.
func (book Audiobook) MatchKey() string {
	if len(book.Parts) == 1 && AudiobookStandsAlone(book.Parts[0].Path) {
		return standAloneKeyPrefix + strings.ToLower(book.Parts[0].Path)
	}

	return folderBookKeyPrefix + strings.ToLower(book.Folder+matchKeySeparator+strings.TrimSpace(book.Title))
}

// PartPlacement is the placement of the part at index (zero-based, play order): one
// audiobook level and the part number index + 1, which orders the files of the book.
func (book Audiobook) PartPlacement(index int) library.Placement {
	title := strings.TrimSpace(book.Title)
	if title == "" {
		title = path.Base(book.Parts[index].Path)
	}

	return library.Placement{
		Levels:     []library.Level{{Kind: library.ItemKindAudiobook, MatchKey: book.MatchKey(), Title: title}},
		PartNumber: library.NumberOf(index + 1),
	}
}

// firstPartValue returns the first value of the parts that is not blank, trimmed.
func firstPartValue(parts []AudiobookFile, valueOf func(AudiobookFile) string) string {
	for _, part := range parts {
		if value := strings.TrimSpace(valueOf(part)); value != "" {
			return value
		}
	}

	return ""
}

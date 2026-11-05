package naming

import (
	"path"
	"sort"
	"strings"
	"unicode"

	"github.com/magpeek/mivo/internal/domain/library"
)

// extensionM4B is the one container that is always a whole book by itself, even in a
// folder shared with other books.
const extensionM4B = ".m4b"

// sortKeySeparator joins title and path in the sort key of the result; NUL cannot occur in
// either and sorts before every other character, so the title decides first.
const sortKeySeparator = "\x00"

// AudiobookFile is one audio file with the tags the grouping needs. Path is slash-separated
// and relative to the library root. Disc and Track are zero when the tag is missing.
// ComposerIsNarrator is set by the tag reader only when the file says that the composer
// field holds the narrator (for example a "Narrated by" label); a composer is otherwise a
// composer. Author is the author tag of the file (album artist, else artist).
type AudiobookFile struct {
	Path               string
	Author             string
	Album              string
	Composer           string
	Narrator           string
	Disc               int
	Track              int
	ComposerIsNarrator bool
}

// Audiobook is one book: its title, folder, author, narrator and its part files in play
// order.
type Audiobook struct {
	Title    string
	Folder   string
	Author   string
	Narrator string
	Parts    []AudiobookFile
}

// IsAudiobookFileName reports whether the file name has an audio extension a book can use;
// the set is the shared audio set of music libraries (library.IsAudioFileName).
func IsAudiobookFileName(name string) bool {
	return library.IsAudioFileName(name)
}

// GroupAudiobooks groups audio files into books, ignoring files without an audio extension.
// A folder is one book, except that a file in the library root and an M4B file are each a
// book of their own; M4A and MP3 files join the folder's book. Inside a folder, files with
// different album tags form separate books; untagged files join the book of the first
// album (in natural path order). Parts are ordered by (disc, track) when every part has a
// track number, else by natural file-name order. The result is sorted by folder and title
// so that rescans are stable.
func GroupAudiobooks(files []AudiobookFile) []Audiobook {
	groups := make(map[string][]AudiobookFile)

	for _, file := range files {
		if !IsAudiobookFileName(file.Path) {
			continue
		}

		key := folderKey(file)
		groups[key] = append(groups[key], file)
	}

	books := make([]Audiobook, 0, len(groups))

	for _, group := range groups {
		for _, parts := range splitByAlbum(group) {
			books = append(books, buildAudiobook(parts))
		}
	}

	sort.Slice(books, func(i, j int) bool {
		if books[i].Folder != books[j].Folder {
			return books[i].Folder < books[j].Folder
		}

		return naturalLess(books[i].Title+sortKeySeparator+books[i].Parts[0].Path,
			books[j].Title+sortKeySeparator+books[j].Parts[0].Path)
	})

	return books
}

func folderKey(file AudiobookFile) string {
	folder := path.Dir(file.Path)
	if folder == "." || strings.EqualFold(path.Ext(file.Path), extensionM4B) {
		return file.Path
	}

	return folder
}

// splitByAlbum separates the files of one folder by distinct album tag. Untagged files
// follow the first album because a partly tagged folder is almost always one book.
func splitByAlbum(group []AudiobookFile) [][]AudiobookFile {
	ordered := append([]AudiobookFile(nil), group...)
	sort.SliceStable(ordered, func(i, j int) bool { return naturalLess(ordered[i].Path, ordered[j].Path) })

	var (
		books    [][]AudiobookFile
		untagged []AudiobookFile
	)

	indexByAlbum := make(map[string]int)

	for _, file := range ordered {
		album := strings.ToLower(strings.TrimSpace(file.Album))
		if album == "" {
			untagged = append(untagged, file)

			continue
		}

		index, known := indexByAlbum[album]
		if !known {
			index = len(books)
			indexByAlbum[album] = index
			books = append(books, nil)
		}

		books[index] = append(books[index], file)
	}

	if len(books) == 0 {
		return [][]AudiobookFile{untagged}
	}

	books[0] = append(books[0], untagged...)

	return books
}

func buildAudiobook(parts []AudiobookFile) Audiobook {
	orderParts(parts)

	folder := path.Dir(parts[0].Path)
	if folder == "." {
		folder = ""
	}

	return Audiobook{
		Title:    audiobookTitle(parts, folder),
		Folder:   folder,
		Author:   firstPartValue(parts, func(part AudiobookFile) string { return part.Author }),
		Narrator: audiobookNarrator(parts),
		Parts:    parts,
	}
}

// audiobookTitle prefers the album tag, then the folder name; a single file outside a
// folder book (library root or M4B in a shared folder) falls back to its file name.
func audiobookTitle(parts []AudiobookFile, folder string) string {
	for _, part := range parts {
		if album := strings.TrimSpace(part.Album); album != "" {
			return album
		}
	}

	if len(parts) == 1 && (folder == "" || strings.EqualFold(path.Ext(parts[0].Path), extensionM4B)) {
		return fileStem(parts[0].Path)
	}

	return path.Base(folder)
}

func fileStem(name string) string {
	base := path.Base(name)

	return strings.TrimSuffix(base, path.Ext(base))
}

// audiobookNarrator returns the first explicit narrator, else the first composer whose tag
// says it is the narrator.
func audiobookNarrator(parts []AudiobookFile) string {
	if narrator := firstPartValue(parts, func(part AudiobookFile) string { return part.Narrator }); narrator != "" {
		return narrator
	}

	for _, part := range parts {
		if composer := strings.TrimSpace(part.Composer); composer != "" && part.ComposerIsNarrator {
			return composer
		}
	}

	return ""
}

func orderParts(parts []AudiobookFile) {
	byTrack := true

	for _, part := range parts {
		if part.Track <= 0 {
			byTrack = false

			break
		}
	}

	sort.SliceStable(parts, func(i, j int) bool {
		if byTrack && (parts[i].Disc != parts[j].Disc || parts[i].Track != parts[j].Track) {
			if parts[i].Disc != parts[j].Disc {
				return parts[i].Disc < parts[j].Disc
			}

			return parts[i].Track < parts[j].Track
		}

		return naturalLess(parts[i].Path, parts[j].Path)
	})
}

// naturalLess compares case-insensitively and treats digit runs as numbers, so "2" sorts
// before "10". Equal-looking names fall back to plain string order for a total order.
func naturalLess(left, right string) bool {
	leftRunes, rightRunes := []rune(strings.ToLower(left)), []rune(strings.ToLower(right))

	for len(leftRunes) > 0 && len(rightRunes) > 0 {
		if unicode.IsDigit(leftRunes[0]) && unicode.IsDigit(rightRunes[0]) {
			leftNumber, leftRest := digitRun(leftRunes)
			rightNumber, rightRest := digitRun(rightRunes)

			if leftNumber != rightNumber {
				return numberLess(leftNumber, rightNumber)
			}

			leftRunes, rightRunes = leftRest, rightRest

			continue
		}

		if leftRunes[0] != rightRunes[0] {
			return leftRunes[0] < rightRunes[0]
		}

		leftRunes, rightRunes = leftRunes[1:], rightRunes[1:]
	}

	if len(leftRunes) != len(rightRunes) {
		return len(leftRunes) < len(rightRunes)
	}

	return left < right
}

// digitRun splits a leading run of digits off and returns it without leading zeros.
func digitRun(runes []rune) (digits string, rest []rune) {
	end := 0
	for end < len(runes) && unicode.IsDigit(runes[end]) {
		end++
	}

	return strings.TrimLeft(string(runes[:end]), "0"), runes[end:]
}

// numberLess compares two digit strings without leading zeros as numbers of any length:
// fewer digits is smaller, equal length compares as text.
func numberLess(left, right string) bool {
	if len(left) != len(right) {
		return len(left) < len(right)
	}

	return left < right
}

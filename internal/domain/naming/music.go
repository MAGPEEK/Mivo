package naming

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Names used when neither tags nor folders say who or what an album is. They are catalog
// data, not UI text, like the other fallbacks of the name parser.
const (
	unknownArtist = "Unknown Artist"
	unknownAlbum  = "Unknown Album"
)

var (
	// "01 Title", "01 - Title", "01. Title", "01_Title".
	trackFilePattern = regexp.MustCompile(`^(\d{1,3})(?:\s*[-._]\s*|\s+)(\S.*)$`)
	// "1-05 Title": disc and track number.
	discTrackFilePattern = regexp.MustCompile(`^(\d{1,2})-(\d{1,3})(?:\s*[-._]\s*|\s+)(\S.*)$`)
	// "CD 2", "Disc2", "disk-3".
	discFolderPattern = regexp.MustCompile(`(?i)^(?:cd|disc|disk)\s*[-_.]?\s*(\d{1,2})$`)
)

// musicFolderInfo is what the layout "Artist/Album/[CD n/]NN Title.ext" says.
type musicFolderInfo struct {
	artist string
	album  string
	year   library.OptionalNumber
	disc   int
	track  library.OptionalNumber
	title  string
}

// PlaceMusic decides where one audio file belongs: artist, album and track levels. Tags win
// over the folder layout field by field, so a file with only a title tag still gets its
// artist and album from the folders. A zero AudioTags value means "no tags". The error wraps
// domain.ErrInvalid for an invalid path.
func PlaceMusic(filePath string, tags media.AudioTags) (library.Placement, error) {
	if _, err := library.ParseRelativePath(filePath); err != nil {
		return library.Placement{}, fmt.Errorf("place music: %w", err)
	}

	folder := parseMusicPath(filePath)
	fromTags := tags.Placement()

	artist := firstNonBlank(fromTags.ArtistName, folder.artist, unknownArtist)
	album := firstNonBlank(fromTags.AlbumTitle, folder.album, unknownAlbum)
	title := firstNonBlank(fromTags.TrackTitle, folder.title)
	disc := folder.disc

	if tags.Disc > 0 || disc == 0 {
		disc = fromTags.Disc
	}

	year := pickNumber(fromTags.Year, folder.year)
	track := pickNumber(fromTags.Track, folder.track)

	placement := library.Placement{Levels: []library.Level{
		{Kind: library.ItemKindArtist, MatchKey: library.ArtistKey(artist), Title: artist},
		{Kind: library.ItemKindAlbum, MatchKey: library.AlbumKey(artist, album, year), Title: album, Year: year},
		{Kind: library.ItemKindTrack, MatchKey: library.TrackKey(disc, track, title), Title: title, Number: track},
	}}

	if err := placement.Validate(); err != nil {
		return library.Placement{}, fmt.Errorf("place music %q: %w", filePath, err)
	}

	return placement, nil
}

// parseMusicPath reads the folder layout. The last folder is the album (or the disc folder,
// then the one above), the folder above the album is the artist; deeper nesting such as
// "Genre/Artist/Album" keeps the two nearest folders.
func parseMusicPath(filePath string) musicFolderInfo {
	elements := strings.Split(filePath, "/")
	fileName := elements[len(elements)-1]
	folders := elements[:len(elements)-1]

	info := parseMusicFileName(fileName)

	if len(folders) > 0 {
		if disc, ok := discNumberOf(folders[len(folders)-1]); ok {
			info.disc = disc
			folders = folders[:len(folders)-1]
		}
	}

	if len(folders) > 0 {
		parsed := ParseTitle(folders[len(folders)-1])
		info.album = parsed.Title
		info.year = parsed.Year
		folders = folders[:len(folders)-1]
	}

	if len(folders) > 0 {
		info.artist = strings.TrimSpace(folders[len(folders)-1])
	}

	return info
}

// parseMusicFileName splits "NN Title.ext" or "D-NN Title.ext" into numbers and title. A
// name without a leading number is all title.
func parseMusicFileName(fileName string) musicFolderInfo {
	stem := strings.TrimSuffix(fileName, path.Ext(fileName))

	if match := discTrackFilePattern.FindStringSubmatch(stem); match != nil {
		disc, _ := strconv.Atoi(match[1])
		track, _ := strconv.Atoi(match[2])

		return musicFolderInfo{disc: disc, track: library.NumberOf(track), title: cleanTrackTitle(match[3])}
	}

	if match := trackFilePattern.FindStringSubmatch(stem); match != nil {
		track, _ := strconv.Atoi(match[1])

		return musicFolderInfo{track: library.NumberOf(track), title: cleanTrackTitle(match[2])}
	}

	return musicFolderInfo{title: cleanTrackTitle(stem)}
}

func discNumberOf(folder string) (int, bool) {
	match := discFolderPattern.FindStringSubmatch(strings.TrimSpace(folder))
	if match == nil {
		return 0, false
	}

	disc, err := strconv.Atoi(match[1])

	return disc, err == nil && disc > 0
}

// cleanTrackTitle turns underscores into spaces and collapses whitespace; unlike movie
// names, dots stay because they are part of titles ("Mr. Brightside").
func cleanTrackTitle(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(text, "_", " ")), " ")
}

func pickNumber(tagValue int, fallback library.OptionalNumber) library.OptionalNumber {
	if tagValue > 0 {
		return library.NumberOf(tagValue)
	}

	return fallback
}

// firstNonBlank returns the first value with visible text; "" if there is none.
func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	return ""
}

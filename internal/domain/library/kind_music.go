package library

import (
	"strconv"
	"strings"
)

// KindMusic is a library of artists, albums and tracks (ADR 0027 §3). It is declared here, in
// its own file, and listed in Kind.IsValid.
const KindMusic Kind = "music"

// Music item kinds. A track owns the audio file; artists and albums only group.
const (
	ItemKindArtist ItemKind = "artist"
	ItemKindAlbum  ItemKind = "album"
	ItemKindTrack  ItemKind = "track"
)

// Separators of music match keys. NormalizeKey never emits "/", so a key cannot be forged by
// a name that contains the separator.
const (
	musicKeySeparator = "/"
	trackKeyPrefix    = "d"
	trackKeyMarker    = "t"
)

// validMusicParent is the hierarchy of ADR 0027 §3: artists are top level, an album belongs
// to an artist and a track to an album. Any other child is not a music kind.
func validMusicParent(child, parent ItemKind) bool {
	switch child {
	case ItemKindArtist:
		return parent == noParent
	case ItemKindAlbum:
		return parent == ItemKindArtist
	case ItemKindTrack:
		return parent == ItemKindAlbum
	case ItemKindMovie, ItemKindShow, ItemKindSeason, ItemKindEpisode, ItemKindExtra,
		ItemKindAudiobook, ItemKindPhotoAlbum, ItemKindPhoto:
		return false
	default:
		return false
	}
}

// ArtistKey is the match key of an artist among its siblings: the normalised name. A name
// without any letter or digit ("!!!", "+/-") keeps its trimmed, lowercased text, because an
// empty key would be rejected and merge such artists.
func ArtistKey(name string) string {
	if key := NormalizeKey(name); key != "" {
		return key
	}

	return strings.ToLower(strings.TrimSpace(name))
}

// AlbumKey is the match key of an album: artist key, normalised album title and the year
// ("" when unknown). The disc number is deliberately not part of it, so a multi-disc album is
// one album.
func AlbumKey(artistName, albumTitle string, year OptionalNumber) string {
	return ArtistKey(artistName) + musicKeySeparator + NormalizeKey(albumTitle) +
		musicKeySeparator + year.String()
}

// TrackKey is the match key of a track within its album: disc, track number and the
// normalised title ("d1t5 title"). A missing track number is left out so the title decides.
func TrackKey(disc int, track OptionalNumber, title string) string {
	key := trackKeyPrefix + strconv.Itoa(disc)
	if number, ok := track.Get(); ok {
		key += trackKeyMarker + strconv.Itoa(number)
	}

	return key + " " + NormalizeKey(title)
}

// TrackOrder is what orders the tracks of an album: disc, track number and file name.
type TrackOrder struct {
	Disc     int
	Track    OptionalNumber
	FileName string
}

// CompareTracks orders by disc, then track number (a track without number sorts after
// numbered ones), then case-folded file name. It returns a negative, zero or positive number.
func CompareTracks(left, right TrackOrder) int {
	if left.Disc != right.Disc {
		return compareInts(left.Disc, right.Disc)
	}

	leftNumber, leftHas := left.Track.Get()
	rightNumber, rightHas := right.Track.Get()

	switch {
	case leftHas && rightHas && leftNumber != rightNumber:
		return compareInts(leftNumber, rightNumber)
	case leftHas && !rightHas:
		return -1
	case !leftHas && rightHas:
		return 1
	}

	return compareText(SortTitle(left.FileName), SortTitle(right.FileName))
}

func compareInts(left, right int) int {
	if left < right {
		return -1
	}

	return 1
}

func compareText(left, right string) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

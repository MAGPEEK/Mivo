package media

import "strings"

// VariousArtists is the album artist that groups compilations into one album.
const VariousArtists = "Various Artists"

// firstDisc is the disc number of an album whose files carry no disc tag.
const firstDisc = 1

// AudioTags are the tags of one audio file as read by an adapter. Empty text and zero
// numbers mean the tag is absent. Compilation is the iTunes/ID3 compilation flag ("1").
// Composer, Narrator and ComposerIsNarrator serve audiobooks: Narrator is an explicit
// narrator tag, and ComposerIsNarrator is set only when the file says that its composer
// field holds the narrator (see naming.AudiobookFile).
type AudioTags struct {
	AlbumArtist         string
	Artist              string
	Album               string
	Title               string
	Genre               string
	Disc                int
	Track               int
	Year                int
	Compilation         bool
	MusicBrainzArtistID string
	MusicBrainzAlbumID  string
	MusicBrainzTrackID  string
	Composer            string
	Narrator            string
	ComposerIsNarrator  bool
}

// AudioPlacement is what the tags alone say about where a track belongs. Fields the tags
// do not carry are empty (or zero), so a folder fallback can fill them field by field.
type AudioPlacement struct {
	// ArtistName is the artist the album is filed under: the album artist, else the artist;
	// VariousArtists for a compilation without an album artist.
	ArtistName string
	// TrackArtist is the artist of the track itself (the artist tag, else ArtistName).
	TrackArtist string
	AlbumTitle  string
	TrackTitle  string
	Disc        int
	Track       int
	Year        int
}

// Placement derives the placement from the tags. All "Various Artists" spellings and
// compilation=1 end up under the single artist VariousArtists, so a compilation stays one
// album instead of splitting per track artist. A missing disc is disc 1.
func (tags AudioTags) Placement() AudioPlacement {
	artist := strings.TrimSpace(tags.Artist)
	albumArtist := strings.TrimSpace(tags.AlbumArtist)

	if albumArtist == "" {
		albumArtist = artist
	}

	if IsVariousArtists(albumArtist) || (tags.Compilation && strings.TrimSpace(tags.AlbumArtist) == "") {
		albumArtist = VariousArtists
	}

	trackArtist := artist
	if trackArtist == "" {
		trackArtist = albumArtist
	}

	disc := tags.Disc
	if disc < firstDisc {
		disc = firstDisc
	}

	return AudioPlacement{
		ArtistName:  albumArtist,
		TrackArtist: trackArtist,
		AlbumTitle:  strings.TrimSpace(tags.Album),
		TrackTitle:  strings.TrimSpace(tags.Title),
		Disc:        disc,
		Track:       max(tags.Track, 0),
		Year:        max(tags.Year, 0),
	}
}

// IsVariousArtists recognises the common spellings of the compilation album artist; the
// name parser also applies it to artist folders, so a "VA" folder groups like the tag does.
func IsVariousArtists(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "various artists", "various", "va", "v.a.", "verschiedene interpreten", "verschiedene":
		return true
	default:
		return false
	}
}

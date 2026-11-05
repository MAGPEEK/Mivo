package ffmpeg

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	artistID = "0383dadf-2a4e-4d10-a46a-e9e041da8eb4"
	albumID  = "f5093c06-23e3-404f-aeaa-40f72885ee3a"
	trackID  = "9a1f2a6e-7f43-4c5f-8f7e-2f1f3c6d8b10"
	// hangingToolTimeout ends a read against the hanging fake ffprobe quickly.
	hangingToolTimeout = 200 * time.Millisecond
)

func TestDescribeAudioTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		output       string
		wantTags     media.AudioTags
		wantDuration time.Duration
	}{
		{
			name: "ID3 and MP4 style lower-case keys with total counts",
			output: `{"streams":[{"index":0,"codec_type":"audio"}],"format":{"duration":"183.500000","tags":{
				"title":"Under Pressure","artist":"Queen feat. Bowie","album_artist":"Queen","album":"Hot Space",
				"genre":"Rock","track":"4/11","disc":"1/2","date":"1982-04-21","compilation":"0"}}}`,
			wantTags: media.AudioTags{
				AlbumArtist: "Queen", Artist: "Queen feat. Bowie", Album: "Hot Space", Title: "Under Pressure",
				Genre: "Rock", Disc: 1, Track: 4, Year: 1982,
			},
			wantDuration: 183500 * time.Millisecond,
		},
		{
			name: "Vorbis comment keys in upper case with MusicBrainz IDs",
			output: `{"streams":[{"index":0,"codec_type":"audio"}],"format":{"duration":"2.000000","tags":{
				"TITLE":"Song","ARTIST":"Someone","ALBUMARTIST":"Various Artists","ALBUM":"Hits",
				"TRACKNUMBER":"07","DISCNUMBER":"2","DATE":"2020","COMPILATION":"1",
				"MUSICBRAINZ_ARTISTID":"` + artistID + `","MUSICBRAINZ_ALBUMID":"` + albumID + `",
				"MUSICBRAINZ_TRACKID":"` + trackID + `"}}}`,
			wantTags: media.AudioTags{
				AlbumArtist: "Various Artists", Artist: "Someone", Album: "Hits", Title: "Song",
				Disc: 2, Track: 7, Year: 2020, Compilation: true,
				MusicBrainzArtistID: artistID, MusicBrainzAlbumID: albumID, MusicBrainzTrackID: trackID,
			},
			wantDuration: 2 * time.Second,
		},
		{
			name: "Picard ID3 and MP4 MusicBrainz keys, several artist IDs, upper-case UUID",
			output: `{"streams":[{"index":0,"codec_type":"audio"}],"format":{"tags":{
				"MusicBrainz Artist Id":"` + artistID + `;` + albumID + `",
				"musicbrainz album id":"F5093C06-23E3-404F-AEAA-40F72885EE3A",
				"MusicBrainz Track Id":"not-a-uuid","year":"1999","compilation":"true"}}}`,
			wantTags: media.AudioTags{
				MusicBrainzArtistID: artistID, MusicBrainzAlbumID: albumID, Year: 1999, Compilation: true,
			},
		},
		{
			name: "Ogg keeps its comments on the audio stream; format tags win",
			output: `{"streams":[{"index":0,"codec_type":"audio","tags":{"title":"Stream Title","track":"4","album":"Stream Album"}}],
				"format":{"duration":"1.006500","tags":{"album":"Format Album"}}}`,
			wantTags:     media.AudioTags{Title: "Stream Title", Album: "Format Album", Track: 4},
			wantDuration: 1006500 * time.Microsecond,
		},
		{
			name:   "untagged file with unknown duration",
			output: `{"streams":[{"index":0,"codec_type":"audio"}],"format":{"duration":"N/A"}}`,
		},
		{
			name: "a container tag wins over a stream tag in another spelling",
			output: `{"streams":[{"index":0,"codec_type":"audio","tags":{"title":"Stream Title","ALBUM":"Stream Album"}}],
				"format":{"tags":{"TITLE":"Container Title","album":"Container Album"}}}`,
			wantTags: media.AudioTags{Title: "Container Title", Album: "Container Album"},
		},
		{
			name:         "raw ADTS stream: the stream duration stands in for the missing format duration",
			output:       `{"streams":[{"index":0,"codec_type":"audio","duration":"2.500000"}],"format":{}}`,
			wantDuration: 2500 * time.Millisecond,
		},
		{
			name: "garbage numbers and placeholders are absent values",
			output: `{"streams":[{"index":1,"codec_type":"video"},{"index":0,"codec_type":"audio"}],"format":{"tags":{
				"track":"x/12","disc":"-1","date":"unknown","title":"N/A","compilation":"no"}}}`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			tags, duration, err := describeAudioTags([]byte(test.output))
			if err != nil {
				t.Fatalf("describeAudioTags: %v", err)
			}

			if tags != test.wantTags {
				t.Errorf("tags = %+v, want %+v", tags, test.wantTags)
			}

			if duration != test.wantDuration {
				t.Errorf("duration = %v, want %v", duration, test.wantDuration)
			}
		})
	}
}

func TestDescribeAudioTagsRejectsUnusableOutput(t *testing.T) {
	t.Parallel()

	for name, output := range map[string]string{
		"not JSON":          `{`,
		"no format section": `{"streams":[{"index":0,"codec_type":"audio"}]}`,
		"no audio stream":   `{"streams":[{"index":0,"codec_type":"video"}],"format":{"duration":"1.0"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, _, err := describeAudioTags([]byte(output)); !errors.Is(err, domain.ErrUnsupported) {
				t.Errorf("error = %v, want domain.ErrUnsupported", err)
			}
		})
	}
}

func TestParseOrdinal(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]int{
		"3": 3, "03": 3, "3/12": 3, " 4 / 11 ": 4, "": 0, "/12": 0, "x": 0, "-2": 0, "2a": 0,
		"99999999999999999999": 0,
	} {
		if got := parseOrdinal(text); got != want {
			t.Errorf("parseOrdinal(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestParseYear(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]int{
		"2001": 2001, "2001-05-03": 2001, "2001-05-03T10:00:00Z": 2001, "20010503": 2001,
		"03.05.2001": 2001, "May 1999": 1999, "": 0, "01": 0, "0000": 0, "abc": 0,
		"12345": 0, "123456789": 0, "12345 2001": 2001,
	} {
		if got := parseYear(text); got != want {
			t.Errorf("parseYear(%q) = %d, want %d", text, got, want)
		}
	}
}

func TestParseFlag(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]bool{
		"1": true, "true": true, "TRUE": true, "yes": true, " 1 ": true,
		"0": false, "": false, "no": false, "false": false, "2": false,
	} {
		if got := parseFlag(text); got != want {
			t.Errorf("parseFlag(%q) = %t, want %t", text, got, want)
		}
	}
}

func TestMusicBrainzID(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]string{
		artistID:                                artistID,
		" " + artistID + " ":                    artistID,
		artistID + "/" + albumID:                artistID,
		"0383DADF-2A4E-4D10-A46A-E9E041DA8EB4":  artistID,
		"0383dadf2a4e4d10a46ae9e041da8eb4":      "",
		"0383dadf-2a4e-4d10-a46a-e9e041da8ebz":  "",
		"0383dadf-2a4e-4d10-a46a-e9e041da8eb4x": "",
		"":                                      "",
	} {
		if got := musicBrainzID(text); got != want {
			t.Errorf("musicBrainzID(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestReadAudioTagsTimesOut is not parallel: the fake tool configures itself through the
// environment.
func TestReadAudioTagsTimesOut(t *testing.T) {
	prober := newTestProber(t, fakeTool(t, fakeModeHang).executable)
	prober.probeTimeout = hangingToolTimeout

	_, _, err := prober.ReadAudioTags(t.Context(), filepath.Join(t.TempDir(), "song.flac"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded", err)
	}
}

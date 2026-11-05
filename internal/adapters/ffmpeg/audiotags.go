package ffmpeg

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// readTagsOperation names the prober's tag runs in debug logs.
	readTagsOperation = "read audio tags"
	// ordinalSeparator splits "track/total" and "disc/total" values.
	ordinalSeparator = "/"
	// yearDigits is the length of a year in a date tag.
	yearDigits = 4
	// uuidGroupSeparator and lowerHexDigits make up a MusicBrainz ID, a lower-case UUID.
	uuidGroupSeparator = "-"
	lowerHexDigits     = "0123456789abcdef"
	// multipleValueSeparators split several values of one tag: FFmpeg joins repeated Vorbis
	// comments with ";", ID3v2.3 separates values with "/".
	multipleValueSeparators = ";/"
)

// Tag keys, each list in priority order. Lookups ignore case, so one entry covers the
// lower-case keys FFmpeg normalises ID3 and MP4 tags to and the upper-case Vorbis comment
// keys it leaves as they are; FFmpeg already maps ALBUMARTIST, TRACKNUMBER and DISCNUMBER
// for Vorbis comments, the extra spellings cover other writers.
func albumArtistKeys() []string { return []string{"album_artist", "albumartist", "album artist"} }
func trackKeys() []string       { return []string{"track", "tracknumber"} }
func discKeys() []string        { return []string{"disc", "discnumber"} }
func yearKeys() []string        { return []string{"date", "year"} }

// MusicBrainz keys: Picard writes "MusicBrainz … Id" to ID3 TXXX frames and MP4 freeform
// atoms and MUSICBRAINZ_… to Vorbis comments. The track ID is the recording ID.
func musicBrainzArtistKeys() []string {
	return []string{"musicbrainz_artistid", "musicbrainz artist id"}
}

func musicBrainzAlbumKeys() []string {
	return []string{"musicbrainz_albumid", "musicbrainz album id"}
}

func musicBrainzTrackKeys() []string {
	return []string{"musicbrainz_trackid", "musicbrainz track id"}
}

// trueFlagValues are the compilation flag values that mean "set".
func trueFlagValues() []string { return []string{"1", "true", "yes"} }

// ReadAudioTags reads the tags and the duration of the audio file at absolutePath. Unlike
// a playback probe it accepts audio-only files: a music file needs no video stream. The
// caller has resolved the path and checked that it lies inside a library root (ADR 0022).
// Absent tags are zero values and an unknown duration is 0. Errors wrap
//   - domain.ErrInvalid for a relative path or a refused Windows device path,
//   - context.DeadlineExceeded after the per-file timeout, context.Canceled on cancellation,
//   - domain.ErrUnavailable when ffprobe cannot be started,
//   - domain.ErrUnsupported when ffprobe failed on the file (including a demuxer outside
//     the input whitelist) or the file has no audio stream.
//
// Messages name the file by its base name only; ffprobe's error output goes to the debug log.
func (p *Prober) ReadAudioTags(ctx context.Context, absolutePath string) (media.AudioTags, time.Duration, error) {
	input, err := inputArguments(absolutePath)
	if err != nil {
		return media.AudioTags{}, 0, fmt.Errorf("read audio tags: %w", err)
	}

	probeContext, cancel := context.WithTimeout(ctx, p.probeTimeout)
	defer cancel()

	output, err := p.ffprobe.output(probeContext, probeArguments(input))
	if err != nil {
		p.logRunFailure(ctx, readTagsOperation, absolutePath, err)

		return media.AudioTags{}, 0, fmt.Errorf("read audio tags %s: %w", filepath.Base(absolutePath), classifyRunError(err))
	}

	tags, duration, err := describeAudioTags(output)
	if err != nil {
		return media.AudioTags{}, 0, fmt.Errorf("read audio tags %s: %w", filepath.Base(absolutePath), err)
	}

	return tags, duration, nil
}

// audioProbeFormat is the format section with the container tags, which probeFormat omits.
type audioProbeFormat struct {
	Duration string            `json:"duration"`
	Tags     map[string]string `json:"tags"`
}

type audioProbeOutput struct {
	Streams []probeStream     `json:"streams"`
	Format  *audioProbeFormat `json:"format"`
}

// describeAudioTags maps ffprobe's -show_format -show_streams JSON to the tags and the
// duration. Errors wrap domain.ErrUnsupported.
func describeAudioTags(output []byte) (media.AudioTags, time.Duration, error) {
	var parsed audioProbeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return media.AudioTags{}, 0, fmt.Errorf("decode ffprobe output: %w: %w", domain.ErrUnsupported, err)
	}

	if parsed.Format == nil {
		return media.AudioTags{}, 0, fmt.Errorf("ffprobe output has no format section: %w", domain.ErrUnsupported)
	}

	audioIndex := slices.IndexFunc(parsed.Streams, func(stream probeStream) bool {
		return streamKind(stream.CodecType) == media.StreamKindAudio
	})
	if audioIndex < 0 {
		return media.AudioTags{}, 0, fmt.Errorf("file has no audio stream: %w", domain.ErrUnsupported)
	}

	duration, _ := parseSeconds(parsed.Format.Duration)
	tags := mergedTags(parsed.Format.Tags, parsed.Streams[audioIndex].Tags)

	return audioTags(tags), max(duration, 0), nil
}

// mergedTags combines the container tags with those of the audio stream. Ogg keeps its
// Vorbis comments on the stream, other containers on the format; a container tag wins
// over a stream tag of the same key.
func mergedTags(formatTags, streamTags map[string]string) map[string]string {
	merged := make(map[string]string, len(formatTags)+len(streamTags))
	maps.Copy(merged, streamTags)
	maps.Copy(merged, formatTags)

	return merged
}

func audioTags(tags map[string]string) media.AudioTags {
	return media.AudioTags{
		AlbumArtist:         firstTag(tags, albumArtistKeys()),
		Artist:              tag(tags, "artist"),
		Album:               tag(tags, "album"),
		Title:               tag(tags, "title"),
		Genre:               tag(tags, "genre"),
		Disc:                parseOrdinal(firstTag(tags, discKeys())),
		Track:               parseOrdinal(firstTag(tags, trackKeys())),
		Year:                parseYear(firstTag(tags, yearKeys())),
		Compilation:         parseFlag(tag(tags, "compilation")),
		MusicBrainzArtistID: musicBrainzID(firstTag(tags, musicBrainzArtistKeys())),
		MusicBrainzAlbumID:  musicBrainzID(firstTag(tags, musicBrainzAlbumKeys())),
		MusicBrainzTrackID:  musicBrainzID(firstTag(tags, musicBrainzTrackKeys())),
	}
}

// firstTag returns the value of the first key in keys that has a non-empty value.
func firstTag(tags map[string]string, keys []string) string {
	for _, key := range keys {
		if value := tag(tags, key); value != "" {
			return value
		}
	}

	return ""
}

// parseOrdinal reads a track or disc number: "3" or "3/12" yield 3. Anything else yields 0
// (absent), so a malformed tag falls back to the file name instead of failing the scan.
func parseOrdinal(text string) int {
	number, _, _ := strings.Cut(text, ordinalSeparator)

	return parseSmallCount(number)
}

// parseYear returns the first run of at least four digits as a year: "2001", "2001-05-03",
// "20010503" and "03.05.2001" all yield 2001. Year 0 and texts without a year yield 0.
func parseYear(text string) int {
	for run := range strings.FieldsFuncSeq(text, isNotDigit) {
		if len(run) >= yearDigits {
			year, _ := strconv.Atoi(run[:yearDigits])

			return year
		}
	}

	return 0
}

func isNotDigit(character rune) bool {
	return character < '0' || character > '9'
}

// parseFlag reads the compilation flag ("1" in ID3 TCMP and MP4 cpil, "1" or "true" from
// other writers).
func parseFlag(text string) bool {
	return slices.Contains(trueFlagValues(), strings.ToLower(strings.TrimSpace(text)))
}

// musicBrainzID returns the first ID of a tag in lower case, or "" when it is no UUID: a
// broken ID must not link a track to the wrong MusicBrainz entity.
func musicBrainzID(text string) string {
	if end := strings.IndexAny(text, multipleValueSeparators); end >= 0 {
		text = text[:end]
	}

	candidate := strings.ToLower(strings.TrimSpace(text))
	if !isUUID(candidate) {
		return ""
	}

	return candidate
}

// uuidGroupLengths are the lengths of the dash-separated groups of a UUID (8-4-4-4-12).
func uuidGroupLengths() []int { return []int{8, 4, 4, 4, 12} }

// isUUID accepts the lower-case 8-4-4-4-12 hexadecimal form.
func isUUID(text string) bool {
	lengths := uuidGroupLengths()

	groups := strings.Split(text, uuidGroupSeparator)
	if len(groups) != len(lengths) {
		return false
	}

	for position, group := range groups {
		if len(group) != lengths[position] || strings.Trim(group, lowerHexDigits) != "" {
			return false
		}
	}

	return true
}

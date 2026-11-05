package scan_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	musicID   = "lib-music"
	musicRoot = "music"
)

// tagOutcome is the scripted result of reading the tags of one absolute path.
type tagOutcome struct {
	tags media.AudioTags
	err  error
}

// scriptedTags returns scripted tags per absolute path (default: no tags) and records reads.
type scriptedTags struct {
	mutex    sync.Mutex
	outcomes map[string]tagOutcome
	read     []string
}

func newScriptedTags() *scriptedTags {
	return &scriptedTags{outcomes: map[string]tagOutcome{}}
}

func (reader *scriptedTags) script(relativePath string, outcome tagOutcome) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.outcomes[absolutePath(musicRoot, relativePath)] = outcome
}

func (reader *scriptedTags) ReadAudioTags(ctx context.Context, path string) (media.AudioTags, time.Duration, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	reader.read = append(reader.read, path)

	if err := ctx.Err(); err != nil {
		return media.AudioTags{}, 0, fmt.Errorf("read tags: %w", err)
	}

	outcome := reader.outcomes[path]

	return outcome.tags, time.Minute, outcome.err
}

// readPaths returns the sorted relative paths read so far and forgets them.
func (reader *scriptedTags) takeReads() []string {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()

	paths := make([]string, 0, len(reader.read))
	for _, path := range reader.read {
		paths = append(paths, path[len(fakeRootPrefix+musicRoot+"/"):])
	}

	reader.read = nil

	return slices.Sorted(slices.Values(paths))
}

// trackDetails is one SaveTrackDetails call.
type trackDetails struct {
	disc int
	tags media.AudioTags
}

// memoryMusicCatalog records the track details per track ID; it rejects malformed IDs like the
// sqlite writer does (any ID that is not "mbid-ok").
type memoryMusicCatalog struct {
	mutex   sync.Mutex
	details map[string]trackDetails
	err     error
}

func newMemoryMusicCatalog() *memoryMusicCatalog {
	return &memoryMusicCatalog{details: map[string]trackDetails{}}
}

func (catalog *memoryMusicCatalog) SaveTrackDetails(_ context.Context, trackID string, disc int, tags media.AudioTags) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.err != nil {
		return catalog.err
	}

	for _, id := range []string{tags.MusicBrainzArtistID, tags.MusicBrainzAlbumID, tags.MusicBrainzTrackID} {
		if id != "" && id != validMusicBrainzID {
			return fmt.Errorf("MusicBrainz ID %q: %w", id, domain.ErrInvalid)
		}
	}

	catalog.details[trackID] = trackDetails{disc: disc, tags: tags}

	return nil
}

func (catalog *memoryMusicCatalog) of(trackID string) (trackDetails, bool) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	details, found := catalog.details[trackID]

	return details, found
}

const validMusicBrainzID = "mbid-ok"

// newMusicHarness is a harness with a music library and music ports.
func newMusicHarness(t *testing.T) (*harness, *scriptedTags, *memoryMusicCatalog) {
	t.Helper()

	h := newHarness(t)
	tags, catalog := newScriptedTags(), newMemoryMusicCatalog()
	h.music.Tags, h.music.Catalog = tags, catalog
	h.libraries.set(oneFolder(musicID, "Music", musicRoot, library.KindMusic))

	return h, tags, catalog
}

// audioDescription is a FLAC file; pictures adds attached pictures at the given indexes.
func audioDescription(pictures ...int) media.Description {
	description := media.Description{
		Container: media.ContainerUnknown, FormatName: "flac", Duration: time.Minute,
		Streams: []media.Stream{{Index: 0, Kind: media.StreamKindAudio, Codec: "flac", Channels: 2}},
	}

	for _, index := range pictures {
		description.Streams = append(description.Streams,
			media.Stream{Index: index, Kind: media.StreamKindVideo, Codec: "mjpeg", AttachedPicture: true})
	}

	return description
}

// audioAt is an audio file of the music library.
func audioAt(relativePath string, size int64) library.FileState {
	state := fileAt(relativePath, size)
	state.Class = library.FileClassAudio

	return state
}

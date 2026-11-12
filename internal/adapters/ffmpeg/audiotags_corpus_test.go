package ffmpeg_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Generated music fixtures: one second of sine tone, covers of coverSize square pixels.
const (
	fixtureDuration          = time.Second
	fixtureDurationTolerance = 100 * time.Millisecond
	coverSize                = 48
	fixtureAlbumID           = "f5093c06-23e3-404f-aeaa-40f72885ee3a"
	fixtureArtistID          = "0383dadf-2a4e-4d10-a46a-e9e041da8eb4"
	fixtureTrackID           = "9a1f2a6e-7f43-4c5f-8f7e-2f1f3c6d8b10"
	coverFormatJPEG          = "jpeg"
	coverFormatPNG           = "png"
	noCoverFormat            = ""
	fixtureGenerateMax       = 30 * time.Second
	// notOnWhitelistMessage is how FFmpeg reports a demuxer outside -format_whitelist.
	notOnWhitelistMessage = "not on whitelist"
)

// musicFixture is one generated audio file and what reading it must return.
type musicFixture struct {
	name     string
	encoder  []string
	metadata []string
	cover    string
	wantTags media.AudioTags
}

// commonMetadata are the tags every container stores under FFmpeg's generic keys.
func commonMetadata() []string {
	return []string{
		"-metadata", "title=Under Pressure", "-metadata", "artist=Queen feat. Bowie",
		"-metadata", "album_artist=Queen", "-metadata", "album=Hot Space", "-metadata", "genre=Rock",
		"-metadata", "track=4/11", "-metadata", "disc=1/2", "-metadata", "date=1982-04-21",
	}
}

func commonTags() media.AudioTags {
	return media.AudioTags{
		AlbumArtist: "Queen", Artist: "Queen feat. Bowie", Album: "Hot Space", Title: "Under Pressure",
		Genre: "Rock", Disc: 1, Track: 4, Year: 1982,
	}
}

func musicFixtures() []musicFixture {
	musicBrainzTags := commonTags()
	musicBrainzTags.MusicBrainzAlbumID = fixtureAlbumID
	musicBrainzTags.MusicBrainzArtistID = fixtureArtistID
	musicBrainzTags.MusicBrainzTrackID = fixtureTrackID

	compilation := media.AudioTags{Title: "Hit", Artist: "Someone", Album: "Hits", Track: 7, Compilation: true}

	return []musicFixture{
		{
			name: "cover.flac", encoder: []string{"-c:a", "flac"}, cover: coverFormatJPEG,
			metadata: append(commonMetadata(),
				"-metadata", "MUSICBRAINZ_ALBUMID="+fixtureAlbumID, "-metadata", "MUSICBRAINZ_ARTISTID="+fixtureArtistID,
				"-metadata", "MUSICBRAINZ_TRACKID="+fixtureTrackID),
			wantTags: musicBrainzTags,
		},
		{
			name: "cover.mp3", encoder: []string{"-c:a", "libmp3lame", "-id3v2_version", "3"}, cover: coverFormatJPEG,
			metadata: append(commonMetadata(),
				"-metadata", "MusicBrainz Album Id="+fixtureAlbumID, "-metadata", "MusicBrainz Artist Id="+fixtureArtistID,
				"-metadata", "MusicBrainz Track Id="+fixtureTrackID),
			wantTags: musicBrainzTags,
		},
		{
			name: "cover.m4a", encoder: []string{"-c:a", "aac"}, cover: coverFormatPNG,
			metadata: commonMetadata(), wantTags: commonTags(),
		},
		{
			name: "plain.flac", encoder: []string{"-c:a", "flac"}, cover: noCoverFormat,
			metadata: []string{
				"-metadata", "TITLE=Hit", "-metadata", "ARTIST=Someone", "-metadata", "ALBUM=Hits",
				"-metadata", "TRACKNUMBER=07", "-metadata", "COMPILATION=1",
			},
			wantTags: compilation,
		},
		{
			name: "plain.mp3", encoder: []string{"-c:a", "libmp3lame"}, cover: noCoverFormat,
		},
		{
			name: "stream-tags.opus", encoder: []string{"-c:a", "libopus"}, cover: noCoverFormat,
			metadata: []string{"-metadata", "title=Hit", "-metadata", "TRACKNUMBER=7", "-metadata", "ALBUMARTIST=Queen"},
			wantTags: media.AudioTags{Title: "Hit", Track: 7, AlbumArtist: "Queen"},
		},
	}
}

// musicTools are the FFmpeg executables and the two adapters under test.
type musicTools struct {
	ffmpegPath string
	prober     *ffmpeg.Prober
	extractor  *ffmpeg.CoverExtractor
}

func requireMusicTools(t *testing.T) musicTools {
	t.Helper()

	ffprobePath := requireFFprobe(t)
	ffmpegPath := requireFFmpeg(t)
	requireFFmpegFeature(t, ffmpegPath, ffmpegFeature{listing: "-devices", name: "lavfi", reason: "the music and chapter fixtures need the lavfi device and encoders such as libmp3lame"})

	logger := slog.New(slog.DiscardHandler)

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: ffprobePath, Logger: logger})
	if err != nil {
		t.Fatalf("NewProber: %v", err)
	}

	extractor, err := ffmpeg.NewCoverExtractor(ffmpeg.CoverExtractorOptions{
		FFmpegPath: ffmpegPath, FFprobePath: ffprobePath, Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewCoverExtractor: %v", err)
	}

	return musicTools{ffmpegPath: ffmpegPath, prober: prober, extractor: extractor}
}

// runFFmpeg runs ffmpeg with arguments and fails the test with its output on error.
func (tools musicTools) runFFmpeg(t *testing.T, arguments ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), fixtureGenerateMax)
	defer cancel()

	// The executable comes from PATH and the arguments from this test, never from a file.
	command := exec.CommandContext(ctx, tools.ffmpegPath, //nolint:gosec // see above
		append([]string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg %v: %v\n%s", arguments, err, output)
	}
}

// generate writes fixture into directory and returns its path.
func (tools musicTools) generate(t *testing.T, directory string, fixture musicFixture) string {
	t.Helper()

	arguments := []string{"-f", "lavfi", "-i", "sine=frequency=440:duration=1"}
	mapping := []string{"-map", "0:a"}

	if fixture.cover != noCoverFormat {
		coverPath := filepath.Join(directory, fixture.name+"-cover."+fixture.cover)
		tools.runFFmpeg(t, "-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=%dx%d", coverSize, coverSize), "-frames:v", "1", coverPath)
		arguments = append(arguments, "-i", coverPath)
		mapping = append(mapping, "-map", "1:v", "-c:v", "copy", "-disposition:v", "attached_pic")
	}

	path := filepath.Join(directory, fixture.name)
	arguments = append(append(append(arguments, mapping...), fixture.encoder...), fixture.metadata...)
	tools.runFFmpeg(t, append(arguments, path)...)

	return path
}

func TestReadAudioTagsAndExtractCoverWithFFmpeg(t *testing.T) {
	t.Parallel()

	tools := requireMusicTools(t)
	directory := t.TempDir()

	for _, fixture := range musicFixtures() {
		path := tools.generate(t, directory, fixture)

		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()

			tools.assertTags(t, path, fixture.wantTags)
			assertCover(t, tools.extractor, path, fixture.cover)
		})
	}
}

func (tools musicTools) assertTags(t *testing.T, path string, want media.AudioTags) {
	t.Helper()

	tags, duration, err := tools.prober.ReadAudioTags(t.Context(), path)
	if err != nil {
		t.Fatalf("ReadAudioTags: %v", err)
	}

	if tags != want {
		t.Errorf("tags = %+v, want %+v", tags, want)
	}

	if duration < fixtureDuration-fixtureDurationTolerance || duration > fixtureDuration+fixtureDurationTolerance {
		t.Errorf("duration = %v, want about %v", duration, fixtureDuration)
	}
}

func assertCover(t *testing.T, extractor *ffmpeg.CoverExtractor, path, wantFormat string) {
	t.Helper()

	data, err := extractor.ExtractCover(t.Context(), path)
	if wantFormat == noCoverFormat {
		if !errors.Is(err, ffmpeg.ErrNoCover) || !errors.Is(err, domain.ErrNotFound) {
			t.Errorf("ExtractCover error = %v, want ErrNoCover", err)
		}

		return
	}

	if err != nil {
		t.Fatalf("ExtractCover: %v", err)
	}

	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode extracted cover: %v", err)
	}

	if format != wantFormat || config.Width != coverSize || config.Height != coverSize {
		t.Errorf("cover = %s %dx%d, want %s %dx%d", format, config.Width, config.Height, wantFormat, coverSize, coverSize)
	}
}

func TestAudioReadersRefuseADemuxerOutsideTheWhitelist(t *testing.T) {
	t.Parallel()

	tools := requireMusicTools(t)
	directory := t.TempDir()
	target := tools.generate(t, directory, musicFixture{name: "target.flac", encoder: []string{"-c:a", "flac"}})

	// An ffconcat list named .flac would make FFmpeg open the file it names (ADR 0022).
	list := filepath.Join(directory, "list.flac")
	content := "ffconcat version 1.0\nfile '" + filepath.ToSlash(target) + "'\n"

	if err := os.WriteFile(list, []byte(content), 0o600); err != nil {
		t.Fatalf("write list: %v", err)
	}

	// The debug log carries ffprobe's error output, which names the refusal as the cause, so
	// the test cannot pass because ffprobe failed for another reason.
	var debugLog bytes.Buffer

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{
		FFprobePath: requireFFprobe(t),
		Logger:      slog.New(slog.NewTextHandler(&debugLog, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	if err != nil {
		t.Fatalf("NewProber: %v", err)
	}

	if _, _, err := prober.ReadAudioTags(t.Context(), list); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("ReadAudioTags error = %v, want domain.ErrUnsupported", err)
	}

	if !bytes.Contains(debugLog.Bytes(), []byte(notOnWhitelistMessage)) {
		t.Errorf("debug log does not name the whitelist refusal %q:\n%s", notOnWhitelistMessage, debugLog.String())
	}

	if _, err := tools.extractor.ExtractCover(t.Context(), list); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("ExtractCover error = %v, want domain.ErrUnsupported", err)
	}
}

func TestAudioReadersHonourCancellation(t *testing.T) {
	t.Parallel()

	tools := requireMusicTools(t)
	path := tools.generate(t, t.TempDir(), musicFixtures()[0])

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, _, err := tools.prober.ReadAudioTags(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("ReadAudioTags error = %v, want context.Canceled", err)
	}

	if _, err := tools.extractor.ExtractCover(ctx, path); !errors.Is(err, context.Canceled) {
		t.Errorf("ExtractCover error = %v, want context.Canceled", err)
	}
}

func TestAudioReadersRejectRelativePaths(t *testing.T) {
	t.Parallel()

	// No tool starts for a refused path, so the executables need not exist.
	logger := slog.New(slog.DiscardHandler)

	prober, err := ffmpeg.NewProber(ffmpeg.ProberOptions{FFprobePath: "ffprobe", Logger: logger})
	if err != nil {
		t.Fatalf("NewProber: %v", err)
	}

	extractor, err := ffmpeg.NewCoverExtractor(ffmpeg.CoverExtractorOptions{
		FFmpegPath: "ffmpeg", FFprobePath: "ffprobe", Logger: logger,
	})
	if err != nil {
		t.Fatalf("NewCoverExtractor: %v", err)
	}

	if _, _, err := prober.ReadAudioTags(t.Context(), "song.flac"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ReadAudioTags error = %v, want domain.ErrInvalid", err)
	}

	if _, err := extractor.ExtractCover(t.Context(), "song.flac"); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("ExtractCover error = %v, want domain.ErrInvalid", err)
	}
}

package ffmpeg_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Small folder discs generated per test run (the disc corpus of M3-16i will add real-looking
// ones): a DVD title of one program stream cut into two VOBs at a sector boundary, and a
// Blu-ray title of two transport stream clips with timestamps restarting at the second one.
const (
	discDVDFolder       = "DVD (2001)"
	discBlurayFolder    = "Bluray (2019)"
	discVOBOne          = "VIDEO_TS/VTS_01_1.VOB"
	discVOBTwo          = "VIDEO_TS/VTS_01_2.VOB"
	discClipOne         = "BDMV/STREAM/00001.m2ts"
	discClipTwo         = "BDMV/STREAM/00002.m2ts"
	discDVDSeconds      = 6
	discClipSeconds     = 4
	discSectorSize      = 2048
	discSeekPosition    = 5 * time.Second
	discDurationSlack   = 600 * time.Millisecond
	discGenerateTimeout = time.Minute
)

type discFixture struct {
	tools   hlsTools
	library string
}

// requireDiscFixture generates both discs into a temporary library, or skips without FFmpeg
// or under -short.
func requireDiscFixture(t *testing.T) discFixture {
	t.Helper()

	ffprobePath := requireFFprobe(t)

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("skipping FFmpeg disc integration test: ffmpeg is not on PATH (install FFmpeg, see docs/development.md)")
	}

	fixture := discFixture{
		tools:   hlsTools{ffmpegPath: ffmpegPath, ffprobePath: ffprobePath, prober: newCorpusProber(t, ffprobePath)},
		library: t.TempDir(),
	}
	fixture.generateDVD(t)
	fixture.generateBluray(t)

	return fixture
}

func (fixture discFixture) generateDVD(t *testing.T) {
	t.Helper()

	whole := filepath.Join(t.TempDir(), "title.vob")
	fixture.runFFmpeg(t, "-f", "lavfi", "-i", lavfiVideo(discDVDSeconds), "-f", "lavfi", "-i", lavfiAudio(discDVDSeconds),
		"-c:v", "mpeg2video", "-g", "12", "-b:v", "1M", "-c:a", "ac3", "-f", "vob", whole)

	data, err := os.ReadFile(whole) //nolint:gosec // test file
	if err != nil {
		t.Fatalf("read the VOB: %v", err)
	}

	half := len(data) / discSectorSize / 2 * discSectorSize
	fixture.write(t, discDVDFolder, discVOBOne, data[:half])
	fixture.write(t, discDVDFolder, discVOBTwo, data[half:])
}

func (fixture discFixture) generateBluray(t *testing.T) {
	t.Helper()

	for _, clip := range []string{discClipOne, discClipTwo} {
		target := fixture.path(discBlurayFolder, clip)
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatalf("create the clip folder: %v", err)
		}

		fixture.runFFmpeg(t, "-f", "lavfi", "-i", lavfiVideo(discClipSeconds), "-f", "lavfi", "-i", lavfiAudio(discClipSeconds),
			"-c:v", "libx264", "-g", "25", "-c:a", "ac3", "-f", "mpegts", "-mpegts_m2ts_mode", "1", target)
	}
}

func lavfiVideo(seconds int) string {
	return "testsrc2=size=320x240:rate=25:duration=" + strconv.Itoa(seconds)
}

func lavfiAudio(seconds int) string {
	return "sine=frequency=440:duration=" + strconv.Itoa(seconds)
}

func (fixture discFixture) runFFmpeg(t *testing.T, arguments ...string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), discGenerateTimeout)
	defer cancel()

	command := exec.CommandContext(ctx, fixture.tools.ffmpegPath, append([]string{"-v", "error", "-y"}, arguments...)...) //nolint:gosec // test tool
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generate disc media: %v\n%s", err, output)
	}
}

func (fixture discFixture) path(disc, clip string) string {
	return filepath.Join(fixture.library, disc, filepath.FromSlash(clip))
}

func (fixture discFixture) write(t *testing.T, disc, clip string, data []byte) {
	t.Helper()

	target := fixture.path(disc, clip)
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatalf("create the clip folder: %v", err)
	}

	if err := os.WriteFile(target, data, 0o600); err != nil { //nolint:gosec // test library
		t.Fatalf("write %s: %v", clip, err)
	}
}

func (fixture discFixture) dvd() ffmpeg.DiscInput {
	return ffmpeg.DiscInput{
		LibraryRoot: fixture.library, DiscPath: discDVDFolder, Layout: media.LayoutVideoTS,
		Title: media.DiscTitle{{Path: discVOBOne}, {Path: discVOBTwo}},
	}
}

func (fixture discFixture) bluray() ffmpeg.DiscInput {
	return ffmpeg.DiscInput{
		LibraryRoot: fixture.library, DiscPath: discBlurayFolder, Layout: media.LayoutBDMV,
		Title: media.DiscTitle{
			{Path: discClipOne, Duration: discClipSeconds * time.Second},
			{Path: discClipTwo, Duration: discClipSeconds * time.Second},
		},
	}
}

// TestProbeDiscDescribesFolderDiscTitles probes both generated discs through the local
// ffprobe: the joined VOBs and the concat list each yield one video and one audio stream and
// the duration of the whole title.
func TestProbeDiscDescribesFolderDiscTitles(t *testing.T) {
	fixture := requireDiscFixture(t)

	tests := []struct {
		name          string
		input         ffmpeg.DiscInput
		wantContainer media.Container
		wantVideo     string
		wantDuration  time.Duration
	}{
		{name: "DVD", input: fixture.dvd(), wantContainer: media.ContainerVOB, wantVideo: "mpeg2video", wantDuration: discDVDSeconds * time.Second},
		{name: "Blu-ray", input: fixture.bluray(), wantContainer: media.ContainerM2TS, wantVideo: "h264", wantDuration: 2 * discClipSeconds * time.Second},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			description, err := fixture.tools.prober.ProbeDisc(t.Context(), test.input)
			if err != nil {
				t.Fatalf("ProbeDisc = %v", err)
			}

			if description.Container != test.wantContainer || (description.Duration-test.wantDuration).Abs() > discDurationSlack {
				t.Errorf("container %q, duration %s, want %q, %s",
					description.Container, description.Duration, test.wantContainer, test.wantDuration)
			}

			if len(description.Streams) != 2 || description.Streams[0].Codec != test.wantVideo ||
				description.Streams[1].Codec != "ac3" {
				t.Errorf("streams = %+v, want %s and ac3", description.Streams, test.wantVideo)
			}
		})
	}
}

// TestProbeDiscRefusesAClipThatIsAConcatList: a crafted clip whose content is an ffconcat list
// must not make FFmpeg open the file it names (ADR 0022 Notes M2-24b, ADR 0029).
func TestProbeDiscRefusesAClipThatIsAConcatList(t *testing.T) {
	fixture := requireDiscFixture(t)

	canary := fixture.path(discBlurayFolder, discClipOne)
	list := "ffconcat version 1.0\nfile '" + filepath.ToSlash(canary) + "'\n"
	fixture.write(t, discBlurayFolder, discClipTwo, []byte(list))

	input := fixture.bluray()
	input.Title = input.Title[1:]

	if _, err := fixture.tools.prober.ProbeDisc(t.Context(), input); !errors.Is(err, media.ErrDiscUnsupported) {
		t.Errorf("ProbeDisc = %v, want %v", err, media.ErrDiscUnsupported)
	}
}

// TestProducerStreamsFolderDiscTitles produces both discs as HLS through the producer: the
// Blu-ray with copied video from a seek into its second clip, the DVD fully transcoded.
func TestProducerStreamsFolderDiscTitles(t *testing.T) {
	fixture := requireDiscFixture(t)
	producer := corpusProducer(t, fixture.tools)

	tests := []struct {
		name         string
		input        ffmpeg.DiscInput
		plan         playback.Plan
		start        time.Duration
		wantDuration time.Duration
	}{
		{
			name: "Blu-ray remux after a seek", input: fixture.bluray(), plan: discPlan(playback.ActionCopy),
			start: discSeekPosition, wantDuration: 2*discClipSeconds*time.Second - discSeekPosition,
		},
		{
			name: "DVD transcode", input: fixture.dvd(), plan: discPlan(playback.ActionTranscode),
			wantDuration: discDVDSeconds * time.Second,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			job := appplayback.Job{
				SessionID: producerSessionID, Plan: test.plan,
				LibraryRoot: fixture.library, RelativePath: test.input.DiscPath,
				InputPath: filepath.Join(fixture.library, test.input.DiscPath),
				Layout:    test.input.Layout, DiscTitle: test.input.Title,
				StartPosition: test.start, OutputDir: t.TempDir(),
			}

			production := startCorpusProduction(t, producer, job)
			readyOrFail(t, production)
			<-production.Done()

			if err := production.Ready(t.Context()); err != nil {
				t.Fatalf("Ready after the end = %v", err)
			}

			segments := hlsAssertPlaylist(t, job.OutputDir)

			output, err := fixture.tools.prober.Probe(t.Context(), hlsJoin(t, job.OutputDir, segments...))
			if err != nil {
				t.Fatalf("probe the output: %v", err)
			}

			if (output.Duration - test.wantDuration).Abs() > discDurationSlack {
				t.Errorf("output lasts %s, want %s", output.Duration, test.wantDuration)
			}
		})
	}
}

// discPlan transcodes the AC-3 audio of the discs to AAC and copies or transcodes the video.
func discPlan(video playback.Action) playback.Plan {
	videoDecision := playback.StreamDecision{Kind: media.StreamKindVideo, SourceIndex: 0, Action: video, TargetCodec: "h264"}
	if video == playback.ActionTranscode {
		videoDecision.Encoder = playback.EncoderLibx264
		videoDecision.TargetCodec = playback.TranscodeVideoCodec
	}

	return playback.Plan{
		Mode: playback.ModeTranscode, Delivery: playback.DeliveryHLS,
		Streams: []playback.StreamDecision{
			videoDecision,
			{
				Kind: media.StreamKindAudio, SourceIndex: 1, Action: playback.ActionTranscode,
				Encoder: playback.EncoderAAC, TargetCodec: playback.TranscodeAudioCodec, TargetChannels: 2,
			},
		},
	}
}

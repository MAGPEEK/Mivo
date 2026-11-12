package ffmpeg_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/domain"
)

// The generated cues of gen-testmedia (englishCues); the first one starts at 0.
const (
	firstCueText  = "Generated subtitle one"
	secondCueText = "Generated subtitle two"
	firstCueStart = "00:00.000 --> "
)

func newCorpusSubtitleExtractor(t *testing.T) *ffmpeg.SubtitleExtractor {
	t.Helper()

	extractor, err := ffmpeg.NewSubtitleExtractor(ffmpeg.SubtitleExtractorOptions{
		FFmpegPath: requireFFmpeg(t), Logger: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatalf("NewSubtitleExtractor: %v", err)
	}

	return extractor
}

// extractToString runs ExtractWebVTT into a temporary directory and returns the file.
func extractToString(t *testing.T, source string, track ffmpeg.SubtitleTrack) string {
	t.Helper()

	destination := filepath.Join(t.TempDir(), "cache", "track.vtt")

	err := newCorpusSubtitleExtractor(t).ExtractWebVTT(t.Context(), source, track, destination)
	if err != nil {
		t.Fatalf("ExtractWebVTT: %v", err)
	}

	data, err := os.ReadFile(destination) //nolint:gosec // temporary directory of the test
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	return string(data)
}

func requireWebVTTCues(t *testing.T, webVTT string) {
	t.Helper()

	for _, want := range []string{"WEBVTT", firstCueStart, firstCueText, secondCueText} {
		if !strings.Contains(webVTT, want) {
			t.Errorf("WebVTT lacks %q:\n%s", want, webVTT)
		}
	}
}

func TestExtractEmbeddedTextSubtitles(t *testing.T) {
	_, manifest := requireCorpus(t)

	tests := []struct {
		name  string
		file  string
		track ffmpeg.SubtitleTrack
	}{
		{"srt in matroska", "subtitle-mkv-srt-ass", ffmpeg.SubtitleTrack{Ordinal: 0, Codec: "subrip"}},
		{"ass in matroska", "subtitle-mkv-srt-ass", ffmpeg.SubtitleTrack{Ordinal: 1, Codec: "ass"}},
		{"mov_text in mp4", "subtitle-mp4-movtext", ffmpeg.SubtitleTrack{Ordinal: 0, Codec: "mov_text"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := corpusPath(t, corpusCaseByName(t, manifest, test.file).File)

			requireWebVTTCues(t, extractToString(t, source, test.track))
		})
	}
}

// The styled sample has inline overrides ({\i1\c&H00FF00&}) and a positioned, faded second
// style; WebVTT conversion must leave plain cue text. FFmpeg keeps italic and bold as the
// WebVTT tags <i> and <b>, which a player renders; colours, fonts, positions and fades go.
func TestExtractStripsAssTags(t *testing.T) {
	_, manifest := requireCorpus(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "tracks-mkv-ass-styled-srt").File)

	webVTT := extractToString(t, source, ffmpeg.SubtitleTrack{Ordinal: 0, Codec: "ass"})

	requireWebVTTCues(t, webVTT)

	for _, forbidden := range []string{`{\`, `\i1`, `&H`, `\pos`, `\fad`, `\N`} {
		if strings.Contains(webVTT, forbidden) {
			t.Errorf("WebVTT keeps ASS markup %q:\n%s", forbidden, webVTT)
		}
	}
}

func TestExtractExternalSubtitles(t *testing.T) {
	_, manifest := requireCorpus(t)
	directory := filepath.Dir(corpusPath(t, corpusCaseByName(t, manifest, "subtitle-sidecar-srt-vtt-ass").File))

	for _, name := range []string{"sidecar.srt", "sidecar.ass", "sidecar.vtt"} {
		t.Run(name, func(t *testing.T) {
			requireWebVTTCues(t, extractToString(t, filepath.Join(directory, name), ffmpeg.SubtitleTrack{External: true}))
		})
	}
}

// encodedSRT is a two-cue SRT file; the umlaut makes a wrong decoding visible.
const encodedSRT = "1\r\n00:00:00,000 --> 00:00:01,000\r\nGrüße, Käse\r\n\r\n2\r\n00:01.500 --> 00:02.000\r\nÖl für Åke\r\n"

func TestExtractExternalEncodings(t *testing.T) {
	requireFFmpeg(t)

	tests := []struct {
		name  string
		bytes []byte
	}{
		{"utf-8", []byte(encodedSRT)},
		{"utf-8 with BOM", append([]byte{0xEF, 0xBB, 0xBF}, encodedSRT...)},
		{"utf-16 LE with BOM", utf16Bytes(encodedSRT, false)},
		{"utf-16 BE with BOM", utf16Bytes(encodedSRT, true)},
		{"cp1252", windows1252Bytes(encodedSRT)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			source := filepath.Join(directory, "movie.srt")

			if err := os.WriteFile(source, test.bytes, 0o600); err != nil {
				t.Fatalf("write source: %v", err)
			}

			webVTT := extractToString(t, source, ffmpeg.SubtitleTrack{External: true})

			for _, want := range []string{"Grüße, Käse", "Öl für Åke", "00:01.500 --> 00:02.000"} {
				if !strings.Contains(webVTT, want) {
					t.Errorf("WebVTT lacks %q:\n%s", want, webVTT)
				}
			}
		})
	}
}

func utf16Bytes(text string, bigEndian bool) []byte {
	units := utf16.Encode([]rune(text))
	result := make([]byte, 0, 2+2*len(units))

	if bigEndian {
		result = append(result, 0xFE, 0xFF)
	} else {
		result = append(result, 0xFF, 0xFE)
	}

	for _, unit := range units {
		if bigEndian {
			result = binary.BigEndian.AppendUint16(result, unit)
		} else {
			result = binary.LittleEndian.AppendUint16(result, unit)
		}
	}

	return result
}

// windows1252Bytes encodes text whose runes are all below U+0100, where Windows-1252 and
// Latin-1 agree.
func windows1252Bytes(text string) []byte {
	result := make([]byte, 0, len(text))
	for _, character := range text {
		result = append(result, byte(character&0xFF))
	}

	return result
}

// Bitmap subtitles cannot become WebVTT; the PGS and VobSub clips are refused before ffmpeg
// starts, and a bitmap subtitle file the same.
func TestExtractRefusesBitmapSubtitles(t *testing.T) {
	_, manifest := requireCorpus(t)
	extractor := newCorpusSubtitleExtractor(t)
	destination := filepath.Join(t.TempDir(), "image.vtt")

	tests := []struct {
		name   string
		source string
		track  ffmpeg.SubtitleTrack
	}{
		{"pgs", corpusCaseByName(t, manifest, "tracks-mkv-pgs").File, ffmpeg.SubtitleTrack{Codec: "hdmv_pgs_subtitle"}},
		{"dvdsub", corpusCaseByName(t, manifest, "tracks-mkv-dvdsub").File, ffmpeg.SubtitleTrack{Codec: "dvd_subtitle"}},
		{"sup file", "movie.sup", ffmpeg.SubtitleTrack{External: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := test.source
			if !filepath.IsAbs(source) {
				source = corpusPath(t, source)
			}

			err := extractor.ExtractWebVTT(t.Context(), source, test.track, destination)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ExtractWebVTT error = %v, want ErrInvalid", err)
			}
		})
	}

	if _, err := os.Stat(destination); err == nil {
		t.Errorf("a refused subtitle left %s behind", destination)
	}
}

func TestExtractReportsMissingAndWrongStreams(t *testing.T) {
	_, manifest := requireCorpus(t)
	extractor := newCorpusSubtitleExtractor(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "subtitle-mp4-movtext").File)
	destination := filepath.Join(t.TempDir(), "out.vtt")

	missing := filepath.Join(filepath.Dir(source), "does-not-exist.mkv")
	if err := extractor.ExtractWebVTT(t.Context(), missing, ffmpeg.SubtitleTrack{Codec: "subrip"}, destination); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("missing file error = %v, want ErrNotFound", err)
	}

	// The clip has one subtitle stream; ordinal 5 names none.
	if err := extractor.ExtractWebVTT(t.Context(), source, ffmpeg.SubtitleTrack{Ordinal: 5, Codec: "mov_text"}, destination); !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("missing stream error = %v, want ErrUnsupported", err)
	}

	if _, err := os.Stat(destination); err == nil {
		t.Errorf("a failed extraction left %s behind", destination)
	}
}

// A cancelled context stops the extraction without a result.
func TestExtractHonoursCancellation(t *testing.T) {
	_, manifest := requireCorpus(t)
	extractor := newCorpusSubtitleExtractor(t)
	source := corpusPath(t, corpusCaseByName(t, manifest, "subtitle-mkv-srt-ass").File)
	destination := filepath.Join(t.TempDir(), "out.vtt")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	started := time.Now()
	err := extractor.ExtractWebVTT(ctx, source, ffmpeg.SubtitleTrack{Codec: "subrip"}, destination)

	if !errors.Is(err, ctx.Err()) {
		t.Fatalf("ExtractWebVTT error = %v, want the context error", err)
	}

	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("cancelled extraction took %v", elapsed)
	}
}

const (
	// twoHourCues spaces one cue per 2 s over two hours.
	twoHourCues     = 3600
	twoHourSeconds  = 2 * 3600
	cueSpacing      = 2
	extractionLimit = 10 * time.Second
)

// A 2 h track must come out of a real container within 10 s (task M4-08a). The clip is built
// from a generated SRT with one cue every two seconds.
func TestExtractTwoHourTrackWithinLimit(t *testing.T) {
	ffmpegPath := requireFFmpeg(t)
	requireFFmpegFeature(t, ffmpegPath, ffmpegFeature{listing: "-muxers", name: "matroska", reason: "the 2 h clip is built as MKV with an srt encoder; the extraction is covered by the corpus tests"})
	requireFFmpegFeature(t, ffmpegPath, ffmpegFeature{listing: "-encoders", name: "srt", reason: "the 2 h clip is built with an srt subtitle encoder"})

	directory := t.TempDir()

	var text strings.Builder

	for index := range twoHourCues {
		start := index * cueSpacing
		fmt.Fprintf(&text, "%d\n%s --> %s\nCue number %d\n\n", index+1, srtClock(start), srtClock(start+1), index+1)
	}

	source := filepath.Join(directory, "long.srt")
	if err := os.WriteFile(source, []byte(text.String()), 0o600); err != nil {
		t.Fatalf("write srt: %v", err)
	}

	media := filepath.Join(directory, "long.mkv")
	command := exec.CommandContext(t.Context(), ffmpegPath, "-v", "error", "-f", "srt", "-i", source, //nolint:gosec // test binary and temporary files
		"-map", "0:s:0", "-c:s", "srt", media)

	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build the 2 h clip: %v\n%s", err, output)
	}

	started := time.Now()
	webVTT := extractToString(t, media, ffmpeg.SubtitleTrack{Codec: "subrip"})

	if elapsed := time.Since(started); elapsed > extractionLimit {
		t.Errorf("extracting %d s of cues took %v, want at most %v", twoHourSeconds, elapsed, extractionLimit)
	}

	if want := fmt.Sprintf("Cue number %d", twoHourCues); !strings.Contains(webVTT, want) {
		t.Errorf("WebVTT lacks the last cue %q", want)
	}
}

func srtClock(seconds int) string {
	return fmt.Sprintf("%02d:%02d:%02d,000", seconds/3600, seconds/60%60, seconds%60)
}

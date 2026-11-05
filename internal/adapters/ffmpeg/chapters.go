package ffmpeg

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/big"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// readChaptersOperation names the prober's chapter runs in debug logs.
	readChaptersOperation = "read chapters"
	// timeBaseSeparator splits ffprobe's "numerator/denominator" time base.
	timeBaseSeparator = "/"
	// untitledChapterFormat names a chapter without a title by its 1-based position.
	untitledChapterFormat = "Chapter %d"
	// maxMilliseconds is the longest offset a time.Duration holds, in milliseconds.
	maxMilliseconds = math.MaxInt64 / int64(time.Millisecond)
	// maxChapters caps one file's chapter list; real audiobooks stay far below it.
	maxChapters = 10000
)

// ReadChapters reads the chapter markers of the media file at absolutePath (M4B/MP4,
// Matroska, MP3 ID3 CHAP and every other container FFmpeg reads chapters from). It knows
// nothing about audio, so video chapters use it too. Offsets are truncated to milliseconds;
// a chapter without a title is named "Chapter n" after its position. Chapters are sorted by
// start; empty or negative markers and those sharing the start of an earlier one (MP4 files
// may carry both a Nero and a QuickTime chapter list) are dropped. When a chapter starts
// inside the previous one, the previous one is cut at its start: taggers often let chapters
// overlap slightly or end every chapter at the total duration. So the result passes
// media.ValidateChapters. At most maxChapters are returned; the rest are dropped with a
// debug record. A file without chapters yields an empty slice and no error.
// The caller has resolved the path and checked that it lies inside a library root
// (ADR 0022). Errors wrap
//   - domain.ErrInvalid for a relative path or a refused Windows device path,
//   - context.DeadlineExceeded after the per-file timeout, context.Canceled on cancellation,
//   - domain.ErrUnavailable when ffprobe cannot be started,
//   - domain.ErrUnsupported when ffprobe failed on the file (including a demuxer outside
//     the input whitelist) or its output was unusable.
//
// Messages name the file by its base name only; ffprobe's error output goes to the debug log.
func (p *Prober) ReadChapters(ctx context.Context, absolutePath string) ([]media.Chapter, error) {
	input, err := inputArguments(absolutePath)
	if err != nil {
		return nil, fmt.Errorf("read chapters: %w", err)
	}

	probeContext, cancel := context.WithTimeout(ctx, p.probeTimeout)
	defer cancel()

	output, err := p.ffprobe.output(probeContext, chapterArguments(input))
	if err != nil {
		p.logRunFailure(ctx, readChaptersOperation, absolutePath, err)

		return nil, fmt.Errorf("read chapters %s: %w", filepath.Base(absolutePath), classifyRunError(err))
	}

	chapters, err := describeChapters(output)
	if err != nil {
		return nil, fmt.Errorf("read chapters %s: %w", filepath.Base(absolutePath), err)
	}

	return p.limitChapters(ctx, absolutePath, chapters), nil
}

// limitChapters caps the list at maxChapters, so a hostile file cannot make the caller store
// millions of rows.
func (p *Prober) limitChapters(ctx context.Context, absolutePath string, chapters []media.Chapter) []media.Chapter {
	if len(chapters) <= maxChapters {
		return chapters
	}

	p.logger.LogAttrs(ctx, slog.LevelDebug, "chapter list truncated",
		slog.String("file", filepath.Base(absolutePath)),
		slog.Int("chapters", len(chapters)), slog.Int("kept", maxChapters))

	return chapters[:maxChapters]
}

func chapterArguments(input []string) []string {
	return append([]string{
		"-v", "error", "-hide_banner", "-print_format", "json", "-show_chapters",
	}, input...)
}

// probeChapter is one entry of ffprobe's -show_chapters JSON. Start and end count ticks of
// the time base; the rounded start_time/end_time strings are not used.
type probeChapter struct {
	TimeBase string            `json:"time_base"`
	Start    int64             `json:"start"`
	End      int64             `json:"end"`
	Tags     map[string]string `json:"tags"`
}

type chapterProbeOutput struct {
	Chapters []probeChapter `json:"chapters"`
}

// describeChapters maps ffprobe's -show_chapters JSON to an ordered, non-overlapping chapter
// list. Errors wrap domain.ErrUnsupported.
func describeChapters(output []byte) ([]media.Chapter, error) {
	var parsed chapterProbeOutput
	if err := json.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("decode ffprobe output: %w: %w", domain.ErrUnsupported, err)
	}

	candidates := make([]media.Chapter, 0, len(parsed.Chapters))

	for _, chapter := range parsed.Chapters {
		if converted, ok := convertChapter(chapter); ok {
			candidates = append(candidates, converted)
		}
	}

	slices.SortStableFunc(candidates, func(left, right media.Chapter) int {
		return cmp.Compare(left.Start, right.Start)
	})

	return orderedChapters(candidates), nil
}

// orderedChapters keeps the valid chapters of a list sorted by start, drops those that share
// the start of the previous kept one, cuts the previous one where the next begins inside it
// and names untitled ones by their position in the result.
func orderedChapters(sorted []media.Chapter) []media.Chapter {
	chapters := make([]media.Chapter, 0, len(sorted))

	for _, chapter := range sorted {
		if chapter.Validate() != nil {
			continue
		}

		if last := len(chapters) - 1; last >= 0 {
			previous := &chapters[last]
			if chapter.Start <= previous.Start {
				continue
			}

			previous.End = min(previous.End, chapter.Start)
		}

		if chapter.Title == "" {
			chapter.Title = fmt.Sprintf(untitledChapterFormat, len(chapters)+1)
		}

		chapters = append(chapters, chapter)
	}

	return chapters
}

// convertChapter converts one ffprobe chapter; ok is false when its time base is unusable.
func convertChapter(chapter probeChapter) (media.Chapter, bool) {
	numerator, denominator, ok := parseTimeBase(chapter.TimeBase)
	if !ok {
		return media.Chapter{}, false
	}

	start, startOK := ticksToDuration(chapter.Start, numerator, denominator)
	end, endOK := ticksToDuration(chapter.End, numerator, denominator)

	return media.Chapter{Title: tag(chapter.Tags, "title"), Start: start, End: end}, startOK && endOK
}

// parseTimeBase reads "numerator/denominator" with both parts positive.
func parseTimeBase(text string) (numerator, denominator int64, ok bool) {
	numeratorText, denominatorText, found := strings.Cut(text, timeBaseSeparator)
	if !found {
		return 0, 0, false
	}

	numerator, numeratorErr := strconv.ParseInt(numeratorText, 10, 64)
	denominator, denominatorErr := strconv.ParseInt(denominatorText, 10, 64)

	if numeratorErr != nil || denominatorErr != nil || numerator <= 0 || denominator <= 0 {
		return 0, 0, false
	}

	return numerator, denominator, true
}

// ticksToDuration converts ticks of the time base numerator/denominator to a duration
// truncated to milliseconds. Big integers avoid the overflow of ticks × numerator × 1000 for
// fine time bases such as Matroska's nanoseconds; ok is false when the result does not fit.
func ticksToDuration(ticks, numerator, denominator int64) (time.Duration, bool) {
	milliseconds := big.NewInt(ticks)
	milliseconds.Mul(milliseconds, big.NewInt(numerator))
	milliseconds.Mul(milliseconds, big.NewInt(int64(time.Second/time.Millisecond)))
	milliseconds.Quo(milliseconds, big.NewInt(denominator))

	if milliseconds.CmpAbs(big.NewInt(maxMilliseconds)) > 0 {
		return 0, false
	}

	return time.Duration(milliseconds.Int64()) * time.Millisecond, true
}

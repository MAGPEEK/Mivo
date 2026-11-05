package ffmpeg

import (
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestDescribeChapters(t *testing.T) {
	t.Parallel()

	second := time.Second
	tests := []struct {
		name   string
		output string
		want   []media.Chapter
	}{
		{name: "no chapters", output: `{"chapters":[]}`, want: []media.Chapter{}},
		{name: "chapters section missing", output: `{}`, want: []media.Chapter{}},
		{
			name: "MP4 sample time base with titles",
			output: `{"chapters":[
				{"id":0,"time_base":"1/44100","start":0,"end":44100,"tags":{"title":"One"}},
				{"id":1,"time_base":"1/44100","start":44100,"end":88200,"tags":{"title":"Two"}}]}`,
			want: []media.Chapter{{Title: "One", End: second}, {Title: "Two", Start: second, End: 2 * second}},
		},
		{
			name: "Matroska nanosecond time base truncated to milliseconds",
			output: `{"chapters":[
				{"id":1,"time_base":"1/1000000000","start":0,"end":1234567890,"tags":{"TITLE":"Intro"}}]}`,
			want: []media.Chapter{{Title: "Intro", End: 1234 * time.Millisecond}},
		},
		{
			name: "long chapter in a fine time base does not overflow",
			output: `{"chapters":[
				{"id":0,"time_base":"1/1000000000","start":0,"end":360000000000000}]}`,
			want: []media.Chapter{{Title: "Chapter 1", End: 100 * time.Hour}},
		},
		{
			name: "empty and blank titles are numbered by position",
			output: `{"chapters":[
				{"id":0,"time_base":"1/1000","start":0,"end":1000,"tags":{"title":""}},
				{"id":1,"time_base":"1/1000","start":1000,"end":2000,"tags":{"title":"Middle"}},
				{"id":2,"time_base":"1/1000","start":2000,"end":3000,"tags":{"title":"  "}}]}`,
			want: []media.Chapter{
				{Title: "Chapter 1", End: second},
				{Title: "Middle", Start: second, End: 2 * second},
				{Title: "Chapter 3", Start: 2 * second, End: 3 * second},
			},
		},
		{
			name: "all chapters end at the total duration and are trimmed to the next start",
			output: `{"chapters":[
				{"id":0,"time_base":"1/1000","start":0,"end":3000,"tags":{"title":"One"}},
				{"id":1,"time_base":"1/1000","start":1000,"end":3000,"tags":{"title":"Two"}},
				{"id":2,"time_base":"1/1000","start":2000,"end":3000,"tags":{"title":"Three"}}]}`,
			want: []media.Chapter{
				{Title: "One", End: second},
				{Title: "Two", Start: second, End: 2 * second},
				{Title: "Three", Start: 2 * second, End: 3 * second},
			},
		},
		{
			name: "small overlap trims the previous chapter, a gap stays",
			output: `{"chapters":[
				{"id":0,"time_base":"1/1000","start":0,"end":1050,"tags":{"title":"One"}},
				{"id":1,"time_base":"1/1000","start":1000,"end":1800,"tags":{"title":"Two"}},
				{"id":2,"time_base":"1/1000","start":2000,"end":3000,"tags":{"title":"Three"}}]}`,
			want: []media.Chapter{
				{Title: "One", End: second},
				{Title: "Two", Start: second, End: 1800 * time.Millisecond},
				{Title: "Three", Start: 2 * second, End: 3 * second},
			},
		},
		{
			name: "unordered chapters are sorted, same-start duplicates and empty ones dropped",
			output: `{"chapters":[
				{"id":0,"time_base":"1/1000","start":2000,"end":3000,"tags":{"title":"Three"}},
				{"id":1,"time_base":"1/1000","start":0,"end":2000,"tags":{"title":"One"}},
				{"id":2,"time_base":"1/10000000","start":20000000,"end":30000000,"tags":{"title":"Duplicate"}},
				{"id":3,"time_base":"1/1000","start":3000,"end":3000,"tags":{"title":"Empty"}},
				{"id":4,"time_base":"1/1000","start":-5,"end":10,"tags":{"title":"Negative"}}]}`,
			want: []media.Chapter{{Title: "One", End: 2 * second}, {Title: "Three", Start: 2 * second, End: 3 * second}},
		},
		{
			name: "unusable time bases are skipped",
			output: `{"chapters":[
				{"id":0,"time_base":"N/A","start":0,"end":1000},
				{"id":1,"time_base":"1/0","start":0,"end":1000},
				{"id":2,"time_base":"1000","start":0,"end":1000},
				{"id":3,"time_base":"0/1","start":0,"end":1000},
				{"id":4,"time_base":"-1/1000","start":0,"end":1000},
				{"id":5,"time_base":"x/1000","start":0,"end":1000},
				{"id":6,"time_base":"1000/1","start":0,"end":9223372036854775807},
				{"id":7,"time_base":"1/1000","start":0,"end":1000,"tags":{"title":"Kept"}}]}`,
			want: []media.Chapter{{Title: "Kept", End: second}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := describeChapters([]byte(test.output))
			if err != nil {
				t.Fatalf("describeChapters: %v", err)
			}

			if got == nil || !slices.Equal(got, test.want) {
				t.Errorf("chapters = %+v, want %+v", got, test.want)
			}

			if err := media.ValidateChapters(got); err != nil {
				t.Errorf("result does not validate: %v", err)
			}
		})
	}
}

func TestDescribeChaptersRejectsUnreadableOutput(t *testing.T) {
	t.Parallel()

	_, err := describeChapters([]byte(`{"chapters":`))
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Errorf("error = %v, want domain.ErrUnsupported", err)
	}
}

func TestLimitChaptersTruncatesWithADebugRecord(t *testing.T) {
	t.Parallel()

	var logOutput strings.Builder

	prober := &Prober{logger: slog.New(slog.NewTextHandler(&logOutput, &slog.HandlerOptions{Level: slog.LevelDebug}))}
	chapters := make([]media.Chapter, maxChapters+1)

	for index := range chapters {
		chapters[index] = media.Chapter{Start: time.Duration(index) * time.Second, End: time.Duration(index+1) * time.Second}
	}

	limited := prober.limitChapters(t.Context(), "/books/huge.m4b", chapters)
	if len(limited) != maxChapters || limited[maxChapters-1] != chapters[maxChapters-1] {
		t.Errorf("limited to %d chapters, want the first %d", len(limited), maxChapters)
	}

	if !strings.Contains(logOutput.String(), "chapter list truncated") || !strings.Contains(logOutput.String(), "huge.m4b") {
		t.Errorf("log = %q, want a truncation record naming the file", logOutput.String())
	}

	logOutput.Reset()

	if kept := prober.limitChapters(t.Context(), "/books/small.m4b", chapters[:2]); len(kept) != 2 || logOutput.Len() != 0 {
		t.Errorf("short list: kept %d, log %q; want 2 and no record", len(kept), logOutput.String())
	}
}

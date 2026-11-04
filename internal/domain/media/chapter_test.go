package media_test

import (
	"errors"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func TestValidateChapters(t *testing.T) {
	t.Parallel()

	second := time.Second
	tests := []struct {
		name     string
		chapters []media.Chapter
		wantErr  bool
	}{
		{"empty", nil, false},
		{"ordered with gap", []media.Chapter{{Title: "a", End: second}, {Title: "b", Start: 2 * second, End: 3 * second}}, false},
		{"touching", []media.Chapter{{End: second}, {Start: second, End: 2 * second}}, false},
		{"negative start", []media.Chapter{{Start: -second, End: second}}, true},
		{"end equals start", []media.Chapter{{Start: second, End: second}}, true},
		{"end before start", []media.Chapter{{Start: 2 * second, End: second}}, true},
		{"overlap", []media.Chapter{{End: 2 * second}, {Start: second, End: 3 * second}}, true},
		{"unordered", []media.Chapter{{Start: 2 * second, End: 3 * second}, {End: second}}, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := media.ValidateChapters(test.chapters)
			if (err != nil) != test.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, test.wantErr)
			}

			if test.wantErr && !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestChaptersFromParts(t *testing.T) {
	t.Parallel()

	parts := []media.Part{
		{Title: "One", FileRef: "b/1.mp3", Duration: 10 * time.Second},
		{Title: "Two", FileRef: "b/2.mp3", Duration: 5 * time.Second},
	}

	chapters, err := media.ChaptersFromParts(parts)
	if err != nil {
		t.Fatal(err)
	}

	want := []media.Chapter{
		{Title: "One", Start: 0, End: 10 * time.Second, FileRef: "b/1.mp3"},
		{Title: "Two", Start: 10 * time.Second, End: 15 * time.Second, FileRef: "b/2.mp3"},
	}
	for index := range want {
		if chapters[index] != want[index] {
			t.Errorf("chapter %d = %+v, want %+v", index, chapters[index], want[index])
		}
	}

	if chapters[1].Duration() != 5*time.Second {
		t.Errorf("Duration = %v", chapters[1].Duration())
	}

	if err := media.ValidateChapters(chapters); err != nil {
		t.Errorf("built chapters invalid: %v", err)
	}

	if _, err := media.ChaptersFromParts([]media.Part{{FileRef: "x"}}); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("zero duration err = %v, want ErrInvalid", err)
	}
}

package restapi

import (
	"testing"
	"time"

	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// idSecondPart is the second part file of the mapped audiobook.
const idSecondPart = "2d3e4f5a-6b7c-4d8e-9f0a-1b2c3d4e5f6a"

// TestItemSummaryResponseAudiobook: an audiobook card carries its cover as poster and the
// summed length of its parts, not the primary file's.
func TestItemSummaryResponseAudiobook(t *testing.T) {
	t.Parallel()

	file := media.File{ID: idFile, Description: media.Description{Container: media.ContainerMP4, Duration: 90 * time.Second}}
	summary := applibrary.ItemSummary{
		Item: mapperItem(idEpisode, library.ItemKindAudiobook), PrimaryFile: file,
		Poster: &artwork.Ref{ID: idImage, Tag: "cover"}, PartsDuration: 270500 * time.Millisecond,
	}

	response, err := itemSummaryResponse(summary)
	if err != nil {
		t.Fatalf("itemSummaryResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response),
		[]string{`"kind":"audiobook"`, `"poster":{"id":"` + idImage + `"`, `"durationSeconds":270.5`},
		[]string{`"square"`, `"durationSeconds":90`})

	summary.PartsDuration = 0

	response, err = itemSummaryResponse(summary)
	if err != nil {
		t.Fatalf("itemSummaryResponse without parts duration: %v", err)
	}

	assertFragments(t, mustMarshal(t, response), []string{`"durationSeconds":90`}, nil)
}

// TestItemDetailResponseAudiobookChapters: chapter offsets go out relative to their part
// file, so a chapter of the second part starts at its own file's offset.
func TestItemDetailResponseAudiobookChapters(t *testing.T) {
	t.Parallel()

	detail := applibrary.ItemDetail{
		Item: mapperItem(idEpisode, library.ItemKindAudiobook),
		Chapters: []applibrary.ItemChapter{
			{Chapter: media.Chapter{Title: "One", Start: 0, End: 90 * time.Second}, FileID: idFile},
			{
				Chapter: media.Chapter{Title: "Two", Start: 95 * time.Second, End: 120 * time.Second},
				FileID:  idSecondPart, FileStart: 90 * time.Second,
			},
		},
	}

	response, err := itemDetailResponse(detail)
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response), []string{
		`"chapters":[{"endMs":90000,"fileId":"` + idFile + `","startMs":0,"title":"One"},` +
			`{"endMs":30000,"fileId":"` + idSecondPart + `","startMs":5000,"title":"Two"}]`,
	}, nil)
}

func TestItemDetailResponseAudiobookChaptersAreCapped(t *testing.T) {
	t.Parallel()

	chapters := make([]applibrary.ItemChapter, maxChaptersInDetail+1)
	for index := range chapters {
		start := time.Duration(index) * time.Second
		chapters[index] = applibrary.ItemChapter{Chapter: media.Chapter{Title: "c", Start: start, End: start + time.Second}, FileID: idFile}
	}

	response, err := itemDetailResponse(applibrary.ItemDetail{Item: mapperItem(idEpisode, library.ItemKindAudiobook), Chapters: chapters})
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	if response.Chapters == nil || len(*response.Chapters) != maxChaptersInDetail {
		t.Errorf("chapters = %v, want the first %d", response.Chapters, maxChaptersInDetail)
	}
}

func TestItemDetailResponseOmitsAbsentChapters(t *testing.T) {
	t.Parallel()

	response, err := itemDetailResponse(applibrary.ItemDetail{Item: mapperItem(idEpisode, library.ItemKindAudiobook)})
	if err != nil {
		t.Fatalf("itemDetailResponse: %v", err)
	}

	assertFragments(t, mustMarshal(t, response), nil, []string{`"chapters"`})
}

func TestItemDetailResponseRejectsAChapterWithABadFileID(t *testing.T) {
	t.Parallel()

	detail := applibrary.ItemDetail{
		Item:     mapperItem(idEpisode, library.ItemKindAudiobook),
		Chapters: []applibrary.ItemChapter{{Chapter: media.Chapter{Title: "One", End: time.Second}, FileID: "not-a-uuid"}},
	}

	if _, err := itemDetailResponse(detail); err == nil {
		t.Error("itemDetailResponse accepted a chapter whose file ID is no UUID")
	}
}

package scan_test

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	booksID   = "lib-books"
	booksRoot = "books"
	// partLength is the length the fake tag reader reports for every file.
	partLength = 10 * time.Minute
)

// bookFile is the scripted content of one audio file of the audiobooks library.
type bookFile struct {
	tags       media.AudioTags
	tagErr     error
	chapters   []media.Chapter
	chapterErr error
}

// audiobookTools reads scripted tags and chapters per relative path and records the reads.
type audiobookTools struct {
	mutex        sync.Mutex
	files        map[string]bookFile
	tagReads     []string
	chapterReads []string
}

func newAudiobookTools() *audiobookTools {
	return &audiobookTools{files: map[string]bookFile{}}
}

func (tools *audiobookTools) script(relativePath string, file bookFile) {
	tools.mutex.Lock()
	defer tools.mutex.Unlock()

	tools.files[relativePath] = file
}

func relativeOf(absolute string) string {
	return strings.TrimPrefix(absolute, fakeRootPrefix+booksRoot+"/")
}

func (tools *audiobookTools) ReadAudioTags(ctx context.Context, path string) (media.AudioTags, time.Duration, error) {
	tools.mutex.Lock()
	defer tools.mutex.Unlock()

	tools.tagReads = append(tools.tagReads, relativeOf(path))

	if err := ctx.Err(); err != nil {
		return media.AudioTags{}, 0, fmt.Errorf("read tags: %w", err)
	}

	file := tools.files[relativeOf(path)]

	return file.tags, partLength, file.tagErr
}

func (tools *audiobookTools) ReadChapters(ctx context.Context, path string) ([]media.Chapter, error) {
	tools.mutex.Lock()
	defer tools.mutex.Unlock()

	tools.chapterReads = append(tools.chapterReads, relativeOf(path))

	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("read chapters: %w", err)
	}

	file := tools.files[relativeOf(path)]

	return slices.Clone(file.chapters), file.chapterErr
}

// takeReads returns the sorted tag and chapter reads so far and forgets them.
func (tools *audiobookTools) takeReads() (tagReads, chapterReads []string) {
	tools.mutex.Lock()
	defer tools.mutex.Unlock()

	tagReads, chapterReads = slices.Sorted(slices.Values(tools.tagReads)), slices.Sorted(slices.Values(tools.chapterReads))
	tools.tagReads, tools.chapterReads = nil, nil

	return tagReads, chapterReads
}

// memoryChapters stores chapters per file ID and validates them like the sqlite writer.
type memoryChapters struct {
	mutex    sync.Mutex
	chapters map[string][]media.Chapter
	writes   int
	// failures, when positive, fails that many writes after skip successful ones.
	failures int
	skip     int
}

func newMemoryChapters() *memoryChapters {
	return &memoryChapters{chapters: map[string][]media.Chapter{}}
}

func (catalog *memoryChapters) ReplaceFileChapters(_ context.Context, fileID string, chapters []media.Chapter) error {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	if catalog.failures > 0 && catalog.skip > 0 {
		catalog.skip--
	} else if catalog.failures > 0 {
		catalog.failures--

		return fmt.Errorf("replace chapters of %s: %w", fileID, domain.ErrUnavailable)
	}

	if err := media.ValidateChapters(chapters); err != nil {
		return fmt.Errorf("replace chapters of %s: %w", fileID, err)
	}

	catalog.writes++
	catalog.chapters[fileID] = slices.Clone(chapters)

	return nil
}

func (catalog *memoryChapters) of(fileID string) []media.Chapter {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.chapters[fileID]
}

func (catalog *memoryChapters) failNext(count int) {
	catalog.failAfter(0, count)
}

// failAfter fails count writes after skip successful ones.
func (catalog *memoryChapters) failAfter(skip, count int) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	catalog.skip, catalog.failures = skip, count
}

func (catalog *memoryChapters) writeCount() int {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	return catalog.writes
}

// fileOf returns the ID and placement key of the stored file at path.
func (catalog *memoryCatalogV2) fileOf(path string) (id, placementKey string) {
	catalog.mutex.Lock()
	defer catalog.mutex.Unlock()

	for _, file := range catalog.files {
		if file.state.RelativePath == path {
			return file.id, file.placementKey
		}
	}

	return "", ""
}

// newAudiobookHarness is a harness with an audiobooks library and audiobook ports.
func newAudiobookHarness(t *testing.T) (*harness, *audiobookTools, *memoryChapters) {
	t.Helper()

	h := newHarness(t)
	tools, chapters := newAudiobookTools(), newMemoryChapters()
	h.audiobooks = scan.AudiobookPorts{Tags: tools, Chapters: tools, Catalog: chapters}
	h.libraries.set(oneFolder(booksID, "Books", booksRoot, library.KindAudiobooks))

	return h, tools, chapters
}

// scanBooks scans the audiobooks library with the given files and returns the final status.
func (h *harness) scanBooks(t *testing.T, files ...library.FileState) scan.Status {
	t.Helper()

	h.walker.set(booksRoot, walkResult{files: files})

	return h.scan(t, booksID)
}

// chapter is a chapter from start to end minutes.
func chapter(title string, startMinute, endMinute int) media.Chapter {
	return media.Chapter{Title: title, Start: time.Duration(startMinute) * time.Minute, End: time.Duration(endMinute) * time.Minute}
}

// containsText reports whether text contains part.
func containsText(text, part string) bool {
	return strings.Contains(text, part)
}

// intTexts formats numbers for assertKeys.
func intTexts(numbers []int) []string {
	texts := make([]string, 0, len(numbers))
	for _, number := range numbers {
		texts = append(texts, strconv.Itoa(number))
	}

	return texts
}

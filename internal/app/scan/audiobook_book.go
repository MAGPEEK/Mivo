package scan

import (
	"context"
	"path"
	"strings"

	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// partChapters are the chapters stored for one saved file, relative to its start: its own
// chapter list when it has a valid one, else one chapter over the whole file titled by the
// title tag or the file name. So a single M4B keeps its chapters and a folder of parts gets
// one chapter per part (file chapters where a part has them), and a file keeps its chapters
// when it later joins or leaves a book. A file of unknown length gets none.
func (scanRun *audiobookRun) partChapters(ctx context.Context, part audiobookResult) []media.Chapter {
	if part.probe.outcome != outcomeProbed {
		return nil
	}

	state := part.job.state

	switch err := media.ValidateChapters(part.chapters); {
	case part.chapterErr != nil:
		scanRun.warn(ctx, "scan could not read chapters, using one chapter for the file", state, part.chapterErr)
	case err != nil:
		scanRun.warn(ctx, "scan ignored invalid chapters, using one chapter for the file", state, err)
	case len(part.chapters) > 0:
		return part.chapters
	}

	duration := part.probe.description.Duration
	if duration <= 0 {
		duration = part.duration
	}

	chapters, err := media.ChaptersFromParts([]media.Part{{Title: partTitle(part), FileRef: state.RelativePath, Duration: duration}})
	if err != nil {
		return nil
	}

	return chapters
}

// partTitle is the title tag of the part, else its file name without the extension.
func partTitle(part audiobookResult) string {
	if title := strings.TrimSpace(part.tags.Title); title != "" {
		return title
	}

	base := path.Base(part.job.state.RelativePath)
	if stem := strings.TrimSuffix(base, path.Ext(base)); stem != "" {
		return stem
	}

	return base
}

// rememberBooks makes the kept unreadable parts known to the refresher and keeps the
// embedded record of every book that has an item by now.
func (scanRun *audiobookRun) rememberBooks(books []groupedBook) {
	scanRun.addKeptParts()

	for _, grouped := range books {
		scanRun.rememberBook(grouped)
	}
}

// addKeptParts adds the kept unreadable parts to the refresh files with the book level of a
// placed sibling, so the refresh of the book sees all its files. Their part number is not
// known (the stored placement is only a key), so they sort after the numbered parts; a part
// without a placed sibling stays out.
func (scanRun *audiobookRun) addKeptParts() {
	levels := map[string]library.Level{}

	for _, file := range scanRun.wanted {
		if chain := scanRun.chains[keyOf(file.state)]; len(chain) > 0 {
			levels[chain[0]] = file.placement.Levels[0]
		}
	}

	for _, kept := range scanRun.keptUnreadable {
		if len(kept.ItemIDs) != 1 {
			continue
		}

		if level, found := levels[kept.ItemIDs[0]]; found {
			scanRun.wanted = append(scanRun.wanted, placedFile{state: kept.State, placement: library.Placement{Levels: []library.Level{level}}})
		}
	}

	scanRun.keptUnreadable = nil
}

// rememberBook keeps the embedded record of a book whose item is known for the refresh.
func (scanRun *audiobookRun) rememberBook(grouped groupedBook) {
	for _, part := range grouped.parts {
		if chain := scanRun.chains[keyOf(part.job.state)]; len(chain) > 0 {
			scanRun.embedded[chain[0]] = bookRecord(grouped)

			return
		}
	}
}

// bookRecord is what the tags say about a book: its title, author and narrator as people,
// and the year and genre of the first part that has one.
func bookRecord(grouped groupedBook) metadata.Record {
	book := grouped.book
	record := metadata.Record{Title: book.Title}

	if book.Author != "" {
		record.People = append(record.People, metadata.Credit{Name: book.Author, Role: metadata.RoleAuthor})
	}

	if book.Narrator != "" {
		record.People = append(record.People, metadata.Credit{Name: book.Narrator, Role: metadata.RoleNarrator, Order: len(record.People)})
	}

	for _, part := range grouped.parts {
		if record.Year == 0 {
			record.Year = part.tags.Year
		}

		if genre := strings.TrimSpace(part.tags.Genre); genre != "" && record.Genres == nil {
			record.Genres = []string{genre}
		}
	}

	return record
}

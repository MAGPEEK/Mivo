package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// summaryTotalLabel names the line that adds up all categories.
const summaryTotalLabel = "total"

// tally counts the outcomes of one category. Skips are split by kind, because a case the
// local FFmpeg cannot build, one excluded by -only and a known gap mean different things.
type tally struct {
	generated   int
	skipped     int
	notSelected int
	gaps        int
	interrupted int
	failed      int
}

func (t *tally) add(entry manifestEntry) {
	switch entry.Status {
	case statusGenerated:
		t.generated++
	case statusFailed:
		t.failed++
	case statusSkipped:
		t.addSkip(entry.SkipKind)
	}
}

func (t *tally) addSkip(kind skipKind) {
	switch kind {
	case skipNotSelected:
		t.notSelected++
	case skipKnownGap:
		t.gaps++
	case skipInterrupted:
		t.interrupted++
	case skipMissingCapability:
		t.skipped++
	}
}

func (t tally) plus(other tally) tally {
	return tally{
		generated:   t.generated + other.generated,
		skipped:     t.skipped + other.skipped,
		notSelected: t.notSelected + other.notSelected,
		gaps:        t.gaps + other.gaps,
		interrupted: t.interrupted + other.interrupted,
		failed:      t.failed + other.failed,
	}
}

func (t tally) String() string {
	text := fmt.Sprintf("%d generated, %d skipped, %d not selected, %d gaps, %d failed",
		t.generated, t.skipped, t.notSelected, t.gaps, t.failed)
	if t.interrupted > 0 {
		text += fmt.Sprintf(", %d interrupted", t.interrupted)
	}

	return text
}

// tallyByCategory counts the entries of every category.
func (m manifest) tallyByCategory() map[string]tally {
	tallies := map[string]tally{}

	for _, entry := range m.Cases {
		counts := tallies[entry.Category]
		counts.add(entry)
		tallies[entry.Category] = counts
	}

	return tallies
}

// writeSummary prints one line per category, a total and the manifest path.
func writeSummary(output io.Writer, m manifest, manifestPath string) error {
	tallies := m.tallyByCategory()
	categories := slices.Sorted(maps.Keys(tallies))
	width := len(summaryTotalLabel)

	for _, category := range categories {
		width = max(width, len(category))
	}

	var text strings.Builder

	fmt.Fprintf(&text, "ffmpeg %s\n", m.FFmpegVersion)

	total := tally{}

	for _, category := range categories {
		fmt.Fprintf(&text, "  %-*s  %s\n", width, category, tallies[category])
		total = total.plus(tallies[category])
	}

	fmt.Fprintf(&text, "  %-*s  %s\nmanifest: %s\n", width, summaryTotalLabel, total, manifestPath)

	if _, err := io.WriteString(output, text.String()); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}

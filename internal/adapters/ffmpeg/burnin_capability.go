package ffmpeg

import (
	"context"
	"slices"
	"strings"
)

const (
	filtersQuery = "-filters"
	// subtitlesFilterName is the libass based filter that renders text subtitles; builds
	// without libass lack it (and the ass filter).
	subtitlesFilterName = "subtitles"
)

// detectBurnInText reports whether FFmpeg lists the subtitles filter. A failing query counts
// as "no": the server then offers no text burn-in, which is the safe answer, and the other
// capabilities stay valid.
func detectBurnInText(ctx context.Context, run commandOutput) bool {
	queryContext, cancel := context.WithTimeout(ctx, capabilityDetectionTimeout)
	defer cancel()

	output, err := run(queryContext, []string{hideBannerFlag, filtersQuery})
	if err != nil {
		return false
	}

	return slices.Contains(parseFilterNames(output), subtitlesFilterName)
}

// parseFilterNames returns the filter names of `ffmpeg -filters` output: after the dashed
// separator every entry is "<flags> <name> <pads> <description>". The flag column is a few
// characters of "TSC." and has its own width, unlike the listings of parseListing.
func parseFilterNames(output []byte) []string {
	var (
		names      []string
		pastLegend bool
	)

	for line := range strings.Lines(string(output)) {
		fields := strings.Fields(line)

		if !pastLegend {
			pastLegend = separatorWidth(strings.TrimRight(line, "\r\n")) > 0

			continue
		}

		const minimumFields = 3 // flags, name and pads

		if len(fields) >= minimumFields && strings.Trim(fields[0], "TSC.") == "" {
			names = append(names, fields[1])
		}
	}

	return names
}

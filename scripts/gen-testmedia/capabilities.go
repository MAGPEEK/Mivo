package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// capabilityTimeout bounds each of the quick listing calls (-encoders, -muxers, -version).
const capabilityTimeout = 30 * time.Second

// errNoCapabilityList reports FFmpeg output without the "---" line that starts the list.
var errNoCapabilityList = errors.New("no capability list found in ffmpeg output")

// capabilities is what the local FFmpeg build can produce.
type capabilities struct {
	version  string
	encoders map[string]struct{}
	muxers   map[string]struct{}
}

// detectCapabilities asks FFmpeg for its version, encoders and muxers.
func detectCapabilities(ctx context.Context, ffmpeg commandRunner) (capabilities, error) {
	versionOutput, err := runWithTimeout(ctx, ffmpeg, "-version")
	if err != nil {
		return capabilities{}, fmt.Errorf("read ffmpeg version: %w", err)
	}

	encoders, err := listCapability(ctx, ffmpeg, "-encoders")
	if err != nil {
		return capabilities{}, err
	}

	muxers, err := listCapability(ctx, ffmpeg, "-muxers")
	if err != nil {
		return capabilities{}, err
	}

	return capabilities{version: parseVersion(versionOutput), encoders: encoders, muxers: muxers}, nil
}

func listCapability(ctx context.Context, ffmpeg commandRunner, listFlag string) (map[string]struct{}, error) {
	output, err := runWithTimeout(ctx, ffmpeg, "-hide_banner", listFlag)
	if err != nil {
		return nil, fmt.Errorf("list ffmpeg %s: %w", listFlag, err)
	}

	names, err := parseCapabilityList(bytes.NewReader(output))
	if err != nil {
		return nil, fmt.Errorf("parse ffmpeg %s: %w", listFlag, err)
	}

	return names, nil
}

func runWithTimeout(ctx context.Context, ffmpeg commandRunner, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, capabilityTimeout)
	defer cancel()

	output, err := ffmpeg.Run(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("run ffmpeg %s: %w", strings.Join(args, " "), err)
	}

	return output, nil
}

// parseCapabilityList parses the output of `ffmpeg -encoders` or `ffmpeg -muxers`.
//
// Both print a legend, a separator line of dashes and then one entry per line: a space, a
// flag column exactly as wide as the separator, a space and the name. Muxer names may list
// aliases separated by commas ("stream_segment,ssegment"); each alias is returned.
func parseCapabilityList(reader io.Reader) (map[string]struct{}, error) {
	scanner := bufio.NewScanner(reader)
	names := make(map[string]struct{})
	flagWidth := 0

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")

		if flagWidth == 0 {
			flagWidth = separatorWidth(line)

			continue
		}

		for _, name := range entryNames(line, flagWidth) {
			names[name] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read capability list: %w", err)
	}

	if flagWidth == 0 {
		return nil, errNoCapabilityList
	}

	return names, nil
}

// separatorWidth returns the number of dashes if line is the separator, otherwise 0.
func separatorWidth(line string) int {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.Trim(trimmed, "-") != "" {
		return 0
	}

	return len(trimmed)
}

// entryNames returns the names of one list entry, or nil if the line is no entry.
func entryNames(line string, flagWidth int) []string {
	// One leading space, the flag column and one space precede the name.
	nameStart := flagWidth + len("  ")
	if len(line) <= nameStart {
		return nil
	}

	fields := strings.Fields(line[nameStart:])
	if len(fields) == 0 {
		return nil
	}

	return strings.Split(fields[0], ",")
}

// parseVersion extracts the version from the first line of `ffmpeg -version`,
// "ffmpeg version <version> Copyright ...". It returns "unknown" for other output.
func parseVersion(output []byte) string {
	firstLine, _, _ := bytes.Cut(output, []byte("\n"))

	const versionField = 2

	fields := strings.Fields(string(firstLine))
	if len(fields) <= versionField || fields[1] != "version" {
		return "unknown"
	}

	return fields[versionField]
}

// missingRequirements describes what the case needs but the local FFmpeg lacks, or returns
// "" if the case can be built.
func (caps capabilities) missingRequirements(c mediaCase) string {
	var missing []string

	for _, encoder := range c.Encoders {
		if _, ok := caps.encoders[encoder]; !ok {
			missing = append(missing, "encoder "+encoder)
		}
	}

	if _, ok := caps.muxers[c.Muxer]; !ok {
		missing = append(missing, "muxer "+c.Muxer)
	}

	if len(missing) == 0 {
		return ""
	}

	return "ffmpeg lacks " + strings.Join(missing, ", ")
}

package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// capabilityDetectionTimeout bounds the three listing calls together. Each answers in well
	// under a second, but on a NAS the executable may first have to be read from a sleeping
	// disk; the bound keeps a hanging FFmpeg from blocking the server start.
	capabilityDetectionTimeout = 30 * time.Second

	hideBannerFlag = "-hide_banner"
	versionQuery   = "-version"
	encodersQuery  = "-encoders"
	muxersQuery    = "-muxers"

	// versionBannerPrefix starts the line of `ffmpeg -version` that carries the version.
	versionBannerPrefix = "ffmpeg version "
	// maxVersionLength bounds the version shown in diagnostics; git builds print the longest
	// versions ("2026-10-01-git-0123456789-full_build-www.gyan.dev").
	maxVersionLength = 64

	// unknownVersion replaces a version token with characters other than printable ASCII:
	// it would end up in logs and diagnostics unescaped, but FFmpeg itself works, so its
	// capabilities still count.
	unknownVersion = "unknown"

	// encoderKindVideo and encoderKindAudio are the first flag of an -encoders entry.
	encoderKindVideo = 'V'
	encoderKindAudio = 'A'
	// experimentalFlag marks encoders FFmpeg uses only with -strict experimental.
	experimentalFlag = 'X'
	// muxingFlag marks a format FFmpeg can write.
	muxingFlag = 'E'

	// muxerHLS and muxerMP4 are what fMP4 HLS needs: the playlist muxer and the mp4 muxer
	// that writes its segments.
	muxerHLS = "hls"
	muxerMP4 = "mp4"
)

var (
	// errNoVersionBanner reports -version output without an "ffmpeg version <version>" line,
	// for example because the configured executable is not FFmpeg.
	errNoVersionBanner = errors.New(`no "ffmpeg version" line in the output`)
	// errNoListing reports -encoders or -muxers output without the dashed separator line
	// that starts the list.
	errNoListing = errors.New("no list separator in the output")
)

// commandOutput runs FFmpeg with arguments and returns its standard output (tool.output).
type commandOutput func(ctx context.Context, arguments []string) ([]byte, error)

// DetectCapabilities asks the ffmpeg executable at ffmpegPath (the configured tools.ffmpeg,
// resolved by the caller) for its version, encoders and muxers, so playback never promises
// a remux or transcode the server cannot deliver (ADR 0015, ADR 0023). Hardware encoders are
// not detected in milestone 2.
//
// Detection is all or nothing: on any error the capabilities are the zero value, a server
// without FFmpeg that can only direct-play. Errors wrap
//   - domain.ErrUnavailable when FFmpeg cannot be started, fails, does not answer within the
//     detection timeout or prints output this package does not understand,
//   - the caller's context error when ctx ended; it says nothing about FFmpeg,
//   - domain.ErrInvalid for a nil logger, before FFmpeg runs.
//
// A failed detection is logged here, once at warn level with FFmpeg's error output, which
// stays inside this package (ADR 0024 Notes M2-39); the caller only continues without
// FFmpeg. A detection ended by ctx is not logged.
func DetectCapabilities(ctx context.Context, ffmpegPath string, logger *slog.Logger) (playback.ServerCapabilities, error) {
	if logger == nil {
		return playback.ServerCapabilities{}, fmt.Errorf("detect ffmpeg capabilities: logger is nil: %w", domain.ErrInvalid)
	}

	ffmpeg := tool{name: ffmpegToolName, executable: ffmpegPath}

	capabilities, err := detectCapabilities(ctx, ffmpeg.output, capabilityDetectionTimeout)
	if err != nil && ctx.Err() == nil {
		logDetectionFailure(ctx, logger, err)
	}

	return capabilities, err
}

// logDetectionFailure writes the one warning of a failed detection: it changes what the
// server can do until a restart, so it is worth a warning even without a playback request.
func logDetectionFailure(ctx context.Context, logger *slog.Logger, err error) {
	attributes := append([]slog.Attr{slog.Any("error", err)}, exitAttributes(err)...)
	logger.LogAttrs(ctx, slog.LevelWarn, "ffmpeg capability detection failed: only direct play available", attributes...)
}

// detectCapabilities runs the three queries under one timeout and parses their output.
func detectCapabilities(ctx context.Context, run commandOutput, timeout time.Duration) (playback.ServerCapabilities, error) {
	detectContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var outputs capabilityOutputs

	queries := []struct {
		flag   string
		output *[]byte
	}{
		{flag: versionQuery, output: &outputs.version},
		{flag: encodersQuery, output: &outputs.encoders},
		{flag: muxersQuery, output: &outputs.muxers},
	}

	for _, query := range queries {
		output, err := run(detectContext, []string{hideBannerFlag, query.flag})
		if err != nil {
			return playback.ServerCapabilities{}, fmt.Errorf("detect ffmpeg capabilities: ffmpeg %s: %w",
				query.flag, unavailableUnlessCancelled(ctx, err))
		}

		*query.output = output
	}

	capabilities, err := outputs.parse()
	if err != nil {
		return playback.ServerCapabilities{}, fmt.Errorf("detect ffmpeg capabilities: %w: %w", domain.ErrUnavailable, err)
	}

	return capabilities, nil
}

// unavailableUnlessCancelled marks a failed query as FFmpeg being unavailable, unless the
// caller gave up (shutdown during startup) or the runner already said so. A query killed by
// the cancellation fails with its own error (an exit code), so the caller's context error is
// wrapped in that case: the result must read as the cancellation.
func unavailableUnlessCancelled(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		if errors.Is(err, ctxErr) {
			return err
		}

		return fmt.Errorf("%w: %w", ctxErr, err)
	}

	if errors.Is(err, domain.ErrUnavailable) {
		return err
	}

	return fmt.Errorf("%w: %w", domain.ErrUnavailable, err)
}

// capabilityOutputs is the standard output of `ffmpeg -hide_banner` with -version, -encoders
// and -muxers.
type capabilityOutputs struct {
	version  []byte
	encoders []byte
	muxers   []byte
}

// parse turns the three outputs into capabilities. An encoder counts only with the expected
// kind and when it is not experimental; HLS muxing needs both the hls and the mp4 muxer.
func (o capabilityOutputs) parse() (playback.ServerCapabilities, error) {
	version, err := parseVersion(o.version)
	if err != nil {
		return playback.ServerCapabilities{}, fmt.Errorf("parse ffmpeg %s: %w", versionQuery, err)
	}

	encoders, err := parseListing(o.encoders)
	if err != nil {
		return playback.ServerCapabilities{}, fmt.Errorf("parse ffmpeg %s: %w", encodersQuery, err)
	}

	muxers, err := parseListing(o.muxers)
	if err != nil {
		return playback.ServerCapabilities{}, fmt.Errorf("parse ffmpeg %s: %w", muxersQuery, err)
	}

	capabilities := playback.ServerCapabilities{
		FFmpegVersion: version,
		HLSMuxing:     hasMuxer(muxers, muxerHLS) && hasMuxer(muxers, muxerMP4),
	}

	if hasEncoder(encoders, playback.EncoderLibx264, encoderKindVideo) {
		capabilities.H264Encoder = playback.EncoderLibx264
	}

	if hasEncoder(encoders, playback.EncoderAAC, encoderKindAudio) {
		capabilities.AACEncoder = playback.EncoderAAC
	}

	return capabilities, nil
}

// parseVersion returns the version token of the "ffmpeg version <version> Copyright ..."
// line, cut to maxVersionLength. Distribution builds append their revision
// ("7.1.5-0+deb13u1"), which is kept: it is what users report in bug reports.
func parseVersion(output []byte) (string, error) {
	for line := range strings.Lines(string(output)) {
		rest, isBanner := strings.CutPrefix(line, versionBannerPrefix)
		if !isBanner {
			continue
		}

		fields := strings.Fields(rest)
		if len(fields) == 0 {
			continue
		}

		version := fields[0]
		if !isPrintableASCII(version) {
			return unknownVersion, nil
		}

		return version[:min(len(version), maxVersionLength)], nil
	}

	return "", errNoVersionBanner
}

func isPrintableASCII(text string) bool {
	for index := range len(text) {
		if text[index] <= ' ' || text[index] > '~' {
			return false
		}
	}

	return true
}

// listingEntry is one line of an -encoders or -muxers list: the flag column and the names
// (muxers list aliases separated by commas, "stream_segment,ssegment").
type listingEntry struct {
	flags string
	names []string
}

// parseListing reads the list printed by -encoders or -muxers: a legend, a separator line of
// dashes as wide as the flag column, then one entry per line formed by a space, the flag
// column, a space and the name column. Matching the name column, never substrings of a line,
// keeps libx264rgb or a description mentioning "aac" from counting. Lines of another shape
// (a trailing "Exiting with exit code 0") are ignored.
func parseListing(output []byte) ([]listingEntry, error) {
	flagWidth := 0

	var entries []listingEntry

	for line := range strings.Lines(string(output)) {
		line = strings.TrimRight(line, "\r\n")

		if flagWidth == 0 {
			flagWidth = separatorWidth(line)

			continue
		}

		if entry, isEntry := parseListingEntry(line, flagWidth); isEntry {
			entries = append(entries, entry)
		}
	}

	if flagWidth == 0 {
		return nil, errNoListing
	}

	return entries, nil
}

// separatorWidth returns the number of dashes when line is the separator, otherwise 0.
func separatorWidth(line string) int {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.Trim(trimmed, "-") != "" {
		return 0
	}

	return len(trimmed)
}

func parseListingEntry(line string, flagWidth int) (listingEntry, bool) {
	flagStart := len(" ")
	nameStart := flagStart + flagWidth + len(" ")

	if len(line) <= nameStart || line[0] != ' ' || line[nameStart-1] != ' ' || line[nameStart] == ' ' {
		return listingEntry{}, false
	}

	flags := line[flagStart : flagStart+flagWidth]
	if !isFlagColumn(flags) {
		return listingEntry{}, false
	}

	name, _, _ := strings.Cut(line[nameStart:], " ")

	return listingEntry{flags: flags, names: strings.Split(name, ",")}, true
}

// isFlagColumn accepts letters, dots and spaces, the characters of FFmpeg's flag columns.
func isFlagColumn(flags string) bool {
	for index := range len(flags) {
		character := flags[index]
		isLetter := (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')

		if !isLetter && character != '.' && character != ' ' {
			return false
		}
	}

	return true
}

// hasEncoder reports a usable encoder: listed under name, of the given kind and not
// experimental (an experimental encoder would need -strict experimental, which the HLS
// arguments never pass).
func hasEncoder(entries []listingEntry, name string, kind byte) bool {
	return slices.ContainsFunc(entries, func(entry listingEntry) bool {
		return entry.flags[0] == kind &&
			!strings.ContainsRune(entry.flags, experimentalFlag) &&
			slices.Contains(entry.names, name)
	})
}

// hasMuxer reports a format listed under name that FFmpeg can write.
func hasMuxer(entries []listingEntry, name string) bool {
	return slices.ContainsFunc(entries, func(entry listingEntry) bool {
		return strings.ContainsRune(entry.flags, muxingFlag) && slices.Contains(entry.names, name)
	})
}

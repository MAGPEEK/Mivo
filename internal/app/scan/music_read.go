package scan

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/naming"
)

// musicJob is one audio file to probe and read; placed is its stored entry when it changed.
type musicJob struct {
	state  library.FileState
	placed *PlacedFileState
}

// musicResult is what probing and reading one audio file found. probe.err ends the scan;
// tagErr only means the tags could not be read.
type musicResult struct {
	job    musicJob
	probe  probeResult
	tags   media.AudioTags
	tagErr error
}

// readTrack probes one file and, when the probe succeeded, reads its tags, in one probe slot.
func (scanRun *musicRun) readTrack(ctx context.Context, job musicJob) musicResult {
	result := musicResult{job: job}

	releaseSlot, err := scanRun.base.service.acquireProbeSlot(ctx)
	if err != nil {
		result.probe = probeResult{err: err}

		return result
	}

	defer releaseSlot()

	probed, absolutePath := scanRun.base.describe(ctx, probeJob{state: job.state, placed: job.placed})

	result.probe = probed
	if probed.err != nil || probed.outcome != outcomeProbed {
		return result
	}

	tags, _, err := scanRun.ports.Tags.ReadAudioTags(ctx, absolutePath)

	switch {
	case ctx.Err() != nil:
		result.probe.err = fmt.Errorf("read tags: %w", ctx.Err())
	case errors.Is(err, domain.ErrUnavailable):
		result.probe.err = fail(ErrorCodeProberUnavailable, causeProberUnavailable, fmt.Errorf("read tags: %w", err))
	case err != nil:
		result.tagErr = err
	default:
		result.tags = tags
	}

	return result
}

// recordTrack places and saves one read file. Unreadable tags fall back to the folder layout
// with one warning; a file that could not be probed is still saved, with its probe error.
func (scanRun *musicRun) recordTrack(ctx context.Context, result musicResult) error {
	if result.probe.err != nil {
		return result.probe.err
	}

	job := result.job
	if result.tagErr != nil {
		scanRun.warnUnreadableTags(ctx, job.state, result.tagErr)
	}

	track, err := naming.PlaceMusicTrack(job.state.PathForParsing(), result.tags)
	if err != nil {
		// The walker only reports storable paths, so this is a defect, not a file problem.
		return fail(ErrorCodeFailed, "place audio file", fmt.Errorf("place %s: %w", job.state.RelativePath, err))
	}

	saved := probeJob{state: job.state, placement: track.Placement, placed: job.placed}
	if err := scanRun.saveProbed(ctx, saved, result.probe); err != nil {
		return err
	}

	// A failure here ends the scan after the file was saved, so the next scan finds it
	// unchanged and does not write the details again (BACKLOG, m3-13e notes).
	if err := scanRun.saveTrackDetails(ctx, job.state, track.Disc, result.tags); err != nil {
		return err
	}

	scanRun.wanted = append(scanRun.wanted, placedFile{state: job.state, placement: track.Placement})

	return nil
}

// saveProbed stores the file with its description or its probe error and counts it.
func (scanRun *musicRun) saveProbed(ctx context.Context, job probeJob, probed probeResult) error {
	if probed.outcome == outcomeProbed {
		return scanRun.recordProbed(ctx, probeResult{job: job, outcome: outcomeProbed, description: probed.description})
	}

	scanRun.base.logFile(ctx, "scan could not probe file", job,
		slog.String("code", string(probed.probeError)), slog.String("error", probed.reason))

	if err := scanRun.save(ctx, job, media.Description{}, probed.probeError); err != nil {
		return err
	}

	scanRun.base.tracker.count(func(counts *Counts) { counts.Failed++ })

	return nil
}

// saveTrackDetails stores disc and MusicBrainz IDs of the saved track. Malformed IDs are a
// tag problem, not a catalog one: the track is saved again without them and one warning is
// counted.
func (scanRun *musicRun) saveTrackDetails(ctx context.Context, state library.FileState, disc int, tags media.AudioTags) error {
	chain := scanRun.chains[keyOf(state)]
	if len(chain) == 0 {
		return fail(ErrorCodeFailed, "save track details", fmt.Errorf("track %s has no item chain: %w", state.RelativePath, domain.ErrInvalid))
	}

	trackID := chain[len(chain)-1]

	err := scanRun.ports.Catalog.SaveTrackDetails(ctx, trackID, disc, tags)
	if errors.Is(err, domain.ErrInvalid) && hasMusicBrainzIDs(tags) {
		scanRun.warn(ctx, "scan ignored malformed MusicBrainz IDs", state, err)

		tags.MusicBrainzArtistID, tags.MusicBrainzAlbumID, tags.MusicBrainzTrackID = "", "", ""
		err = scanRun.ports.Catalog.SaveTrackDetails(ctx, trackID, disc, tags)
	}

	if err != nil {
		return catalogFailure("save track details of "+state.RelativePath, err)
	}

	return nil
}

func hasMusicBrainzIDs(tags media.AudioTags) bool {
	return tags.MusicBrainzArtistID != "" || tags.MusicBrainzAlbumID != "" || tags.MusicBrainzTrackID != ""
}

// warnUnreadableTags logs the one warning of a file whose tags could not be read.
func (scanRun *musicRun) warnUnreadableTags(ctx context.Context, state library.FileState, err error) {
	scanRun.warn(ctx, "scan could not read audio tags, using the folder names", state, err)
}

// warn logs a per-file warning and counts it. The error text comes from the tag reader or
// the catalog, whose errors carry no absolute library path.
func (scanRun *musicRun) warn(ctx context.Context, message string, state library.FileState, err error) {
	base := scanRun.base
	base.service.logger.LogAttrs(ctx, slog.LevelWarn, message,
		slog.String("library", base.library.ID), slog.String("path", state.RelativePath), slog.String("error", err.Error()))
	base.tracker.count(func(counts *Counts) { counts.MetadataWarnings++ })
}

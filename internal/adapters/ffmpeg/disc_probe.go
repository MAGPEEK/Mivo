package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// discProbeOperation names the prober's disc runs in debug logs.
const discProbeOperation = "probe disc"

// ProbeDisc describes the main title of a folder disc: its duration and the video, audio and
// subtitle streams FFmpeg finds in the joined clips (DVD and Blu-ray subtitles are pictures,
// see media.Stream.IsBitmapSubtitle). The caller has checked that the disc lies inside the
// library root; the clip paths are validated here (DiscInput). Errors wrap
//   - the errors of PrepareDiscInput (domain.ErrInvalid, media.ErrDiscNoTitle,
//     media.ErrDiscUnsupported for an ISO image),
//   - context errors and domain.ErrUnavailable like Probe,
//   - media.ErrDiscUnsupported (and so domain.ErrUnsupported) when ffprobe failed on the
//     title, for example because the FFmpeg build lacks the concat or mpeg demuxer, or its
//     output was unusable.
func (p *Prober) ProbeDisc(ctx context.Context, input DiscInput) (media.Description, error) {
	name := path.Base(input.DiscPath)

	prepared, err := PrepareDiscInput(input)
	if err != nil {
		return media.Description{}, fmt.Errorf("probe disc %s: %w", name, err)
	}

	defer p.closeDiscInput(ctx, prepared)

	probeContext, cancel := context.WithTimeout(ctx, p.probeTimeout)
	defer cancel()

	output, err := p.ffprobe.output(probeContext, probeArguments(prepared.arguments))
	if err != nil {
		p.logRunFailure(ctx, discProbeOperation, input.DiscPath, err)

		return media.Description{}, fmt.Errorf("probe disc %s: %w", name, classifyDiscRunError(err))
	}

	description, err := describeDisc(output, input)
	if err != nil {
		return media.Description{}, fmt.Errorf("probe disc %s: %w", name, err)
	}

	return description, nil
}

// closeDiscInput removes the list of a finished run; a failure only leaves a small file in
// the temporary directory, so it is logged, not returned.
func (p *Prober) closeDiscInput(ctx context.Context, prepared *DiscInputArguments) {
	if err := prepared.Close(); err != nil {
		p.logger.LogAttrs(ctx, slog.LevelWarn, "disc list could not be removed", slog.String("error", err.Error()))
	}
}

// describeDisc maps ffprobe's output for a disc title. The container is the clips' one (the
// concat demuxer reports "concat"), and the duration comes from the title when the disc
// structures name one: FFmpeg can only estimate it for joined VOBs and does not know it for
// a concat list. Errors wrap media.ErrDiscUnsupported.
func describeDisc(output []byte, input DiscInput) (media.Description, error) {
	description, err := describe(output, input.extension())
	if err != nil {
		return media.Description{}, fmt.Errorf("%w: %w", media.ErrDiscUnsupported, err)
	}

	description.Container = discContainer(input.Layout)

	if duration := input.Title.Duration(); duration > 0 {
		description.Duration = duration
	}

	return description, nil
}

// discContainer is the container of a folder disc's clips.
func discContainer(layout media.Layout) media.Container {
	if layout == media.LayoutVideoTS {
		return media.ContainerVOB
	}

	return media.ContainerM2TS
}

// classifyDiscRunError is classifyRunError with failures caused by the title marked as
// disc_unsupported, so a disc that FFmpeg cannot read shows that code.
func classifyDiscRunError(err error) error {
	classified := classifyRunError(err)
	if errors.Is(classified, domain.ErrUnsupported) {
		return fmt.Errorf("%w: %w", media.ErrDiscUnsupported, err)
	}

	return classified
}

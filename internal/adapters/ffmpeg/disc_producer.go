package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
)

const (
	// inheritedDescriptorDirectory and firstInheritedDescriptor name the clips FFmpeg inherits
	// on Linux: exec.Cmd.ExtraFiles[i] becomes descriptor 3+i (see inheritedInputPath).
	inheritedDescriptorDirectory = "/proc/self/fd/"
	firstInheritedDescriptor     = 3
)

// prepareDisc builds the run of a disc job: the clips of job.DiscTitle, relative to the disc
// folder job.RelativePath or extents of the image job.RelativePath, joined as
// PrepareDiscInput describes and read from StartPosition (-ss before the input, so the concat
// demuxer seeks into the right clip). On Linux FFmpeg reads every folder clip through a
// descriptor opened inside the library root, like a single file, and an image through the
// descriptor it was checked with.
func (p *Producer) prepareDisc(ctx context.Context, job appplayback.Job) (preparedRun, error) {
	input := DiscInput{
		LibraryRoot: job.LibraryRoot, DiscPath: job.RelativePath, Layout: job.Layout, Title: job.DiscTitle,
	}

	prepared, inherited, err := p.discInput(ctx, input)
	if err != nil {
		return preparedRun{}, err
	}

	release := func() { p.closeDiscInput(job.SessionID, prepared) }

	arguments, err := HLSArguments(HLSJob{
		Plan: job.Plan, Disc: prepared, StartPosition: job.StartPosition, OutputDirectory: job.OutputDir,
	})
	if err != nil {
		closeAll(inherited)
		release()

		return preparedRun{}, err
	}

	return preparedRun{arguments: arguments, inherited: inherited, release: release}, nil
}

// discInput prepares the input of a disc and returns the descriptors FFmpeg inherits, if any.
func (p *Producer) discInput(ctx context.Context, input DiscInput) (*DiscInputArguments, []*os.File, error) {
	if !p.inheritInput {
		prepared, err := PrepareDiscInput(ctx, input)

		return prepared, nil, err
	}

	if input.Layout == media.LayoutISO {
		return inheritedImageInput(ctx, input)
	}

	clips, err := input.clips()
	if err != nil {
		return nil, nil, err
	}

	urls, inherited, err := p.inheritedClipURLs(input.LibraryRoot, clips)
	if err != nil {
		return nil, nil, err
	}

	prepared, err := prepareFolderClips(input.Layout, clips, urls)
	if err != nil {
		closeAll(inherited)

		return nil, nil, err
	}

	return prepared, inherited, nil
}

// inheritedImageInput reads the image through the descriptor it was checked with, which
// FFmpeg inherits, so the file FFmpeg reads is the one opened inside the library root.
func inheritedImageInput(ctx context.Context, input DiscInput) (*DiscInputArguments, []*os.File, error) {
	image, err := openImage(ctx, input)
	if err != nil {
		return nil, nil, fmt.Errorf("prepare disc input: %w", err)
	}

	prepared, err := subfileArguments(image, fileProtocol+inheritedInputPath)
	if err != nil {
		closeQuietly(image.file)

		return nil, nil, fmt.Errorf("prepare disc input: %w", err)
	}

	return prepared, []*os.File{image.file}, nil
}

// inheritedClipURLs opens every clip inside root and returns the descriptor URL of each,
// which FFmpeg inherits.
func (p *Producer) inheritedClipURLs(root string, clips []discClip) ([]string, []*os.File, error) {
	urls := make([]string, 0, len(clips))
	inherited := make([]*os.File, 0, len(clips))

	for index, clip := range clips {
		file, err := p.inputs.Open(root, clip.relativePath)
		if err != nil {
			closeAll(inherited)

			return nil, nil, fmt.Errorf("open disc clip %d: %w", index, err)
		}

		inherited = append(inherited, file)
		urls = append(urls, fileProtocol+inheritedDescriptorDirectory+strconv.Itoa(firstInheritedDescriptor+index))
	}

	return urls, inherited, nil
}

// closeDiscInput releases the concat list of a finished production; a failure
// leaves only a small file in the temporary directory, so it is logged.
func (p *Producer) closeDiscInput(sessionID string, prepared *DiscInputArguments) {
	if err := prepared.Close(); err != nil {
		p.logger.Warn("disc input could not be released",
			slog.String("playback_id", sessionID), slog.String("error", err.Error()))
	}
}

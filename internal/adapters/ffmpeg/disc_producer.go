package ffmpeg

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
)

const (
	// inheritedDescriptorDirectory and firstInheritedDescriptor name the clips FFmpeg inherits
	// on Linux: exec.Cmd.ExtraFiles[i] becomes descriptor 3+i (see inheritedInputPath).
	inheritedDescriptorDirectory = "/proc/self/fd/"
	firstInheritedDescriptor     = 3
)

// prepareDisc builds the run of a folder disc job: the clips of job.DiscTitle, relative to the
// disc folder job.RelativePath, joined as PrepareDiscInput describes and read from StartPosition
// (-ss before the input, so the concat demuxer seeks into the right clip). On Linux FFmpeg
// reads every clip through a descriptor opened inside the library root, like a single file.
func (p *Producer) prepareDisc(job appplayback.Job) (preparedRun, error) {
	input := DiscInput{
		LibraryRoot: job.LibraryRoot, DiscPath: job.RelativePath, Layout: job.Layout, Title: job.DiscTitle,
	}

	clips, err := input.clips()
	if err != nil {
		return preparedRun{}, err
	}

	urls, inherited, err := p.discClipURLs(job.LibraryRoot, clips)
	if err != nil {
		return preparedRun{}, err
	}

	prepared, err := prepareDiscInput(input.Layout, clips, urls)
	if err != nil {
		closeAll(inherited)

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

// discClipURLs returns the URL of each clip: its path, or on Linux the descriptor of the clip
// opened inside root, which FFmpeg inherits.
func (p *Producer) discClipURLs(root string, clips []discClip) ([]string, []*os.File, error) {
	urls := make([]string, 0, len(clips))

	if !p.inheritInput {
		for _, clip := range clips {
			urls = append(urls, fileProtocol+clip.absolutePath)
		}

		return urls, nil, nil
	}

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

// closeDiscInput removes the list of a finished production; a failure leaves only a small
// file in the temporary directory, so it is logged.
func (p *Producer) closeDiscInput(sessionID string, prepared *DiscInputArguments) {
	if err := prepared.Close(); err != nil {
		p.logger.Warn("disc list could not be removed",
			slog.String("session", sessionID), slog.String("error", err.Error()))
	}
}

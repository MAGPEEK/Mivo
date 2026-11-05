package playback

import (
	"context"
	"fmt"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ErrDiscPlaybackUnavailable reports a disc while the server has no disc ports. It wraps
// domain.ErrUnsupported: the client can do nothing about it.
var ErrDiscPlaybackUnavailable = fmt.Errorf("disc playback is not configured: %w", domain.ErrUnsupported)

// DirectoryResolver turns the root-relative path of a disc folder into an absolute path,
// refusing paths that leave the library root. Implemented by adapters/fs/libraryfs
// (ResolveDirectory).
type DirectoryResolver interface {
	// ResolveDirectory returns the absolute path of a directory below root. Errors wrap
	// domain.ErrNotFound (also for a file), domain.ErrForbidden or domain.ErrUnavailable like
	// PathResolver.Resolve.
	ResolveDirectory(root, relativePath string) (string, error)
}

// DiscTitleResolver selects the main title of a disc; the catalog stores only the disc's
// layout, so the title is resolved again when a session starts. Implemented by
// adapters/disc (Resolver).
type DiscTitleResolver interface {
	// Resolve returns the title of the disc at the absolute disc root discPath
	// (media.Layout.DiscRoot). Errors wrap media.ErrDiscEncrypted, media.ErrDiscUnsupported
	// or media.ErrDiscNoTitle for a disc that cannot be played, context errors, or the I/O
	// error of reading the disc.
	Resolve(ctx context.Context, discPath string, layout media.Layout) (media.DiscTitle, error)
}

// DiscKeyframeLocator is KeyframeLocator for a disc title. Implemented by adapters/ffmpeg
// ((*Prober).DiscKeyframeAtOrBefore).
type DiscKeyframeLocator interface {
	// DiscKeyframeAtOrBefore returns the position of the last video keyframe at or before
	// position in the title of disc inside the absolute library folder libraryRoot, measured
	// like KeyframeAtOrBefore. Errors like KeyframeAtOrBefore; the service then starts at 0.
	DiscKeyframeAtOrBefore(ctx context.Context, libraryRoot string, disc media.DiscSource, position time.Duration) (time.Duration, error)
}

// DiscPorts are the ports only disc sources need (ADR 0029). Without them (zero value) a disc
// fails to start with ErrDiscPlaybackUnavailable.
type DiscPorts struct {
	Directories DirectoryResolver
	Titles      DiscTitleResolver
	Keyframes   DiscKeyframeLocator
}

func (ports DiscPorts) complete() bool {
	return ports.Directories != nil && ports.Titles != nil && ports.Keyframes != nil
}

// prepareDisc resolves the disc root of a disc file (the image, or the folder holding BDMV or
// VIDEO_TS) and its main title. A disc that cannot be played wraps domain.ErrUnsupported
// with its code in the message; the scan normally stored that code already.
func (s *Service) prepareDisc(ctx context.Context, playable PlayableFile) (string, *media.DiscSource, error) {
	if !s.discs.complete() {
		return "", nil, ErrDiscPlaybackUnavailable
	}

	file := playable.File
	disc := media.DiscSource{RelativePath: file.Layout.DiscRoot(file.RelativePath), Layout: file.Layout}

	resolve := s.discs.Directories.ResolveDirectory
	if file.Layout == media.LayoutISO {
		resolve = s.paths.Resolve
	}

	discPath, err := resolve(playable.LibraryRoot, disc.RelativePath)
	if err != nil {
		return "", nil, s.resolveFailure(ctx, file, err)
	}

	title, err := s.discs.Titles.Resolve(ctx, discPath, disc.Layout)
	if err != nil {
		return "", nil, discTitleFailure(ctx, disc.RelativePath, err)
	}

	disc.Title = title

	return discPath, &disc, nil
}

// discTitleFailure maps a disc error to domain.ErrUnsupported: media.ErrDiscNoTitle wraps
// domain.ErrNotFound, which would tell the client that the file does not exist.
func discTitleFailure(ctx context.Context, discRoot string, err error) error {
	if code, found := media.DiscErrorCode(err); found && ctx.Err() == nil {
		return fmt.Errorf("resolve disc title of %s: %s: %w", discRoot, code, domain.ErrUnsupported)
	}

	return fmt.Errorf("resolve disc title of %s: %w", discRoot, err)
}

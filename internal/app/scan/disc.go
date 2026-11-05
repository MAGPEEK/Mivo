package scan

import (
	"context"
	"fmt"
	"maps"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/media"
)

// DirectoryResolver turns the root-relative path of a disc folder into the absolute path the
// disc resolver opens, confined to the root. Implemented by adapters/fs/libraryfs
// (ResolveDirectory).
type DirectoryResolver interface {
	// ResolveDirectory returns the absolute path of a directory below root. Errors wrap
	// domain.ErrNotFound (also for a file), domain.ErrForbidden or domain.ErrUnavailable like
	// PathResolver.Resolve.
	ResolveDirectory(root, relativePath string) (string, error)
}

// DiscTitleResolver selects the main title of a disc. Implemented by adapters/disc (Resolver).
type DiscTitleResolver interface {
	// Resolve returns the title of the disc at discPath, an absolute disc root
	// (media.Layout.DiscRoot) inside a library folder. Errors wrap media.ErrDiscEncrypted,
	// media.ErrDiscUnsupported or media.ErrDiscNoTitle for a disc that cannot be played,
	// context errors, or the I/O error of reading the disc.
	Resolve(ctx context.Context, discPath string, layout media.Layout) (media.DiscTitle, error)
}

// DiscProber describes the main title of a disc. Implemented by adapters/ffmpeg
// ((*Prober).ProbeDiscTitle).
type DiscProber interface {
	// ProbeDiscTitle describes the title of disc inside the absolute library folder root.
	// Errors follow Prober.Probe; a title ffprobe cannot read wraps media.ErrDiscUnsupported.
	ProbeDiscTitle(ctx context.Context, libraryRoot string, disc media.DiscSource) (media.Description, error)
}

// DiscPorts are the ports only discs need (ADR 0029). Without them (zero value) discs are
// skipped: nothing is stored for them and the entries stored at or below a disc survive, so
// the first scan with the ports places and probes them (discHandling).
type DiscPorts struct {
	Directories DirectoryResolver
	Titles      DiscTitleResolver
	Prober      DiscProber
}

func (ports DiscPorts) complete() bool {
	return ports.Directories != nil && ports.Titles != nil && ports.Prober != nil
}

// describeDisc resolves the main title of a disc entry and probes it; the caller holds a
// probe slot. The returned path is the absolute disc root, empty when it was not resolved.
func (scanRun *run) describeDisc(ctx context.Context, job probeJob) (probeResult, string) {
	ports := scanRun.service.discs
	if !ports.complete() {
		// The walk skips discs without the ports, so this is a bug; nothing is stored.
		return probeResult{job: job, err: fail(ErrorCodeFailed, "disc without disc ports",
			fmt.Errorf("probe disc %s: disc ports missing: %w", job.state.RelativePath, domain.ErrInvalid))}, ""
	}

	layout := job.state.Layout
	discRoot := layout.DiscRoot(job.state.RelativePath)

	root, absolutePath, failure := scanRun.resolveDiscRoot(job, discRoot)
	if failure != nil {
		return *failure, ""
	}

	title, err := ports.Titles.Resolve(ctx, absolutePath, layout)
	if err != nil {
		return discProbeFailure(ctx, job, err), absolutePath
	}

	description, err := ports.Prober.ProbeDiscTitle(ctx, root, media.DiscSource{
		RelativePath: discRoot, Layout: layout, Title: title,
	})
	if err != nil {
		return discProbeFailure(ctx, job, err), absolutePath
	}

	return validatedDescription(job, description), absolutePath
}

// resolveDiscRoot resolves the disc root inside its library folder: the image file, or the
// folder that holds the BDMV or VIDEO_TS folder.
func (scanRun *run) resolveDiscRoot(job probeJob, discRoot string) (string, string, *probeResult) {
	root, failure := scanRun.folderRootOf(job)
	if failure != nil {
		return "", "", failure
	}

	resolve := scanRun.service.discs.Directories.ResolveDirectory
	if job.state.Layout == media.LayoutISO {
		resolve = scanRun.service.resolver.Resolve
	}

	absolutePath, err := resolve(root, discRoot)
	if err != nil {
		failure := resolveFailure(job, err)

		return "", "", &failure
	}

	return root, absolutePath, nil
}

// discProbeFailure stores a disc error under its own code (disc_encrypted, disc_unsupported,
// disc_no_title); every other error follows the prober's contract (probeFailure).
func discProbeFailure(ctx context.Context, job probeJob, err error) probeResult {
	if ctx.Err() == nil {
		if code, found := media.DiscProbeError(err); found {
			return discFailure(job, code, err.Error())
		}
	}

	return probeFailure(ctx, job, err)
}

func discFailure(job probeJob, code media.ProbeError, reason string) probeResult {
	return probeResult{job: job, outcome: outcomeProbeFailed, probeError: code, reason: reason}
}

// unsupportedDiscsOf returns a copy of the discs of the library this server start probed as
// disc_unsupported.
func (service *Service) unsupportedDiscsOf(libraryID string) map[fileKey]struct{} {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	return maps.Clone(service.unsupportedDiscs[libraryID])
}

// rememberUnsupportedDisc records a disc probed as disc_unsupported. The code also stands for
// an FFmpeg build that lacks a demuxer or protocol (ADR 0029), which only a restart can
// change, so the disc is probed again by the first scan after the next start; a corrupt or
// non-disc image then does not fail every scan in between.
func (service *Service) rememberUnsupportedDisc(libraryID string, state library.FileState) {
	service.mutex.Lock()
	defer service.mutex.Unlock()

	if service.unsupportedDiscs[libraryID] == nil {
		service.unsupportedDiscs[libraryID] = map[fileKey]struct{}{}
	}

	service.unsupportedDiscs[libraryID][keyOf(state)] = struct{}{}
}

// folderRootOf returns the library folder of the job's file, or the result that ends the
// scan for a file of an unknown folder.
func (scanRun *run) folderRootOf(job probeJob) (string, *probeResult) {
	root, found := folderRoot(scanRun.library, job.state.RootID)
	if !found {
		return "", &probeResult{job: job, err: fail(ErrorCodeFailed, "file of an unknown library folder",
			fmt.Errorf("probe file: folder %q: %w", job.state.RootID, domain.ErrNotFound))}
	}

	return root, nil
}

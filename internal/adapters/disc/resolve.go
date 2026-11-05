package disc

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// Image is a disc image file tree that also reports where each file lies in the image.
// *iso.Image implements it; pass img.WithContext(ctx) so reads are cancellable.
type Image interface {
	fs.FS
	Extents(name string) ([]iso.Extent, error)
}

// titleClip is one selected clip file before it is mapped to a path or image extents.
// start is the presentation time the title's span begins at inside the clip (media.DiscClip.Start).
type titleClip struct {
	path     string
	start    time.Duration
	duration time.Duration
}

// Resolver selects the main title of disc folders and images on the local file system.
type Resolver struct{}

// NewResolver returns a Resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// Resolve selects the main title of the disc at discPath: a BDMV or VIDEO_TS folder (opened as
// an os.Root, so nothing outside it is read) or an ISO image file. The caller has already
// checked that discPath lies inside a library root.
func (*Resolver) Resolve(ctx context.Context, discPath string, layout media.Layout) (media.DiscTitle, error) {
	if layout == media.LayoutISO {
		return resolveImageFile(ctx, discPath)
	}
	if !layout.IsDisc() {
		return nil, fmt.Errorf("resolve disc %s: layout %q: %w", discPath, layout, domain.ErrInvalid)
	}

	root, err := os.OpenRoot(discPath)
	if err != nil {
		return nil, fmt.Errorf("resolve disc %s: %w", discPath, err)
	}
	defer func() { _ = root.Close() }() // read-only; a close error loses nothing

	title, err := ResolveTree(ctx, root.FS(), layout)
	if err != nil {
		return nil, fmt.Errorf("resolve disc %s: %w", discPath, err)
	}

	return title, nil
}

func resolveImageFile(ctx context.Context, imagePath string) (media.DiscTitle, error) {
	imageFile, err := os.Open(imagePath) //nolint:gosec // the scan validated the path inside a library root
	if err != nil {
		return nil, fmt.Errorf("resolve disc image %s: %w", imagePath, err)
	}
	defer func() { _ = imageFile.Close() }() // read-only; a close error loses nothing

	info, err := imageFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("resolve disc image %s: %w", imagePath, err)
	}
	img, err := iso.Open(ctx, imageFile, info.Size())
	if err != nil {
		return nil, fmt.Errorf("resolve disc image %s: %w", imagePath, classifyImageError(ctx, err))
	}

	title, err := ResolveImage(ctx, img.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("resolve disc image %s: %w", imagePath, err)
	}

	return title, nil
}

// ResolveTree selects the main title of a folder disc. tree is rooted at the BDMV or VIDEO_TS
// folder or at its parent; clip paths in the result are relative to that root. A disc that
// looks CSS or AACS protected fails with media.ErrDiscEncrypted.
func ResolveTree(ctx context.Context, tree fs.FS, layout media.Layout) (media.DiscTitle, error) {
	files := newFileTree(ctx, tree)
	clips, err := selectTitle(files, layout)
	if err != nil {
		return nil, err
	}
	if err := files.checkEncryption(layout, clips); err != nil {
		return nil, err
	}

	title := make(media.DiscTitle, 0, len(clips))
	for _, clip := range clips {
		title = append(title, media.DiscClip{Path: clip.path, Start: clip.start, Duration: clip.duration})
	}

	return validated(title)
}

func selectTitle(files *fileTree, layout media.Layout) ([]titleClip, error) {
	switch layout {
	case media.LayoutBDMV:
		return selectBDMV(files)
	case media.LayoutVideoTS:
		return selectVideoTS(files)
	case media.LayoutFile, media.LayoutISO:
		return nil, notFolderDisc(layout)
	default:
		return nil, notFolderDisc(layout)
	}
}

func notFolderDisc(layout media.Layout) error {
	return fmt.Errorf("select disc title: layout %q is not a folder disc: %w", layout, domain.ErrInvalid)
}

// validated is the last line of defence: a title the domain rejects is never returned.
func validated(title media.DiscTitle) (media.DiscTitle, error) {
	if err := title.Validate(); err != nil {
		return nil, fmt.Errorf("select disc title: %w", err)
	}

	return title, nil
}

package ffmpeg

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// FFmpeg input of a disc image's main title (ADR 0029, Notes M3-16g). FFmpeg cannot read a
// file inside an ISO image, but the title is a list of byte extents of the image, and FFmpeg's
// subfile protocol reads such a range of another URL, seekable and without any network
// protocol: "subfile,,start,<first byte>,end,<byte after the last>,,:file:<image>". The
// extents then take the place of the clip files of a folder disc.
const (
	subfileProtocol = "subfile"
	// subfileURLPrefix, subfileEndOption and subfileOptionsEnd frame the options of a
	// subfile URL; the image URL follows them.
	subfileURLPrefix  = subfileProtocol + ",,start,"
	subfileEndOption  = ",end,"
	subfileOptionsEnd = ",,:"
	// imageProtocols lets a subfile URL open the image; imageProgramStreamProtocols also lets
	// the concat protocol join the DVD's extents.
	imageProtocols              = allowedProtocols + "," + subfileProtocol
	imageProgramStreamProtocols = imageProtocols + ",concat"
	// imageClipOptions follow every Blu-ray clip of a list (see concatList).
	imageClipOptions = "option format_whitelist " + transportStreamDemuxer + "\noption protocol_whitelist " + imageProtocols + "\n"
	// imageBDMVFolder and imageVideoTSFolder are the disc structures at the image root.
	imageBDMVFolder    = "BDMV"
	imageVideoTSFolder = "VIDEO_TS"
	// decimalBase formats the byte offsets of a subfile URL.
	decimalBase = 10
)

// imageSection is a byte range of the image file.
type imageSection struct {
	offset, length int64
}

// subfileURL is the URL of section inside the image at imageURL.
func (section imageSection) subfileURL(imageURL string) string {
	return subfileURLPrefix + strconv.FormatInt(section.offset, decimalBase) +
		subfileEndOption + strconv.FormatInt(section.offset+section.length, decimalBase) +
		subfileOptionsEnd + imageURL
}

// imageClips validates an image input and returns its clips. Errors wrap domain.ErrInvalid
// and media.ErrDiscNoTitle for an empty title.
func (input DiscInput) imageClips() ([]discClip, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}

	clips := make([]discClip, 0, len(input.Title))

	for index, clip := range input.Title {
		if clip.Path != "" {
			return nil, fmt.Errorf("disc clip %d: an image clip is an extent, not a path: %w", index, domain.ErrInvalid)
		}

		clips = append(clips, discClip{
			section: imageSection{offset: clip.Offset, length: clip.Length},
			start:   clip.Start, duration: clip.Duration,
		})
	}

	return clips, nil
}

// checkedImage is an image whose layout and clips were checked.
type checkedImage struct {
	file   *os.File
	layout media.Layout
	clips  []discClip
}

// openImage validates input, opens its image inside the library root (os.Root, so no link
// leads out of it), finds the disc structure with the iso reader and checks that every clip
// lies inside the image. The caller closes the file (or hands it to FFmpeg). Errors wrap
// domain.ErrInvalid for a bad input or extents beyond the image, the error of opening the
// image, media.ErrDiscUnsupported for an image without a readable disc file system or
// structure, and ctx.Err().
func openImage(ctx context.Context, input DiscInput) (checkedImage, error) {
	clips, err := input.imageClips()
	if err != nil {
		return checkedImage{}, err
	}

	file, err := os.OpenInRoot(input.LibraryRoot, filepath.FromSlash(input.DiscPath))
	if err != nil {
		return checkedImage{}, fmt.Errorf("open the disc image: %w", err)
	}

	layout, err := checkImage(ctx, file, clips)
	if err != nil {
		_ = file.Close() // read-only; the check's error is the one to report

		return checkedImage{}, err
	}

	return checkedImage{file: file, layout: layout, clips: clips}, nil
}

func checkImage(ctx context.Context, image *os.File, clips []discClip) (media.Layout, error) {
	info, err := image.Stat()
	if err != nil {
		return "", fmt.Errorf("stat the disc image: %w", err)
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("the disc image is no regular file: %w", domain.ErrInvalid)
	}

	size := info.Size()
	for index, clip := range clips {
		if clip.section.offset > size || clip.section.length > size-clip.section.offset {
			return "", fmt.Errorf("disc clip %d lies beyond the image (%d bytes): %w", index, size, domain.ErrInvalid)
		}
	}

	return imageLayout(ctx, image, size)
}

// imageLayout reads the image's file system and returns the disc structure at its root, a
// Blu-ray before a DVD like the disc resolver.
func imageLayout(ctx context.Context, image *os.File, size int64) (media.Layout, error) {
	tree, err := iso.Open(ctx, image, size)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", fmt.Errorf("read the disc image: %w", ctxErr)
		}

		return "", fmt.Errorf("read the disc image: %w: %w", media.ErrDiscUnsupported, err)
	}

	for _, candidate := range []struct {
		folder string
		layout media.Layout
	}{{imageBDMVFolder, media.LayoutBDMV}, {imageVideoTSFolder, media.LayoutVideoTS}} {
		if info, err := fs.Stat(tree, candidate.folder); err == nil && info.IsDir() {
			return candidate.layout, nil
		}
	}

	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("read the disc image: %w", err)
	}

	return "", fmt.Errorf("disc image without a BDMV or VIDEO_TS folder: %w", media.ErrDiscUnsupported)
}

// prepareImageInput checks the image of input and returns the input that reads its title from
// the image path: see subfileArguments.
func prepareImageInput(ctx context.Context, input DiscInput) (*DiscInputArguments, error) {
	image, err := openImage(ctx, input)
	if err != nil {
		return nil, err
	}

	_ = image.file.Close() // read-only and only needed for the checks; FFmpeg opens the path

	imageURL := fileProtocol + filepath.Join(input.LibraryRoot, filepath.FromSlash(input.DiscPath))

	return subfileArguments(image, imageURL)
}

// subfileArguments builds the input that reads the clips of image through subfile URLs of
// imageURL: a DVD's extents joined by the concat protocol into one program stream, a
// Blu-ray's extents as clips of a concat list. The whitelists add only subfile (and concat
// for a DVD) to file; no network protocol is ever allowed.
func subfileArguments(image checkedImage, imageURL string) (*DiscInputArguments, error) {
	urls := make([]string, 0, len(image.clips))
	for _, clip := range image.clips {
		urls = append(urls, clip.section.subfileURL(imageURL))
	}

	if image.layout == media.LayoutVideoTS {
		return &DiscInputArguments{
			arguments: []string{
				forceFormatOption, programStreamDemuxer,
				formatWhitelistOption, programStreamDemuxer,
				protocolWhitelistOption, imageProgramStreamProtocols,
				concatProtocolPrefix + joinConcatURLs(urls),
			},
			layout: image.layout,
		}, nil
	}

	return writeConcatList(concatEntries(image.clips, urls), imageClipOptions)
}

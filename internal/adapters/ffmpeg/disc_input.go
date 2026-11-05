package ffmpeg

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// FFmpeg input of a folder disc's main title (ADR 0029, Notes M3-16f). DVD title VOBs are one
// MPEG program stream cut into files, so the concat protocol joins their bytes and the mpeg
// demuxer reads the result. Blu-ray clips are separate transport streams whose timestamps
// restart, so the concat demuxer reads them one after the other from a generated list.
const (
	// concatProtocolPrefix and concatProtocolSeparator form "concat:URL1|URL2".
	concatProtocolPrefix    = "concat:"
	concatProtocolSeparator = "|"
	// programStreamProtocols lets the concat protocol open its file: parts; still no network.
	programStreamProtocols = allowedProtocols + ",concat"
	programStreamDemuxer   = "mpeg"
	transportStreamDemuxer = "mpegts"
	concatDemuxer          = "concat"
	// forceFormatOption names the demuxer instead of letting FFmpeg guess it from the content.
	forceFormatOption = "-f"
	// concatSafeOption with concatUnsafePaths lets the list name absolute paths and file:
	// URLs. Only paths built by DiscInput.clips from validated parts are written to it.
	concatSafeOption  = "-safe"
	concatUnsafePaths = "0"
	// discListDirectoryPattern names the private directory of one list (os.MkdirTemp makes it
	// 0700); discListName is the list inside it.
	discListDirectoryPattern             = "disc-title-*"
	discListName                         = "title.ffconcat"
	discListPermissions      fs.FileMode = 0o600
	discListHeader                       = "ffconcat version 1.0\n"
	// forbiddenDiscPathCharacters: "|" separates the concat protocol's URLs, line breaks end a
	// list directive and NUL ends a C string; none of them occurs in a disc's clip names.
	forbiddenDiscPathCharacters = "|\n\r\x00"
	// discRootPath is DiscPath for a disc folder that is the library root itself.
	discRootPath = "."
	// discExtensionVOB and discExtensionM2TS pick the container inside the probed family.
	discExtensionVOB  = ".vob"
	discExtensionM2TS = ".m2ts"
)

// DiscInput is the main title of a folder disc inside a library root.
type DiscInput struct {
	// LibraryRoot is the absolute library root the disc lies in.
	LibraryRoot string
	// DiscPath is the disc folder, slash-separated and relative to LibraryRoot: the folder the
	// clip paths of Title are relative to (the folder given to the disc resolver). "" or "."
	// is the library root itself.
	DiscPath string
	// Layout is media.LayoutVideoTS or media.LayoutBDMV.
	Layout media.Layout
	// Title lists the clips in playback order, as the disc resolver selected them.
	Title media.DiscTitle
}

// discClip is one validated clip of a DiscInput.
type discClip struct {
	// relativePath is slash-separated and relative to the library root, for InputOpener.
	relativePath string
	absolutePath string
	duration     time.Duration
}

// clips validates input and returns its clips. A clip path is accepted only when it is a
// clean, local, slash-separated path without characters that the concat protocol or the
// list syntax interpret, so every clip lies inside the library root. Errors wrap
// domain.ErrInvalid, media.ErrDiscNoTitle for an empty title and media.ErrDiscUnsupported
// for an ISO image (read through a loopback source, M3-16g).
func (input DiscInput) clips() ([]discClip, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}

	clips := make([]discClip, 0, len(input.Title))

	for index, clip := range input.Title {
		if !isSafeRelativePath(clip.Path) {
			return nil, fmt.Errorf("disc clip %d: path %q is not a safe relative path: %w", index, clip.Path, domain.ErrInvalid)
		}

		relative := path.Join(input.DiscPath, clip.Path)
		clips = append(clips, discClip{
			relativePath: relative,
			absolutePath: filepath.Join(input.LibraryRoot, filepath.FromSlash(relative)),
			duration:     clip.Duration,
		})
	}

	return clips, nil
}

func (input DiscInput) validate() error {
	switch {
	case input.Layout == media.LayoutISO:
		return fmt.Errorf("disc input: images are not read as folders: %w", media.ErrDiscUnsupported)
	case !input.Layout.IsDisc():
		return fmt.Errorf("disc input: layout %q is no folder disc: %w", input.Layout, domain.ErrInvalid)
	case !filepath.IsAbs(input.LibraryRoot):
		return fmt.Errorf("disc input: library root %q is not absolute: %w", input.LibraryRoot, domain.ErrInvalid)
	case isWindowsDevicePath(input.LibraryRoot):
		return fmt.Errorf("disc input: library root in the Windows device namespace: %w", domain.ErrInvalid)
	case strings.ContainsAny(input.LibraryRoot, forbiddenDiscPathCharacters):
		return fmt.Errorf("disc input: library root contains a reserved character: %w", domain.ErrInvalid)
	case input.DiscPath != "" && input.DiscPath != discRootPath && !isSafeRelativePath(input.DiscPath):
		return fmt.Errorf("disc input: disc path %q is not a safe relative path: %w", input.DiscPath, domain.ErrInvalid)
	}

	if err := input.Title.Validate(); err != nil {
		return fmt.Errorf("disc input: %w", err)
	}

	return nil
}

// isSafeRelativePath accepts a clean, local, slash-separated path. Backslashes are refused
// because Windows would read them as separators that the checks did not see.
func isSafeRelativePath(slashPath string) bool {
	return slashPath != "" &&
		!strings.ContainsAny(slashPath, forbiddenDiscPathCharacters+`\`) &&
		path.Clean(slashPath) == slashPath &&
		filepath.IsLocal(filepath.FromSlash(slashPath))
}

// extension is the clip extension that refines the probed container.
func (input DiscInput) extension() string {
	if input.Layout == media.LayoutVideoTS {
		return discExtensionVOB
	}

	return discExtensionM2TS
}

// DiscInputArguments are the FFmpeg input options of a disc title, ending with the input URL.
// Create them with PrepareDiscInput and Close them once FFmpeg has ended: a Blu-ray input
// owns a private directory with its concat list.
type DiscInputArguments struct {
	arguments []string
	directory string
}

// Close removes the list directory; it is safe on nil and on inputs without a list.
func (arguments *DiscInputArguments) Close() error {
	if arguments == nil || arguments.directory == "" {
		return nil
	}

	if err := os.RemoveAll(arguments.directory); err != nil {
		return fmt.Errorf("remove the disc list directory: %w", err)
	}

	return nil
}

// PrepareDiscInput validates input and returns the FFmpeg input that reads its clips by path
// (ADR 0029): the concat protocol for a DVD, a concat list in a new private temporary
// directory for a Blu-ray. Errors wrap those of DiscInput.clips, or the I/O error of writing
// the list.
func PrepareDiscInput(input DiscInput) (*DiscInputArguments, error) {
	clips, err := input.clips()
	if err != nil {
		return nil, fmt.Errorf("prepare disc input: %w", err)
	}

	urls := make([]string, 0, len(clips))
	for _, clip := range clips {
		urls = append(urls, fileProtocol+clip.absolutePath)
	}

	return prepareDiscInput(input.Layout, clips, urls)
}

// prepareDiscInput builds the input of clips read from urls, one URL per clip in title order.
func prepareDiscInput(layout media.Layout, clips []discClip, urls []string) (*DiscInputArguments, error) {
	if layout == media.LayoutVideoTS {
		return &DiscInputArguments{arguments: programStreamArguments(urls)}, nil
	}

	return writeConcatList(clips, urls)
}

// programStreamArguments forces the mpeg demuxer and allows exactly it, so the bytes of the
// joined VOBs can select no other demuxer.
func programStreamArguments(urls []string) []string {
	return []string{
		forceFormatOption, programStreamDemuxer,
		formatWhitelistOption, programStreamDemuxer,
		protocolWhitelistOption, programStreamProtocols,
		concatProtocolPrefix + strings.Join(urls, concatProtocolSeparator),
	}
}

// writeConcatList writes the list into a new private directory. The outer whitelist allows
// only the concat demuxer for the list itself; each clip is reopened with the whitelists of a
// single MPEG-TS file (see concatList).
func writeConcatList(clips []discClip, urls []string) (*DiscInputArguments, error) {
	directory, err := os.MkdirTemp("", discListDirectoryPattern)
	if err != nil {
		return nil, fmt.Errorf("prepare disc input: create the list directory: %w", err)
	}

	listPath := filepath.Join(directory, discListName)
	if err := os.WriteFile(listPath, []byte(concatList(clips, urls)), discListPermissions); err != nil {
		_ = os.RemoveAll(directory) // best effort; the write error is the one to report

		return nil, fmt.Errorf("prepare disc input: write the list: %w", err)
	}

	return &DiscInputArguments{
		arguments: []string{
			forceFormatOption, concatDemuxer, concatSafeOption, concatUnsafePaths,
			formatWhitelistOption, concatDemuxer,
			protocolWhitelistOption, allowedProtocols,
			fileProtocol + listPath,
		},
		directory: directory,
	}, nil
}

// concatList returns the ffconcat list of urls. The option lines replace the whitelists the
// concat demuxer copies into each clip: with "concat" still allowed, a clip whose content is
// itself an ffconcat list could open further local files. The duration lines (when the title
// knows them) let FFmpeg seek into a later clip without opening the ones before it.
func concatList(clips []discClip, urls []string) string {
	var list strings.Builder

	list.WriteString(discListHeader)

	for index, url := range urls {
		list.WriteString("file " + quoteConcatToken(url) + "\n")
		list.WriteString("option format_whitelist " + transportStreamDemuxer + "\n")
		list.WriteString("option protocol_whitelist " + allowedProtocols + "\n")

		if duration := clips[index].duration; duration > 0 {
			list.WriteString("duration " + formatSeconds(duration) + "\n")
		}
	}

	return list.String()
}

// quoteConcatToken quotes text for the list syntax: inside single quotes every character is
// literal, and a single quote is written as '\”.
func quoteConcatToken(text string) string {
	return "'" + strings.ReplaceAll(text, "'", `'\''`) + "'"
}

// inputArguments returns the ffmpeg input options of job, ending with -i and the input URL.
func (job HLSJob) inputArguments() ([]string, error) {
	if job.Disc == nil {
		return ffmpegInputArguments(job.InputPath)
	}

	if len(job.Disc.arguments) == 0 {
		return nil, fmt.Errorf("disc input was not prepared: %w", domain.ErrInvalid)
	}

	arguments := slices.Clone(job.Disc.arguments)

	return slices.Insert(arguments, len(arguments)-1, "-i"), nil
}

package ffmpeg

import (
	"context"
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

// FFmpeg input of a disc's main title (ADR 0029, Notes M3-16f and M3-16g). DVD title VOBs are
// one MPEG program stream cut into files, so their bytes are joined (the concat protocol for
// a folder, over subfile ranges for an image) and the mpeg demuxer reads the result.
// Blu-ray clips are separate transport streams whose timestamps restart, so the concat
// demuxer reads them one after the other from a generated list.
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
	// concatSafeOption with concatUnsafePaths lets the list name absolute paths and URLs. Only
	// URLs built by this package from validated clips are written to it.
	concatSafeOption  = "-safe"
	concatUnsafePaths = "0"
	// discListDirectoryPattern names the private directory of one list (os.MkdirTemp makes it
	// 0700 on Unix; on Windows it inherits the ACL of the user's temporary directory);
	// discListName is the list inside it.
	discListDirectoryPattern             = "disc-title-*"
	discListName                         = "title.ffconcat"
	discListPermissions      fs.FileMode = 0o600
	discListHeader                       = "ffconcat version 1.0\n"
	// folderClipOptions follow every clip of a folder disc's list: they replace the whitelists
	// the concat demuxer copies into the clip (see concatList).
	folderClipOptions = "option format_whitelist " + transportStreamDemuxer + "\noption protocol_whitelist " + allowedProtocols + "\n"
	// forbiddenDiscPathCharacters: "|" separates the concat protocol's URLs, line breaks end a
	// list directive and NUL ends a C string; none of them occurs in a disc's clip names.
	forbiddenDiscPathCharacters = "|\n\r\x00"
	// discRootPath is DiscPath for a disc folder that is the library root itself.
	discRootPath = "."
	// discExtensionVOB and discExtensionM2TS pick the container inside the probed family.
	discExtensionVOB  = ".vob"
	discExtensionM2TS = ".m2ts"
)

// DiscInput is the main title of a disc inside a library root: a disc folder or an image.
type DiscInput struct {
	// LibraryRoot is the absolute library root the disc lies in.
	LibraryRoot string
	// DiscPath is slash-separated and relative to LibraryRoot. For a folder disc it is the
	// folder the clip paths of Title are relative to (the folder given to the disc resolver);
	// "" or "." is the library root itself. For media.LayoutISO it is the image file.
	DiscPath string
	// Layout is media.LayoutVideoTS, media.LayoutBDMV or media.LayoutISO.
	Layout media.Layout
	// Title lists the clips in playback order, as the disc resolver selected them: paths for
	// a folder disc, image extents for an image.
	Title media.DiscTitle
}

// discClip is one validated clip of a DiscInput.
type discClip struct {
	// relativePath is slash-separated and relative to the library root, for InputOpener; set
	// with absolutePath for a folder disc.
	relativePath string
	absolutePath string
	// section locates the clip in an image.
	section  imageSection
	start    time.Duration
	duration time.Duration
}

// clips validates a folder disc input and returns its clips. A clip path is accepted only
// when it is a clean, local, slash-separated path without characters that the concat
// protocol or the list syntax interpret, so every clip lies inside the library root. Errors
// wrap domain.ErrInvalid and media.ErrDiscNoTitle for an empty title.
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
			start:        clip.Start,
			duration:     clip.Duration,
		})
	}

	return clips, nil
}

func (input DiscInput) validate() error {
	switch {
	case !input.Layout.IsDisc():
		return fmt.Errorf("disc input: layout %q is no disc: %w", input.Layout, domain.ErrInvalid)
	case !filepath.IsAbs(input.LibraryRoot):
		return fmt.Errorf("disc input: library root %q is not absolute: %w", input.LibraryRoot, domain.ErrInvalid)
	case isWindowsDevicePath(input.LibraryRoot):
		return fmt.Errorf("disc input: library root in the Windows device namespace: %w", domain.ErrInvalid)
	case strings.ContainsAny(input.LibraryRoot, forbiddenDiscPathCharacters):
		return fmt.Errorf("disc input: library root contains a reserved character: %w", domain.ErrInvalid)
	case !input.validDiscPath():
		return fmt.Errorf("disc input: disc path %q is not a safe relative path: %w", input.DiscPath, domain.ErrInvalid)
	}

	if err := input.Title.Validate(); err != nil {
		return fmt.Errorf("disc input: %w", err)
	}

	return nil
}

// validDiscPath accepts the library root itself only for a folder disc: an image is a file.
func (input DiscInput) validDiscPath() bool {
	if input.Layout == media.LayoutISO {
		return input.DiscPath != discRootPath && isSafeRelativePath(input.DiscPath)
	}

	return input.DiscPath == "" || input.DiscPath == discRootPath || isSafeRelativePath(input.DiscPath)
}

// isSafeRelativePath accepts a clean, local, slash-separated path. Backslashes are refused
// because Windows would read them as separators that the checks did not see.
func isSafeRelativePath(slashPath string) bool {
	return slashPath != "" &&
		!strings.ContainsAny(slashPath, forbiddenDiscPathCharacters+`\`) &&
		path.Clean(slashPath) == slashPath &&
		filepath.IsLocal(filepath.FromSlash(slashPath))
}

// discExtension is the clip extension of a folder layout; it refines the probed container.
func discExtension(layout media.Layout) string {
	if layout == media.LayoutVideoTS {
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
	// layout is the folder layout of the clips: the input's own, or the one found in an image.
	layout media.Layout
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

// PrepareDiscInput validates input and returns the FFmpeg input that reads its title (ADR
// 0029): the concat protocol for a DVD and a concat list in a new private temporary directory
// for a Blu-ray, reading the clips by path for a folder disc and as subfile ranges of the
// image for an image (see subfileArguments). ctx bounds the preparation only. Errors wrap those
// of DiscInput.clips and openImage, or the I/O error of writing the list.
func PrepareDiscInput(ctx context.Context, input DiscInput) (*DiscInputArguments, error) {
	var (
		prepared *DiscInputArguments
		err      error
	)

	if input.Layout == media.LayoutISO {
		prepared, err = prepareImageInput(ctx, input)
	} else {
		prepared, err = prepareFolderInput(input)
	}

	if err != nil {
		return nil, fmt.Errorf("prepare disc input: %w", err)
	}

	return prepared, nil
}

func prepareFolderInput(input DiscInput) (*DiscInputArguments, error) {
	clips, err := input.clips()
	if err != nil {
		return nil, err
	}

	urls := make([]string, 0, len(clips))
	for _, clip := range clips {
		urls = append(urls, fileProtocol+clip.absolutePath)
	}

	return prepareFolderClips(input.Layout, clips, urls)
}

// prepareFolderClips builds the input of folder clips read from urls (file: paths or
// inherited descriptors), one URL per clip in title order.
func prepareFolderClips(layout media.Layout, clips []discClip, urls []string) (*DiscInputArguments, error) {
	if layout == media.LayoutVideoTS {
		return &DiscInputArguments{arguments: programStreamArguments(urls), layout: layout}, nil
	}

	return writeConcatList(concatEntries(clips, urls), folderClipOptions)
}

// programStreamArguments forces the mpeg demuxer and allows exactly it, so the bytes of the
// joined VOBs can select no other demuxer.
func programStreamArguments(urls []string) []string {
	return []string{
		forceFormatOption, programStreamDemuxer,
		formatWhitelistOption, programStreamDemuxer,
		protocolWhitelistOption, programStreamProtocols,
		concatProtocolPrefix + joinConcatURLs(urls),
	}
}

// joinConcatURLs joins the parts of a concat protocol URL; no part contains the separator.
func joinConcatURLs(urls []string) string {
	return strings.Join(urls, concatProtocolSeparator)
}

// concatEntry is one clip of a concat list.
type concatEntry struct {
	url             string
	start, duration time.Duration
}

func concatEntries(clips []discClip, urls []string) []concatEntry {
	entries := make([]concatEntry, 0, len(clips))
	for index, clip := range clips {
		entries = append(entries, concatEntry{url: urls[index], start: clip.start, duration: clip.duration})
	}

	return entries
}

// writeConcatList writes the list of a Blu-ray title into a new private directory. The outer
// whitelist allows only the concat demuxer and the file protocol for the list itself; each
// clip is reopened with clipOptions (see concatList).
func writeConcatList(entries []concatEntry, clipOptions string) (*DiscInputArguments, error) {
	directory, err := os.MkdirTemp("", discListDirectoryPattern)
	if err != nil {
		return nil, fmt.Errorf("create the list directory: %w", err)
	}

	listPath := filepath.Join(directory, discListName)
	if err := os.WriteFile(listPath, []byte(concatList(entries, clipOptions)), discListPermissions); err != nil {
		_ = os.RemoveAll(directory) // best effort; the write error is the one to report

		return nil, fmt.Errorf("write the list: %w", err)
	}

	return &DiscInputArguments{
		arguments: []string{
			forceFormatOption, concatDemuxer, concatSafeOption, concatUnsafePaths,
			formatWhitelistOption, concatDemuxer,
			protocolWhitelistOption, allowedProtocols,
			fileProtocol + listPath,
		},
		directory: directory,
		layout:    media.LayoutBDMV,
	}, nil
}

// concatList returns the ffconcat list of entries. The clipOptions lines replace the
// whitelists the concat demuxer copies into each clip: with "concat" still allowed, a clip
// whose content is itself an ffconcat list could open further files. The duration lines
// (when the title knows them) let FFmpeg seek into a later clip without opening the ones
// before it. A clip with a known span (a Blu-ray play item's IN and OUT time) is cut to it
// with inpoint and outpoint, which are timestamps of the clip, so the parts the playlist
// skips are neither played nor overlap the next clip.
func concatList(entries []concatEntry, clipOptions string) string {
	var list strings.Builder

	list.WriteString(discListHeader)

	for _, entry := range entries {
		list.WriteString("file " + quoteConcatToken(entry.url) + "\n")
		list.WriteString(clipOptions)

		if entry.start > 0 && entry.duration > 0 {
			list.WriteString("inpoint " + formatSeconds(entry.start) + "\n")
			list.WriteString("outpoint " + formatSeconds(entry.start+entry.duration) + "\n")
		}

		if entry.duration > 0 {
			list.WriteString("duration " + formatSeconds(entry.duration) + "\n")
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

package ffmpeg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// HLS modes of the fake tool: it plays FFmpeg's HLS muxer with the producer's arguments. It
// copies the input it was given into its only segment, so tests can tell which file the
// producer handed over (a path or an inherited descriptor).
const (
	// fakeModeHLSLive writes one segment and keeps running like a long transcode.
	fakeModeHLSLive = "hls-live"
	// fakeModeHLSComplete writes one segment, ends the playlist and exits cleanly.
	fakeModeHLSComplete = "hls-complete"
	// fakeModeHLSFailAfterSegment writes one segment, then fails like a broken input.
	fakeModeHLSFailAfterSegment = "hls-fail-after-segment"
	// fakeModeHLSHeaderOnly writes a playlist without segments and keeps running.
	fakeModeHLSHeaderOnly = "hls-header-only"
	// fakeModeHLSLostFinalRename writes one segment, then the final playlist only under its
	// temporary name, and exits cleanly: FFmpeg on Windows when a reader held the playlist
	// during its last rename.
	fakeModeHLSLostFinalRename = "hls-lost-final-rename"

	// fakeSegmentDelay holds the first segment back, so Ready has to poll for it.
	fakeSegmentDelay = 300 * time.Millisecond
	// fakeTemporarySuffix mirrors the temp_file flag: files appear complete under their name.
	fakeTemporarySuffix = ".tmp"
	fakeSegmentName     = "seg-00000.m4s"
	fakePlaylistHeader  = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:0\n" +
		"#EXT-X-PLAYLIST-TYPE:EVENT\n#EXT-X-INDEPENDENT-SEGMENTS\n#EXT-X-MAP:URI=\"init.mp4\"\n"
	fakePlaylistSegment = "#EXTINF:6.000000,\n" + fakeSegmentName + "\n"
	fakePlaylistEnd     = "#EXT-X-ENDLIST\n"
)

// isFakeHLSMode reports the modes fakeHLS handles.
func isFakeHLSMode(mode string) bool {
	return slices.Contains([]string{
		fakeModeHLSLive, fakeModeHLSComplete, fakeModeHLSFailAfterSegment, fakeModeHLSHeaderOnly,
		fakeModeHLSLostFinalRename,
	}, mode)
}

// fakeHLS writes the HLS output the mode asks for into the directory of the playlist URL,
// the last argument, and ends as the mode says.
func fakeHLS(mode string, arguments []string) int {
	if err := recordFakeArguments(arguments); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	if err := writeFakeHLS(mode, arguments); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	switch mode {
	case fakeModeHLSComplete, fakeModeHLSLostFinalRename:
		return 0
	case fakeModeHLSFailAfterSegment:
		fmt.Fprint(os.Stderr, fakeFailStderr)

		return fakeFailExitCode
	default:
		time.Sleep(fakeHangDuration)

		return 0
	}
}

func writeFakeHLS(mode string, arguments []string) error {
	directory, input, err := fakeHLSPaths(arguments)
	if err != nil {
		return err
	}

	time.Sleep(fakeSegmentDelay)

	playlist := fakePlaylistHeader

	if mode != fakeModeHLSHeaderOnly {
		// The input comes from the producer under test: a file in its temporary library or
		// the inherited descriptor.
		data, err := os.ReadFile(input) //nolint:gosec // see above
		if err != nil {
			return fmt.Errorf("read input: %w", err)
		}

		if err := writeFakeFile(directory, fakeSegmentName, data); err != nil {
			return err
		}

		playlist += fakePlaylistSegment
	}

	if mode == fakeModeHLSComplete {
		playlist += fakePlaylistEnd
	}

	if err := writeFakeFile(directory, hlsPlaylistName, []byte(playlist)); err != nil {
		return err
	}

	if mode != fakeModeHLSLostFinalRename {
		return nil
	}

	// The rename of the final playlist is the one that failed.
	final := filepath.Join(directory, hlsPlaylistName+fakeTemporarySuffix)
	if err := os.WriteFile(final, []byte(playlist+fakePlaylistEnd), 0o600); err != nil { //nolint:gosec // see writeFakeFile
		return fmt.Errorf("write the final playlist: %w", err)
	}

	return nil
}

// fakeHLSPaths returns the output directory and the input path of an HLSArguments list.
func fakeHLSPaths(arguments []string) (string, string, error) {
	inputFlag := slices.Index(arguments, "-i")
	if inputFlag < 0 || inputFlag+1 >= len(arguments) {
		return "", "", errors.New("no -i input in the arguments")
	}

	input := strings.TrimPrefix(arguments[inputFlag+1], fileProtocol)
	playlist := strings.TrimPrefix(arguments[len(arguments)-1], fileProtocol)

	return filepath.Dir(filepath.FromSlash(playlist)), input, nil
}

// writeFakeFile writes under a temporary name and renames, like FFmpeg's temp_file flag.
func writeFakeFile(directory, name string, data []byte) error {
	path := filepath.Join(directory, name)

	// The directory comes from the producer under test, through the arguments.
	if err := os.WriteFile(path+fakeTemporarySuffix, data, 0o600); err != nil { //nolint:gosec // see above
		return fmt.Errorf("write %s: %w", name, err)
	}

	if err := os.Rename(path+fakeTemporarySuffix, path); err != nil { //nolint:gosec // see above
		return fmt.Errorf("rename %s: %w", name, err)
	}

	return nil
}

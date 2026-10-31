package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	// wholeFileRange asks for every byte as a range, so one request proves range support
	// (206) and fetches the file to probe.
	wholeFileRange = "bytes=0-"
	// codeStreamNotReady is the problem code of an HLS file requested before the first
	// segment exists (503 with Retry-After).
	codeStreamNotReady = "stream_not_ready"
	// maxPlaylistBytes bounds a playlist; a 30-second clip lists a handful of segments.
	maxPlaylistBytes = 1 << 20
)

var (
	errBadContentRange = errors.New("unexpected Content-Range")
	errShortBody       = errors.New("the body is shorter than announced")
	errTooLarge        = errors.New("the response exceeds the size limit")
)

// streamDownloader fetches the stream of a session the way a player does.
type streamDownloader struct {
	client *apiClient
	poll   pollSettings
}

// direct fetches the original file of a direct-play session with a range request
// over the whole file and writes it to destination. The answer must be 206 with a
// Content-Range that covers the file, and the body must be complete.
func (downloader streamDownloader) direct(ctx context.Context, target *url.URL, destination io.Writer) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("build the range request: %w", err)
	}

	request.Header.Set("Range", wholeFileRange)

	response, err := downloader.client.http.Do(request)
	if err != nil {
		return fmt.Errorf("range request %s: %w", target, err)
	}

	defer closeBody(response)

	if response.StatusCode != http.StatusPartialContent {
		return newProblemError(response)
	}

	size, err := wholeFileSize(response.Header.Get("Content-Range"))
	if err != nil {
		return err
	}

	written, err := io.Copy(destination, response.Body)
	if err != nil {
		return fmt.Errorf("download %s: %w", target, err)
	}

	if written != size {
		return fmt.Errorf("%w: %d of %d bytes", errShortBody, written, size)
	}

	return nil
}

// wholeFileSize parses "bytes 0-<last>/<size>" and checks that the range covers the file.
func wholeFileSize(contentRange string) (int64, error) {
	span, total, found := strings.Cut(strings.TrimPrefix(contentRange, "bytes "), "/")
	first, last, isSpan := strings.Cut(span, "-")

	size, sizeErr := strconv.ParseInt(total, 10, 64)
	end, endErr := strconv.ParseInt(last, 10, 64)

	if !found || !isSpan || first != "0" || sizeErr != nil || endErr != nil || size <= 0 || end != size-1 {
		return 0, fmt.Errorf("%w %q, want bytes 0-<size-1>/<size>", errBadContentRange, contentRange)
	}

	return size, nil
}

// hls polls the playlist until FFmpeg finished it (EXT-X-ENDLIST), then writes the
// init segment and every media segment in order to destination, which ffprobe reads as one
// fragmented MP4.
func (downloader streamDownloader) hls(ctx context.Context, playlistURL *url.URL, destination io.Writer) error {
	playlist, err := downloader.finishedPlaylist(ctx, playlistURL)
	if err != nil {
		return err
	}

	for _, reference := range append([]string{playlist.initURI}, playlist.segments...) {
		target, err := resolve(playlistURL, reference)
		if err != nil {
			return err
		}

		if err := downloader.fetchStreamFile(ctx, target, destination); err != nil {
			return err
		}
	}

	return nil
}

func (downloader streamDownloader) finishedPlaylist(ctx context.Context, playlistURL *url.URL) (mediaPlaylist, error) {
	for {
		var text strings.Builder

		buffer := &limitedWriter{writer: &text, remaining: maxPlaylistBytes}
		if err := downloader.fetchStreamFile(ctx, playlistURL, buffer); err != nil {
			return mediaPlaylist{}, err
		}

		playlist, err := parsePlaylist(text.String())
		if err != nil {
			return mediaPlaylist{}, fmt.Errorf("playlist %s: %w", playlistURL, err)
		}

		if playlist.ended {
			return playlist, nil
		}

		if err := downloader.poll.wait(ctx, 0); err != nil {
			return mediaPlaylist{}, fmt.Errorf("wait for the end of the playlist: %w", err)
		}
	}
}

// fetchStreamFile GETs one file of an HLS session into destination. While the first
// segment is not ready, the server answers 503 stream_not_ready; the request is repeated
// after its Retry-After.
func (downloader streamDownloader) fetchStreamFile(ctx context.Context, target *url.URL, destination io.Writer) error {
	for {
		err := downloader.fetchOnce(ctx, target, destination)

		problem, isProblem := errors.AsType[*problemError](err)
		if !isProblem || problem.code != codeStreamNotReady {
			return err
		}

		if err := downloader.poll.wait(ctx, problem.retryAfter); err != nil {
			return fmt.Errorf("wait for %s: %w", target, err)
		}
	}
}

func (downloader streamDownloader) fetchOnce(ctx context.Context, target *url.URL, destination io.Writer) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return fmt.Errorf("build GET %s: %w", target, err)
	}

	response, err := downloader.client.http.Do(request)
	if err != nil {
		return fmt.Errorf("GET %s: %w", target, err)
	}

	defer closeBody(response)

	if response.StatusCode != http.StatusOK {
		return newProblemError(response)
	}

	if _, err := io.Copy(destination, response.Body); err != nil {
		return fmt.Errorf("download %s: %w", target, err)
	}

	return nil
}

// limitedWriter fails once more than remaining bytes are written, so a broken server
// cannot fill memory with an endless playlist.
type limitedWriter struct {
	writer    io.Writer
	remaining int
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.remaining {
		return 0, fmt.Errorf("%w of %d bytes", errTooLarge, maxPlaylistBytes)
	}

	w.remaining -= len(data)

	written, err := w.writer.Write(data)
	if err != nil {
		return written, fmt.Errorf("buffer the playlist: %w", err)
	}

	return written, nil
}

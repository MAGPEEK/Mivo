package main

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/platform/brand"
)

const (
	envThrottleAhead = brand.EnvPrefix + "_TRANSCODE_THROTTLE_AHEAD_SECONDS"

	// throttleAheadSeconds is the smallest lead the configuration accepts; the generated clip
	// is far longer, so FFmpeg is well ahead of a client that stopped reading.
	throttleAheadSeconds = "20"

	// throttleCap forces a transcode of the long clip.
	throttleCap = 100_000

	// proofOfPause is how long a paused session must stay paused before the test believes it.
	proofOfPause = 2 * time.Second
)

// startThrottledTranscode starts a transcode of the long clip, reads the playlist and the
// first segment like a player that then stops (a closed tab), and waits until the server has
// paused FFmpeg. Linux only: the pause needs job control signals (ADR 0030 section 2).
func startThrottledTranscode(t *testing.T) (streamingServer, apigen.PlaybackSession) {
	t.Helper()

	if runtime.GOOS != "linux" {
		t.Skip("skipping: pausing FFmpeg needs Linux job control signals (run it in Docker, see CLAUDE.md)")
	}

	server := startStreamingServer(t, streamingLibrary(t, longClipName), map[string]string{envThrottleAhead: throttleAheadSeconds})
	clip := server.file(t, longClipName)

	session := server.mustStartSession(t, clip.ID.String(), sessionOptions{maxBitRate: throttleCap})
	if session.Mode != apigen.PlaybackModeTranscode {
		t.Fatalf("session mode = %s, want a transcode", session.Mode)
	}

	fetchBody(t, server.baseURL+session.StreamURL, http.StatusOK)
	fetchBody(t, segmentURL(server.baseURL+session.StreamURL), http.StatusOK)

	awaitPaused(t, server, session, true)

	return server, session
}

// awaitPaused polls the session list until the session's paused flag has the wanted value.
func awaitPaused(t *testing.T, server streamingServer, session apigen.PlaybackSession, want bool) {
	t.Helper()

	deadline := time.Now().Add(pauseWait)

	for {
		var list apigen.PlaybackSessionList

		getJSON(t, server.baseURL+"/api/v1/playback/sessions", &list)

		for _, listed := range list.Sessions {
			if listed.ID == session.ID && listed.Paused != nil && *listed.Paused == want {
				return
			}
		}

		if time.Now().After(deadline) {
			t.Fatalf("session %s did not reach paused=%t within %s: %+v", session.ID, want, pauseWait, list.Sessions)
		}

		time.Sleep(pollInterval)
	}
}

// TestThrottlePausesATranscodeThatTheClientStoppedReading: (f) FFmpeg is paused when it is far
// ahead of a client that stopped requesting segments, stays paused, and resumes when the client
// catches up.
func TestThrottlePausesATranscodeThatTheClientStoppedReading(t *testing.T) {
	t.Parallel()

	server, session := startThrottledTranscode(t)

	time.Sleep(proofOfPause)
	awaitPaused(t, server, session, true)

	// The client comes back and reads the newest segment the playlist lists: the lead is gone.
	playlistURL := server.baseURL + session.StreamURL
	playlist := fetchBody(t, playlistURL, http.StatusOK)

	var newest string

	for line := range strings.SplitSeq(playlist, "\n") {
		if strings.HasPrefix(line, "seg-") && strings.HasSuffix(line, ".m4s") {
			newest = line
		}
	}

	if newest == "" {
		t.Fatalf("playlist lists no segment:\n%s", playlist)
	}

	fetchBody(t, strings.TrimSuffix(playlistURL, "index.m3u8")+newest, http.StatusOK)
	awaitPaused(t, server, session, false)

	server.stop(t)
}

// TestShutdownKillsAPausedFFmpeg: a paused FFmpeg is stopped and reaped within the playback
// close bound, and nothing of the session stays behind (M4-15, ADR 0030 section 2).
func TestShutdownKillsAPausedFFmpeg(t *testing.T) {
	t.Parallel()

	server, _ := startThrottledTranscode(t)
	transcode := server.environment[envPathsTranscode]

	if running := ffmpegProcessesUsing(t, transcode); len(running) == 0 {
		t.Fatalf("no ffmpeg process works in %s although the session is paused", transcode)
	}

	started := time.Now()
	server.stop(t)

	if took := time.Since(started); took > playbackCloseTimeout {
		t.Errorf("shutdown with a paused FFmpeg took %s, want at most %s", took, playbackCloseTimeout)
	}

	if left := ffmpegProcessesUsing(t, transcode); len(left) != 0 {
		t.Errorf("ffmpeg processes survived the shutdown: %v", left)
	}

	assertNoSessionDirectories(t, transcode)
}

// ffmpegProcessesUsing lists the process IDs whose command line names the directory, read
// from /proc.
func ffmpegProcessesUsing(t *testing.T, directory string) []string {
	t.Helper()

	commandLines, err := filepath.Glob("/proc/[0-9]*/cmdline")
	if err != nil {
		t.Fatalf("glob /proc: %v", err)
	}

	var found []string

	for _, path := range commandLines {
		content, err := os.ReadFile(path) //nolint:gosec // paths come from the /proc glob
		if err != nil {
			continue // the process ended meanwhile
		}

		command := strings.ReplaceAll(string(content), "\x00", " ")
		if strings.Contains(command, "ffmpeg") && strings.Contains(command, directory) {
			found = append(found, filepath.Base(filepath.Dir(path)))
		}
	}

	return found
}

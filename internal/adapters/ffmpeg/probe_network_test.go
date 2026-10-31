package ffmpeg_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// segmentPlaceholder is replaced by the URL of the test server in the manifests below.
const segmentPlaceholder = "SEGMENT_URL"

// dashManifest is a DASH manifest; FFmpeg detects it by content, so it is probed as DASH
// even when the file is named .mkv.
const dashManifest = `<?xml version="1.0" encoding="UTF-8"?>
<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" profiles="urn:mpeg:dash:profile:isoff-on-demand:2011"
     type="static" mediaPresentationDuration="PT2S" minBufferTime="PT1S">
  <Period>
    <AdaptationSet mimeType="video/mp4">
      <Representation id="1" bandwidth="1000" codecs="avc1.640028">
        <BaseURL>SEGMENT_URL</BaseURL>
      </Representation>
    </AdaptationSet>
  </Period>
</MPD>
`

// hlsPlaylist is an HLS media playlist. FFmpeg only detects HLS with an .m3u8 or .m3u
// extension, so the library walker never hands one to the prober; the case guards the
// prober on its own.
const hlsPlaylist = `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:2
#EXTINF:2.0,
SEGMENT_URL
#EXT-X-ENDLIST
`

// TestProbeNeverFollowsNetworkReferences: a file whose content makes FFmpeg open further
// URLs must not reach the network (SSRF, ADR 0022). The control run opens the same file
// without the demuxer whitelist and with http and tcp added to the protocol whitelist; a
// request it sends proves that the local FFmpeg has the demuxer and follows the reference.
// The prober then runs with both of its restrictions and must send no request, so a pass
// means they held, not that the demuxer is missing: -format_whitelist refuses dash and hls
// before they read the manifest, and -protocol_whitelist file would refuse the URL even if a
// demuxer got that far. The control run cannot tell the two apart; hostile_corpus_test.go
// checks FFmpeg's whitelist error for the demuxer restriction.
func TestProbeNeverFollowsNetworkReferences(t *testing.T) {
	ffprobePath := requireFFprobe(t)

	tests := []struct {
		name     string
		fileName string
		content  string
	}{
		{name: "DASH manifest named mkv", fileName: "x.mkv", content: dashManifest},
		{name: "HLS playlist", fileName: "x.m3u8", content: hlsPlaylist},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32

			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				http.NotFound(writer, request)
			}))
			t.Cleanup(server.Close)

			path := filepath.Join(t.TempDir(), test.fileName)
			content := strings.ReplaceAll(test.content, segmentPlaceholder, server.URL+"/segment.mp4")

			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatalf("write %s: %v", test.fileName, err)
			}

			requireNetworkReference(t, ffprobePath, path, &requests)
			requests.Store(0)

			_, err := newCorpusProber(t, ffprobePath).Probe(t.Context(), path)
			if !errors.Is(err, domain.ErrUnsupported) {
				t.Errorf("Probe error = %v, want ErrUnsupported", err)
			}

			if count := requests.Load(); count != 0 {
				t.Errorf("the server received %d requests, want none", count)
			}
		})
	}
}

// requireNetworkReference runs ffprobe without the demuxer whitelist and with the network
// allowed, and skips the test when the local FFmpeg does not contact the server even then
// (no demuxer for the format).
func requireNetworkReference(t *testing.T, ffprobePath, path string, requests *atomic.Int32) {
	t.Helper()

	// The executable comes from PATH and the arguments are a slice; no shell is involved.
	control := exec.CommandContext(t.Context(), ffprobePath, //nolint:gosec // see above
		"-v", "quiet", "-protocol_whitelist", "file,http,tcp", "-show_format", "file:"+path)

	// The control run fails because the server answers 404; only the request matters.
	_ = control.Run()

	if requests.Load() == 0 {
		t.Skipf("the local FFmpeg does not open the URL in %s even with the network allowed "+
			"(demuxer missing); nothing to protect against", filepath.Base(path))
	}
}

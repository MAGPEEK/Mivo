package ffmpeg

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
)

// Shortened real `ffmpeg -hide_banner -decoders` and `-demuxers` output (FFmpeg 9.0.2).
const (
	decodersWithAll = `Decoders:
 V..... = Video
 ...X.. = Codec is experimental
 ------
 V....D av1                  Alliance for Open Media AV1
 VFS..D hevc                 HEVC (High Efficiency Video Coding)
 V..... libdav1d             dav1d AV1 decoder by VideoLAN (codec av1)
 VF...D webp                 WebP image
`
	decodersHardwareAV1Only = `Decoders:
 ------
 V....D av1                  Alliance for Open Media AV1
 V..X.D libaom-av1           libaom AV1 (codec av1)
`
	demuxersWithMov = `Formats:
 D.. = Demuxing supported
 ---
 D   mov,mp4,m4a,3gp,3g2,mj2 QuickTime / MOV
 D   webp_pipe       piped webp sequence
`
	demuxersWithoutMov = `Formats:
 ---
 D   webp_pipe       piped webp sequence
 .E  mov             QuickTime / MOV
`
)

// fakeListings answers -decoders and -demuxers with fixed output.
func fakeListings(decoders, demuxers string) commandOutput {
	return func(_ context.Context, arguments []string) ([]byte, error) {
		if arguments[len(arguments)-1] == decodersQuery {
			return []byte(decoders), nil
		}

		return []byte(demuxers), nil
	}
}

func TestDetectPhotoDecodersFromListings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		decoders, demuxers string
		want               PhotoDecoders
	}{
		{name: "full build", decoders: decodersWithAll, demuxers: demuxersWithMov, want: PhotoDecoders{HEIC: true, AVIF: true}},
		{name: "hardware or experimental AV1 only", decoders: decodersHardwareAV1Only, demuxers: demuxersWithMov},
		{name: "no ISOBMFF demuxer", decoders: decodersWithAll, demuxers: demuxersWithoutMov},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := detectPhotoDecoders(t.Context(), fakeListings(test.decoders, test.demuxers))
			if err != nil || got != test.want {
				t.Errorf("detectPhotoDecoders = %+v, %v; want %+v", got, err, test.want)
			}
		})
	}
}

func TestDetectPhotoDecodersFailures(t *testing.T) {
	t.Parallel()

	failing := func(context.Context, []string) ([]byte, error) {
		return nil, &exitError{tool: ffmpegToolName, code: 1}
	}

	if got, err := detectPhotoDecoders(t.Context(), failing); !errors.Is(err, domain.ErrUnavailable) || got != (PhotoDecoders{}) {
		t.Errorf("failing ffmpeg = %+v, %v; want zero value and ErrUnavailable", got, err)
	}

	garbage := fakeListings("not a listing", demuxersWithMov)
	if got, err := detectPhotoDecoders(t.Context(), garbage); !errors.Is(err, domain.ErrUnavailable) || got != (PhotoDecoders{}) {
		t.Errorf("unparsable output = %+v, %v; want zero value and ErrUnavailable", got, err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	cancelled := func(ctx context.Context, _ []string) ([]byte, error) { return nil, ctx.Err() }
	if _, err := detectPhotoDecoders(ctx, cancelled); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled detection = %v, want context.Canceled", err)
	}
}

func TestDetectPhotoDecodersValidatesAndLogs(t *testing.T) {
	t.Parallel()

	if _, err := DetectPhotoDecoders(t.Context(), "ffmpeg", nil); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("nil logger = %v, want ErrInvalid", err)
	}

	var logs bytes.Buffer

	got, err := DetectPhotoDecoders(t.Context(), "", slog.New(slog.NewJSONHandler(&logs, nil)))
	if !errors.Is(err, domain.ErrUnavailable) || got != (PhotoDecoders{}) {
		t.Errorf("missing executable = %+v, %v; want zero value and ErrUnavailable", got, err)
	}

	if warnings := strings.Count(logs.String(), `"level":"WARN"`); warnings != 1 {
		t.Errorf("warnings = %d, want 1", warnings)
	}
}

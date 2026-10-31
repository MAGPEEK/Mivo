package stream

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestWithStartAtZero(t *testing.T) {
	t.Parallel()

	const ffmpegBody = "#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:6\n#EXT-X-PLAYLIST-TYPE:EVENT\n" +
		"#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:6.000000,\nseg-00000.m4s\n"

	tests := map[string]struct {
		playlist, want string
	}{
		"typical ffmpeg playlist": {
			playlist: "#EXTM3U\n" + ffmpegBody,
			want:     "#EXTM3U\n#EXT-X-START:TIME-OFFSET=0\n" + ffmpegBody,
		},
		"crlf line ends": {
			playlist: "#EXTM3U\r\n#EXT-X-VERSION:7\r\n#EXTINF:6.0,\r\nseg-00000.m4s\r\n",
			want:     "#EXTM3U\r\n#EXT-X-START:TIME-OFFSET=0\r\n#EXT-X-VERSION:7\r\n#EXTINF:6.0,\r\nseg-00000.m4s\r\n",
		},
		"header only without newline": {
			playlist: "#EXTM3U",
			want:     "#EXTM3U\n#EXT-X-START:TIME-OFFSET=0",
		},
		"header only with newline": {
			playlist: "#EXTM3U\n",
			want:     "#EXTM3U\n#EXT-X-START:TIME-OFFSET=0\n",
		},
		"existing start tag": {
			playlist: "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-START:TIME-OFFSET=12.5,PRECISE=YES\n#EXTINF:6.0,\nseg-00000.m4s\n",
			want:     "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-START:TIME-OFFSET=12.5,PRECISE=YES\n#EXTINF:6.0,\nseg-00000.m4s\n",
		},
		"existing start tag on the last line without newline": {
			playlist: "#EXTM3U\r\n#EXT-X-START:TIME-OFFSET=-6",
			want:     "#EXTM3U\r\n#EXT-X-START:TIME-OFFSET=-6",
		},
		"not starting with the header": {
			playlist: "#EXT-X-VERSION:7\n#EXTM3U\n",
			want:     "#EXT-X-VERSION:7\n#EXTM3U\n",
		},
		"header with trailing characters": {
			playlist: "#EXTM3Ux\n#EXT-X-VERSION:7\n",
			want:     "#EXTM3Ux\n#EXT-X-VERSION:7\n",
		},
		"header with a lone carriage return": {
			playlist: "#EXTM3U\r#EXT-X-VERSION:7\n",
			want:     "#EXTM3U\r#EXT-X-VERSION:7\n",
		},
		"leading whitespace": {
			playlist: " #EXTM3U\n",
			want:     " #EXTM3U\n",
		},
		"empty file": {playlist: "", want: ""},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			original := []byte(test.playlist)
			got := withStartAtZero(original)

			if string(got) != test.want {
				t.Errorf("withStartAtZero(%q) = %q, want %q", test.playlist, got, test.want)
			}

			if string(original) != test.playlist {
				t.Errorf("withStartAtZero modified its input to %q", original)
			}

			if strings.Count(string(got), startTagPrefix) > 1 {
				t.Errorf("withStartAtZero(%q) has more than one start tag: %q", test.playlist, got)
			}
		})
	}
}

func TestReadBounded(t *testing.T) {
	t.Parallel()

	const limit = 64

	tests := map[string]struct {
		size     int
		sizeHint int64
		wantErr  error
	}{
		"empty":                         {size: 0, sizeHint: 0},
		"below the limit":               {size: limit - 1, sizeHint: limit - 1},
		"exactly the limit":             {size: limit, sizeHint: limit},
		"one byte over the limit":       {size: limit + 1, sizeHint: limit + 1, wantErr: errPlaylistTooLarge},
		"far over the limit":            {size: limit * 10, sizeHint: limit * 10, wantErr: errPlaylistTooLarge},
		"grew after the size was taken": {size: limit, sizeHint: 1},
		"grew beyond the limit":         {size: limit * 10, sizeHint: 1, wantErr: errPlaylistTooLarge},
		"negative size hint":            {size: limit / 2, sizeHint: -1},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			content := bytes.Repeat([]byte{'a'}, test.size)
			// One byte per read makes every growth step of the buffer visible.
			body, err := readBounded(iotest.OneByteReader(bytes.NewReader(content)), test.sizeHint, limit)

			if !errors.Is(err, test.wantErr) {
				t.Fatalf("readBounded error = %v, want %v", err, test.wantErr)
			}

			if test.wantErr == nil && !bytes.Equal(body, content) {
				t.Errorf("readBounded = %d bytes, want %d", len(body), len(content))
			}

			if cap(body) > limit+1 {
				t.Errorf("readBounded allocated %d bytes, want at most %d", cap(body), limit+1)
			}
		})
	}
}

var errDiskRead = errors.New("disk read failed")

func TestReadBoundedReportsReadErrors(t *testing.T) {
	t.Parallel()

	_, err := readBounded(io.MultiReader(strings.NewReader("#EXTM3U\n"), iotest.ErrReader(errDiskRead)), 0, maxPlaylistBytes)
	if !errors.Is(err, errDiskRead) {
		t.Errorf("readBounded error = %v, want %v", err, errDiskRead)
	}
}

func TestMaxPlaylistBytesIsFourMebibytes(t *testing.T) {
	t.Parallel()

	const fourMebibytes = 4 << 20

	if maxPlaylistBytes != fourMebibytes {
		t.Errorf("maxPlaylistBytes = %d, want %d (ADR 0024 note M2-37)", maxPlaylistBytes, fourMebibytes)
	}
}

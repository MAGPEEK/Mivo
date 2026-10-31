package main

import (
	"errors"
	"slices"
	"testing"
)

func TestParsePlaylist(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		text         string
		wantEnded    bool
		wantInit     string
		wantSegments []string
		wantErr      error
	}{
		{
			name:      "finished with CRLF and attributes after the URI",
			text:      "#EXTM3U\r\n#EXT-X-VERSION:7\r\n#EXT-X-MAP:URI=\"init.mp4\",BYTERANGE=\"720@0\"\r\n#EXTINF:2.0,\r\nseg-00000.m4s\r\n#EXTINF:1.5,\r\nseg-00001.m4s\r\n#EXT-X-ENDLIST\r\n",
			wantEnded: true, wantInit: "init.mp4", wantSegments: []string{"seg-00000.m4s", "seg-00001.m4s"},
		},
		{
			name:     "running",
			text:     "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:2.0,\nseg-00000.m4s\n",
			wantInit: "init.mp4", wantSegments: []string{"seg-00000.m4s"},
		},
		{name: "running before the first segment", text: "#EXTM3U\n#EXT-X-VERSION:7\n"},
		{name: "quoted comma in the URI", text: "#EXTM3U\n#EXT-X-MAP:BYTERANGE=\"1@0\",URI=\"a,b.mp4\"\n", wantInit: "a,b.mp4"},
		{name: "no header", text: "seg-00000.m4s\n", wantErr: errNotAPlaylist},
		{name: "segments without init", text: "#EXTM3U\nseg-00000.m4s\n", wantErr: errNoInitSegment},
		{name: "finished without init", text: "#EXTM3U\n#EXT-X-ENDLIST\n", wantErr: errNoInitSegment},
		{name: "finished without segments", text: "#EXTM3U\n#EXT-X-MAP:URI=\"init.mp4\"\n#EXT-X-ENDLIST\n", wantErr: errNoMediaSegment},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			playlist, err := parsePlaylist(test.text)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("parsePlaylist error = %v, want %v", err, test.wantErr)
			}

			if playlist.ended != test.wantEnded || playlist.initURI != test.wantInit || !slices.Equal(playlist.segments, test.wantSegments) {
				t.Errorf("parsePlaylist = %+v, want ended %t, init %q, segments %q",
					playlist, test.wantEnded, test.wantInit, test.wantSegments)
			}
		})
	}
}

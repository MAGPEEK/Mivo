package main

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// Fixtures are the verbatim output of FFmpeg 9.0.2 (gyan.dev full build).
const (
	encodersFixture = "ffmpeg-9.0.2-encoders.txt"
	muxersFixture   = "ffmpeg-9.0.2-muxers.txt"
	versionFixture  = "ffmpeg version 9.0.2-full_build-www.gyan.dev Copyright (c) 2000-2026 the FFmpeg developers\n" +
		"built with gcc 16.2.0 (Rev3, Built by MSYS2 project)\n"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}

	return data
}

func TestParseCapabilityListReadsRealEncoderOutput(t *testing.T) {
	t.Parallel()

	encoders, err := parseCapabilityList(strings.NewReader(string(readFixture(t, encodersFixture))))
	if err != nil {
		t.Fatalf("parse encoders: %v", err)
	}

	for _, want := range []string{"libx264", "libx265", "aac", "ac3", "rv20", "real_144", "libsvtav1", "a64multi", "wmav2"} {
		if _, ok := encoders[want]; !ok {
			t.Errorf("encoder %q missing from parsed list", want)
		}
	}

	// Legend lines and descriptions must not leak into the names.
	for _, unwanted := range []string{"Encoders:", "V.....", "=", "Video", "------"} {
		if _, ok := encoders[unwanted]; ok {
			t.Errorf("parsed list contains legend token %q", unwanted)
		}
	}

	const minimumEncoders = 200
	if len(encoders) < minimumEncoders {
		t.Errorf("parsed %d encoders, want at least %d", len(encoders), minimumEncoders)
	}
}

func TestParseCapabilityListReadsRealMuxerOutput(t *testing.T) {
	t.Parallel()

	muxers, err := parseCapabilityList(strings.NewReader(string(readFixture(t, muxersFixture))))
	if err != nil {
		t.Fatalf("parse muxers: %v", err)
	}

	wanted := []string{
		"matroska", "mp4", "ipod", "mov", "avi", "mpegts", "webm", "flv", "asf", "ogv", "3gp",
		"mpeg", "vob", "mxf", "rm", "3g2",
		// Aliases listed in one entry are both usable with -f.
		"stream_segment", "ssegment",
	}
	for _, want := range wanted {
		if _, ok := muxers[want]; !ok {
			t.Errorf("muxer %q missing from parsed list", want)
		}
	}

	for _, unwanted := range []string{"Formats:", "D..", "E", "---", "stream_segment,ssegment"} {
		if _, ok := muxers[unwanted]; ok {
			t.Errorf("parsed list contains unexpected token %q", unwanted)
		}
	}
}

func TestParseCapabilityList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr error
	}{
		{
			name:  "windows line endings",
			input: "Formats:\r\n E = Muxing\r\n ---\r\n  E  mp4             MP4\r\n  E  webm            WebM\r\n",
			want:  []string{"mp4", "webm"},
		},
		{
			name:  "short and blank lines after the separator are ignored",
			input: " ------\n\n V\n V....D libx264              libx264 H.264\n",
			want:  []string{"libx264"},
		},
		{
			name:    "no separator",
			input:   "ffmpeg: unrecognized option '-muxers'\n",
			wantErr: errNoCapabilityList,
		},
		{
			name:    "empty output",
			input:   "",
			wantErr: errNoCapabilityList,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseCapabilityList(strings.NewReader(test.input))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}

			if len(got) != len(test.want) {
				t.Fatalf("got %d names %v, want %v", len(got), got, test.want)
			}

			for _, name := range test.want {
				if _, ok := got[name]; !ok {
					t.Errorf("name %q missing in %v", name, got)
				}
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{name: "gyan build", output: versionFixture, want: "9.0.2-full_build-www.gyan.dev"},
		{name: "debian build", output: "ffmpeg version 7.1.1-1+b1 Copyright (c) 2000-2025\n", want: "7.1.1-1+b1"},
		{name: "unexpected output", output: "something else entirely\n", want: "unknown"},
		{name: "empty", output: "", want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := parseVersion([]byte(test.output)); got != test.want {
				t.Errorf("parseVersion = %q, want %q", got, test.want)
			}
		})
	}
}

func TestMissingRequirements(t *testing.T) {
	t.Parallel()

	caps := capabilitiesOf([]string{"libx264", "aac"}, []string{"matroska"})

	tests := []struct {
		name string
		c    mediaCase
		want string
	}{
		{
			name: "everything available",
			c:    mediaCase{Encoders: []string{"libx264", "aac"}, Muxer: "matroska"},
			want: "",
		},
		{
			name: "missing encoders and muxer are all named",
			c:    mediaCase{Encoders: []string{"libx265", "aac", "ac3"}, Muxer: "mxf"},
			want: "ffmpeg lacks encoder libx265, encoder ac3, muxer mxf",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := caps.missingRequirements(test.c); got != test.want {
				t.Errorf("missingRequirements = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDetectCapabilitiesUsesFFmpegListings(t *testing.T) {
	t.Parallel()

	ffmpeg := newFixtureFFmpeg(t)

	caps, err := detectCapabilities(context.Background(), ffmpeg)
	if err != nil {
		t.Fatalf("detectCapabilities: %v", err)
	}

	if caps.version != "9.0.2-full_build-www.gyan.dev" {
		t.Errorf("version = %q", caps.version)
	}

	if _, ok := caps.encoders["libx264"]; !ok {
		t.Error("libx264 not detected")
	}

	if _, ok := caps.muxers["matroska"]; !ok {
		t.Error("matroska not detected")
	}
}

func TestDetectCapabilitiesReportsFailures(t *testing.T) {
	t.Parallel()

	failingListing := errors.New("listing failed")

	tests := []struct {
		name     string
		failArgs string
		garbage  string
	}{
		{name: "version", failArgs: "-version"},
		{name: "encoders", failArgs: "-hide_banner -encoders"},
		{name: "muxers", failArgs: "-hide_banner -muxers"},
		{name: "unparsable muxers", garbage: "-hide_banner -muxers"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ffmpeg := newFixtureFFmpeg(t)
			if test.failArgs != "" {
				ffmpeg.listingErrors[test.failArgs] = failingListing
			}

			if test.garbage != "" {
				ffmpeg.listings[test.garbage] = []byte("not a list\n")
			}

			if _, err := detectCapabilities(context.Background(), ffmpeg); err == nil {
				t.Fatal("detectCapabilities succeeded, want an error")
			}
		})
	}
}

func capabilitiesOf(encoders, muxers []string) capabilities {
	caps := capabilities{version: "test", encoders: map[string]struct{}{}, muxers: map[string]struct{}{}}
	for _, name := range encoders {
		caps.encoders[name] = struct{}{}
	}

	for _, name := range muxers {
		caps.muxers[name] = struct{}{}
	}

	return caps
}

package main

import (
	"path"
	"slices"
	"strings"
	"testing"
	"time"
)

func findCase(t *testing.T, name string) mediaCase {
	t.Helper()

	for _, c := range allCases() {
		if c.Name == name {
			return c
		}
	}

	t.Fatalf("case %q not in registry", name)

	return mediaCase{}
}

func TestBuildArgsForRepresentativeCases(t *testing.T) {
	t.Parallel()

	reproducible := []string{"-map_metadata", "-1", "-fflags", "+bitexact", "-flags", "+bitexact"}
	prefix := []string{"-hide_banner", "-nostdin", "-loglevel", "error", "-y"}

	tests := []struct {
		name string
		want []string
	}{
		{
			name: "video-mkv-h264-aac",
			want: slices.Concat(prefix,
				[]string{
					"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
					"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
					"-map", "0:v:0", "-map", "1:a:0", "-ac", "2",
				},
				reproducible,
				[]string{
					"-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-pix_fmt", "yuv420p",
					"-c:a", "aac",
					"-t", "2", "-f", "matroska", "out.partial",
				}),
		},
		{
			// RealAudio 1.0 forces an 8 kHz mono source; the muxer differs from the extension.
			name: "video-rmvb-rv20-ra144",
			want: slices.Concat(prefix,
				[]string{
					"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=2",
					"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=8000:duration=2",
					"-map", "0:v:0", "-map", "1:a:0", "-ac", "1",
				},
				reproducible,
				[]string{
					"-c:v", "rv20", "-pix_fmt", "yuv420p", "-c:a", "real_144",
					"-t", "2", "-f", "rm", "out.partial",
				}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := buildArgs(findCase(t, test.name), "out.partial")
			if !slices.Equal(got, test.want) {
				t.Errorf("buildArgs =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}

func TestBuildArgsForHDR10TagsFramesAndAddsMetadata(t *testing.T) {
	t.Parallel()

	args := strings.Join(buildArgs(findCase(t, "video-mkv-hevc10-hdr10-ac3"), "out.partial"), " ")

	for _, want := range []string{
		"-vf setparams=color_primaries=bt2020:color_trc=smpte2084:colorspace=bt2020nc",
		"-pix_fmt yuv420p10le",
		"-profile:v main10",
		"hdr10=1",
		"master-display=G(13250,34500)",
		"max-cll=1000,400",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("HDR10 args lack %q:\n%s", want, args)
		}
	}
}

func TestSourceArgsWithSingleInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		source sourceSpec
		want   []string
	}{
		{
			name:   "audio only maps the first input",
			source: sourceSpec{SampleRate: 44100, Channels: 2, Duration: 5 * time.Second},
			want: []string{
				"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=5",
				"-map", "0:a:0", "-ac", "2",
			},
		},
		{
			name:   "video only",
			source: sourceSpec{Width: 64, Height: 48, FrameRate: 10, Duration: 500 * time.Millisecond},
			want: []string{
				"-f", "lavfi", "-i", "testsrc2=size=64x48:rate=10:duration=0.5",
				"-map", "0:v:0",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := sourceArgs(test.source); !slices.Equal(got, test.want) {
				t.Errorf("sourceArgs = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRegistryCasesAreConsistent(t *testing.T) {
	t.Parallel()

	outputByName := map[string]string{}
	nameByOutput := map[string]string{}

	for _, c := range allCases() {
		checkCaseShape(t, c)

		if output, seen := outputByName[c.Name]; seen && output != c.Output {
			t.Errorf("alternatives of %s write different files: %s and %s", c.Name, output, c.Output)
		}

		if name, seen := nameByOutput[c.Output]; seen && name != c.Name {
			t.Errorf("%s and %s both write %s", name, c.Name, c.Output)
		}

		outputByName[c.Name] = c.Output
		nameByOutput[c.Output] = c.Name
	}
}

func checkCaseShape(t *testing.T, c mediaCase) {
	t.Helper()

	if !strings.HasPrefix(c.Name, c.Category+"-") {
		t.Errorf("%s: name does not start with category %q", c.Name, c.Category)
	}

	if path.Dir(c.Output) != c.Category || path.Ext(c.Output) == "" || strings.Contains(c.Output, `\`) {
		t.Errorf("%s: output %q must be <category>/<file>.<ext> with forward slashes", c.Name, c.Output)
	}

	if c.Muxer == "" || len(c.Encoders) == 0 || c.Expect.Container == "" || c.Expect.ProbeFormat == "" {
		t.Errorf("%s: muxer, encoders and expected container are required", c.Name)
	}

	if c.Source.Duration != clipDuration {
		t.Errorf("%s: video clips last %v, got %v", c.Name, clipDuration, c.Source.Duration)
	}

	if (c.Expect.Video != nil) != c.Source.hasVideo() || (c.Expect.Audio != nil) != c.Source.hasAudio() {
		t.Errorf("%s: expected streams do not match the source", c.Name)
	}
}

func TestRegistryCoversRequiredVideoContainers(t *testing.T) {
	t.Parallel()

	extensions := map[string]bool{}
	for _, c := range allCases() {
		extensions[path.Ext(c.Output)] = true
	}

	required := []string{
		".mkv", ".mp4", ".m4v", ".mov", ".avi", ".ts", ".m2ts", ".mts", ".webm", ".flv", ".wmv",
		".ogv", ".3gp", ".mpg", ".vob", ".mxf", ".rm", ".rmvb",
	}
	for _, extension := range required {
		if !extensions[extension] {
			t.Errorf("no case writes a %s file", extension)
		}
	}
}

func TestExpectedFillsPropertiesFromSource(t *testing.T) {
	t.Parallel()

	c := findCase(t, "video-3gp-h263-aac")
	got := c.expected()

	if got.Video.Width != cifWidth || got.Video.Height != cifHeight {
		t.Errorf("video size = %dx%d, want CIF", got.Video.Width, got.Video.Height)
	}

	if got.Audio.SampleRate != defaultSampleRate || got.Audio.Channels != stereoChannels {
		t.Errorf("audio = %d Hz/%d ch", got.Audio.SampleRate, got.Audio.Channels)
	}

	if c.Expect.Video.Width != 0 {
		t.Error("expected() modified the shared case expectation")
	}
}

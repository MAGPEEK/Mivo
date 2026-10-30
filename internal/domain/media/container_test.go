package media_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// ffprobe reports these format names for whole container families.
const (
	probeMatroska = "matroska,webm"
	probeISOBMFF  = "mov,mp4,m4a,3gp,3g2,mj2"
)

func TestIdentifyContainerCoversTheCorpus(t *testing.T) {
	t.Parallel()

	// One row per video file of the corpus manifest (scripts/gen-testmedia/cases_*.go):
	// the ffprobe format_name and the extension of the generated file.
	tests := []struct {
		name       string
		formatName string
		extension  string
		want       media.Container
	}{
		{name: "mkv", formatName: probeMatroska, extension: ".mkv", want: media.ContainerMKV},
		{name: "webm", formatName: probeMatroska, extension: ".webm", want: media.ContainerWebM},
		{name: "mp4", formatName: probeISOBMFF, extension: ".mp4", want: media.ContainerMP4},
		{name: "m4v", formatName: probeISOBMFF, extension: ".m4v", want: media.ContainerM4V},
		{name: "mov", formatName: probeISOBMFF, extension: ".mov", want: media.ContainerMOV},
		{name: "3gp", formatName: probeISOBMFF, extension: ".3gp", want: media.Container3GP},
		{name: "avi xvid", formatName: "avi", extension: ".avi", want: media.ContainerAVI},
		{name: "mpegts", formatName: "mpegts", extension: ".ts", want: media.ContainerMPEGTS},
		{name: "m2ts", formatName: "mpegts", extension: ".m2ts", want: media.ContainerM2TS},
		{name: "mts is m2ts", formatName: "mpegts", extension: ".mts", want: media.ContainerM2TS},
		{name: "flv", formatName: "flv", extension: ".flv", want: media.ContainerFLV},
		{name: "ogv", formatName: "ogg", extension: ".ogv", want: media.ContainerOGG},
		{name: "mpg", formatName: "mpeg", extension: ".mpg", want: media.ContainerMPEG},
		{name: "vob", formatName: "mpeg", extension: ".vob", want: media.ContainerVOB},
		{name: "wmv is asf", formatName: "asf", extension: ".wmv", want: media.ContainerASF},
		{name: "mxf", formatName: "mxf", extension: ".mxf", want: media.ContainerMXF},
		{name: "rm", formatName: "rm", extension: ".rm", want: media.ContainerRM},
		{name: "rmvb is rm", formatName: "rm", extension: ".rmvb", want: media.ContainerRM},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := media.IdentifyContainer(test.formatName, test.extension)
			if got != test.want {
				t.Errorf("IdentifyContainer(%q, %q) = %q, want %q",
					test.formatName, test.extension, got, test.want)
			}
		})
	}
}

func TestIdentifyContainerNormalisesInputAndUsesFamilyDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		formatName string
		extension  string
		want       media.Container
	}{
		{name: "extension without dot", formatName: probeMatroska, extension: "webm", want: media.ContainerWebM},
		{name: "upper-case extension", formatName: "mpegts", extension: ".M2TS", want: media.ContainerM2TS},
		{name: "upper-case vob", formatName: "mpeg", extension: ".VOB", want: media.ContainerVOB},
		{name: "upper-case format name", formatName: "AVI", extension: ".avi", want: media.ContainerAVI},
		{name: "spaces around tokens", formatName: " matroska , webm ", extension: ".mkv", want: media.ContainerMKV},
		{name: "mk3d is mkv", formatName: probeMatroska, extension: ".mk3d", want: media.ContainerMKV},
		{name: "matroska without extension", formatName: probeMatroska, extension: "", want: media.ContainerMKV},
		{name: "isobmff without extension", formatName: probeISOBMFF, extension: "", want: media.ContainerMP4},
		{name: "isobmff with 3g2", formatName: probeISOBMFF, extension: ".3g2", want: media.Container3GP},
		{name: "isobmff with m4a", formatName: probeISOBMFF, extension: ".m4a", want: media.ContainerMP4},
		{name: "mpegts without extension", formatName: "mpegts", extension: "", want: media.ContainerMPEGTS},
		{name: "mpeg with mpeg extension", formatName: "mpeg", extension: ".mpeg", want: media.ContainerMPEG},
		{name: "vob format name", formatName: "vob", extension: ".mpg", want: media.ContainerVOB},
		{name: "asf with asf extension", formatName: "asf", extension: ".asf", want: media.ContainerASF},
		{name: "divx extension in avi", formatName: "avi", extension: ".divx", want: media.ContainerAVI},
		{name: "known token after unknown", formatName: "foo,avi", extension: ".avi", want: media.ContainerAVI},
		{name: "extension cannot overrule format", formatName: probeMatroska, extension: ".mp4", want: media.ContainerMKV},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := media.IdentifyContainer(test.formatName, test.extension)
			if got != test.want {
				t.Errorf("IdentifyContainer(%q, %q) = %q, want %q",
					test.formatName, test.extension, got, test.want)
			}
		})
	}
}

func TestIdentifyContainerReturnsUnknownForOtherInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		formatName string
		extension  string
	}{
		{name: "empty", formatName: "", extension: ""},
		{name: "extension only", formatName: "", extension: ".mkv"},
		{name: "audio format", formatName: "wav", extension: ".wav"},
		{name: "mp3 claiming mkv", formatName: "mp3", extension: ".mkv"},
		{name: "image format", formatName: "png_pipe", extension: ".png"},
		{name: "separators only", formatName: ",,", extension: ".avi"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := media.IdentifyContainer(test.formatName, test.extension)
			if got != media.ContainerUnknown {
				t.Errorf("IdentifyContainer(%q, %q) = %q, want %q",
					test.formatName, test.extension, got, media.ContainerUnknown)
			}
		})
	}
}

func TestContainersListsTheADRNamesInOrder(t *testing.T) {
	t.Parallel()

	// ADR 0023: mkv webm mp4 m4v mov 3gp avi mpegts m2ts flv ogg mpeg vob asf mxf rm.
	want := []string{
		"mkv", "webm", "mp4", "m4v", "mov", "3gp", "avi", "mpegts",
		"m2ts", "flv", "ogg", "mpeg", "vob", "asf", "mxf", "rm",
	}

	got := media.Containers()
	if len(got) != len(want) {
		t.Fatalf("Containers() has %d entries, want %d: %v", len(got), len(want), got)
	}

	for index, container := range got {
		if string(container) != want[index] {
			t.Errorf("Containers()[%d] = %q, want %q", index, container, want[index])
		}
	}

	got[0] = media.ContainerUnknown
	if media.Containers()[0] != media.ContainerMKV {
		t.Error("Containers() returned shared storage; a caller's change leaked")
	}
}

func TestParseContainerRoundTripsKnownNames(t *testing.T) {
	t.Parallel()

	known := append(media.Containers(), media.ContainerUnknown)
	for _, container := range known {
		t.Run(string(container), func(t *testing.T) {
			t.Parallel()

			got, err := media.ParseContainer(string(container))
			if err != nil {
				t.Fatalf("ParseContainer(%q) error = %v, want nil", container, err)
			}

			if got != container {
				t.Errorf("ParseContainer(%q) = %q, want %q", container, got, container)
			}
		})
	}
}

func TestParseContainerRejectsOtherText(t *testing.T) {
	t.Parallel()

	for _, text := range []string{"", "MKV", " mkv", "matroska,webm", "wav", "ts"} {
		t.Run(text, func(t *testing.T) {
			t.Parallel()

			got, err := media.ParseContainer(text)
			if !errors.Is(err, domain.ErrInvalid) {
				t.Fatalf("ParseContainer(%q) error = %v, want ErrInvalid", text, err)
			}

			if got != media.ContainerUnknown {
				t.Errorf("ParseContainer(%q) = %q on error, want %q", text, got, media.ContainerUnknown)
			}
		})
	}
}

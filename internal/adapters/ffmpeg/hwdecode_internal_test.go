package ffmpeg

import (
	"errors"
	"slices"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

func TestDetectHardwareVerifiesDecodeSeparately(t *testing.T) {
	executable, _ := hardwareFixture(t, "ffmpeg-9.0.2-encoders.txt", "ffmpeg-9.0.2-hwaccels.txt",
		map[string]fakeHardwareOutcome{
			nvencName + fakeDecodeSuffix: {Exit: 1, Stderr: "Cannot load libcuda.so.1\n"},
		})
	pattern, _ := renderNodePattern(t)

	results, err := DetectHardwareResults(t.Context(), testOptions(executable, pattern))
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	// NVENC encodes but its decode test failed; QSV and VAAPI passed both; AMF never decodes.
	want := map[playback.HardwareAPI]bool{
		playback.HardwareAPINVENC: false, playback.HardwareAPIQSV: true,
		playback.HardwareAPIVAAPI: true, playback.HardwareAPIAMF: false,
	}

	for _, result := range results {
		if !result.Encoder.Usable {
			t.Errorf("%s must stay usable when only its decode failed", result.Encoder.Name)
		}

		if result.Encoder.Decode != want[result.Encoder.API] {
			t.Errorf("%s decode = %v, want %v", result.Encoder.Name, result.Encoder.Decode, want[result.Encoder.API])
		}
	}
}

func TestDetectHardwareWithoutASampleDecodesInSoftware(t *testing.T) {
	executable, _ := hardwareFixture(t, "ffmpeg-9.0.2-encoders.txt", "ffmpeg-9.0.2-hwaccels.txt",
		map[string]fakeHardwareOutcome{"libx264": {Exit: 1, Stderr: "Unknown encoder 'libx264'\n"}})
	pattern, _ := renderNodePattern(t)

	results, err := DetectHardwareResults(t.Context(), testOptions(executable, pattern))
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	for _, result := range results {
		if !result.Encoder.Usable || result.Encoder.Decode {
			t.Errorf("%s = %+v, want usable without hardware decode", result.Encoder.Name, result.Encoder)
		}
	}
}

func TestDetectHardwareRejectsADeviceThatCouldInjectOptions(t *testing.T) {
	executable, _ := hardwareFixture(t, "ffmpeg-9.0.2-encoders.txt", "ffmpeg-9.0.2-hwaccels.txt", nil)

	for _, device := range []string{"/dev/dri/renderD128,vendor=x", "/dev/dri/renderD128:0"} {
		options := testOptions(executable, "")
		options.Device = device

		if _, err := DetectHardwareResults(t.Context(), options); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("device %q: error = %v, want ErrInvalid", device, err)
		}
	}
}

func TestDecodeTestArgumentsMatchASession(t *testing.T) {
	encoder := playback.HardwareEncoder{
		API: playback.HardwareAPIVAAPI, Name: vaapiName, Device: "/dev/dri/renderD128", Decode: true,
	}
	want := []string{
		"-hide_banner", "-nostdin", "-v", "error", "-vaapi_device", "/dev/dri/renderD128", "-hwaccel", "vaapi",
		"-hwaccel_device", "/dev/dri/renderD128", "-f", "h264", "-i", "-", "-vf", "format=nv12,hwupload",
		"-c:v", vaapiName, "-f", "null", "-",
	}

	if got := decodeTestArguments(encoder); !slices.Equal(got, want) {
		t.Fatalf("arguments = %v, want %v", got, want)
	}
}

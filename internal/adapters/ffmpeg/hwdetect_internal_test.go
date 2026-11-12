package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// fakeHardwareVariable names the JSON file that configures fakeHardware.
	fakeHardwareVariable = "MIVO_FFMPEG_FAKE_HARDWARE"

	nvencName = "h264_nvenc"
	qsvName   = "h264_qsv"
	vaapiName = "h264_vaapi"
	amfName   = "h264_amf"

	// fakeDecodeSuffix selects the outcome of an encoder's decode test.
	fakeDecodeSuffix = "+decode"
)

// fakeHardwareConfig is what the fake prints and how each encoder's test encode ends.
type fakeHardwareConfig struct {
	Encoders string                         `json:"encoders"`
	Hwaccels string                         `json:"hwaccels"`
	Outcomes map[string]fakeHardwareOutcome `json:"outcomes"`
}

type fakeHardwareOutcome struct {
	Stderr string `json:"stderr"`
	Exit   int    `json:"exit"`
	Hang   bool   `json:"hang"`
}

// fakeHardware plays an FFmpeg for hardware detection: the two listings come from the
// configuration, a test encode ends as configured for its encoder (default: success).
func fakeHardware(arguments []string) int {
	if err := recordFakeArguments(arguments); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	data, err := os.ReadFile(os.Getenv(fakeHardwareVariable)) //nolint:gosec // the path comes from the test itself
	if err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	var config fakeHardwareConfig
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Fprintln(os.Stderr, err)

		return 1
	}

	listing := ""

	switch {
	case slices.Contains(arguments, encodersQuery):
		listing = config.Encoders
	case slices.Contains(arguments, hwaccelsQuery):
		listing = config.Hwaccels
	default:
		return fakeHardwareEncode(arguments, config)
	}

	if _, err := io.WriteString(os.Stdout, listing); err != nil {
		return 1
	}

	return 0
}

func fakeHardwareEncode(arguments []string, config fakeHardwareConfig) int {
	position := slices.Index(arguments, "-c:v")
	if position < 0 || position+1 >= len(arguments) {
		return 1
	}

	name := arguments[position+1]

	// The sample of the decode tests: some bytes on standard output.
	if name == "libx264" {
		if outcome, found := config.Outcomes[name]; found && outcome.Exit != 0 {
			fmt.Fprint(os.Stderr, outcome.Stderr)

			return outcome.Exit
		}

		if _, err := io.WriteString(os.Stdout, "sample"); err != nil {
			return 1
		}

		return 0
	}

	// A decode test names its outcome name+fakeDecodeSuffix.
	if slices.Contains(arguments, "-hwaccel") {
		name += fakeDecodeSuffix
	}

	outcome := config.Outcomes[name]
	if outcome.Hang {
		time.Sleep(fakeHangDuration)
	}

	fmt.Fprint(os.Stderr, outcome.Stderr)

	return outcome.Exit
}

// hardwareFixture configures the fake with listing fixtures and returns the recorded calls.
func hardwareFixture(t *testing.T, encodersFile, hwaccelsFile string,
	outcomes map[string]fakeHardwareOutcome,
) (string, func() [][]string) {
	t.Helper()

	config := fakeHardwareConfig{
		Encoders: string(readFixture(t, encodersFile)),
		Hwaccels: string(readFixture(t, hwaccelsFile)),
		Outcomes: outcomes,
	}

	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode config: %v", err)
	}

	directory := t.TempDir()
	configPath := filepath.Join(directory, "hardware.json")
	argumentsPath := filepath.Join(directory, "arguments.jsonl")

	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := os.WriteFile(argumentsPath, nil, 0o600); err != nil {
		t.Fatalf("create arguments file: %v", err)
	}

	executable := fakeTool(t, fakeModeHardware).executable

	t.Setenv(fakeHardwareVariable, configPath)
	t.Setenv(fakeArgumentsVariable, argumentsPath)

	return executable, func() [][]string { return readRecordedCalls(t, argumentsPath) }
}

// renderNodePattern creates a fake render node and returns the glob that finds it, and the node.
func renderNodePattern(t *testing.T) (string, string) {
	t.Helper()

	directory := t.TempDir()
	node := filepath.Join(directory, "renderD128")

	if err := os.WriteFile(node, nil, 0o600); err != nil {
		t.Fatalf("create render node: %v", err)
	}

	return filepath.Join(directory, "renderD*"), node
}

func testOptions(executable, pattern string) HardwareDetectionOptions {
	return HardwareDetectionOptions{
		Executable:        executable,
		Logger:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		TestTimeout:       2 * time.Second,
		RenderNodePattern: pattern,
	}
}

func candidateNames(candidates []hardwareCandidate) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.name)
	}

	return names
}

func TestParseHardwareCandidatesOnRealListings(t *testing.T) {
	tests := []struct {
		name     string
		encoders string
		hwaccels string
		want     []string
	}{
		{"FFmpeg 7.1.5 Docker image", "ffmpeg-7.1.5-encoders.txt", "ffmpeg-7.1.5-hwaccels.txt", []string{nvencName, qsvName, vaapiName}},
		{"FFmpeg 9.0.2 full build", "ffmpeg-9.0.2-encoders.txt", "ffmpeg-9.0.2-hwaccels.txt", []string{nvencName, qsvName, vaapiName, amfName}},
		{"Jellyfin-FFmpeg", "jellyfin-ffmpeg-encoders.txt", "jellyfin-ffmpeg-hwaccels.txt", []string{nvencName, qsvName, vaapiName, amfName}},
		{"stock static build", "ffmpeg-static-encoders.txt", "ffmpeg-static-hwaccels.txt", nil},
		{"vaapi encoder without its hwaccel", "jellyfin-ffmpeg-encoders.txt", "ffmpeg-static-hwaccels.txt", []string{nvencName, amfName}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseHardwareCandidates(readFixture(t, test.encoders), readFixture(t, test.hwaccels))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}

			if names := candidateNames(got); !slices.Equal(names, test.want) {
				t.Fatalf("candidates = %v, want %v", names, test.want)
			}
		})
	}
}

func TestParseHardwareCandidatesRejectsAForeignListing(t *testing.T) {
	if _, err := parseHardwareCandidates([]byte("not ffmpeg\n"), nil); !errors.Is(err, errNoListing) {
		t.Fatalf("error = %v, want errNoListing", err)
	}
}

func TestMessageForStderr(t *testing.T) {
	tests := []struct {
		stderr string
		want   string
	}{
		{"Cannot open /dev/dri/renderD128: Permission denied", HardwareMessagePermissionDenied},
		{"Failed to open /dev/dri/renderD128: No such file or directory", HardwareMessageDeviceNotFound},
		{"[h264_nvenc @ 0x1] OpenEncodeSessionEx failed: unsupported device (2)", HardwareMessageUnsupported},
		{"Cannot load libcuda.so.1", HardwareMessageDriverFailed},
		{"[AVHWDeviceContext @ 0x1] Failed to initialise VAAPI connection: -1 (unknown libva error).", HardwareMessageDriverFailed},
		{"No NVENC capable devices found", HardwareMessageDeviceNotFound},
		{"[h264_amf @ 0x1] DLL amfrt64.dll failed to open", HardwareMessageDriverFailed},
		{"Conversion failed!", HardwareMessageFailed},
		{"", HardwareMessageFailed},
	}

	for _, test := range tests {
		if got := messageForStderr(test.stderr); got != test.want {
			t.Errorf("messageForStderr(%q) = %q, want %q", test.stderr, got, test.want)
		}
	}
}

func TestMessagesCarryNoPath(t *testing.T) {
	for _, message := range []string{
		HardwareMessageDeviceNotFound, HardwareMessagePermissionDenied, HardwareMessageDriverFailed,
		HardwareMessageUnsupported, HardwareMessageTimeout, HardwareMessageFailed, HardwareMessageNoDevice,
	} {
		if strings.ContainsAny(message, "/\\ ") {
			t.Errorf("message %q is not a short token", message)
		}
	}
}

func TestParseHwaccelsIgnoresOtherText(t *testing.T) {
	got := parseHwaccels([]byte("ffmpeg banner line\nHardware acceleration methods:\nvaapi\n\nqsv extra words\ncuda\r\n"))
	if want := []string{"vaapi", "cuda"}; !slices.Equal(got, want) {
		t.Fatalf("hwaccels = %v, want %v", got, want)
	}
}

func TestTestEncodeArguments(t *testing.T) {
	tests := []struct {
		candidate hardwareCandidate
		want      []string
		absent    []string
	}{
		{hardwareCandidates()[2], []string{"vaapi=hw:/dev/dri/renderD129", "-filter_hw_device", "format=nv12,hwupload", vaapiName}, nil},
		{hardwareCandidates()[1], []string{"vaapi=va:/dev/dri/renderD129", "qsv=hw@va", qsvName}, []string{"vaapi=hw:/dev/dri/renderD129"}},
		{hardwareCandidates()[0], []string{nvencName, testSourceFilter, "null"}, []string{"-init_hw_device", "-vf"}},
		{hardwareCandidates()[3], []string{amfName}, []string{"-init_hw_device"}},
	}

	for _, test := range tests {
		arguments := testEncodeArguments(test.candidate, "/dev/dri/renderD129")

		for _, want := range test.want {
			if !slices.Contains(arguments, want) {
				t.Errorf("%s: arguments %v lack %q", test.candidate.name, arguments, want)
			}
		}

		for _, unwanted := range test.absent {
			if slices.Contains(arguments, unwanted) {
				t.Errorf("%s: arguments %v contain %q", test.candidate.name, arguments, unwanted)
			}
		}
	}
}

func TestDetectHardwareReportsSuccessAndFailure(t *testing.T) {
	executable, calls := hardwareFixture(t, "ffmpeg-9.0.2-encoders.txt", "ffmpeg-9.0.2-hwaccels.txt",
		map[string]fakeHardwareOutcome{
			nvencName: {Exit: 1, Stderr: "Cannot load libcuda.so.1\n"},
			qsvName:   {Exit: 1, Stderr: "Failed to open /dev/dri/renderD128: No such file or directory\n"},
			amfName:   {Exit: 1, Stderr: "something else\n"},
		})
	pattern, node := renderNodePattern(t)

	results, err := DetectHardwareResults(t.Context(), testOptions(executable, pattern))
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	want := []HardwareResult{
		{Encoder: playback.HardwareEncoder{API: playback.HardwareAPINVENC, Name: nvencName, Codec: "h264"}, Message: HardwareMessageDriverFailed},
		{Encoder: playback.HardwareEncoder{API: playback.HardwareAPIQSV, Name: qsvName, Codec: "h264", Device: node}, Message: HardwareMessageDeviceNotFound},
		{Encoder: playback.HardwareEncoder{API: playback.HardwareAPIVAAPI, Name: vaapiName, Codec: "h264", Device: node, Usable: true, Decode: true}},
		{Encoder: playback.HardwareEncoder{API: playback.HardwareAPIAMF, Name: amfName, Codec: "h264"}, Message: HardwareMessageFailed},
	}

	if !slices.Equal(results, want) {
		t.Fatalf("results = %+v, want %+v", results, want)
	}

	// Two listings, one test encode per candidate, in order, plus the decode sample and the decode test of VAAPI.
	recorded := calls()
	if len(recorded) != 2+len(want)+2 {
		t.Fatalf("calls = %d, want %d: %v", len(recorded), 2+len(want)+2, recorded)
	}
}

func TestDetectHardwareReturnsTheEncodersOfTheDomain(t *testing.T) {
	executable, _ := hardwareFixture(t, "jellyfin-ffmpeg-encoders.txt", "jellyfin-ffmpeg-hwaccels.txt", nil)
	pattern, _ := renderNodePattern(t)

	encoders, err := DetectHardware(t.Context(), testOptions(executable, pattern))
	if err != nil {
		t.Fatalf("DetectHardware: %v", err)
	}

	if len(encoders) != 4 {
		t.Fatalf("encoders = %d, want 4", len(encoders))
	}

	for _, encoder := range encoders {
		if !encoder.Usable || !encoder.Available() {
			t.Errorf("%s not usable although its test encode exited 0", encoder.Name)
		}
	}
}

func TestDetectHardwareUsesTheConfiguredDevice(t *testing.T) {
	executable, calls := hardwareFixture(t, "ffmpeg-7.1.5-encoders.txt", "ffmpeg-7.1.5-hwaccels.txt", nil)
	options := testOptions(executable, filepath.Join(t.TempDir(), "renderD*"))
	options.Device = "/dev/dri/renderD129"

	results, err := DetectHardwareResults(t.Context(), options)
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	for _, result := range results {
		if result.Encoder.API == playback.HardwareAPIVAAPI && result.Encoder.Device != options.Device {
			t.Errorf("vaapi device = %q, want %q", result.Encoder.Device, options.Device)
		}
	}

	found := slices.ContainsFunc(calls(), func(arguments []string) bool {
		return slices.Contains(arguments, "vaapi=hw:"+options.Device)
	})
	if !found {
		t.Errorf("no test encode used the configured device: %v", calls())
	}
}

func TestDetectHardwareWithoutCandidatesRunsNothing(t *testing.T) {
	executable, calls := hardwareFixture(t, "ffmpeg-static-encoders.txt", "ffmpeg-static-hwaccels.txt", nil)

	encoders, err := DetectHardware(t.Context(), testOptions(executable, ""))
	if err != nil {
		t.Fatalf("DetectHardware: %v", err)
	}

	if len(encoders) != 0 {
		t.Fatalf("encoders = %+v, want none", encoders)
	}

	for _, arguments := range calls() {
		if !slices.Contains(arguments, encodersQuery) && !slices.Contains(arguments, hwaccelsQuery) {
			t.Errorf("ran a test encode: %v", arguments)
		}
	}
}

func TestDetectHardwareSkipsRenderNodeEncodersWithoutDRM(t *testing.T) {
	executable, calls := hardwareFixture(t, "ffmpeg-7.1.5-encoders.txt", "ffmpeg-7.1.5-hwaccels.txt", nil)

	results, err := DetectHardwareResults(t.Context(), testOptions(executable, filepath.Join(t.TempDir(), "renderD*")))
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	for _, result := range results {
		switch result.Encoder.API {
		case playback.HardwareAPIVAAPI, playback.HardwareAPIQSV:
			if result.Encoder.Usable || result.Message != HardwareMessageNoDevice {
				t.Errorf("%s = %+v, want skipped", result.Encoder.Name, result)
			}
		case playback.HardwareAPINVENC:
			if !result.Encoder.Usable {
				t.Errorf("nvenc must still be tested: %+v", result)
			}
		case playback.HardwareAPIAMF:
		}
	}

	// Listings plus the single NVENC test encode, the decode sample and the NVENC decode test.
	if got := len(calls()); got != 5 {
		t.Fatalf("calls = %d, want 5: %v", got, calls())
	}
}

func TestDetectHardwareTimeoutIsAFailureNotAnError(t *testing.T) {
	executable, _ := hardwareFixture(t, "jellyfin-ffmpeg-encoders.txt", "jellyfin-ffmpeg-hwaccels.txt",
		map[string]fakeHardwareOutcome{nvencName: {Hang: true}})
	pattern, _ := renderNodePattern(t)
	options := testOptions(executable, pattern)
	options.TestTimeout = 3 * time.Second

	results, err := DetectHardwareResults(t.Context(), options)
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	if results[0].Encoder.Usable || results[0].Message != HardwareMessageTimeout {
		t.Fatalf("nvenc = %+v, want a timeout", results[0])
	}

	if !results[1].Encoder.Usable {
		t.Fatalf("the candidate after a timeout must still run: %+v", results[1])
	}
}

func TestDetectHardwareStopsWhenTheCallerCancels(t *testing.T) {
	executable, _ := hardwareFixture(t, "jellyfin-ffmpeg-encoders.txt", "jellyfin-ffmpeg-hwaccels.txt",
		map[string]fakeHardwareOutcome{nvencName: {Hang: true}})
	pattern, _ := renderNodePattern(t)

	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()

	started := time.Now()

	_, err := DetectHardware(ctx, testOptions(executable, pattern))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want the context error", err)
	}

	if time.Since(started) > 5*time.Second {
		t.Fatal("detection did not stop with its context")
	}
}

func TestDetectHardwareRejectsBadOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := map[string]HardwareDetectionOptions{
		"nil logger":       {Executable: "ffmpeg"},
		"no executable":    {Logger: logger},
		"empty everything": {},
	}

	for name, options := range tests {
		if _, err := DetectHardware(t.Context(), options); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}
}

func TestDetectHardwareReportsAnUnavailableFFmpeg(t *testing.T) {
	options := testOptions(filepath.Join(t.TempDir(), "missing-ffmpeg"), "")

	if _, err := DetectHardware(t.Context(), options); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestDetectHardwareReportsAForeignListing(t *testing.T) {
	executable, _ := hardwareFixture(t, "ffmpeg-static-hwaccels.txt", "ffmpeg-static-hwaccels.txt", nil)

	if _, err := DetectHardware(t.Context(), testOptions(executable, "")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

func TestDetectHardwareWithAFailingListing(t *testing.T) {
	executable := fakeTool(t, fakeModeFail).executable

	if _, err := DetectHardware(t.Context(), testOptions(executable, "")); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", err)
	}
}

// TestDetectHardwareOnThisMachine runs the real FFmpeg. What the machine has is unknown, so it
// only asserts that detection works and that the result is consistent.
func TestDetectHardwareOnThisMachine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in -short mode")
	}

	executable, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg is not on PATH; hardware detection needs it")
	}

	options := testOptions(executable, "")
	options.TestTimeout = hardwareTestTimeout

	results, err := DetectHardwareResults(t.Context(), options)
	if err != nil {
		t.Fatalf("DetectHardwareResults: %v", err)
	}

	for _, result := range results {
		if result.Encoder.Name == "" || result.Encoder.Codec != "h264" {
			t.Errorf("incomplete encoder: %+v", result)
		}

		if result.Encoder.Usable != (result.Message == "") {
			t.Errorf("%s: Usable = %t with message %q", result.Encoder.Name, result.Encoder.Usable, result.Message)
		}
	}

	t.Logf("hardware encoders on this machine: %+v", results)
}

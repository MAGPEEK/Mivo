package ffmpeg

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// hardwareTestTimeout bounds one test encode. A healthy device finishes one second of
	// 720p in well under a second; a wedged driver must not block the server start.
	hardwareTestTimeout = 10 * time.Second

	hwaccelsQuery = "-hwaccels"

	// defaultRenderNodePattern finds the DRM render nodes VAAPI and QSV encode on.
	defaultRenderNodePattern = "/dev/dri/renderD*"

	// hwaccelsHeader starts the `-hwaccels` output; the method names follow, one per line.
	hwaccelsHeader = "Hardware acceleration methods:"

	// testSourceFilter is the one-second 720p input of every test encode.
	testSourceFilter = "testsrc2=size=1280x720:rate=25:duration=1"
	// testHardwareDeviceName names the device inside a test encode's filter graph.
	testHardwareDeviceName = "hw"
	// testSurfaceFilter uploads the frames of the software source to the device.
	testSurfaceFilter = "format=nv12,hwupload"
	// testSurfaceFilterQSV reserves extra surfaces, which QSV needs to start its session.
	testSurfaceFilterQSV = "format=nv12,hwupload=extra_hw_frames=64"
)

// Short failure reasons of a test encode. They carry no path and no driver text, so they may
// reach the API; the raw stderr tail only goes to the server log (ADR 0024 Notes M2-39).
const (
	// HardwareMessageDeviceNotFound reports a missing device node or a machine without the hardware.
	HardwareMessageDeviceNotFound = "device_not_found"
	// HardwareMessagePermissionDenied reports a device the server user may not open.
	HardwareMessagePermissionDenied = "permission_denied"
	// HardwareMessageDriverFailed reports a driver or runtime library that is missing or broken.
	HardwareMessageDriverFailed = "driver_failed"
	// HardwareMessageUnsupported reports hardware that cannot encode the test stream.
	HardwareMessageUnsupported = "unsupported"
	// HardwareMessageTimeout reports a test encode that did not finish within its timeout.
	HardwareMessageTimeout = "timeout"
	// HardwareMessageFailed is any other failed test encode.
	HardwareMessageFailed = "failed"
	// HardwareMessageNoDevice marks a VAAPI or QSV candidate skipped because the machine has no
	// DRM render node; nothing was run.
	HardwareMessageNoDevice = "no_render_node"
)

// failurePatterns map lower-case stderr fragments to a message. The first matching row wins,
// so the more specific causes come first.
type failurePattern struct {
	fragments []string
	message   string
}

func failurePatterns() []failurePattern {
	return []failurePattern{
		{[]string{"permission denied"}, HardwareMessagePermissionDenied},
		{[]string{
			"no such file", "cannot open display", "no capable devices", "no device", "failed to open /dev",
			"device creation failed: -19", "no nvenc capable",
		}, HardwareMessageDeviceNotFound},
		{[]string{
			"cannot load", "libcuda", "failed to initialise vaapi", "failed to initialize vaapi", "vainitialize",
			"libva", "driver", "failed to create a vaapi", "unable to load", ".dll",
		}, HardwareMessageDriverFailed},
		{[]string{
			"not supported", "unsupported", "function not implemented", "unknown encoder",
			"no usable encoding profile", "openencodesessionex failed",
		}, HardwareMessageUnsupported},
	}
}

// errMissingExecutable reports detection options without the FFmpeg executable.
var errMissingExecutable = errors.New("no ffmpeg executable configured")

// HardwareDetectionOptions configure DetectHardware.
type HardwareDetectionOptions struct {
	// Executable is the configured tools.ffmpeg, resolved by the caller.
	Executable string
	// Device is transcode.hwaccel_device; empty selects the first DRM render node.
	Device string
	// Logger receives the raw stderr tail of failed test encodes. Required.
	Logger *slog.Logger
	// TestTimeout bounds each test encode; zero selects 10 seconds.
	TestTimeout time.Duration
	// RenderNodePattern is the glob of the DRM render nodes; empty selects /dev/dri/renderD*.
	// Tests point it at a temporary directory.
	RenderNodePattern string
}

// HardwareResult is the outcome of one candidate: the encoder as the playback domain sees it
// plus the short reason of a failure (empty when Encoder.Usable).
type HardwareResult struct {
	Encoder playback.HardwareEncoder
	Message string
}

// hardwareCandidate is a hardware H.264 encoder FFmpeg can list.
type hardwareCandidate struct {
	api  playback.HardwareAPI
	name string
	// hwaccel is the -hwaccels entry the encoder needs to initialise its device; empty when the
	// encoder opens its own session (NVENC, AMF).
	hwaccel string
	// needsRenderNode marks the encoders that run on a DRM render node.
	needsRenderNode bool
}

// hardwareCandidates lists the candidates in the order they are tested.
func hardwareCandidates() []hardwareCandidate {
	return []hardwareCandidate{
		{api: playback.HardwareAPINVENC, name: "h264_nvenc"},
		{api: playback.HardwareAPIQSV, name: "h264_qsv", hwaccel: "qsv", needsRenderNode: true},
		{api: playback.HardwareAPIVAAPI, name: "h264_vaapi", hwaccel: "vaapi", needsRenderNode: true},
		{api: playback.HardwareAPIAMF, name: "h264_amf"},
	}
}

// DetectHardware lists the hardware H.264 encoders of the FFmpeg build and test-encodes one
// second with each, sequentially, so only encoders that work on this machine come back Usable
// (ADR 0030 §5). A stock build without hardware encoders yields an empty slice and runs nothing
// besides the two listings; a machine without a render node skips VAAPI and QSV. The caller
// serialises detection against hardware transcode starts.
//
// Errors wrap domain.ErrUnavailable when a listing fails, domain.ErrInvalid for missing
// options, and the caller's context error when ctx ended. A failing test encode is no error.
func DetectHardware(ctx context.Context, options HardwareDetectionOptions) ([]playback.HardwareEncoder, error) {
	results, err := DetectHardwareResults(ctx, options)
	if err != nil {
		return nil, err
	}

	encoders := make([]playback.HardwareEncoder, 0, len(results))
	for _, result := range results {
		encoders = append(encoders, result.Encoder)
	}

	return encoders, nil
}

// DetectHardwareResults is DetectHardware with the failure reason of each candidate, for
// diagnostics. Candidates skipped for lack of a render node are reported unusable with
// HardwareMessageNoDevice.
func DetectHardwareResults(ctx context.Context, options HardwareDetectionOptions) ([]HardwareResult, error) {
	if options.Logger == nil {
		return nil, fmt.Errorf("detect hardware encoders: logger is nil: %w", domain.ErrInvalid)
	}

	if options.Executable == "" {
		return nil, fmt.Errorf("detect hardware encoders: %w: %w", domain.ErrInvalid, errMissingExecutable)
	}

	ffmpeg := tool{name: ffmpegToolName, executable: options.Executable}

	candidates, err := listHardwareCandidates(ctx, ffmpeg)
	if err != nil {
		return nil, err
	}

	return testCandidates(ctx, ffmpeg, candidates, options)
}

// listHardwareCandidates reads -encoders and -hwaccels and returns the candidates the build
// can drive. The listing alone proves nothing; testCandidates does.
func listHardwareCandidates(ctx context.Context, ffmpeg tool) ([]hardwareCandidate, error) {
	listContext, cancel := context.WithTimeout(ctx, capabilityDetectionTimeout)
	defer cancel()

	encoders, err := ffmpeg.output(listContext, []string{hideBannerFlag, encodersQuery})
	if err != nil {
		return nil, fmt.Errorf("detect hardware encoders: ffmpeg %s: %w", encodersQuery, unavailableUnlessCancelled(ctx, err))
	}

	hwaccels, err := ffmpeg.output(listContext, []string{hideBannerFlag, hwaccelsQuery})
	if err != nil {
		return nil, fmt.Errorf("detect hardware encoders: ffmpeg %s: %w", hwaccelsQuery, unavailableUnlessCancelled(ctx, err))
	}

	candidates, err := parseHardwareCandidates(encoders, hwaccels)
	if err != nil {
		return nil, fmt.Errorf("detect hardware encoders: %w: %w", domain.ErrUnavailable, err)
	}

	return candidates, nil
}

// parseHardwareCandidates returns the candidates listed as non-experimental video encoders
// whose hwaccel (if any) is listed too.
func parseHardwareCandidates(encodersOutput, hwaccelsOutput []byte) ([]hardwareCandidate, error) {
	entries, err := parseListing(encodersOutput)
	if err != nil {
		return nil, fmt.Errorf("parse ffmpeg %s: %w", encodersQuery, err)
	}

	hwaccels := parseHwaccels(hwaccelsOutput)

	var found []hardwareCandidate

	for _, candidate := range hardwareCandidates() {
		if !hasCodec(entries, candidate.name, encoderKindVideo) {
			continue
		}

		if candidate.hwaccel != "" && !slices.Contains(hwaccels, candidate.hwaccel) {
			continue
		}

		found = append(found, candidate)
	}

	return found, nil
}

// parseHwaccels returns the method names after the header of `-hwaccels`.
func parseHwaccels(output []byte) []string {
	var names []string

	seenHeader := false

	for line := range strings.Lines(string(output)) {
		line = strings.TrimSpace(line)

		if !seenHeader {
			seenHeader = line == hwaccelsHeader

			continue
		}

		if line != "" && !strings.ContainsAny(line, " \t") {
			names = append(names, line)
		}
	}

	return names
}

// testCandidates test-encodes each candidate in order.
func testCandidates(ctx context.Context, ffmpeg tool, candidates []hardwareCandidate,
	options HardwareDetectionOptions,
) ([]HardwareResult, error) {
	results := make([]HardwareResult, 0, len(candidates))
	renderNode := options.Device

	if renderNode == "" {
		renderNode = firstRenderNode(options.RenderNodePattern)
	}

	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("detect hardware encoders: %w", err)
		}

		encoder := playback.HardwareEncoder{API: candidate.api, Name: candidate.name, Codec: playback.TranscodeVideoCodec}

		if candidate.needsRenderNode {
			encoder.Device = renderNode
			if renderNode == "" {
				results = append(results, HardwareResult{Encoder: encoder, Message: HardwareMessageNoDevice})

				continue
			}
		}

		result, err := testCandidate(ctx, candidateTest{ffmpeg: ffmpeg, candidate: candidate, encoder: encoder}, options)
		if err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return results, nil
}

// firstRenderNode returns the lexically first match of the pattern, empty when there is none.
func firstRenderNode(pattern string) string {
	if pattern == "" {
		pattern = defaultRenderNodePattern
	}

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		return ""
	}

	slices.Sort(matches)

	return matches[0]
}

// candidateTest bundles what one test encode needs.
type candidateTest struct {
	ffmpeg    tool
	candidate hardwareCandidate
	encoder   playback.HardwareEncoder
}

// testCandidate runs one test encode. Only the caller's cancellation is an error.
func testCandidate(ctx context.Context, job candidateTest, options HardwareDetectionOptions) (HardwareResult, error) {
	ffmpeg, candidate, encoder := job.ffmpeg, job.candidate, job.encoder

	timeout := options.TestTimeout
	if timeout <= 0 {
		timeout = hardwareTestTimeout
	}

	testContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, err := ffmpeg.output(testContext, testEncodeArguments(candidate, encoder.Device))
	if err == nil {
		encoder.Usable = true

		return HardwareResult{Encoder: encoder}, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return HardwareResult{}, fmt.Errorf("detect hardware encoders: %w", ctxErr)
	}

	message := failureMessage(err)
	attributes := append([]slog.Attr{
		slog.String("encoder", candidate.name), slog.String("reason", message), slog.Any("error", err),
	}, exitAttributes(err)...)
	options.Logger.LogAttrs(ctx, slog.LevelInfo, "hardware encoder test failed", attributes...)

	return HardwareResult{Encoder: encoder, Message: message}, nil
}

// failureMessage maps a failed test encode to a short reason.
func failureMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return HardwareMessageTimeout
	}

	var exit *exitError
	if !errors.As(err, &exit) {
		return HardwareMessageFailed
	}

	return messageForStderr(exit.stderrTail())
}

// messageForStderr maps FFmpeg's error output to a short reason by known patterns.
func messageForStderr(stderr string) string {
	lower := strings.ToLower(stderr)

	for _, pattern := range failurePatterns() {
		for _, fragment := range pattern.fragments {
			if strings.Contains(lower, fragment) {
				return pattern.message
			}
		}
	}

	return HardwareMessageFailed
}

// testEncodeArguments encodes one second of generated 720p video to the null muxer.
func testEncodeArguments(candidate hardwareCandidate, device string) []string {
	arguments := []string{hideBannerFlag, "-nostdin", "-v", "error"}
	filter := ""

	switch candidate.api {
	case playback.HardwareAPIVAAPI:
		arguments = append(arguments, "-init_hw_device", "vaapi="+testHardwareDeviceName+":"+device,
			"-filter_hw_device", testHardwareDeviceName)
		filter = testSurfaceFilter
	case playback.HardwareAPIQSV:
		// On Linux QSV sits on top of the VAAPI driver, derived from the render node.
		arguments = append(arguments, "-init_hw_device", "vaapi=va:"+device,
			"-init_hw_device", "qsv="+testHardwareDeviceName+"@va", "-filter_hw_device", testHardwareDeviceName)
		filter = testSurfaceFilterQSV
	case playback.HardwareAPINVENC, playback.HardwareAPIAMF:
		// These encoders take software frames and open their own session.
	}

	arguments = append(arguments, "-f", "lavfi", "-i", testSourceFilter)
	if filter != "" {
		arguments = append(arguments, "-vf", filter)
	}

	return append(arguments, "-c:v", candidate.name, "-f", "null", "-")
}

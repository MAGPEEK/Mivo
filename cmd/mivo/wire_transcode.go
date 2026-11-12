package main

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/transcoder"
	"github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/platform/config"
)

const (
	// maxAutoEncoderThreads bounds the thread count resolved from the CPU count: x264 gains
	// nothing beyond it and a many-core server would spend them on one session (ADR 0030 §1).
	maxAutoEncoderThreads = 16

	// cpusPerEncoderThread leaves half of the CPUs to the rest of the server.
	cpusPerEncoderThread = 2

	// soxProbeTimeout bounds the one-time libsoxr query at startup.
	soxProbeTimeout = 10 * time.Second

	// hardwareStartHold is how long a hardware session start keeps a hardware test off the
	// device: until its first segment, at most this long.
	hardwareStartHold = 30 * time.Second

	// forbiddenHardwareDeviceCharacters would let a configured device name add FFmpeg device
	// options (ffmpeg.HardwareDetectionOptions).
	forbiddenHardwareDeviceCharacters = ",:"

	hardwareDeviceSetting = "transcode.hwaccel_device"
)

// resolveEncoderThreads turns transcode.threads into the thread count of the encoder: 0 selects
// half the CPUs (at least one, at most maxAutoEncoderThreads), so a transcode leaves room for
// the web app and the scan on a small NAS (ADR 0030 §1). An explicit value is the
// administrator's choice and passes through.
func resolveEncoderThreads(configured, cpus int) int {
	if configured > 0 {
		return configured
	}

	return min(max(1, cpus/cpusPerEncoderThread), maxAutoEncoderThreads)
}

// throttleAhead maps transcode.throttle_ahead_seconds to the playback option; 0 turns the
// throttle off.
func throttleAhead(settings config.Transcode) time.Duration {
	return time.Duration(settings.ThrottleAheadSeconds) * time.Second
}

// lowPriority reports transcode.priority = low.
func lowPriority(settings config.Transcode) bool {
	return settings.Priority == config.PriorityLow
}

// producerOptions maps the transcode settings and the detected FFmpeg features into the
// options of the stream producer.
func (app application) producerOptions(tools externalTools, inputs ffmpeg.InputOpener, hardware ffmpeg.HardwareEncoderSource) ffmpeg.ProducerOptions {
	settings := app.config.Transcode

	return ffmpeg.ProducerOptions{
		FFmpegPath: tools.ffmpeg, Inputs: inputs, Logger: app.logger,
		Preset:         settings.Preset,
		Threads:        resolveEncoderThreads(settings.Threads, runtime.NumCPU()),
		LowPriority:    lowPriority(settings),
		SoxResampler:   tools.soxResampler,
		HardwareSource: hardware,
	}
}

// detectSoxResampler asks FFmpeg once whether it was built with libsoxr. A failed query only
// costs the better resampler, so it is logged at debug and answers false.
func (app application) detectSoxResampler(ctx context.Context, ffmpegPath string, found bool) bool {
	if !found {
		return false
	}

	probeContext, cancel := context.WithTimeout(ctx, soxProbeTimeout)
	defer cancel()

	available, err := ffmpeg.HasSoxResampler(probeContext, ffmpegPath)
	if err != nil {
		app.logger.DebugContext(ctx, "libsoxr detection failed, using the default resampler", "error", err)

		return false
	}

	return available
}

// checkHardwareDevice refuses a configured device that could add FFmpeg device options.
func checkHardwareDevice(device string) error {
	if strings.ContainsAny(device, forbiddenHardwareDeviceCharacters) {
		return fmt.Errorf("config: %s %q must not contain any of %q", hardwareDeviceSetting, device, forbiddenHardwareDeviceCharacters)
	}

	return nil
}

// hardwareDetector adapts the FFmpeg detection to the transcoder service.
type hardwareDetector struct {
	options ffmpeg.HardwareDetectionOptions
	// detect is ffmpeg.DetectHardwareResults; tests replace it.
	detect func(ctx context.Context, options ffmpeg.HardwareDetectionOptions) ([]ffmpeg.HardwareResult, error)
}

// Detect implements transcoder.Detector.
func (detector hardwareDetector) Detect(ctx context.Context) ([]transcoder.Device, error) {
	results, err := detector.detect(ctx, detector.options)
	if err != nil {
		return nil, fmt.Errorf("detect hardware encoders: %w", err)
	}

	devices := make([]transcoder.Device, 0, len(results))
	for _, result := range results {
		devices = append(devices, transcoder.Device{Encoder: result.Encoder, Message: result.Message})
	}

	return devices, nil
}

// noHardware is the detector of a server without a usable FFmpeg: nothing to test.
type noHardware struct{}

// Detect implements transcoder.Detector.
func (noHardware) Detect(context.Context) ([]transcoder.Device, error) { return nil, nil }

// newTranscoderService builds the hardware status service. The detection itself starts with
// Start, in the background: a slow or hanging GPU test must not delay the listener.
func (app application) newTranscoderService(tools externalTools, ffmpegUsable bool) (*transcoder.Service, error) {
	settings := app.config.Transcode

	if err := checkHardwareDevice(settings.HWAccelDevice); err != nil {
		return nil, err
	}

	var detector transcoder.Detector = noHardware{}

	if ffmpegUsable {
		detector = hardwareDetector{
			options: ffmpeg.HardwareDetectionOptions{
				Executable: tools.ffmpeg, Device: settings.HWAccelDevice, Logger: app.logger,
			},
			detect: ffmpeg.DetectHardwareResults,
		}
	}

	service, err := transcoder.New(transcoder.Options{
		Detector: detector, Logger: app.logger, Base: tools.capabilities,
		Setting:           playback.HardwareAcceleration(settings.HWAccel),
		ThrottleSupported: ffmpeg.PauseSupported(),
	})
	if err != nil {
		return nil, fmt.Errorf("create transcoder service: %w", err)
	}

	return service, nil
}

// transcoderEncoders lists the tested encoders of the transcoder service for the producer.
type transcoderEncoders struct {
	service *transcoder.Service
}

// HardwareEncoders implements ffmpeg.HardwareEncoderSource.
func (encoders transcoderEncoders) HardwareEncoders() []playback.HardwareEncoder {
	return encoders.service.Capabilities().HardwareEncoders
}

// hardwareStartGuard keeps a hardware test off the device while a hardware session starts:
// it holds the transcoder's start lock from the producer start until the first segment (or
// hardwareStartHold). Software jobs pass untouched.
type hardwareStartGuard struct {
	inner   appplayback.StreamProducer
	acquire func() (release func())
}

// Start implements appplayback.StreamProducer.
func (guard hardwareStartGuard) Start(ctx context.Context, job appplayback.Job) (appplayback.Production, error) {
	if !job.Encoder.Hardware {
		return guard.inner.Start(ctx, job) //nolint:wrapcheck // transparent decorator
	}

	release := guard.acquire()

	production, err := guard.inner.Start(ctx, job)
	if err != nil {
		release()

		return nil, err //nolint:wrapcheck // transparent decorator
	}

	go func() {
		defer release()

		holdContext, cancel := context.WithTimeout(ctx, hardwareStartHold)
		defer cancel()

		// The outcome is the playback service's to handle; this wait only bounds the hold.
		_ = production.Ready(holdContext)
	}()

	return production, nil
}

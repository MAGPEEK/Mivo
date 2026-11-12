package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/adapters/ffmpeg"
	"github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/app/transcoder"
	domainplayback "github.com/magpeek/mivo/internal/domain/playback"
	"github.com/magpeek/mivo/internal/platform/config"
)

func transcodeApp(settings config.Transcode) application {
	return application{
		config: config.Config{Transcode: settings},
		logger: slog.New(slog.DiscardHandler),
	}
}

func TestResolveEncoderThreads(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configured int
		cpus       int
		want       int
	}{
		{name: "auto on a single core", cpus: 1, want: 1},
		{name: "auto on two cores", cpus: 2, want: 1},
		{name: "auto on four cores", cpus: 4, want: 2},
		{name: "auto on eight cores", cpus: 8, want: 4},
		{name: "auto is bounded", cpus: 256, want: maxAutoEncoderThreads},
		{name: "explicit value passes", configured: 3, cpus: 64, want: 3},
		{name: "explicit value above the auto bound passes", configured: 32, cpus: 2, want: 32},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := resolveEncoderThreads(test.configured, test.cpus); got != test.want {
				t.Errorf("resolveEncoderThreads(%d, %d) = %d, want %d", test.configured, test.cpus, got, test.want)
			}
		})
	}
}

type fixedEncoders []domainplayback.HardwareEncoder

func (encoders fixedEncoders) HardwareEncoders() []domainplayback.HardwareEncoder { return encoders }

// TestProducerOptionsFollowTheConfiguration: the transcode settings reach the producer, the
// libsoxr probe result and the hardware source included (M4-07a, M4-10).
func TestProducerOptionsFollowTheConfiguration(t *testing.T) {
	t.Parallel()

	source := fixedEncoders{{Name: "h264_vaapi"}}
	tools := externalTools{ffmpeg: "ffmpeg-path", soxResampler: true}

	low := transcodeApp(config.Transcode{Preset: config.PresetFast, Priority: config.PriorityLow})
	options := low.producerOptions(tools, nil, source)

	if options.FFmpegPath != "ffmpeg-path" || options.Preset != config.PresetFast || !options.LowPriority || !options.SoxResampler {
		t.Errorf("options = %+v, want path, fast preset, low priority and soxr", options)
	}

	if want := resolveEncoderThreads(0, runtime.NumCPU()); options.Threads != want {
		t.Errorf("Threads = %d, want the resolved %d", options.Threads, want)
	}

	if got := options.HardwareSource.HardwareEncoders(); len(got) != 1 || got[0].Name != "h264_vaapi" {
		t.Errorf("hardware source = %v", got)
	}

	normal := transcodeApp(config.Transcode{Preset: config.PresetMedium, Priority: config.PriorityNormal, Threads: 5})
	options = normal.producerOptions(externalTools{}, nil, nil)

	if options.LowPriority || options.SoxResampler || options.Threads != 5 {
		t.Errorf("options = %+v, want normal priority, no soxr and the explicit 5 threads", options)
	}
}

// TestPlaybackOptionsMapTheThrottleAndTheHardwareSetting: without the mapping the throttle
// would stay off for ever (M4-13b) and the decision would ignore transcode.hwaccel.
func TestPlaybackOptionsMapTheThrottleAndTheHardwareSetting(t *testing.T) {
	t.Parallel()

	app := transcodeApp(config.Transcode{MaxSessions: 2, ThrottleAheadSeconds: 60, HWAccel: config.HWAccelVAAPI})
	capabilities := domainplayback.ServerCapabilities{H264Encoder: "libx264"}

	options := app.playbackOptions(externalTools{capabilities: capabilities})

	if options.ThrottleAhead != time.Minute || options.MaxTranscodeSessions != 2 ||
		options.Policy.HardwareAcceleration != domainplayback.HardwareAccelerationVAAPI ||
		options.Capabilities.H264Encoder != "libx264" {
		t.Errorf("options = %+v", options)
	}

	if err := options.Policy.Validate(); err != nil {
		t.Errorf("policy invalid: %v", err)
	}

	off := transcodeApp(config.Transcode{MaxSessions: 1, ThrottleAheadSeconds: 0, HWAccel: config.HWAccelOff})
	if got := off.playbackOptions(externalTools{}); got.ThrottleAhead != 0 {
		t.Errorf("ThrottleAhead = %s, want 0 (off)", got.ThrottleAhead)
	}
}

// TestTextBurnInIsReportedUnavailable: the producer only burns in picture subtitles, so the
// wiring must not offer text burn-in until the hardened text path exists.
func TestTextBurnInIsReportedUnavailable(t *testing.T) {
	t.Parallel()

	got := withSafeBurnIn(domainplayback.ServerCapabilities{BurnInText: true, H264Encoder: "libx264"})
	if got.BurnInText || got.H264Encoder != "libx264" {
		t.Errorf("capabilities = %+v, want BurnInText off and the rest kept", got)
	}
}

func TestCheckHardwareDevice(t *testing.T) {
	t.Parallel()

	for device, wantErr := range map[string]bool{
		"": false, "/dev/dri/renderD128": false, "/dev/dri/renderD128,x": true, "/dev/dri:x": true, "vaapi:/dev/dri": true,
	} {
		if err := checkHardwareDevice(device); (err != nil) != wantErr {
			t.Errorf("checkHardwareDevice(%q) = %v, want error %t", device, err, wantErr)
		}
	}
}

func TestNewTranscoderServiceRefusesADeviceThatAddsOptions(t *testing.T) {
	t.Parallel()

	app := transcodeApp(config.Transcode{HWAccelDevice: "/dev/dri/renderD128,vaapi_device=x"})

	if _, err := app.newTranscoderService(externalTools{}, true); err == nil {
		t.Error("newTranscoderService accepted a device with a comma")
	}
}

// TestTranscoderServiceStartsInTheBackgroundAndMergesEncoders: detection ends after the
// service exists; its encoders then show up in the capabilities (M4-13d).
func TestTranscoderServiceStartsInTheBackgroundAndMergesEncoders(t *testing.T) {
	t.Parallel()

	app := transcodeApp(config.Transcode{HWAccel: config.HWAccelAuto})

	service, err := app.newTranscoderService(externalTools{capabilities: domainplayback.ServerCapabilities{H264Encoder: "libx264"}}, false)
	if err != nil {
		t.Fatalf("newTranscoderService: %v", err)
	}

	<-service.Start(t.Context())

	if capabilities := service.Capabilities(); capabilities.H264Encoder != "libx264" || len(capabilities.HardwareEncoders) != 0 {
		t.Errorf("capabilities without ffmpeg = %+v, want the base and no hardware", capabilities)
	}

	off := transcodeApp(config.Transcode{HWAccel: config.HWAccelOff})

	offService, err := off.newTranscoderService(externalTools{}, true)
	if err != nil {
		t.Fatalf("newTranscoderService: %v", err)
	}

	<-offService.Start(t.Context())

	if _, err := offService.Test(t.Context()); !errors.Is(err, transcoder.ErrDisabled) {
		t.Errorf("Test with hwaccel off = %v, want ErrDisabled: nothing may run", err)
	}
}

func TestHardwareDetectorConvertsTheResults(t *testing.T) {
	t.Parallel()

	var got ffmpeg.HardwareDetectionOptions

	detector := hardwareDetector{
		options: ffmpeg.HardwareDetectionOptions{Executable: "ffmpeg", Device: "/dev/dri/renderD129"},
		detect: func(_ context.Context, options ffmpeg.HardwareDetectionOptions) ([]ffmpeg.HardwareResult, error) {
			got = options

			return []ffmpeg.HardwareResult{
				{Encoder: domainplayback.HardwareEncoder{Name: "h264_vaapi", Usable: true}},
				{Encoder: domainplayback.HardwareEncoder{Name: "h264_nvenc"}, Message: "no device"},
			}, nil
		},
	}

	devices, err := detector.Detect(t.Context())
	if err != nil || len(devices) != 2 || devices[1].Message != "no device" || !devices[0].Encoder.Usable {
		t.Fatalf("Detect = %+v, %v", devices, err)
	}

	if got.Executable != "ffmpeg" || got.Device != "/dev/dri/renderD129" {
		t.Errorf("detection options = %+v, want the configured executable and device", got)
	}

	failing := hardwareDetector{detect: func(context.Context, ffmpeg.HardwareDetectionOptions) ([]ffmpeg.HardwareResult, error) {
		return nil, io.ErrUnexpectedEOF
	}}

	if _, err := failing.Detect(t.Context()); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("Detect error = %v, want the cause wrapped", err)
	}
}

// fakeProduction is a started production whose Ready the test controls.
type fakeProduction struct {
	ready chan struct{}
}

func (production *fakeProduction) Ready(ctx context.Context) error {
	select {
	case <-production.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err() //nolint:wrapcheck // a fake
	}
}

func (*fakeProduction) Stop()                 {}
func (*fakeProduction) Done() <-chan struct{} { return nil }

type fakeProducer struct {
	production *fakeProduction
	err        error
}

func (producer fakeProducer) Start(context.Context, playback.Job) (playback.Production, error) {
	if producer.err != nil {
		return nil, producer.err
	}

	return producer.production, nil
}

// lockCounter counts the hardware start locks held.
type lockCounter struct {
	held     atomic.Int32
	acquired atomic.Int32
}

func (counter *lockCounter) acquire() func() {
	counter.held.Add(1)
	counter.acquired.Add(1)

	return func() { counter.held.Add(-1) }
}

func awaitHeld(t *testing.T, counter *lockCounter, want int32) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for counter.held.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("locks held = %d, want %d", counter.held.Load(), want)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// TestHardwareStartGuardHoldsTheLockUntilTheFirstSegment: a hardware test must not run while a
// hardware session opens the device (M4-13d); software starts do not wait for it.
func TestHardwareStartGuardHoldsTheLockUntilTheFirstSegment(t *testing.T) {
	t.Parallel()

	counter := &lockCounter{}
	production := &fakeProduction{ready: make(chan struct{})}
	guard := hardwareStartGuard{inner: fakeProducer{production: production}, acquire: counter.acquire}

	if _, err := guard.Start(t.Context(), playback.Job{}); err != nil || counter.acquired.Load() != 0 {
		t.Fatalf("software start = %v, locks acquired %d, want none", err, counter.acquired.Load())
	}

	if _, err := guard.Start(t.Context(), playback.Job{Encoder: domainplayback.Encoder{Hardware: true}}); err != nil {
		t.Fatalf("hardware start: %v", err)
	}

	awaitHeld(t, counter, 1)
	close(production.ready)
	awaitHeld(t, counter, 0)
}

func TestHardwareStartGuardReleasesAfterAFailedStart(t *testing.T) {
	t.Parallel()

	counter := &lockCounter{}
	guard := hardwareStartGuard{inner: fakeProducer{err: io.ErrClosedPipe}, acquire: counter.acquire}

	_, err := guard.Start(t.Context(), playback.Job{Encoder: domainplayback.Encoder{Hardware: true}})
	if !errors.Is(err, io.ErrClosedPipe) || counter.held.Load() != 0 || counter.acquired.Load() != 1 {
		t.Errorf("Start = %v, held %d, acquired %d, want the cause, 0 held, 1 acquired", err, counter.held.Load(), counter.acquired.Load())
	}
}

func TestHardwareStartGuardReleasesWhenTheServiceEnds(t *testing.T) {
	t.Parallel()

	counter := &lockCounter{}
	guard := hardwareStartGuard{inner: fakeProducer{production: &fakeProduction{ready: make(chan struct{})}}, acquire: counter.acquire}
	ctx, cancel := context.WithCancel(t.Context())

	if _, err := guard.Start(ctx, playback.Job{Encoder: domainplayback.Encoder{Hardware: true}}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	awaitHeld(t, counter, 1)
	cancel()
	awaitHeld(t, counter, 0)
}

// TestSoxDetectionNeedsAnFFmpeg: without ffmpeg nothing is probed.
func TestSoxDetectionNeedsAnFFmpeg(t *testing.T) {
	t.Parallel()

	if transcodeApp(config.Transcode{}).detectSoxResampler(t.Context(), os.DevNull, false) {
		t.Error("soxr reported without ffmpeg")
	}

	if transcodeApp(config.Transcode{}).detectSoxResampler(t.Context(), os.DevNull, true) {
		t.Error("soxr reported for an executable that cannot answer")
	}
}

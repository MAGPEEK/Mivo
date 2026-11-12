package transcoder

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// ErrBusy reports that a hardware test is already running. It wraps domain.ErrConflict, which
// adapters/http maps to 409.
var ErrBusy = fmt.Errorf("hardware test already running: %w", domain.ErrConflict)

// ErrDisabled reports a hardware test while transcode.hwaccel is off. It wraps domain.ErrInvalid.
var ErrDisabled = fmt.Errorf("hardware acceleration is off: %w", domain.ErrInvalid)

// Device is one tested encoder with the short failure reason of its test. Message lives here,
// not on playback.HardwareEncoder: the playback decision only needs Available, the reason is
// diagnostics for the settings screen (decision of M4-13d).
type Device struct {
	Encoder playback.HardwareEncoder
	// Message is empty when the encoder is usable.
	Message string
}

// Detector lists and test-encodes the hardware encoders. Implemented in cmd/mivo on top of
// adapters/ffmpeg.DetectHardwareResults.
type Detector interface {
	// Detect returns every candidate with its test result; empty on a machine without hardware
	// encoders. It runs one test encode per candidate and must honour ctx.
	Detect(ctx context.Context) ([]Device, error)
}

// Options configure a Service.
type Options struct {
	// Detector runs the hardware detection. Required.
	Detector Detector
	// Logger receives the detection outcome. Required.
	Logger *slog.Logger
	// Base are the FFmpeg capabilities without hardware encoders, detected at startup.
	Base playback.ServerCapabilities
	// Setting is transcode.hwaccel; the zero value is auto.
	Setting playback.HardwareAcceleration
	// ThrottleSupported reports that the platform can pause FFmpeg (ADR 0030 section 2).
	ThrottleSupported bool
	// Now returns the current time; nil selects time.Now.
	Now func() time.Time
}

// Status is the snapshot behind the settings screen.
type Status struct {
	Setting playback.HardwareAcceleration
	// Detecting reports a running test; Devices is the previous result meanwhile.
	Detecting bool
	Devices   []Device
	// ThrottleSupported reports that the platform can pause FFmpeg.
	ThrottleSupported bool
	// LastTestedAt is zero until a detection has succeeded.
	LastTestedAt time.Time
}

// Service tracks the hardware encoders. Safe for concurrent use.
type Service struct {
	detector Detector
	logger   *slog.Logger
	base     playback.ServerCapabilities
	throttle bool
	now      func() time.Time

	// startLock serialises detection (writer) against hardware transcode starts (readers), so
	// a test encode never collides with the first hardware session on the same device.
	startLock sync.RWMutex

	mu         sync.Mutex
	setting    playback.HardwareAcceleration
	detecting  bool
	devices    []Device
	degraded   map[playback.HardwareAPI]bool
	lastTested time.Time
}

// New builds a Service. Detection has not run; call Start.
func New(options Options) (*Service, error) {
	if options.Detector == nil || options.Logger == nil {
		return nil, fmt.Errorf("new transcoder service: detector and logger are required: %w", domain.ErrInvalid)
	}

	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Service{
		detector: options.Detector, logger: options.Logger, base: options.Base,
		throttle: options.ThrottleSupported, now: now, setting: options.Setting,
		degraded: map[playback.HardwareAPI]bool{},
	}, nil
}

// Start runs the detection in the background and returns a channel that closes when it ended.
// Until then Capabilities lists no hardware encoder, so sessions use software. With
// hwaccel=off nothing runs and the channel is already closed.
func (s *Service) Start(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		if s.isOff() {
			s.logger.InfoContext(ctx, "hardware acceleration is off, skipping detection")

			return
		}

		if _, err := s.Test(ctx); err != nil && !errors.Is(err, ErrBusy) {
			s.logger.WarnContext(ctx, "hardware detection failed, using software encoding", slog.Any("error", err))
		}
	}()

	return done
}

// Test re-runs the detection and publishes the result atomically. It returns ErrBusy while
// another test runs and ErrDisabled when hwaccel is off. A failed detection keeps the
// previous result. A successful one clears the degraded marks.
func (s *Service) Test(ctx context.Context) (Status, error) {
	if err := s.begin(); err != nil {
		return Status{}, err
	}

	devices, err := s.detectExclusively(ctx)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.detecting = false

	if err != nil {
		return Status{}, fmt.Errorf("test hardware acceleration: %w", err)
	}

	s.devices = devices
	s.degraded = map[playback.HardwareAPI]bool{}
	s.lastTested = s.now()

	if len(devices) == 0 {
		s.logger.InfoContext(ctx, "no hardware encoders found, using software encoding")
	}

	return s.statusLocked(), nil
}

func (s *Service) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.setting == playback.HardwareAccelerationOff {
		return ErrDisabled
	}

	if s.detecting {
		return ErrBusy
	}

	s.detecting = true

	return nil
}

func (s *Service) detectExclusively(ctx context.Context) ([]Device, error) {
	s.startLock.Lock()
	defer s.startLock.Unlock()

	devices, err := s.detector.Detect(ctx)
	if err != nil {
		return nil, fmt.Errorf("detect: %w", err)
	}

	return slices.Clone(devices), nil
}

// AcquireHardwareStart must be held by a caller while it starts a hardware transcode; it
// waits for a running detection and blocks detection until released.
func (s *Service) AcquireHardwareStart() (release func()) {
	s.startLock.RLock()

	return s.startLock.RUnlock
}

// MarkDegraded excludes the encoders of the API until the next successful Test. It implements
// playback.HardwareStatus.
func (s *Service) MarkDegraded(api playback.HardwareAPI) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.degraded[api] = true
}

// SetSetting applies a changed transcode.hwaccel. It does not start a detection.
func (s *Service) SetSetting(setting playback.HardwareAcceleration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setting = setting
}

// Status returns the current snapshot.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.statusLocked()
}

// Capabilities merges the FFmpeg base capabilities with the usable hardware encoders for
// playback.Decide. It never blocks on a running detection.
func (s *Service) Capabilities() playback.ServerCapabilities {
	s.mu.Lock()
	defer s.mu.Unlock()

	capabilities := s.base
	capabilities.HardwareEncoders = nil

	if s.setting == playback.HardwareAccelerationOff {
		return capabilities
	}

	for _, device := range s.statusLocked().Devices {
		capabilities.HardwareEncoders = append(capabilities.HardwareEncoders, device.Encoder)
	}

	return capabilities
}

func (s *Service) isOff() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.setting == playback.HardwareAccelerationOff
}

func (s *Service) statusLocked() Status {
	devices := make([]Device, len(s.devices))
	for index, device := range s.devices {
		device.Encoder.Degraded = device.Encoder.Degraded || s.degraded[device.Encoder.API]
		devices[index] = device
	}

	return Status{
		Setting: s.setting, Detecting: s.detecting, Devices: devices,
		ThrottleSupported: s.throttle, LastTestedAt: s.lastTested,
	}
}

package transcoder_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/app/transcoder"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// fakeDetector is a scripted Detector. When entered is set, Detect announces itself and waits
// for release, which lets a test observe the service during a slow detection.
type fakeDetector struct {
	devices []transcoder.Device
	err     error
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (d *fakeDetector) Detect(ctx context.Context) ([]transcoder.Device, error) {
	d.calls.Add(1)

	if d.entered != nil {
		d.entered <- struct{}{}

		select {
		case <-d.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return d.devices, d.err
}

func vaapiEncoder() playback.HardwareEncoder {
	return playback.HardwareEncoder{API: playback.HardwareAPIVAAPI, Name: "h264_vaapi", Usable: true}
}

func nvencEncoder() playback.HardwareEncoder {
	return playback.HardwareEncoder{API: playback.HardwareAPINVENC, Name: "h264_nvenc"}
}

func testNow() time.Time { return time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC) }

func newService(t *testing.T, detector *fakeDetector, setting playback.HardwareAcceleration) *transcoder.Service {
	t.Helper()

	service, err := transcoder.New(transcoder.Options{
		Detector: detector, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Base:    playback.ServerCapabilities{H264Encoder: "libx264"},
		Setting: setting, ThrottleSupported: true, Now: testNow,
	})
	if err != nil {
		t.Fatal(err)
	}

	return service
}

func TestNewRequiresDetectorAndLogger(t *testing.T) {
	if _, err := transcoder.New(transcoder.Options{}); !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestTestPublishesResult(t *testing.T) {
	detector := &fakeDetector{devices: []transcoder.Device{{Encoder: vaapiEncoder()}, {Encoder: nvencEncoder(), Message: "device_not_found"}}}
	service := newService(t, detector, "")

	status, err := service.Test(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	if len(status.Devices) != 2 || status.Devices[1].Message != "device_not_found" ||
		!status.LastTestedAt.Equal(testNow()) || !status.ThrottleSupported || status.Detecting {
		t.Fatalf("status = %+v", status)
	}

	capabilities := service.Capabilities()
	if capabilities.H264Encoder != "libx264" || len(capabilities.HardwareEncoders) != 2 {
		t.Fatalf("capabilities = %+v", capabilities)
	}
}

func TestTestFailureKeepsPreviousResult(t *testing.T) {
	detector := &fakeDetector{devices: []transcoder.Device{{Encoder: vaapiEncoder()}}}
	service := newService(t, detector, "")

	if _, err := service.Test(t.Context()); err != nil {
		t.Fatal(err)
	}

	detector.err = domain.ErrUnavailable
	if _, err := service.Test(t.Context()); !errors.Is(err, domain.ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}

	status := service.Status()
	if len(status.Devices) != 1 || status.Detecting {
		t.Fatalf("status = %+v", status)
	}
}

func TestCPUOnlyMachineEndsWithEmptyList(t *testing.T) {
	service := newService(t, &fakeDetector{}, "")

	<-service.Start(t.Context())

	status := service.Status()
	if len(status.Devices) != 0 || status.LastTestedAt.IsZero() {
		t.Fatalf("status = %+v", status)
	}
}

func TestStartFailureLeavesSoftware(t *testing.T) {
	service := newService(t, &fakeDetector{err: errors.New("boom")}, "")

	<-service.Start(t.Context())

	if got := service.Capabilities().HardwareEncoders; len(got) != 0 {
		t.Fatalf("encoders = %v", got)
	}
}

func TestMarkDegradedUntilNextSuccessfulTest(t *testing.T) {
	detector := &fakeDetector{devices: []transcoder.Device{{Encoder: vaapiEncoder()}}}
	service := newService(t, detector, "")

	if _, err := service.Test(t.Context()); err != nil {
		t.Fatal(err)
	}

	service.MarkDegraded(playback.HardwareAPIVAAPI)

	if encoders := service.Capabilities().HardwareEncoders; len(encoders) != 1 || encoders[0].Available() {
		t.Fatalf("encoders = %+v", encoders)
	}

	detector.err = errors.New("boom")
	_, _ = service.Test(t.Context())

	if service.Capabilities().HardwareEncoders[0].Available() {
		t.Fatal("failed test cleared the degraded mark")
	}

	detector.err = nil
	if _, err := service.Test(t.Context()); err != nil {
		t.Fatal(err)
	}

	if !service.Capabilities().HardwareEncoders[0].Available() {
		t.Fatal("successful test did not clear the degraded mark")
	}
}

func TestOffNeverDetects(t *testing.T) {
	detector := &fakeDetector{devices: []transcoder.Device{{Encoder: vaapiEncoder()}}}
	service := newService(t, detector, playback.HardwareAccelerationOff)

	<-service.Start(t.Context())

	if _, err := service.Test(t.Context()); !errors.Is(err, transcoder.ErrDisabled) {
		t.Fatalf("err = %v", err)
	}

	if detector.calls.Load() != 0 || len(service.Capabilities().HardwareEncoders) != 0 {
		t.Fatalf("calls = %d", detector.calls.Load())
	}
}

func TestSetSettingOffHidesEncoders(t *testing.T) {
	service := newService(t, &fakeDetector{devices: []transcoder.Device{{Encoder: vaapiEncoder()}}}, "")
	if _, err := service.Test(t.Context()); err != nil {
		t.Fatal(err)
	}

	service.SetSetting(playback.HardwareAccelerationOff)

	if len(service.Capabilities().HardwareEncoders) != 0 {
		t.Fatal("encoders listed while off")
	}
}

func TestSlowDetectionBlocksHardwareStartsAndSessionsUseSoftware(t *testing.T) {
	detector := &fakeDetector{
		devices: []transcoder.Device{{Encoder: vaapiEncoder()}},
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	service := newService(t, detector, "")
	done := service.Start(t.Context())

	<-detector.entered

	if !service.Status().Detecting || len(service.Capabilities().HardwareEncoders) != 0 {
		t.Fatal("capabilities must be software-only while detecting")
	}

	if _, err := service.Test(t.Context()); !errors.Is(err, transcoder.ErrBusy) || !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("err = %v, want ErrBusy", err)
	}

	acquired := make(chan struct{})

	go func() {
		service.AcquireHardwareStart()()
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("hardware start did not wait for detection")
	case <-time.After(50 * time.Millisecond):
	}

	close(detector.release)
	<-done
	<-acquired

	if len(service.Capabilities().HardwareEncoders) != 1 {
		t.Fatal("result not published")
	}
}

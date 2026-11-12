package restapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/app/transcoder"
	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// transcoderTestStartGrace is how long testTranscoder waits for the refusals a run reports
	// at once (a run is active, hwaccel is off) before it answers 202 for a run that goes on.
	// transcoder.Service.Test decides those before it detects, so they arrive within
	// microseconds.
	transcoderTestStartGrace = 100 * time.Millisecond
	// transcoderTestTimeout bounds a background run: ADR 0030 section 5 allows 10 seconds per
	// encoder, and a machine lists a handful.
	transcoderTestTimeout = 2 * time.Minute
)

// GetTranscoderStatus answers the hardware acceleration setting and the tested encoders.
func (api *API) GetTranscoderStatus(_ context.Context, _ apigen.GetTranscoderStatusRequestObject) (apigen.GetTranscoderStatusResponseObject, error) {
	if api.streaming.Transcoder == nil {
		problem := notImplementedProblem()

		return apigen.GetTranscoderStatusdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	return apigen.GetTranscoderStatus200JSONResponse(newTranscoderStatus(api.streaming.Transcoder.Status())), nil
}

// TestTranscoder starts a detection run in the background and answers 202 with the status as it
// is now. The use case blocks until the run ended, so the run goes on after the response and is
// detached from the request context; only a refusal it reports at once (409 while another run
// is active) is answered here.
func (api *API) TestTranscoder(ctx context.Context, _ apigen.TestTranscoderRequestObject) (apigen.TestTranscoderResponseObject, error) {
	if api.streaming.Transcoder == nil {
		problem := notImplementedProblem()

		return apigen.TestTranscoderdefaultApplicationProblemPlusJSONResponse{Body: problem, StatusCode: int(problem.Status)}, nil
	}

	outcome := make(chan error, 1)

	go api.runTranscoderTest(context.WithoutCancel(ctx), outcome)

	select {
	case err := <-outcome:
		if err != nil {
			return nil, operationFailure(operationTestTranscoder, err,
				http.StatusBadRequest, http.StatusConflict, http.StatusServiceUnavailable)
		}
	case <-time.After(transcoderTestStartGrace):
	case <-ctx.Done():
	}

	return apigen.TestTranscoder202JSONResponse(newTranscoderStatus(api.streaming.Transcoder.Status())), nil
}

// runTranscoderTest owns the run: it reports the outcome to the request while that listens and
// logs a failure of the detection itself, which nobody else would see.
func (api *API) runTranscoderTest(ctx context.Context, outcome chan<- error) {
	ctx, cancel := context.WithTimeout(ctx, transcoderTestTimeout)
	defer cancel()

	_, err := api.streaming.Transcoder.Test(ctx)
	outcome <- err

	if err != nil && !errors.Is(err, transcoder.ErrBusy) && !errors.Is(err, transcoder.ErrDisabled) {
		api.logger.WarnContext(ctx, "transcoder test failed", slog.Any("error", err))
	}
}

func newTranscoderStatus(status transcoder.Status) apigen.TranscoderStatus {
	setting := status.Setting
	if setting == "" {
		setting = playback.HardwareAccelerationAuto
	}

	devices := make([]apigen.TranscoderDevice, 0, len(status.Devices))
	for _, device := range status.Devices {
		devices = append(devices, newTranscoderDevice(device))
	}

	return apigen.TranscoderStatus{
		Hwaccel: apigen.HardwareAccelerationSetting(setting), Devices: devices, ThrottleSupported: status.ThrottleSupported,
		LastTestedAt: optionalTimestamp(status.LastTestedAt),
	}
}

func newTranscoderDevice(device transcoder.Device) apigen.TranscoderDevice {
	encoder := device.Encoder

	return apigen.TranscoderDevice{
		API: apigen.HardwareAccelerationAPI(encoder.API), Encoder: encoder.Name, Codec: encoder.Codec,
		Usable: encoder.Usable, Degraded: encoder.Degraded, Message: pathFreeMessage(device.Message),
	}
}

// pathFreeMessage drops a message that names a path: the contract promises none, and the
// detector's texts come from FFmpeg and the OS, which quote device nodes. Dropping is safer
// than trying to cut the path out of a sentence.
func pathFreeMessage(message string) *string {
	if strings.ContainsAny(message, `/\`) {
		return nil
	}

	return optionalString(message)
}

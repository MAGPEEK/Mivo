package ffmpeg

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/magpeek/mivo/internal/domain/playback"
)

const (
	// decodeSampleSource is the generated 720p clip a decode test reads: the software
	// encoder makes it, so the test needs no file and stays independent of the library.
	decodeSampleSource = "testsrc2=size=1280x720:rate=25:duration=1"
	// decodeSampleBytesLimit bounds the sample; one second of 720p test video is far below it.
	decodeSampleBytesLimit = 4 << 20
	// decodeSampleFormat is the raw H.264 stream the decode test reads from standard input.
	decodeSampleFormat = "h264"
)

// decodeTester verifies, after a successful test encode, that the same device also decodes
// H.264 in hardware (ADR 0030 §5): hardware decode is used only for combinations a run proved.
// It builds its sample on first use, so a machine without hardware encoders runs nothing.
type decodeTester struct {
	ffmpeg  tool
	options HardwareDetectionOptions
	sample  []byte
	// sampled is set once the sample was tried, so a failing sample is not retried per candidate.
	sampled bool
}

// supportsDecode reports whether hardware decode is worth testing for the encoder's API; AMF
// decodes through DirectX in a way the filter chain here does not reproduce.
func supportsDecode(api playback.HardwareAPI) bool {
	return api == playback.HardwareAPINVENC || api == playback.HardwareAPIQSV || api == playback.HardwareAPIVAAPI
}

// timeout bounds the sample and each decode test like a test encode.
func (tester *decodeTester) timeout() time.Duration {
	if tester.options.TestTimeout > 0 {
		return tester.options.TestTimeout
	}

	return hardwareTestTimeout
}

// withDecode sets Decode on the result of a usable encoder.
func (tester *decodeTester) withDecode(ctx context.Context, result HardwareResult) (HardwareResult, error) {
	if !result.Encoder.Usable {
		return result, nil
	}

	decode, err := tester.verify(ctx, result.Encoder)
	result.Encoder.Decode = decode

	return result, err
}

// verify returns whether encoder decodes in hardware. Only the caller's cancellation is an
// error; a failing test means "decode in software".
func (tester *decodeTester) verify(ctx context.Context, encoder playback.HardwareEncoder) (bool, error) {
	if !supportsDecode(encoder.API) {
		return false, nil
	}

	sample, err := tester.sampleStream(ctx)
	if err != nil || len(sample) == 0 {
		return false, err
	}

	testContext, cancel := context.WithTimeout(ctx, tester.timeout())
	defer cancel()

	encoder.Decode = true

	_, runErr := tester.ffmpeg.pipedOutput(testContext, decodeTestArguments(encoder), sample, maxOutputBytes)
	if runErr == nil {
		return true, nil
	}

	if ctxErr := ctx.Err(); ctxErr != nil {
		return false, fmt.Errorf("detect hardware encoders: %w", ctxErr)
	}

	attributes := append([]slog.Attr{
		slog.String("encoder", encoder.Name), slog.String("reason", failureMessage(runErr)), slog.Any("error", runErr),
	}, exitAttributes(runErr)...)
	tester.options.Logger.LogAttrs(ctx, slog.LevelInfo, "hardware decode test failed", attributes...)

	return false, nil
}

// sampleStream returns the sample, generating it on first use. A machine whose FFmpeg cannot
// encode it (no libx264) gets none, and its encoders decode in software.
func (tester *decodeTester) sampleStream(ctx context.Context) ([]byte, error) {
	if tester.sampled {
		return tester.sample, nil
	}

	tester.sampled = true

	sampleContext, cancel := context.WithTimeout(ctx, tester.timeout())
	defer cancel()

	sample, err := tester.ffmpeg.boundedOutput(sampleContext, decodeSampleArguments(), decodeSampleBytesLimit)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("detect hardware encoders: %w", ctxErr)
		}

		tester.options.Logger.LogAttrs(ctx, slog.LevelInfo, "hardware decode sample could not be made",
			slog.Any("error", err))

		return nil, nil
	}

	tester.sample = sample

	return sample, nil
}

// decodeSampleArguments encodes the sample clip with the software encoder to standard output.
func decodeSampleArguments() []string {
	return []string{
		hideBannerFlag, "-nostdin", "-v", "error", "-f", "lavfi", "-i", decodeSampleSource,
		"-pix_fmt", transcodePixelFormat, "-c:v", playback.EncoderLibx264, "-preset", "ultrafast",
		"-f", decodeSampleFormat, "-",
	}
}

// decodeTestArguments decodes the sample on the device and encodes it with the encoder to the
// null muxer: the options of a real session, with the input read from standard input.
func decodeTestArguments(encoder playback.HardwareEncoder) []string {
	video := hardwareVideo{encoder: encoder}

	arguments := []string{hideBannerFlag, "-nostdin", "-v", "error"}
	arguments = append(arguments, video.inputArguments()...)
	arguments = append(arguments, "-f", decodeSampleFormat, "-i", "-")

	if upload := video.uploadFilter(); len(upload) > 0 {
		arguments = append(arguments, "-vf", upload[0])
	}

	return append(arguments, "-c:v", encoder.Name, "-f", "null", "-")
}

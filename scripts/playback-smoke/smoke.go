package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
	"uuid"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
)

const (
	// stopTimeout bounds the DELETE of a session after its case, also after a case timeout.
	stopTimeout = 20 * time.Second
	// downloadPattern names the temporary file a stream is written to before probing.
	downloadPattern = "playback-smoke-*.media"
	// labelWidth aligns the case lines; the longest label is a seek case.
	labelWidth = 34
	modeWidth  = 11
)

var errCasesFailed = errors.New("at least one case did not play as expected")

// smokeRunner plays the cases one after another against the server. Cases run sequentially
// so that no case waits for the transcode limit and every timing in the output is the time
// of one session.
type smokeRunner struct {
	client  *apiClient
	prober  prober
	profile apigen.ClientProfile
	files   fileIndex
	poll    pollSettings
	// caseTimeout bounds one case from the session's creation to the end of the probe.
	caseTimeout time.Duration
	// workDir receives the temporary downloads.
	workDir string
}

// caseOutcome is the result of one case.
type caseOutcome struct {
	mode     string
	detail   string
	failures []string
	elapsed  time.Duration
}

func (outcome *caseOutcome) fail(format string, arguments ...any) {
	outcome.failures = append(outcome.failures, fmt.Sprintf(format, arguments...))
}

// runCase creates the session, checks the plan, downloads and probes the stream and stops
// the session. Every mismatch ends up in the outcome; nothing is returned as an error, so
// the next case still runs.
func (runner *smokeRunner) runCase(ctx context.Context, smoke smokeCase) (outcome caseOutcome) {
	began := time.Now()

	defer func() { outcome.elapsed = time.Since(began) }()

	fileID, err := runner.files.lookup(smoke.clip.fileName())
	if err != nil {
		outcome.fail("%v", err)

		return outcome
	}

	caseContext, cancel := context.WithTimeout(ctx, runner.caseTimeout)
	defer cancel()

	session, err := createSession(caseContext, runner.client, runner.request(fileID, smoke), runner.poll)
	if err != nil {
		outcome.fail("%v", err)

		return outcome
	}

	defer runner.stop(ctx, session.ID, &outcome)

	outcome.mode = string(session.Mode)
	runner.verify(caseContext, smoke, session, &outcome)

	return outcome
}

func (runner *smokeRunner) request(fileID uuid.UUID, smoke smokeCase) apigen.PlaybackRequest {
	request := apigen.PlaybackRequest{FileID: fileID, Profile: runner.profile}

	if smoke.start > 0 {
		start := smoke.start.Seconds()
		request.StartPositionSeconds = &start
	}

	return request
}

// stop deletes the session with its own deadline, so a case that ran out of time still
// cleans up on the server.
func (runner *smokeRunner) stop(ctx context.Context, id uuid.UUID, outcome *caseOutcome) {
	stopContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer cancel()

	if err := stopSession(stopContext, runner.client, id); err != nil {
		outcome.fail("%v", err)
	}
}

func (runner *smokeRunner) verify(ctx context.Context, smoke smokeCase, session apigen.PlaybackSession, outcome *caseOutcome) {
	mismatches, fatal := planMismatches(session, smoke.want)
	outcome.failures = append(outcome.failures, mismatches...)

	if fatal {
		return
	}

	if mismatch, found := startMismatch(session, smoke); found {
		outcome.fail("%s", mismatch)
	}

	output, err := runner.download(ctx, session)
	if err != nil {
		outcome.fail("%v%s", err, runner.failureCode(ctx, session.ID))

		return
	}

	start := seconds(session.StartPositionSeconds)
	outcome.failures = append(outcome.failures, outputMismatches(smoke, start, output)...)
	outcome.detail = describeOutput(session, start, output)
}

// download writes the stream into a temporary file, probes it and removes the file.
func (runner *smokeRunner) download(ctx context.Context, session apigen.PlaybackSession) (outputSummary, error) {
	target, err := resolve(runner.client.base, session.StreamURL)
	if err != nil {
		return outputSummary{}, err
	}

	file, err := os.CreateTemp(runner.workDir, downloadPattern)
	if err != nil {
		return outputSummary{}, fmt.Errorf("create the download file: %w", err)
	}

	// The file is temporary; failing to remove it changes no result.
	defer func() { _ = os.Remove(file.Name()) }()

	err = runner.fetch(ctx, session.Delivery, target, file)
	if closeErr := file.Close(); err == nil && closeErr != nil {
		err = fmt.Errorf("write the download file: %w", closeErr)
	}

	if err != nil {
		return outputSummary{}, err
	}

	return runner.prober.Probe(ctx, file.Name())
}

func (runner *smokeRunner) fetch(ctx context.Context, delivery apigen.PlaybackDelivery, target *url.URL, destination io.Writer) error {
	if delivery == apigen.PlaybackDeliveryFile {
		return runner.downloader().direct(ctx, target, destination)
	}

	return runner.downloader().hls(ctx, target, destination)
}

func (runner *smokeRunner) downloader() streamDownloader {
	return streamDownloader{client: runner.client, poll: runner.poll}
}

// failureCode adds the session's state to a download error, for example the failure code
// of an FFmpeg run that broke off; it is empty when the session cannot be read.
func (runner *smokeRunner) failureCode(ctx context.Context, id uuid.UUID) string {
	readContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), stopTimeout)
	defer cancel()

	session, err := getSession(readContext, runner.client, id)
	if err != nil {
		return ""
	}

	return fmt.Sprintf(" (session %s, failure code %s)", session.State, valueOr(session.FailureCode, "none"))
}

// describeOutput is the detail of a passing case line.
func describeOutput(session apigen.PlaybackSession, start time.Duration, output outputSummary) string {
	source := "HLS"
	if session.Delivery == apigen.PlaybackDeliveryFile {
		source = "original file, range 206"
	}

	detail := fmt.Sprintf("video %s %s, audio %s %s, %.2f s, %s",
		streamAction(session, apigen.MediaStreamKindVideo), output.videoCodec,
		streamAction(session, apigen.MediaStreamKindAudio), output.audioCodec,
		output.duration.Seconds(), source)

	if start > 0 {
		detail += fmt.Sprintf(", starts at %.3f s", start.Seconds())
	}

	return detail
}

// report prints the line of one case.
func report(out io.Writer, smoke smokeCase, outcome caseOutcome) {
	status, detail := "ok  ", outcome.detail
	if len(outcome.failures) > 0 {
		status, detail = "FAIL", strings.Join(outcome.failures, "; ")
	}

	mode := outcome.mode
	if mode == "" {
		mode = "-"
	}

	// A failed write to the terminal cannot be reported anywhere else.
	_, _ = fmt.Fprintf(out, "%s %-*s %-*s %s (%s)\n", status, labelWidth, smoke.label, modeWidth, mode,
		detail, outcome.elapsed.Round(time.Millisecond))
}

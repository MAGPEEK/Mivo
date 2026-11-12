package ffmpeg

import (
	"context"
	"log/slog"

	appplayback "github.com/magpeek/mivo/internal/app/playback"
	"github.com/magpeek/mivo/internal/domain/media"
	"github.com/magpeek/mivo/internal/domain/playback"
)

// Software fallback of a hardware transcode (ADR 0030 §5): a hardware encoder that fails before
// the first segment is replaced by libx264 on the same input and position, so the viewer notices
// a delay and nothing else.

// prepare builds the run of job. For a hardware transcode it also builds the software run
// that stands in when the hardware one fails.
func (p *Producer) prepare(ctx context.Context, job appplayback.Job) (preparedRun, error) {
	run, err := p.prepareRun(ctx, job)
	if err != nil || !usesHardwareVideo(job.Plan) {
		return run, err
	}

	softwareJob := job
	softwareJob.Plan = softwarePlan(job.Plan)

	software, err := p.prepareRun(ctx, softwareJob)
	if err != nil {
		closeAll(run.inherited)

		if run.release != nil {
			run.release()
		}

		return preparedRun{}, err
	}

	run.fallback = &software
	run.release = joinReleases(run.release, software)

	return run, nil
}

// joinReleases frees the hardware run and the fallback run together, once the production has
// ended: the fallback's inherited files must stay open until then.
func joinReleases(first func(), fallback preparedRun) func() {
	return func() {
		if first != nil {
			first()
		}

		closeAll(fallback.inherited)

		if fallback.release != nil {
			fallback.release()
		}
	}
}

// usesHardwareVideo reports a plan whose video is transcoded by a hardware encoder.
func usesHardwareVideo(plan playback.Plan) bool {
	video, found := plan.Video()

	return found && video.Action == playback.ActionTranscode && video.Hardware
}

// softwarePlan returns plan with its video encoded by libx264. The plan itself stays as it
// is: its slices belong to the caller.
func softwarePlan(plan playback.Plan) playback.Plan {
	streams := make([]playback.StreamDecision, len(plan.Streams))

	for index, decision := range plan.Streams {
		if decision.Kind == media.StreamKindVideo && decision.Action == playback.ActionTranscode {
			decision.Encoder, decision.Hardware = playback.EncoderLibx264, false
		}

		streams[index] = decision
	}

	plan.Streams = streams

	return plan
}

// Fallback reports that the hardware encoder failed before its first segment and software
// encodes the session instead; the playback service marks the encoder degraded then.
func (p *production) Fallback() bool {
	return p.fellBack.Load()
}

// fallBack replaces a hardware run that ended before its first segment with the software run
// and returns the process to wait for. It returns its inputs when there is nothing to replace:
// a copy or software job, a process that was stopped, or one that wrote a segment already.
func (p *production) fallBack(ctx context.Context, failed *process, waitErr error) (*process, error) {
	if p.fallbackStart == nil || failed.ctx.Err() != nil || playlistListsSegment(p.directory) {
		return failed, waitErr
	}

	attributes := []slog.Attr{slog.String("playback_id", p.sessionID), slog.String(stderrKey, failed.stderrTail())}
	if code, exited := exitCode(orNoSegment(waitErr)); exited {
		attributes = append(attributes, slog.Int(exitCodeKey, code))
	}

	p.logger.LogAttrs(ctx, slog.LevelInfo, "hardware encoder failed before the first segment, retrying in software", attributes...)

	software, err := p.fallbackStart()
	if err != nil {
		p.logger.LogAttrs(ctx, slog.LevelWarn, "the software retry could not be started",
			slog.String("playback_id", p.sessionID), slog.String("error", err.Error()))

		return failed, waitErr
	}

	// Only a retry that runs counts as a fallback; the service marks the encoder degraded on it.
	p.fellBack.Store(true)
	p.pause.bind(software.command.Process)

	if software.priorityError != nil {
		p.logger.LogAttrs(ctx, slog.LevelWarn, "ffmpeg runs at normal priority",
			slog.String("playback_id", p.sessionID), slog.String("error", software.priorityError.Error()))
	}

	return software, software.wait()
}

// orNoSegment names a clean exit without segments the way outcome does.
func orNoSegment(waitErr error) error {
	if waitErr == nil {
		return errNoSegment
	}

	return waitErr
}

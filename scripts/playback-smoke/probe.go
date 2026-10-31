package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// probeWaitDelay is how long Wait waits for ffprobe's pipes after the process was
	// killed on cancellation.
	probeWaitDelay = 5 * time.Second
	// maxProbeStderr bounds the ffprobe error output kept in an error message.
	maxProbeStderr = 2048
)

var errNoDuration = errors.New("ffprobe reports no duration")

// outputSummary is what the smoke test checks of a downloaded stream.
type outputSummary struct {
	videoCodec   string
	videoProfile string
	pixelFormat  string
	audioCodec   string
	duration     time.Duration
}

// prober describes a downloaded stream.
type prober interface {
	Probe(ctx context.Context, path string) (outputSummary, error)
}

// ffprobe runs the local ffprobe as an external process (ADR 0015).
type ffprobe struct {
	executable string
}

// Probe runs ffprobe without a shell; the process is killed when ctx is done.
func (p ffprobe) Probe(ctx context.Context, path string) (outputSummary, error) {
	// The executable comes from the -ffprobe flag of a developer tool and path is a temporary
	// file of this tool; no shell is involved.
	command := exec.CommandContext(ctx, p.executable, //nolint:gosec // see comment above
		"-v", "error", "-print_format", "json", "-show_format", "-show_streams", path)

	var stdout, stderr bytes.Buffer

	command.Stdout, command.Stderr = &stdout, &stderr
	command.WaitDelay = probeWaitDelay

	if err := command.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if len(message) > maxProbeStderr {
			message = message[len(message)-maxProbeStderr:]
		}

		return outputSummary{}, fmt.Errorf("ffprobe %s: %w: %s", path, err, message)
	}

	summary, err := parseProbe(stdout.Bytes())
	if err != nil {
		return outputSummary{}, fmt.Errorf("ffprobe %s: %w", path, err)
	}

	return summary, nil
}

type probeDocument struct {
	Streams []probeStream `json:"streams"`
	Format  struct {
		Duration string `json:"duration"`
	} `json:"format"`
}

type probeStream struct {
	CodecType   string `json:"codec_type"`
	CodecName   string `json:"codec_name"`
	Profile     string `json:"profile"`
	PixelFormat string `json:"pix_fmt"`
	Disposition struct {
		Default         int `json:"default"`
		AttachedPicture int `json:"attached_pic"`
	} `json:"disposition"`
}

// parseProbe reads ffprobe's JSON: the first video stream that is no cover picture, the
// default audio stream (the first audio stream when none is marked) and the format duration.
func parseProbe(data []byte) (outputSummary, error) {
	var document probeDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return outputSummary{}, fmt.Errorf("decode the ffprobe output: %w", err)
	}

	seconds, err := strconv.ParseFloat(document.Format.Duration, 64)
	if err != nil {
		return outputSummary{}, fmt.Errorf("%w (%q)", errNoDuration, document.Format.Duration)
	}

	summary := outputSummary{duration: time.Duration(seconds * float64(time.Second))}

	if video, found := firstStream(document.Streams, isVideo); found {
		summary.videoCodec, summary.videoProfile, summary.pixelFormat = video.CodecName, video.Profile, video.PixelFormat
	}

	audio, found := firstStream(document.Streams, isDefaultAudio)
	if !found {
		audio, found = firstStream(document.Streams, isAudio)
	}

	if found {
		summary.audioCodec = audio.CodecName
	}

	return summary, nil
}

func firstStream(streams []probeStream, matches func(probeStream) bool) (probeStream, bool) {
	for _, stream := range streams {
		if matches(stream) {
			return stream, true
		}
	}

	return probeStream{}, false
}

func isVideo(stream probeStream) bool {
	return stream.CodecType == "video" && stream.Disposition.AttachedPicture == 0
}

func isAudio(stream probeStream) bool {
	return stream.CodecType == "audio"
}

func isDefaultAudio(stream probeStream) bool {
	return isAudio(stream) && stream.Disposition.Default == 1
}

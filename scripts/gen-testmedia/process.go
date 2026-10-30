package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// processWaitDelay is how long Wait waits for FFmpeg's output pipes after the process
	// was killed on cancellation before it gives up.
	processWaitDelay = 5 * time.Second
	// maxStderrBytes limits the FFmpeg error output kept in a failure reason; the tail holds
	// the actual error.
	maxStderrBytes = 2048
	// truncatedMarker precedes stderr output whose beginning was dropped.
	truncatedMarker = "[truncated] "
)

// commandRunner runs FFmpeg with the given arguments and returns its standard output.
type commandRunner interface {
	Run(ctx context.Context, args []string) ([]byte, error)
}

// execRunner runs a real FFmpeg executable as an external process (ADR 0015).
type execRunner struct {
	executable string
}

// Run starts the executable without a shell. The process is killed when ctx is done. On
// failure the error contains the tail of FFmpeg's stderr.
func (r execRunner) Run(ctx context.Context, args []string) ([]byte, error) {
	// The executable comes from the -ffmpeg flag of a developer tool and args are built from
	// static case data; no shell is involved, so nothing can be injected.
	cmd := exec.CommandContext(ctx, r.executable, args...) //nolint:gosec // see comment above

	var stdout bytes.Buffer

	stderr := newTailBuffer(maxStderrBytes)
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = processWaitDelay

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = errors.Join(ctxErr, err)
		}

		if output := stderr.String(); output != "" {
			return nil, fmt.Errorf("%w: %s", err, output)
		}

		return nil, err
	}

	return stdout.Bytes(), nil
}

// tailBuffer is an io.Writer that keeps only the last limit bytes written to it.
type tailBuffer struct {
	limit     int
	data      []byte
	truncated bool
}

func newTailBuffer(limit int) *tailBuffer {
	return &tailBuffer{limit: limit}
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)

	if excess := len(b.data) - b.limit; excess > 0 {
		// Start at a rune boundary so a cut never leaves half a UTF-8 character in the
		// failure reason (FFmpeg prints localized file names).
		for excess < len(b.data) && !utf8.RuneStart(b.data[excess]) {
			excess++
		}

		b.data = append(b.data[:0], b.data[excess:]...)
		b.truncated = true
	}

	return len(p), nil
}

// String returns the kept output, trimmed and marked when earlier output was dropped.
func (b *tailBuffer) String() string {
	text := strings.TrimSpace(string(b.data))
	if b.truncated {
		return truncatedMarker + text
	}

	return text
}

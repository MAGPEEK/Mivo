package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// caseTimeout bounds one FFmpeg run. A 2 s clip takes well under a second with software
	// encoders; the margin covers slow machines (weak ARM NAS) and the slowest AV1 encoder.
	caseTimeout = 2 * time.Minute
	// processWaitDelay is how long Wait waits for FFmpeg's output pipes after the process
	// was killed on cancellation before it gives up.
	processWaitDelay = 5 * time.Second
	// maxStderrBytes limits the FFmpeg error output kept in a failure reason; the tail holds
	// the actual error.
	maxStderrBytes = 2048
	// partialSuffix marks files FFmpeg is still writing; they are renamed when complete.
	partialSuffix = ".partial"
	// outputDirPermissions keeps the corpus readable for the owner and group only.
	outputDirPermissions = 0o750
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

		return nil, fmt.Errorf("%w: %s", err, stderr.String())
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
		b.data = append(b.data[:0], b.data[excess:]...)
		b.truncated = true
	}

	return len(p), nil
}

// String returns the kept output, trimmed and marked when earlier output was dropped.
func (b *tailBuffer) String() string {
	text := strings.TrimSpace(string(b.data))
	if b.truncated {
		return "[truncated] " + text
	}

	return text
}

// generator builds planned cases with a fixed number of parallel FFmpeg processes.
type generator struct {
	ffmpeg   commandRunner
	outDir   string
	force    bool
	jobs     int
	progress io.Writer

	progressMutex sync.Mutex
}

// generateAll runs every plan and returns one manifest entry per plan, in plan order.
func (g *generator) generateAll(ctx context.Context, plans []plannedCase) []manifestEntry {
	entries := make([]manifestEntry, len(plans))
	work := make(chan int)

	var workers sync.WaitGroup

	for range max(g.jobs, 1) {
		workers.Go(func() {
			for index := range work {
				entries[index] = g.generate(ctx, plans[index])
				g.report(entries[index])
			}
		})
	}

	for index := range plans {
		work <- index
	}

	close(work)
	workers.Wait()

	return entries
}

// generate carries out one plan.
func (g *generator) generate(ctx context.Context, plan plannedCase) manifestEntry {
	target := filepath.Join(g.outDir, filepath.FromSlash(plan.Case.Output))
	exists := isNonEmptyFile(target)

	switch plan.Action {
	case actionKeepOnly:
		if exists {
			return newEntry(plan.Case, statusGenerated, "")
		}

		return newEntry(plan.Case, statusSkipped, plan.Reason)
	case actionSkip:
		return newEntry(plan.Case, statusSkipped, plan.Reason)
	case actionBuild:
		if exists && !g.force {
			return newEntry(plan.Case, statusGenerated, "")
		}

		if err := g.build(ctx, plan.Case, target); err != nil {
			return newEntry(plan.Case, statusFailed, err.Error())
		}

		return newEntry(plan.Case, statusGenerated, "")
	}

	return newEntry(plan.Case, statusFailed, fmt.Sprintf("unknown plan action %d", plan.Action))
}

// build writes the case to a partial file and renames it to target once FFmpeg succeeded,
// so an interrupted run never leaves a truncated file that a later run would keep.
func (g *generator) build(ctx context.Context, c mediaCase, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), outputDirPermissions); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	partial := target + partialSuffix

	caseCtx, cancel := context.WithTimeout(ctx, caseTimeout)
	defer cancel()

	if _, err := g.ffmpeg.Run(caseCtx, buildArgs(c, partial)); err != nil {
		return errors.Join(fmt.Errorf("ffmpeg: %w", err), removePartial(partial))
	}

	if err := os.Rename(partial, target); err != nil {
		return errors.Join(fmt.Errorf("finish %s: %w", c.Output, err), removePartial(partial))
	}

	return nil
}

func (g *generator) report(entry manifestEntry) {
	g.progressMutex.Lock()
	defer g.progressMutex.Unlock()

	line := fmt.Sprintf("%-9s %s", entry.Status, entry.Name)
	if entry.Reason != "" {
		line += " (" + firstLine(entry.Reason) + ")"
	}

	// Progress output is informational; the manifest is the authoritative result.
	_, _ = fmt.Fprintln(g.progress, line)
}

func removePartial(partial string) error {
	if err := os.Remove(partial); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove partial output: %w", err)
	}

	return nil
}

// isNonEmptyFile reports whether path is a regular file with content. Any stat error counts
// as missing, so the case is built again and a real problem surfaces as an FFmpeg failure.
func isNonEmptyFile(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")

	return line
}

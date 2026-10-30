package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// caseTimeout bounds one FFmpeg run. A 2 s clip takes well under a second with software
	// encoders; the margin covers slow machines (weak ARM NAS) and the slowest AV1 encoder.
	caseTimeout = 2 * time.Minute
	// partialSuffix marks files that are still being written; they are renamed when complete.
	partialSuffix = ".partial"
	// outputDirPermissions keeps the corpus readable for the owner and group only.
	outputDirPermissions = 0o750
	// interruptedReason explains cases a cancelled run did not build.
	interruptedReason = "run interrupted before the case was built"
	// inputFilePermissions protects the temporary text inputs, which only FFmpeg reads.
	inputFilePermissions = 0o600
)

// generator builds planned cases with a fixed number of parallel FFmpeg processes.
type generator struct {
	ffmpeg   commandRunner
	outDir   string
	force    bool
	jobs     int
	progress io.Writer
	// previous holds the encoders of files earlier runs built, for the entries of kept files.
	previous previousEncoders

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
	target := g.path(plan.Case.Output)

	switch plan.Action {
	case actionKeepOnly:
		return g.keptOrSkipped(plan.Case, target, skipNotSelected, plan.Reason)
	case actionSkip:
		return newSkippedEntry(plan.Case, skipMissingCapability, plan.Reason)
	case actionGap:
		return newSkippedEntry(plan.Case, skipKnownGap, plan.Reason)
	case actionBuild:
		return g.buildOrKeep(ctx, plan.Case, target)
	}

	return newEntry(plan.Case, statusFailed, fmt.Sprintf("unknown plan action %d", plan.Action))
}

// keptOrSkipped lists a file left by an earlier run as kept and skips the case otherwise.
func (g *generator) keptOrSkipped(c mediaCase, target string, kind skipKind, reason string) manifestEntry {
	if isNonEmptyFile(target) {
		return g.keptEntry(c)
	}

	return newSkippedEntry(c, kind, reason)
}

// keptEntry lists the file of an earlier run with the encoders that run recorded, never with
// the alternative this run would choose.
func (g *generator) keptEntry(c mediaCase) manifestEntry {
	return newKeptEntry(c, g.previous.of(c))
}

// buildOrKeep builds the case unless its file exists and -force is not given. Cases the run
// could not build because it was interrupted are not failures; they keep an earlier file or
// are skipped.
func (g *generator) buildOrKeep(ctx context.Context, c mediaCase, target string) manifestEntry {
	if isNonEmptyFile(target) && !g.force {
		// Sidecars are cheap text; restoring deleted ones keeps the manifest truthful.
		if err := g.writeSidecars(c, false); err != nil {
			return newEntry(c, statusFailed, err.Error())
		}

		return g.keptEntry(c)
	}

	if ctx.Err() != nil {
		return g.keptOrSkipped(c, target, skipInterrupted, interruptedReason)
	}

	if err := g.build(ctx, c, target); err != nil {
		if ctx.Err() != nil {
			return g.keptOrSkipped(c, target, skipInterrupted, interruptedReason)
		}

		return newEntry(c, statusFailed, err.Error())
	}

	return newBuiltEntry(c)
}

// build writes the case to a partial file and renames it to target once FFmpeg succeeded,
// so an interrupted run never leaves a truncated file that a later run would keep. Text
// inputs live in a temporary directory that is removed afterwards.
func (g *generator) build(ctx context.Context, c mediaCase, target string) (err error) {
	if err := os.MkdirAll(filepath.Dir(target), outputDirPermissions); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	inputDir, err := writeTextInputs(c.Inputs)
	if err != nil {
		return err
	}

	defer func() { err = errors.Join(err, removeTextInputs(inputDir)) }()

	partial := target + partialSuffix

	caseCtx, cancel := context.WithTimeout(ctx, caseTimeout)
	defer cancel()

	if _, err := g.ffmpeg.Run(caseCtx, buildArgs(c, inputDir, partial)); err != nil {
		return errors.Join(fmt.Errorf("ffmpeg: %w", err), removePartial(partial))
	}

	if err := os.Rename(partial, target); err != nil {
		return errors.Join(fmt.Errorf("finish %s: %w", c.Output, err), removePartial(partial))
	}

	return g.writeSidecars(c, true)
}

// writeSidecars writes the sidecar files of a case. Without overwrite, existing ones are kept.
func (g *generator) writeSidecars(c mediaCase, overwrite bool) error {
	for _, sidecar := range c.Sidecars {
		path := g.path(sidecar.Output)
		if !overwrite && isNonEmptyFile(path) {
			continue
		}

		if err := writeFileAtomically(path, []byte(sidecar.Content)); err != nil {
			return fmt.Errorf("sidecar %s: %w", sidecar.Output, err)
		}
	}

	return nil
}

// path converts a corpus path with forward slashes into a path below the output directory.
func (g *generator) path(output string) string {
	return filepath.Join(g.outDir, filepath.FromSlash(output))
}

func (g *generator) report(entry manifestEntry) {
	g.progressMutex.Lock()
	defer g.progressMutex.Unlock()

	line := fmt.Sprintf("%-9s %s", entry.progressLabel(), entry.Name)
	if entry.Reason != "" {
		line += " (" + firstLine(entry.Reason) + ")"
	}

	// Progress output is informational; the manifest is the authoritative result.
	_, _ = fmt.Fprintln(g.progress, line)
}

// writeTextInputs writes the text inputs of a case to a new temporary directory and returns
// it, or "" when the case has none.
func writeTextInputs(inputs []textInput) (string, error) {
	if len(inputs) == 0 {
		return "", nil
	}

	dir, err := os.MkdirTemp("", "gen-testmedia-inputs-*")
	if err != nil {
		return "", fmt.Errorf("create input directory: %w", err)
	}

	for _, input := range inputs {
		if err := os.WriteFile(filepath.Join(dir, input.FileName), []byte(input.Content), inputFilePermissions); err != nil {
			return "", errors.Join(fmt.Errorf("write input %s: %w", input.FileName, err), removeTextInputs(dir))
		}
	}

	return dir, nil
}

func removeTextInputs(dir string) error {
	if dir == "" {
		return nil
	}

	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove input directory: %w", err)
	}

	return nil
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

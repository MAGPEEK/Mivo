package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// helperModeVariable switches the test binary into a fake external process, so execRunner is
// tested with a real child process but without FFmpeg.
const helperModeVariable = "GEN_TESTMEDIA_HELPER_MODE"

func TestHelperProcess(t *testing.T) {
	mode := os.Getenv(helperModeVariable)
	if mode == "" {
		t.Skip("helper process for execRunner tests, not a test on its own")
	}

	switch mode {
	case "stdout":
		fmt.Print("Encoders:\n ------\n")
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, strings.Repeat("noise ", maxStderrBytes)+"Unknown encoder 'libfoo'\n")
		os.Exit(1)
	case "fail-silently":
		os.Exit(1)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	}
}

func helperRunner(t *testing.T, mode string) execRunner {
	t.Helper()
	t.Setenv(helperModeVariable, mode)

	return execRunner{executable: os.Args[0]}
}

func helperArgs() []string { return []string{"-test.run=^TestHelperProcess$"} }

func TestExecRunnerReturnsStdout(t *testing.T) {
	output, err := helperRunner(t, "stdout").Run(context.Background(), helperArgs())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if !strings.HasPrefix(string(output), "Encoders:") {
		t.Errorf("stdout = %q", output)
	}
}

func TestExecRunnerPutsTruncatedStderrIntoTheError(t *testing.T) {
	_, err := helperRunner(t, "fail").Run(context.Background(), helperArgs())
	if err == nil {
		t.Fatal("Run succeeded, want an error")
	}

	message := err.Error()
	if !strings.Contains(message, truncatedMarker) || !strings.HasSuffix(message, "Unknown encoder 'libfoo'") {
		t.Errorf("error does not end with the stderr tail: %q", message)
	}

	if len(message) > maxStderrBytes+len("exit status 1: "+truncatedMarker) {
		t.Errorf("error is %d bytes long, stderr was not truncated", len(message))
	}
}

func TestExecRunnerWithoutStderrHasNoTrailingSeparator(t *testing.T) {
	_, err := helperRunner(t, "fail-silently").Run(context.Background(), helperArgs())
	if err == nil {
		t.Fatal("Run succeeded, want an error")
	}

	if message := err.Error(); message != "exit status 1" {
		t.Errorf("error = %q, want the bare exit status", message)
	}
}

func TestExecRunnerKillsTheProcessOnCancel(t *testing.T) {
	runner := helperRunner(t, "hang")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	started := time.Now()

	_, err := runner.Run(ctx, helperArgs())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}

	if elapsed := time.Since(started); elapsed > 30*time.Second {
		t.Errorf("Run returned after %v, the process was not killed", elapsed)
	}
}

func TestExecRunnerReportsMissingExecutable(t *testing.T) {
	t.Parallel()

	runner := execRunner{executable: filepath.Join(t.TempDir(), "no-such-ffmpeg")}
	if _, err := runner.Run(context.Background(), []string{"-version"}); err == nil {
		t.Error("Run succeeded with a missing executable")
	}
}

func TestTailBufferKeepsTheEnd(t *testing.T) {
	t.Parallel()

	buffer := newTailBuffer(5)
	for _, chunk := range []string{"abc", "defg", "hi"} {
		if _, err := buffer.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}

	if got := buffer.String(); got != truncatedMarker+"efghi" {
		t.Errorf("String = %q", got)
	}

	short := newTailBuffer(10)
	if _, err := short.Write([]byte(" ok \n")); err != nil {
		t.Fatal(err)
	}

	if got := short.String(); got != "ok" {
		t.Errorf("String = %q", got)
	}
}

func TestTailBufferCutsAtARuneBoundary(t *testing.T) {
	t.Parallel()

	// "ü" and "€" are two and three bytes long; a 4-byte tail of "xü€" would start with the
	// second byte of "ü" without the boundary rule.
	buffer := newTailBuffer(4)
	if _, err := buffer.Write([]byte("xü€")); err != nil {
		t.Fatal(err)
	}

	got := buffer.String()
	if !utf8.ValidString(got) || got != truncatedMarker+"€" {
		t.Errorf("String = %q, want the tail to start at a whole character", got)
	}
}

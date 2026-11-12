package ffmpeg

import (
	"context"
	"testing"
)

// startHangingChild starts the fake tool in hang mode with files and kills it when the test
// ends. Tests using it cannot run in parallel (fakeTool sets the environment).
func startHangingChild(t *testing.T, files processFiles) *process {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())

	child, err := fakeTool(t, fakeModeHang).start(ctx, nil, files)
	if err != nil {
		cancel()
		t.Fatalf("start: %v", err)
	}

	t.Cleanup(func() {
		cancel()

		_ = child.wait()
	})

	return child
}

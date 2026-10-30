package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeMediaContent is what fakeFFmpeg writes instead of real media.
const fakeMediaContent = "fake media"

// fakeFFmpeg is an in-memory FFmpeg: listing commands answer from the real output fixtures,
// encoding commands write a small file to the output path (the last argument).
type fakeFFmpeg struct {
	listings      map[string][]byte
	listingErrors map[string]error
	// failOutputs maps an output file name to the stderr text of a failing encode.
	failOutputs map[string]string

	mutex   sync.Mutex
	encodes [][]string
}

func newFixtureFFmpeg(t *testing.T) *fakeFFmpeg {
	t.Helper()

	return &fakeFFmpeg{
		listings: map[string][]byte{
			"-version":               []byte(versionFixture),
			"-hide_banner -encoders": readFixture(t, encodersFixture),
			"-hide_banner -muxers":   readFixture(t, muxersFixture),
		},
		listingErrors: map[string]error{},
		failOutputs:   map[string]string{},
	}
}

func (f *fakeFFmpeg) Run(ctx context.Context, args []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	key := strings.Join(args, " ")
	if err, ok := f.listingErrors[key]; ok {
		return nil, err
	}

	if output, ok := f.listings[key]; ok {
		return output, nil
	}

	output := args[len(args)-1]

	f.mutex.Lock()
	f.encodes = append(f.encodes, args)
	f.mutex.Unlock()

	// Like FFmpeg, a failing encode may leave a partial file behind.
	if err := os.WriteFile(output, []byte(fakeMediaContent), 0o600); err != nil {
		return nil, err
	}

	for name, stderr := range f.failOutputs {
		if filepath.Base(output) == name+partialSuffix {
			return nil, errors.New("exit status 1: " + stderr)
		}
	}

	return nil, nil
}

// encodedOutputs returns the output paths of all encode calls.
func (f *fakeFFmpeg) encodedOutputs() []string {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	outputs := make([]string, 0, len(f.encodes))
	for _, args := range f.encodes {
		outputs = append(outputs, args[len(args)-1])
	}

	return outputs
}

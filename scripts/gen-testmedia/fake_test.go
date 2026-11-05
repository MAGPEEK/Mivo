package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	// onEncode, when set, runs at the start of every encode.
	onEncode func()

	mutex  sync.Mutex
	encode [][]string
	inputs []seenInput
}

// seenInput is a text input file as FFmpeg found it during an encode.
type seenInput struct {
	path    string
	content string
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

	if f.onEncode != nil {
		f.onEncode()
	}

	// Like FFmpeg, an encode fails when a file input is missing.
	if err := f.recordEncode(args); err != nil {
		return nil, err
	}

	output := args[len(args)-1]

	// Like FFmpeg, a failing encode may leave a partial file behind.
	if err := os.WriteFile(output, fakeContent(args), 0o600); err != nil {
		return nil, err
	}

	for name, stderr := range f.failOutputs {
		if filepath.Base(output) == name+partialSuffix {
			return nil, errors.New("exit status 1: " + stderr)
		}
	}

	return nil, nil
}

// recordEncode remembers the arguments and reads every input that is a file, not lavfi.
func (f *fakeFFmpeg) recordEncode(args []string) error {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	f.encode = append(f.encode, args)

	for index := 1; index+1 < len(args); index++ {
		if args[index] != "-i" || args[index-1] == "lavfi" {
			continue
		}

		content, err := os.ReadFile(args[index+1])
		if err != nil {
			return fmt.Errorf("input %s: %w", args[index+1], err)
		}

		f.inputs = append(f.inputs, seenInput{path: args[index+1], content: string(content)})
	}

	return nil
}

// encodedOutputs returns the output paths of all encode calls.
func (f *fakeFFmpeg) encodedOutputs() []string {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	outputs := make([]string, 0, len(f.encode))
	for _, args := range f.encode {
		outputs = append(outputs, args[len(args)-1])
	}

	return outputs
}

// textInputs returns the file inputs of all encode calls.
func (f *fakeFFmpeg) textInputs() []seenInput {
	f.mutex.Lock()
	defer f.mutex.Unlock()

	return append([]seenInput(nil), f.inputs...)
}

// Sizes of the fake disc sources: enough sectors to split a VOB and enough source packets
// for the aligned units the AACS stand-in marks.
const (
	fakeProgramStreamSectors = 4
	fakeSourcePackets        = markedAlignedUnits * alignedUnitSize / sourcePacketSize
	fakeVideoStreamID        = 0xE0
)

// fakeContent is what an encode writes: the disc assembly reads the structure of its source
// program streams (dvd muxer) and m2ts clips, so those get a minimal valid layout; every
// other output gets fakeMediaContent.
func fakeContent(args []string) []byte {
	isDiscSource := strings.Contains(filepath.ToSlash(args[len(args)-1]), discSourcesPath+"/")

	switch {
	case isDiscSource && slices.Contains(args, "dvd"):
		return fakeProgramStream()
	case isDiscSource && slices.Contains(args, "-mpegts_m2ts_mode"):
		return fakeSourcePacketStream()
	default:
		return []byte(fakeMediaContent)
	}
}

// fakeProgramStream is a run of MPEG-2 packs that each start a video PES packet.
func fakeProgramStream() []byte {
	stream := make([]byte, fakeProgramStreamSectors*dvdSectorSize)

	for start := 0; start < len(stream); start += dvdSectorSize {
		copy(stream[start:], []byte{0, 0, 1, packStartCode, mpeg2PackMarker})
		copy(stream[start+packHeaderSize:], []byte{0, 0, 1, fakeVideoStreamID, 0, 0, pesMarker})
	}

	return stream
}

// fakeSourcePacketStream is a run of 192-byte source packets with TS sync bytes.
func fakeSourcePacketStream() []byte {
	stream := make([]byte, fakeSourcePackets*sourcePacketSize)

	for packet := range fakeSourcePackets {
		stream[packet*sourcePacketSize+tpExtraHeaderSize] = tsSyncByte
	}

	return stream
}

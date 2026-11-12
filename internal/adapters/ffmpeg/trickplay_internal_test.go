package ffmpeg

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// fakeTrickplayProber answers every probe with description or err.
type fakeTrickplayProber struct {
	description media.Description
	err         error
}

func (p fakeTrickplayProber) Probe(context.Context, string) (media.Description, error) {
	return p.description, p.err
}

func videoDescription() media.Description {
	return media.Description{Streams: []media.Stream{
		{Index: 0, Kind: media.StreamKindAudio},
		{Index: 1, Kind: media.StreamKindVideo, Width: 1920, Height: 1080},
	}}
}

// fakeGenerator returns a generator whose ffmpeg is the test binary in the given mode.
func fakeGenerator(t *testing.T, mode string, prober TrickplayProber) *TrickplayGenerator {
	t.Helper()

	return &TrickplayGenerator{ffmpeg: fakeTool(t, mode), prober: prober, logger: slog.New(slog.DiscardHandler)}
}

func entriesOf(t *testing.T, directory string) []string {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read %s: %v", directory, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	return names
}

func TestGenerateTrickplayKillsFfmpegAndRemovesPartialOutputOnCancel(t *testing.T) {
	generator := fakeGenerator(t, fakeModeHang, fakeTrickplayProber{description: videoDescription()})
	source := trickplayAbsolute(t, "movie.mkv")
	destination := t.TempDir()

	ctx, cancel := context.WithCancel(t.Context())
	time.AfterFunc(200*time.Millisecond, cancel)
	t.Cleanup(cancel)

	started := time.Now()

	_, err := generator.GenerateTrickplay(ctx, source, destination, TrickplayOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}

	if elapsed := time.Since(started); elapsed > killDeadline {
		t.Errorf("returned after %s; ffmpeg was not killed", elapsed)
	}

	if names := entriesOf(t, destination); len(names) != 0 {
		t.Errorf("destination holds %v after cancellation, want nothing", names)
	}
}

func TestGenerateTrickplayHonoursTheTimeoutOption(t *testing.T) {
	generator := fakeGenerator(t, fakeModeHang, fakeTrickplayProber{description: videoDescription()})

	_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "movie.mkv"), t.TempDir(),
		TrickplayOptions{Timeout: 200 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestGenerateTrickplayMapsAFailedRunToUnsupportedAndCleansUp(t *testing.T) {
	generator := fakeGenerator(t, fakeModeFail, fakeTrickplayProber{description: videoDescription()})
	destination := t.TempDir()

	_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "movie.mkv"), destination, TrickplayOptions{})
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}

	if names := entriesOf(t, destination); len(names) != 0 {
		t.Errorf("destination holds %v after a failure, want nothing", names)
	}
}

func TestGenerateTrickplayReportsAnEmptyRunAsUnsupported(t *testing.T) {
	generator := fakeGenerator(t, fakeModeRespond, fakeTrickplayProber{description: videoDescription()})

	_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "movie.mkv"), t.TempDir(), TrickplayOptions{})
	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported (no sheet written)", err)
	}
}

func TestGenerateTrickplayRefusesInputWithoutPlayableVideo(t *testing.T) {
	t.Parallel()

	tests := map[string]media.Description{
		"audio only": {Streams: []media.Stream{{Index: 0, Kind: media.StreamKindAudio}}},
		"cover art only": {Streams: []media.Stream{
			{Index: 0, Kind: media.StreamKindAudio},
			{Index: 1, Kind: media.StreamKindVideo, AttachedPicture: true, Width: 600, Height: 600},
		}},
		"no streams": {},
	}

	for name, description := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			generator := &TrickplayGenerator{prober: fakeTrickplayProber{description: description}}

			_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "a.flac"), t.TempDir(), TrickplayOptions{})
			if !errors.Is(err, domain.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestGenerateTrickplayPassesProbeErrorsOn(t *testing.T) {
	t.Parallel()

	generator := &TrickplayGenerator{prober: fakeTrickplayProber{err: domain.ErrNotFound}}

	_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "gone.mkv"), t.TempDir(), TrickplayOptions{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestGenerateTrickplayRefusesARelativeDestination(t *testing.T) {
	t.Parallel()

	generator := &TrickplayGenerator{prober: fakeTrickplayProber{description: videoDescription()}}

	_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "movie.mkv"), "cache", TrickplayOptions{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

func TestGenerateTrickplayRefusesADestinationThatIsAFile(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	generator := &TrickplayGenerator{prober: fakeTrickplayProber{description: videoDescription()}}

	_, err := generator.GenerateTrickplay(t.Context(), trickplayAbsolute(t, "movie.mkv"), file, TrickplayOptions{})
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

func TestNewTrickplayGeneratorValidatesOptions(t *testing.T) {
	t.Parallel()

	prober := fakeTrickplayProber{}
	logger := slog.New(slog.DiscardHandler)

	tests := map[string]TrickplayGeneratorOptions{
		"no ffmpeg": {Prober: prober, Logger: logger},
		"no prober": {FFmpegPath: "ffmpeg", Logger: logger},
		"no logger": {FFmpegPath: "ffmpeg", Prober: prober},
	}

	for name, options := range tests {
		if _, err := NewTrickplayGenerator(options); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error = %v, want ErrInvalid", name, err)
		}
	}

	if _, err := NewTrickplayGenerator(TrickplayGeneratorOptions{FFmpegPath: "ffmpeg", Prober: prober, Logger: logger}); err != nil {
		t.Errorf("valid options: %v", err)
	}
}

func TestPublishSheetsReplacesEarlierSheetsAndRollsBackOnFailure(t *testing.T) {
	t.Parallel()

	destination := t.TempDir()
	staging := t.TempDir()

	for _, path := range []string{
		filepath.Join(destination, "sheet-001.jpg"), filepath.Join(destination, "sheet-009.jpg"),
		filepath.Join(staging, "sheet-001.jpg"),
	} {
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	if err := publishSheets([]string{filepath.Join(staging, "sheet-001.jpg")}, destination); err != nil {
		t.Fatalf("publishSheets: %v", err)
	}

	if names := entriesOf(t, destination); len(names) != 1 || names[0] != "sheet-001.jpg" {
		t.Errorf("destination = %v, want only the new sheet-001.jpg", names)
	}

	// A staged sheet that no longer exists makes the rename fail; nothing may stay published.
	err := publishSheets([]string{filepath.Join(staging, "sheet-001.jpg"), filepath.Join(staging, "missing.jpg")}, destination)
	if err == nil {
		t.Fatal("publishSheets with a missing sheet succeeded")
	}

	if names := entriesOf(t, destination); len(names) != 0 {
		t.Errorf("destination = %v after a failed publish, want nothing", names)
	}
}

func TestNewManifestRoundsTheHeightLikeTheScaleFilter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		width, height, want int
	}{
		{1920, 1080, 90},
		{320, 240, 120},
		{1280, 720, 90},
		{720, 576, 128},
		{4000, 1, 2},
	}

	for _, test := range tests {
		manifest := newManifest(media.Stream{Width: test.width, Height: test.height}, 3)
		if manifest.TileHeight != test.want || manifest.TileWidth != trickplayTileWidth || manifest.SheetCount != 3 {
			t.Errorf("%dx%d: manifest = %+v, want tile height %d", test.width, test.height, manifest, test.want)
		}
	}
}

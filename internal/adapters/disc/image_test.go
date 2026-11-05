package disc

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

// *iso.Image is the production Image.
var _ Image = (*iso.Image)(nil)

// fakeImage is an in-memory disc image: a file tree plus the extents of its files. Opening
// or locating the path broken fails like a corrupt image.
type fakeImage struct {
	fstest.MapFS
	extents map[string][]iso.Extent
	broken  string
}

func (img fakeImage) corrupt(op, name string) error {
	return &fs.PathError{Op: op, Path: name, Err: fmt.Errorf("%w: bad descriptor", iso.ErrInvalidImage)}
}

func (img fakeImage) Open(name string) (fs.File, error) {
	if img.broken != "" && name == img.broken {
		return nil, img.corrupt("open", name)
	}
	return img.MapFS.Open(name)
}

func (img fakeImage) Extents(name string) ([]iso.Extent, error) {
	if img.broken != "" && name == img.broken {
		return nil, img.corrupt("extents", name)
	}
	extents, ok := img.extents[name]
	if !ok {
		return nil, &fs.PathError{Op: "extents", Path: name, Err: fs.ErrNotExist}
	}
	return extents, nil
}

func TestResolveImage(t *testing.T) {
	t.Parallel()

	bluRay := fakeImage{
		MapFS: fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file(buildMPLS(item("00001", 60), item("00002", 40))),
			"BDMV/STREAM/00001.m2ts":   sized(1), "BDMV/STREAM/00002.m2ts": sized(1),
		},
		extents: map[string][]iso.Extent{
			"BDMV/STREAM/00001.m2ts": {{Offset: 1000, Length: 300}, {Offset: 5000, Length: 100}},
			"BDMV/STREAM/00002.m2ts": {{Offset: 9000, Length: 50}},
		},
	}
	dvd := fakeImage{
		MapFS: fstest.MapFS{
			"VIDEO_TS/VTS_01_0.IFO": file(buildIFO(pgcTime{0, 0, 30, 0, 1}.bcd())),
			"VIDEO_TS/VTS_01_1.VOB": sized(20), "VIDEO_TS/VTS_01_2.VOB": sized(10),
		},
		extents: map[string][]iso.Extent{
			"VIDEO_TS/VTS_01_1.VOB": {{Offset: 2048, Length: 20}},
			"VIDEO_TS/VTS_01_2.VOB": {{Offset: 2068, Length: 0}, {Offset: 2068, Length: 10}},
		},
	}
	cases := []struct {
		name  string
		image fakeImage
		want  media.DiscTitle
	}{
		{"Blu-ray extents split by length", bluRay, media.DiscTitle{
			{Offset: 1000, Length: 300, Duration: 45 * time.Second},
			{Offset: 5000, Length: 100, Duration: 15 * time.Second},
			{Offset: 9000, Length: 50, Duration: 40 * time.Second},
		}},
		{"DVD adjacent extents merged", dvd, media.DiscTitle{{Offset: 2048, Length: 30, Duration: 30 * time.Second}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveImage(context.Background(), tc.image)
			if err != nil {
				t.Fatalf("ResolveImage: %v", err)
			}
			assertTitle(t, got, tc.want)
		})
	}
}

func TestResolveImageFailures(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		image fakeImage
		want  error
	}{
		{"data disc", fakeImage{MapFS: fstest.MapFS{"README.TXT": sized(1)}}, media.ErrDiscUnsupported},
		{"clip without extents", fakeImage{MapFS: fstest.MapFS{"VIDEO_TS/VTS_01_1.VOB": sized(1)}}, fs.ErrNotExist},
		{"clip with empty extents", fakeImage{
			MapFS:   fstest.MapFS{"VIDEO_TS/VTS_01_1.VOB": sized(1)},
			extents: map[string][]iso.Extent{"VIDEO_TS/VTS_01_1.VOB": {{Offset: 4096}}},
		}, media.ErrDiscNoTitle},
		{"no title", fakeImage{MapFS: fstest.MapFS{"BDMV/index.bdmv": sized(1)}}, media.ErrDiscNoTitle},
		{"corrupt BDMV listing", fakeImage{
			MapFS:  fstest.MapFS{"BDMV/STREAM/00001.m2ts": sized(1)},
			broken: "BDMV/STREAM",
		}, media.ErrDiscUnsupported},
		{"corrupt extents", fakeImage{
			MapFS: fstest.MapFS{"VIDEO_TS/VTS_01_1.VOB": sized(1)},
			extents: map[string][]iso.Extent{
				"VIDEO_TS/VTS_01_1.VOB": nil,
			},
			broken: "VIDEO_TS/VTS_01_1.VOB",
		}, media.ErrDiscUnsupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := ResolveImage(context.Background(), tc.image); !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestClassifyImageError(t *testing.T) {
	t.Parallel()

	corrupt := fmt.Errorf("read: %w", iso.ErrInvalidImage)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name        string
		ctx         context.Context
		err         error
		unsupported bool
	}{
		{"corrupt image", context.Background(), corrupt, true},
		{"already coded", context.Background(), fmt.Errorf("%w: %w", media.ErrDiscNoTitle, corrupt), false},
		{"plain I/O error", context.Background(), fs.ErrPermission, false},
		{"cancelled context", cancelled, corrupt, false},
		{"cancellation error", context.Background(), fmt.Errorf("%w: %w", context.Canceled, corrupt), false},
		{"deadline error", context.Background(), fmt.Errorf("%w: %w", context.DeadlineExceeded, corrupt), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := classifyImageError(tc.ctx, tc.err)
			if errors.Is(got, media.ErrDiscUnsupported) != tc.unsupported {
				t.Fatalf("classifyImageError = %v, want unsupported %v", got, tc.unsupported)
			}
			if !errors.Is(got, tc.err) {
				t.Fatalf("classifyImageError lost the cause: %v", got)
			}
		})
	}
}

func TestResolverOnFileSystem(t *testing.T) {
	t.Parallel()

	library := t.TempDir()
	discFolder := filepath.Join(library, "Movie (2019)", "BDMV")
	writeFile(t, filepath.Join(discFolder, "index.bdmv"), []byte("INDX"))
	writeFile(t, filepath.Join(discFolder, "PLAYLIST", "00000.mpls"), buildMPLS(item("00001", 90)))
	writeFile(t, filepath.Join(discFolder, "STREAM", "00001.m2ts"), []byte{0x47})
	notAnImage := filepath.Join(library, "Show S01E02.iso")
	writeFile(t, notAnImage, make([]byte, 4096))

	resolver := NewResolver()
	got, err := resolver.Resolve(context.Background(), discFolder, media.LayoutBDMV)
	if err != nil {
		t.Fatalf("Resolve folder: %v", err)
	}
	assertTitle(t, got, media.DiscTitle{clipAt("STREAM/00001.m2ts", 90*time.Second)})

	if _, err := resolver.Resolve(context.Background(), notAnImage, media.LayoutISO); !errors.Is(err, media.ErrDiscUnsupported) {
		t.Errorf("corrupt image: err = %v, want disc_unsupported", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver.Resolve(cancelled, notAnImage, media.LayoutISO); errors.Is(err, media.ErrDiscUnsupported) {
		t.Errorf("cancelled image open tagged disc_unsupported: %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), filepath.Join(library, "missing.iso"), media.LayoutISO); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing image: err = %v, want ErrNotExist", err)
	}
	if _, err := resolver.Resolve(context.Background(), filepath.Join(library, "missing"), media.LayoutVideoTS); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing folder: err = %v, want ErrNotExist", err)
	}
	if _, err := resolver.Resolve(context.Background(), notAnImage, media.LayoutFile); !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("file layout: err = %v, want ErrInvalid", err)
	}
}

func writeFile(t *testing.T, name string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

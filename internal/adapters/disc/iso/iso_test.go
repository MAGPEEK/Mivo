//nolint:gosec // fixture writer: sizes are small test constants, the PRNG is not security relevant
package iso

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math/rand"
	"testing"
	"testing/fstest"

	"github.com/magpeek/mivo/internal/domain"
)

func udfFiles() []testFile {
	return []testFile{
		{"BDMV/index.bdmv", patterned(40, 1), modeShort},
		{"BDMV/STREAM/00000.m2ts", patterned(5000, 2), modeSplit},
		{"BDMV/STREAM/00001.m2ts", patterned(7001, 3), modeLong},
		{"BDMV/PLAYLIST/00000.mpls", patterned(100, 4), modeInline},
		{"BDMV/PLAYLIST/00001.mpls", patterned(101, 5), modeInline},
		{"BDMV/CLIPINF/00000.clpi", patterned(9000, 6), modeCont},
		{"BDMV/CLIPINF/00001.clpi", patterned(8000, 7), modeShort},
		{"VIDEO_TS/VTS_01_1.VOB", patterned(3001, 8), modeShort},
		{"Ünïcode/empty.bin", nil, modeShort},
	}
}

func isoFiles() []testFile {
	return []testFile{
		{"BDMV/INDEX.BDM", patterned(40, 1), modeShort},
		{"BDMV/STREAM/00000.M2T", patterned(5000, 2), modeMulti},
		{"VIDEO_TS/VTS_01_1.VOB", patterned(3001, 8), modeShort},
		{"VIDEO_TS/VIDEO_TS.IFO", patterned(2048, 9), modeShort},
	}
}

func paths(files []testFile) []string {
	out := make([]string, len(files))
	for i, item := range files {
		out[i] = item.path
	}
	return out
}

func openImage(t *testing.T, data []byte) *Image {
	t.Helper()
	img, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return img
}

func checkTree(t *testing.T, img *Image, files []testFile, exts map[string][]Extent) {
	t.Helper()
	if err := fstest.TestFS(img, paths(files)...); err != nil {
		t.Fatalf("TestFS: %v", err)
	}
	for _, item := range files {
		got, err := fs.ReadFile(img, item.path)
		if err != nil || !bytes.Equal(got, item.data) {
			t.Errorf("%s: content mismatch (err %v, %d bytes want %d)", item.path, err, len(got), len(item.data))
		}
		extents, err := img.Extents(item.path)
		if err != nil {
			t.Fatalf("Extents(%s): %v", item.path, err)
		}
		if !equalExtents(extents, exts[item.path]) && len(item.data) > 0 {
			t.Errorf("%s: extents %v want %v", item.path, extents, exts[item.path])
		}
		assertReadAt(t, img, item)
	}
}

func equalExtents(a, b []Extent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// assertReadAt reads the middle of the file across extent boundaries via io.ReaderAt.
func assertReadAt(t *testing.T, img *Image, item testFile) {
	t.Helper()
	if len(item.data) < 4 {
		return
	}
	handle, err := img.Open(item.path)
	if err != nil {
		t.Fatal(err)
	}
	readerAt, ok := handle.(io.ReaderAt)
	if !ok {
		t.Fatalf("%s does not implement io.ReaderAt", item.path)
	}
	from, to := len(item.data)/4, len(item.data)*3/4
	buf := make([]byte, to-from)
	if n, err := readerAt.ReadAt(buf, int64(from)); err != nil || n != len(buf) || !bytes.Equal(buf, item.data[from:to]) {
		t.Errorf("%s: ReadAt = %d, %v", item.path, n, err)
	}
	tail := make([]byte, 10)
	if n, err := readerAt.ReadAt(tail, int64(len(item.data)-5)); n != 5 || !errors.Is(err, io.EOF) {
		t.Errorf("%s: ReadAt past end = %d, %v", item.path, n, err)
	}
}

func TestUDFTree(t *testing.T) {
	data, exts := buildUDF(udfFiles())
	img := openImage(t, data)
	if _, ok := img.vol.(*udfVolume); !ok {
		t.Fatalf("volume is %T, want UDF", img.vol)
	}
	checkTree(t, img, udfFiles(), exts)
}

func TestISO9660Tree(t *testing.T) {
	data, exts := buildISO9660(isoFiles())
	img := openImage(t, data)
	if _, ok := img.vol.(*iso9660Volume); !ok {
		t.Fatalf("volume is %T, want ISO 9660", img.vol)
	}
	checkTree(t, img, isoFiles(), exts)
	if _, err := img.Open("bdmv/stream/00000.m2t"); err != nil {
		t.Errorf("case-insensitive lookup: %v", err)
	}
	if extents, _ := img.Extents("BDMV/STREAM/00000.M2T"); len(extents) != 2 {
		t.Errorf("multi-extent file has %d extents, want 2", len(extents))
	}
}

func TestUDFFallsBackToISO9660(t *testing.T) {
	data, _ := buildISO9660(isoFiles())
	data = append(data, make([]byte, (anchorSector+4)*sectorSize-len(data))...)
	anchor := data[anchorSector*sectorSize:]
	copy(anchor[16:], []byte{0, 8, 0, 0, 1, 1, 0, 0}) // garbage volume sequence at sector 257
	finishTag(anchor, tagAnchor, anchorSector)
	img := openImage(t, data)
	if _, ok := img.vol.(*iso9660Volume); !ok {
		t.Fatalf("volume is %T, want ISO 9660 fallback", img.vol)
	}
	if _, err := fs.Stat(img, "VIDEO_TS/VIDEO_TS.IFO"); err != nil {
		t.Error(err)
	}
}

func TestLookupErrors(t *testing.T) {
	data, _ := buildUDF(udfFiles())
	img := openImage(t, data)
	tests := []struct {
		name string
		want error
	}{
		{"missing.txt", fs.ErrNotExist},
		{"BDMV/index.bdmv/child", fs.ErrNotExist},
		{"../etc/passwd", fs.ErrInvalid},
		{"/BDMV", fs.ErrInvalid},
	}
	for _, tc := range tests {
		if _, err := img.Open(tc.name); !errors.Is(err, tc.want) {
			t.Errorf("Open(%q) = %v, want %v", tc.name, err, tc.want)
		}
	}
	if _, err := img.Extents("BDMV"); err == nil {
		t.Error("Extents of a directory should fail")
	}
	if _, err := img.ReadDir("BDMV/index.bdmv"); err == nil {
		t.Error("ReadDir of a file should fail")
	}
}

func TestOpenRejectsNonImages(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty": nil,
		"zeros": make([]byte, 300*sectorSize),
		"short": []byte("hello"),
	} {
		_, err := Open(context.Background(), bytes.NewReader(data), int64(len(data)))
		if !errors.Is(err, ErrInvalidImage) || !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalidImage", name, err)
		}
	}
}

func TestCancelledContext(t *testing.T) {
	data, _ := buildUDF(udfFiles())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, bytes.NewReader(data), int64(len(data))); !errors.Is(err, context.Canceled) {
		t.Errorf("Open with cancelled ctx = %v", err)
	}
	img := openImage(t, data).WithContext(ctx)
	if _, err := img.Open("BDMV/index.bdmv"); !errors.Is(err, context.Canceled) {
		t.Errorf("lookup with cancelled ctx = %v", err)
	}
}

func TestHostileDescriptors(t *testing.T) {
	data, _ := buildUDF(udfFiles())
	rootFE := (testPartStart + 2) * sectorSize // root file entry is the first allocated block
	tests := []struct {
		name   string
		mutate func(image []byte)
	}{
		{"huge directory length", func(image []byte) { image[rootFE+56+4] = 0x7f }},
		{"allocation overruns entry", func(image []byte) { image[rootFE+172+2] = 0xff }},
		{"unsupported allocation type", func(image []byte) { image[rootFE+34] = 2 }},
		{"extent beyond image", func(image []byte) { image[rootFE+176+7] = 0x7f }},
		{"bad checksum", func(image []byte) { image[rootFE+tagChecksumIndex]++ }},
	}
	for _, tc := range tests {
		image := append([]byte(nil), data...)
		tc.mutate(image)
		img, err := Open(context.Background(), bytes.NewReader(image), int64(len(image)))
		if err == nil {
			_, err = img.ReadDir(".")
		}
		if !errors.Is(err, ErrInvalidImage) {
			t.Errorf("%s: err = %v, want ErrInvalidImage", tc.name, err)
		}
	}
}

func TestTruncatedAndCorruptImagesDoNotPanic(_ *testing.T) {
	udf, _ := buildUDF(udfFiles())
	iso, _ := buildISO9660(isoFiles())
	for _, original := range [][]byte{udf, iso} {
		for cut := 0; cut < len(original); cut += sectorSize / 2 {
			openAndWalk(original[:cut])
		}
		rng := rand.New(rand.NewSource(1))
		for range 400 {
			image := append([]byte(nil), original...)
			for range 1 + rng.Intn(8) {
				image[rng.Intn(len(image))] = byte(rng.Intn(256))
			}
			openAndWalk(image)
		}
	}
}

func FuzzOpen(f *testing.F) {
	udf, _ := buildUDF(udfFiles()[:4])
	iso, _ := buildISO9660(isoFiles()[:2])
	f.Add(udf)
	f.Add(iso)
	f.Fuzz(func(_ *testing.T, data []byte) {
		openAndWalk(data)
	})
}

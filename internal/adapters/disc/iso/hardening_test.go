package iso

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestHugeExtendedAttributeLengthIsRejected(t *testing.T) {
	data, _ := buildUDF(udfFiles())
	rootFE := (testPartStart + 2) * sectorSize
	for _, length := range []uint32{0xFFFFFFF0, 0x80000000, sectorSize} {
		image := append([]byte(nil), data...)
		binary.LittleEndian.PutUint32(image[rootFE+168:], length)
		img, err := Open(context.Background(), bytes.NewReader(image), int64(len(image)))
		if err == nil {
			_, err = img.ReadDir(".")
		}
		if !errors.Is(err, ErrInvalidImage) {
			t.Errorf("EA length %#x: err = %v, want ErrInvalidImage", length, err)
		}
	}
}

// shortReader returns fewer bytes than asked without an error.
type shortReader struct{ data []byte }

func (r shortReader) ReadAt(buf []byte, off int64) (int, error) {
	return copy(buf, r.data[off:min(int64(len(r.data)), off+10)]), nil
}

func TestShortReadIsAnError(t *testing.T) {
	src := &source{reader: shortReader{make([]byte, 4096)}, size: 4096}
	if err := src.readAt(context.Background(), make([]byte, 100), 0); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("short read = %v, want ErrUnexpectedEOF", err)
	}
	full := &source{reader: bytes.NewReader(make([]byte, 100)), size: 100}
	if err := full.readAt(context.Background(), make([]byte, 100), 0); err != nil {
		t.Errorf("full read with EOF = %v", err)
	}
}

func TestISO9660RejectsInterleavedRecords(t *testing.T) {
	src := &source{size: 1 << 20}
	for _, offset := range []int{isoUnitSizeOffset, isoGapSizeOffset} {
		rec := isoRecord("A;1", 20, 10, 0)
		rec[offset] = 1
		if _, err := parseRecord(src, rec); !errors.Is(err, ErrInvalidImage) {
			t.Errorf("offset %d: err = %v", offset, err)
		}
	}
}

func TestDuplicateAndAmbiguousNames(t *testing.T) {
	if err := rejectDuplicates([]entry{{name: "a"}, {name: "b"}, {name: "a"}}); !errors.Is(err, ErrInvalidImage) {
		t.Errorf("duplicates = %v", err)
	}
	children := []entry{{name: "INDEX.BDM"}, {name: "Index.bdm"}, {name: "ONLY"}}
	if _, found := findChild(children, "index.bdm"); found {
		t.Error("ambiguous case-insensitive match must not be found")
	}
	if _, found := findChild(children, "only"); !found {
		t.Error("unique case-insensitive match should be found")
	}
	if _, found := findChild(children, "Index.bdm"); !found {
		t.Error("exact match should win over ambiguity")
	}
}

func TestValidName(t *testing.T) {
	tests := map[string]bool{
		"a.txt": true, "Ünï": true, "": false, ".": false, "..": false,
		"a/b": false, `a\b`: false, "a\x00": false, "a\x1f": false, "a\xff": false,
	}
	for name, want := range tests {
		if got := validName(name); got != want {
			t.Errorf("validName(%q) = %v, want %v", name, got, want)
		}
	}
}

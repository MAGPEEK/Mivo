//nolint:gosec // fixture sizes are small test constants
package iso

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"io/fs"
	"testing"
)

func TestSequentialReadSeekAndInfo(t *testing.T) {
	files := udfFiles()
	data, _ := buildUDF(files)
	img := openImage(t, data)
	handle, err := img.Open("BDMV/STREAM/00000.m2ts")
	if err != nil {
		t.Fatal(err)
	}
	seeker, ok := handle.(io.Seeker)
	if !ok {
		t.Fatal("file is not an io.Seeker")
	}
	want := files[1].data
	if pos, err := seeker.Seek(-100, io.SeekEnd); err != nil || pos != int64(len(want)-100) {
		t.Fatalf("Seek end = %d, %v", pos, err)
	}
	rest, err := io.ReadAll(handle)
	if err != nil || !bytes.Equal(rest, want[len(want)-100:]) {
		t.Errorf("read after seek: %v (%d bytes)", err, len(rest))
	}
	if _, err := seeker.Seek(-1, io.SeekStart); err == nil {
		t.Error("negative seek should fail")
	}
	if _, err := seeker.Seek(0, 99); err == nil {
		t.Error("bad whence should fail")
	}
	entries, err := img.ReadDir("BDMV/STREAM")
	if err != nil || len(entries) != 2 {
		t.Fatalf("ReadDir = %v, %v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil || info.Size() != int64(len(want)) || info.Mode().IsDir() || info.Sys() != nil {
		t.Errorf("Info = %v, %v", info, err)
	}
}

func TestPartitionMaps(t *testing.T) {
	data, _ := buildUDF(udfFiles())
	lvd := (anchorSector + 2) * sectorSize
	type2 := func(identifier string) []byte {
		out := make([]byte, 64)
		out[0], out[1] = 2, 64
		copy(out[5:], identifier) // byte 4 is the identifier flags field
		return out
	}
	tests := []struct {
		name    string
		mapData []byte
		wantErr bool
	}{
		{"type 2 sparing", type2("*UDF Sparable Partition"), false},
		{"virtual", type2("*UDF Virtual Partition"), true},
		{"metadata", type2("*UDF Metadata Partition"), true},
		{"unknown type", []byte{9, 6, 0, 0, 0, 0}, true},
		{"too short", []byte{1, 1}, true},
	}
	for _, tc := range tests {
		image := append([]byte(nil), data...)
		binary.LittleEndian.PutUint32(image[lvd+264:], uint32(len(tc.mapData)))
		copy(image[lvd+440:], tc.mapData)
		if tc.mapData[0] == 2 {
			binary.LittleEndian.PutUint16(image[lvd+440+38:], 7)
		}
		finishTag(image[lvd:lvd+sectorSize], tagLogicalVolume, 0)
		img, err := Open(context.Background(), bytes.NewReader(image), int64(len(image)))
		if err == nil {
			// a mapping that opens must also resolve the tree to the right sectors
			_, err = fs.ReadFile(img, "BDMV/index.bdmv")
		}
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", tc.name, err, tc.wantErr)
		}
	}
}

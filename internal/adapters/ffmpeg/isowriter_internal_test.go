//nolint:gosec // fixture writer: record and name lengths are small test constants
package ffmpeg

import (
	"encoding/binary"
	"path"
	"slices"
	"strings"
)

// A minimal ISO 9660 writer for the disc image tests: a primary volume descriptor, a
// terminator and plain directory records, enough for the iso reader. Every file is preceded by
// an empty gap sector, so the clips of a title never touch and FFmpeg has to read them
// through separate sections.
const (
	testISOSector           = 2048
	testISOFirstDescriptor  = 16
	testISOFirstFreeSector  = 18
	testISORootRecordOffset = 156
	testISORecordHeader     = 33
	testISOMinRecord        = 34
	testISOFlagDirectory    = 0x02
	testISOTypePrimary      = 1
	testISOTypeTerminator   = 255
	testISOFileVersion      = ";1"
	testISOSelfName         = "\x00"
	testISOParentName       = "\x01"
)

// TestISOFile is one file of a generated image; Path is slash-separated, upper case.
type TestISOFile struct {
	Path string
	Data []byte
}

type testISOWriter struct {
	data    []byte
	files   []TestISOFile
	offsets map[string]int64
}

// BuildTestISO returns an ISO 9660 image of files and the byte offset of each file in it.
// Exported for the corpus tests of package ffmpeg_test.
func BuildTestISO(files []TestISOFile) ([]byte, map[string]int64) {
	writer := &testISOWriter{
		data:    make([]byte, testISOFirstFreeSector*testISOSector),
		files:   files,
		offsets: map[string]int64{},
	}

	rootSector, rootSize := writer.directory("")

	primary := writer.data[testISOFirstDescriptor*testISOSector:]
	primary[0] = testISOTypePrimary
	copy(primary[1:], "CD001")
	copy(primary[testISORootRecordOffset:], testISORecord(testISOSelfName, rootSector, rootSize, testISOFlagDirectory)[:testISOMinRecord])

	terminator := writer.data[(testISOFirstDescriptor+1)*testISOSector:]
	terminator[0] = testISOTypeTerminator
	copy(terminator[1:], "CD001")

	return writer.data, writer.offsets
}

// appendSectors stores data at the next free sector and returns that sector.
func (writer *testISOWriter) appendSectors(data []byte) int {
	sector := len(writer.data) / testISOSector
	sectors := (len(data) + testISOSector - 1) / testISOSector
	writer.data = append(writer.data, make([]byte, sectors*testISOSector)...)
	copy(writer.data[sector*testISOSector:], data)

	return sector
}

// directory writes dir (and everything below it) and returns its sector and size.
func (writer *testISOWriter) directory(dir string) (int, int) {
	records := [][]byte{
		testISORecord(testISOSelfName, 0, 0, testISOFlagDirectory),
		testISORecord(testISOParentName, 0, 0, testISOFlagDirectory),
	}

	for _, name := range writer.children(dir) {
		full := path.Join(dir, name)

		index := slices.IndexFunc(writer.files, func(file TestISOFile) bool { return file.Path == full })
		if index < 0 {
			sector, size := writer.directory(full)
			records = append(records, testISORecord(name, sector, size, testISOFlagDirectory))

			continue
		}

		data := writer.files[index].Data
		writer.appendSectors(make([]byte, testISOSector)) // the gap
		sector := writer.appendSectors(data)
		writer.offsets[full] = int64(sector) * testISOSector
		records = append(records, testISORecord(name+testISOFileVersion, sector, len(data), 0))
	}

	body := slices.Concat(records...)

	return writer.appendSectors(body), len(body)
}

// children lists the names directly below dir, sorted and without duplicates.
func (writer *testISOWriter) children(dir string) []string {
	var names []string

	for _, file := range writer.files {
		rest := file.Path
		if dir != "" {
			var found bool
			if rest, found = strings.CutPrefix(file.Path, dir+"/"); !found {
				continue
			}
		}

		name, _, _ := strings.Cut(rest, "/")
		names = append(names, name)
	}

	slices.Sort(names)

	return slices.Compact(names)
}

// testISORecord is a directory record; the writer keeps every directory below one sector.
func testISORecord(name string, sector, size int, flags byte) []byte {
	record := make([]byte, (testISORecordHeader+len(name)+1)&^1)
	record[0] = byte(len(record))
	binary.LittleEndian.PutUint32(record[2:], uint32(sector))
	binary.LittleEndian.PutUint32(record[10:], uint32(size))
	record[25] = flags
	record[32] = byte(len(name))
	copy(record[testISORecordHeader:], name)

	return record
}

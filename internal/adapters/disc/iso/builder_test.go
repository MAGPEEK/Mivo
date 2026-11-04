//nolint:gosec // fixture writer: sizes are small test constants, the PRNG is not security relevant
package iso

import (
	"bytes"
	"encoding/binary"
	"sort"
	"strings"
	"unicode/utf16"
)

// Allocation modes for the in-process UDF writer.
const (
	modeShort  = "short"  // one short_ad
	modeLong   = "long"   // one long_ad
	modeInline = "inline" // data inside the file entry
	modeSplit  = "split"  // two non-adjacent extents
	modeCont   = "cont"   // two extents behind an allocation extent descriptor
	modeMulti  = "multi"  // ISO 9660 multi-extent file
)

const (
	testPartStart = 260
	testPad       = 4
)

type testFile struct {
	path string
	data []byte
	mode string
}

// testImage collects the sectors of a generated image.
type testImage struct {
	data  []byte
	next  int // next free partition-relative block (UDF) or sector (ISO)
	exts  map[string][]Extent
	base  int // sector of block 0
	files []testFile
}

func newTestImage(base, firstFree int, files []testFile) *testImage {
	img := &testImage{next: firstFree, base: base, exts: map[string][]Extent{}, files: files}
	img.grow(base + firstFree)
	return img
}

func (img *testImage) grow(sectors int) {
	if need := sectors * sectorSize; need > len(img.data) {
		img.data = append(img.data, make([]byte, need-len(img.data))...)
	}
}

// alloc reserves enough blocks for size bytes and returns the first block number.
func (img *testImage) alloc(size int) int {
	first := img.next
	img.next += max(1, (size+sectorSize-1)/sectorSize)
	img.grow(img.base + img.next)
	return first
}

func (img *testImage) put(lbn int, data []byte) {
	copy(img.data[(img.base+lbn)*sectorSize:], data)
}

func (img *testImage) offset(lbn int) int64 { return int64(img.base+lbn) * sectorSize }

func finishTag(buf []byte, id uint16, location uint32) {
	binary.LittleEndian.PutUint16(buf, id)
	binary.LittleEndian.PutUint32(buf[12:], location)
	buf[tagChecksumIndex] = 0
	var sum byte
	for i := range tagSize {
		sum += buf[i]
	}
	buf[tagChecksumIndex] = sum
}

func encShortAD(length uint32, extType uint32, lbn int) []byte {
	out := make([]byte, 8)
	binary.LittleEndian.PutUint32(out, length|extType<<extentShift)
	binary.LittleEndian.PutUint32(out[4:], uint32(lbn))
	return out
}

func encLongAD(length uint32, lbn int) []byte {
	out := make([]byte, 16)
	binary.LittleEndian.PutUint32(out, length)
	binary.LittleEndian.PutUint32(out[4:], uint32(lbn))
	return out
}

func encodeUDFName(name string) []byte {
	for _, r := range name {
		if r > 0x7f {
			out := []byte{16}
			for _, unit := range utf16.Encode([]rune(name)) {
				out = binary.BigEndian.AppendUint16(out, unit)
			}
			return out
		}
	}
	return append([]byte{8}, name...)
}

func fid(name string, chars byte, icb int) []byte {
	var encoded []byte
	if name != "" {
		encoded = encodeUDFName(name)
	}
	out := make([]byte, (fidHeaderSize+len(encoded)+3)&^3)
	out[18], out[19] = chars, byte(len(encoded))
	copy(out[20:], encLongAD(sectorSize, icb))
	copy(out[fidHeaderSize:], encoded)
	finishTag(out, tagFileID, 0)
	return out
}

// writeFE writes a file entry (extended when ext is set) with the given allocation descriptors.
//
//nolint:revive // mirrors the on-disk fields
func (img *testImage) writeFE(lbn int, fileType byte, kind byte, info int, ads []byte, ext bool) {
	buf := make([]byte, sectorSize)
	buf[27], buf[34] = fileType, kind
	binary.LittleEndian.PutUint64(buf[56:], uint64(info))
	id, lenAD, adBase := uint16(tagFileEntry), 172, 176
	if ext {
		id, lenAD, adBase = tagExtFileEntry, 212, 216
	}
	binary.LittleEndian.PutUint32(buf[lenAD:], uint32(len(ads)))
	copy(buf[adBase:], ads)
	finishTag(buf, id, uint32(lbn))
	img.put(lbn, buf)
}

// writeData stores data in fresh blocks and returns the first block.
func (img *testImage) writeData(data []byte) int {
	lbn := img.alloc(len(data))
	img.put(lbn, data)
	return lbn
}

func (img *testImage) udfFile(item testFile, entryLBN int) {
	size := len(item.data)
	extended := size%2 == 1 // alternate between file entry and extended file entry
	adBase := int64(176)
	if extended {
		adBase = 216
	}
	var kind byte
	var ads []byte
	record := func(chunk []byte, lbn int) Extent { return Extent{Offset: img.offset(lbn), Length: int64(len(chunk))} }
	switch item.mode {
	case modeInline:
		kind, ads = allocInline, item.data
		img.exts[item.path] = []Extent{{Offset: img.offset(entryLBN) + adBase, Length: int64(size)}}
	case modeSplit, modeCont:
		half := size / 2
		first := img.writeData(item.data[:half])
		img.alloc(sectorSize) // gap so the extents are not adjacent
		second := img.writeData(item.data[half:])
		ads = append(encShortAD(uint32(half), 0, first), encShortAD(uint32(size-half), 0, second)...)
		img.exts[item.path] = []Extent{record(item.data[:half], first), record(item.data[half:], second)}
		if item.mode == modeCont {
			aed := img.alloc(sectorSize)
			buf := make([]byte, sectorSize)
			binary.LittleEndian.PutUint32(buf[20:], uint32(len(ads)))
			copy(buf[24:], ads)
			finishTag(buf, tagAllocExt, uint32(aed))
			img.put(aed, buf)
			ads = encShortAD(uint32(len(ads)), extentCont, aed)
		}
	case modeLong:
		kind = allocLong
		lbn := img.writeData(item.data)
		ads = encLongAD(uint32(size), lbn)
		img.exts[item.path] = []Extent{record(item.data, lbn)}
	default:
		lbn := img.writeData(item.data)
		ads = encShortAD(uint32(size), 0, lbn)
		img.exts[item.path] = []Extent{record(item.data, lbn)}
	}
	img.writeFE(entryLBN, 5, kind, size, ads, extended)
}

// udfDir writes the directory dir (path prefix) recursively and returns its file entry block.
func (img *testImage) udfDir(dir string, parent int) int {
	entryLBN := img.alloc(sectorSize)
	if parent < 0 {
		parent = entryLBN
	}
	body := fid("", fidParent|fidDirectory, parent)
	for _, name := range childNames(img.files, dir) {
		full := strings.TrimPrefix(dir+"/"+name, "/")
		if item, ok := findFile(img.files, full); ok {
			child := img.alloc(sectorSize)
			img.udfFile(item, child)
			body = append(body, fid(name, 0, child)...)
			continue
		}
		child := img.udfDir(full, entryLBN)
		body = append(body, fid(name, fidDirectory, child)...)
	}
	lbn := img.writeData(body)
	img.writeFE(entryLBN, fileTypeDir, 0, len(body), encShortAD(uint32(len(body)), 0, lbn), false)
	return entryLBN
}

func findFile(files []testFile, path string) (testFile, bool) {
	for _, item := range files {
		if item.path == path {
			return item, true
		}
	}
	return testFile{}, false
}

// childNames lists the direct children of dir, sorted and de-duplicated.
func childNames(files []testFile, dir string) []string {
	seen := map[string]bool{}
	for _, item := range files {
		rest, ok := item.path, true
		if dir != "" {
			rest, ok = strings.CutPrefix(item.path, dir+"/")
		}
		if ok {
			seen[strings.SplitN(rest, "/", 2)[0]] = true
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// buildUDF returns a UDF image and the expected extents per file path.
func buildUDF(files []testFile) ([]byte, map[string][]Extent) {
	img := newTestImage(testPartStart, 2, files)
	root := img.udfDir("", -1)
	// block 0: file set descriptor pointing at the root file entry
	fsd := make([]byte, sectorSize)
	copy(fsd[400:], encLongAD(sectorSize, root))
	finishTag(fsd, tagFileSet, 0)
	img.put(0, fsd)

	anchor := make([]byte, sectorSize)
	binary.LittleEndian.PutUint32(anchor[16:], 3*sectorSize)
	binary.LittleEndian.PutUint32(anchor[20:], 257)
	finishTag(anchor, tagAnchor, anchorSector)
	copy(img.sectors(anchorSector), anchor)

	part := make([]byte, sectorSize)
	binary.LittleEndian.PutUint16(part[22:], 7)
	binary.LittleEndian.PutUint32(part[188:], testPartStart)
	finishTag(part, tagPartition, 257)
	copy(img.sectors(257), part)

	lvd := make([]byte, sectorSize)
	binary.LittleEndian.PutUint32(lvd[212:], sectorSize)
	copy(lvd[248:], encLongAD(sectorSize, 0))
	binary.LittleEndian.PutUint32(lvd[264:], 6)
	binary.LittleEndian.PutUint32(lvd[268:], 1)
	copy(lvd[440:], []byte{1, 6, 1, 0, 7, 0})
	finishTag(lvd, tagLogicalVolume, 258)
	copy(img.sectors(258), lvd)

	end := make([]byte, sectorSize)
	finishTag(end, tagTerminator, 259)
	copy(img.sectors(259), end)
	img.grow(img.base + img.next + testPad)
	return img.data, img.exts
}

// sectors returns an absolute sector, growing the image when needed.
func (img *testImage) sectors(sector int) []byte {
	img.grow(sector + 1)
	return img.data[sector*sectorSize : (sector+1)*sectorSize]
}

// isoRecord builds a directory record.
func isoRecord(name string, extent, size int, flags byte) []byte {
	raw := []byte(name)
	out := make([]byte, (isoNameOffset+len(raw)+1)&^1)
	out[0], out[25], out[32] = byte(len(out)), flags, byte(len(raw))
	binary.LittleEndian.PutUint32(out[2:], uint32(extent))
	binary.LittleEndian.PutUint32(out[10:], uint32(size))
	copy(out[isoNameOffset:], raw)
	return out
}

// isoDirBody packs records so that none crosses a sector boundary.
func isoDirBody(records [][]byte) []byte {
	var body []byte
	for _, rec := range records {
		if rest := sectorSize - len(body)%sectorSize; len(rec) > rest {
			body = append(body, make([]byte, rest)...)
		}
		body = append(body, rec...)
	}
	return body
}

func (img *testImage) isoDir(dir string) (extent, size int) {
	records := [][]byte{isoRecord("\x00", 0, 0, isoFlagDir), isoRecord("\x01", 0, 0, isoFlagDir)}
	for _, name := range childNames(img.files, dir) {
		full := strings.TrimPrefix(dir+"/"+name, "/")
		if item, ok := findFile(img.files, full); ok {
			records = append(records, img.isoFile(item, name)...)
			continue
		}
		childExtent, childSize := img.isoDir(full)
		records = append(records, isoRecord(name, childExtent, childSize, isoFlagDir))
	}
	body := isoDirBody(records)
	extent = img.writeData(body)
	return extent, len(body)
}

func (img *testImage) isoFile(item testFile, name string) [][]byte {
	if item.mode != modeMulti {
		lbn := img.writeData(item.data)
		img.exts[item.path] = []Extent{{Offset: img.offset(lbn), Length: int64(len(item.data))}}
		return [][]byte{isoRecord(name+";1", lbn, len(item.data), 0)}
	}
	half := len(item.data) / 2
	first := img.writeData(item.data[:half])
	img.alloc(sectorSize)
	second := img.writeData(item.data[half:])
	img.exts[item.path] = []Extent{
		{Offset: img.offset(first), Length: int64(half)},
		{Offset: img.offset(second), Length: int64(len(item.data) - half)},
	}
	return [][]byte{
		isoRecord(name+";1", first, half, isoFlagMultiExtent),
		isoRecord(name+";1", second, len(item.data)-half, 0),
	}
}

// buildISO9660 returns a plain ISO 9660 image (upper-case names are the caller's job).
func buildISO9660(files []testFile) ([]byte, map[string][]Extent) {
	img := newTestImage(0, 18, files)
	rootExtent, rootSize := img.isoDir("")
	pvd := make([]byte, sectorSize)
	pvd[0] = isoTypePrimary
	copy(pvd[1:], "CD001")
	copy(pvd[isoRootRecordOffset:], isoRecord("\x00", rootExtent, rootSize, isoFlagDir)[:isoMinRecord])
	copy(img.sectors(isoFirstDescriptor), pvd)
	end := make([]byte, sectorSize)
	end[0] = isoTypeTerminator
	copy(end[1:], "CD001")
	copy(img.sectors(isoFirstDescriptor+1), end)
	return img.data, img.exts
}

func patterned(size int, seed byte) []byte {
	return bytes.Repeat([]byte{seed, seed + 1, seed + 2}, size/3+1)[:size]
}

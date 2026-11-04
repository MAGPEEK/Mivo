package iso

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"unicode/utf16"
)

// Tag identifiers (ECMA-167 3/7.2.1) and field offsets used by the reader.
const (
	tagAnchor        = 2
	tagPartition     = 5
	tagLogicalVolume = 6
	tagTerminator    = 8
	tagFileSet       = 256
	tagFileID        = 257
	tagAllocExt      = 258
	tagFileEntry     = 261
	tagExtFileEntry  = 266

	tagSize          = 16
	tagChecksumIndex = 4
	anchorSector     = 256
	maxVDSSectors    = 64
	maxPartitionMaps = 16
	maxContinuations = 64

	lengthMask   = 0x3FFFFFFF
	extentShift  = 30
	extentRecord = 0
	extentCont   = 3

	allocShort  = 0
	allocLong   = 1
	allocInline = 3
	fileTypeDir = 4

	fidHeaderSize = 38
	fidDeleted    = 0x04
	fidParent     = 0x08
	fidDirectory  = 0x02
	fidAlign      = 4

	allocTypeMask    = 7
	infoLengthOffset = 56
	fileTypeOffset   = 27
	flagsOffset      = 34
	compressionBytes = 8
	compressionUnits = 16
	utf16UnitSize    = 2
)

// udfICB addresses a block: logical block number inside the partition with the given map index.
type udfICB struct {
	lbn  uint32
	part uint16
}

func (icb udfICB) nodeID() nodeID {
	return nodeID{first: int64(icb.part), second: int64(icb.lbn)}
}

// udfVolume reads UDF 1.02-2.60 with 2048-byte blocks and plain (type 1) partitions.
type udfVolume struct {
	src       *source
	partStart map[uint16]uint32 // partition map index -> first sector
	rootICB   udfICB
}

func tagOK(buf []byte, id uint16) bool {
	if len(buf) < tagSize || binary.LittleEndian.Uint16(buf) != id {
		return false
	}
	var sum byte
	for i := range tagSize {
		if i != tagChecksumIndex {
			sum += buf[i]
		}
	}
	return sum == buf[tagChecksumIndex]
}

// vdsResult is what the volume descriptor sequence contributes.
type vdsResult struct {
	partitions map[uint16]uint32 // partition number -> start sector
	maps       []uint16          // partition map index -> partition number
	fileSet    udfICB
	haveLVD    bool
}

func openUDF(ctx context.Context, src *source) (volume, error) {
	anchor, err := findAnchor(ctx, src)
	if err != nil {
		return nil, err
	}
	var failures []string
	for _, offset := range []int{16, 24} { // main, then reserve sequence
		length := int64(binary.LittleEndian.Uint32(anchor[offset:]))
		start := int64(binary.LittleEndian.Uint32(anchor[offset+4:]))
		vol, err := openFromSequence(ctx, src, start, length)
		if err == nil {
			return vol, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		failures = append(failures, err.Error())
	}
	return nil, corrupt("udf: %s", strings.Join(failures, "; "))
}

func findAnchor(ctx context.Context, src *source) ([]byte, error) {
	last := src.size/sectorSize - 1
	for _, sector := range []int64{anchorSector, last, last - anchorSector} {
		if sector < 0 || (sector+1)*sectorSize > src.size {
			continue
		}
		buf, err := src.sector(ctx, sector)
		if err != nil {
			return nil, err
		}
		if tagOK(buf, tagAnchor) {
			return buf, nil
		}
	}
	return nil, errNotUDF
}

func openFromSequence(ctx context.Context, src *source, start, length int64) (volume, error) {
	vds, err := readSequence(ctx, src, start, length)
	if err != nil {
		return nil, err
	}
	vol := &udfVolume{src: src, partStart: map[uint16]uint32{}}
	for index, number := range vds.maps {
		sector, ok := vds.partitions[number]
		if !ok {
			return nil, corrupt("udf partition %d missing", number)
		}
		vol.partStart[uint16(index)] = sector
	}
	offset, err := vol.blockOffset(vds.fileSet)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, sectorSize)
	if err := src.readAt(ctx, buf, offset); err != nil {
		return nil, err
	}
	if !tagOK(buf, tagFileSet) {
		return nil, corrupt("udf file set descriptor missing")
	}
	vol.rootICB = parseLongAD(buf[400:]).icb
	return vol, nil
}

func readSequence(ctx context.Context, src *source, start, length int64) (vdsResult, error) {
	result := vdsResult{partitions: map[uint16]uint32{}}
	count := min(length/sectorSize, maxVDSSectors)
	for index := range count {
		buf, err := src.sector(ctx, start+index)
		if err != nil {
			return result, err
		}
		switch {
		case tagOK(buf, tagPartition):
			result.partitions[binary.LittleEndian.Uint16(buf[22:])] = binary.LittleEndian.Uint32(buf[188:])
		case tagOK(buf, tagLogicalVolume) && !result.haveLVD:
			if err := parseLogicalVolume(buf, &result); err != nil {
				return result, err
			}
		case tagOK(buf, tagTerminator):
			return result, finishSequence(result)
		}
	}
	return result, finishSequence(result)
}

func finishSequence(result vdsResult) error {
	if !result.haveLVD || len(result.partitions) == 0 {
		return corrupt("udf volume descriptors incomplete")
	}
	return nil
}

func parseLogicalVolume(buf []byte, result *vdsResult) error {
	if block := binary.LittleEndian.Uint32(buf[212:]); block != sectorSize {
		return corrupt("udf logical block size %d unsupported", block)
	}
	result.fileSet = parseLongAD(buf[248:]).icb
	count := binary.LittleEndian.Uint32(buf[268:])
	if count > maxPartitionMaps {
		return corrupt("udf has %d partition maps", count)
	}
	pos := 440
	for range count {
		if pos+2 > len(buf) || buf[pos+1] < 2 || pos+int(buf[pos+1]) > len(buf) {
			return corrupt("udf partition map overruns descriptor")
		}
		number, err := partitionNumber(buf[pos : pos+int(buf[pos+1])])
		if err != nil {
			return err
		}
		result.maps = append(result.maps, number)
		pos += int(buf[pos+1])
	}
	result.haveLVD = len(result.maps) > 0
	return nil
}

func partitionNumber(mapEntry []byte) (uint16, error) {
	switch {
	case mapEntry[0] == 1 && len(mapEntry) >= 6:
		return binary.LittleEndian.Uint16(mapEntry[4:]), nil
	case mapEntry[0] == 2 && len(mapEntry) >= 40:
		// Only sparable maps address blocks like plain partitions; virtual (VAT) and metadata
		// (UDF 2.50) partitions would resolve to wrong sectors.
		if !strings.Contains(string(mapEntry[4:36]), "*UDF Sparable Partition") {
			return 0, corrupt("udf partition map %q unsupported (virtual or metadata partition)",
				strings.TrimRight(string(mapEntry[5:36]), "\x00"))
		}
		return binary.LittleEndian.Uint16(mapEntry[38:]), nil
	}
	return 0, corrupt("udf partition map type %d unsupported", mapEntry[0])
}

type longAD struct {
	length uint32
	icb    udfICB
}

func parseLongAD(buf []byte) longAD {
	return longAD{
		length: binary.LittleEndian.Uint32(buf) & lengthMask,
		icb:    udfICB{lbn: binary.LittleEndian.Uint32(buf[4:]), part: binary.LittleEndian.Uint16(buf[8:])},
	}
}

func (v *udfVolume) blockOffset(icb udfICB) (int64, error) {
	start, ok := v.partStart[icb.part]
	if !ok {
		return 0, corrupt("udf partition reference %d", icb.part)
	}
	return (int64(start) + int64(icb.lbn)) * sectorSize, nil
}

func (v *udfVolume) root() entry {
	return entry{name: ".", dir: true, icb: v.rootICB, id: v.rootICB.nodeID()}
}

// adArea locates the allocation descriptors inside a (extended) file entry sector. The lengths
// are compared as uint64 so they cannot wrap on 32-bit platforms.
func adArea(buf []byte) (start, length int, err error) {
	extended := tagOK(buf, tagExtFileEntry)
	if !extended && !tagOK(buf, tagFileEntry) {
		return 0, 0, corrupt("udf file entry invalid")
	}
	lenEAPos, lenADPos, adBase := 168, 172, 176
	if extended {
		lenEAPos, lenADPos, adBase = 208, 212, 216
	}
	room := uint64(len(buf) - adBase) //nolint:gosec // len(buf) >= adBase
	lenEA := uint64(binary.LittleEndian.Uint32(buf[lenEAPos:]))
	if lenEA > room {
		return 0, 0, corrupt("udf extended attributes overrun file entry")
	}
	lenAD := uint64(binary.LittleEndian.Uint32(buf[lenADPos:]))
	if lenAD > room-lenEA {
		return 0, 0, corrupt("udf allocation descriptors overrun file entry")
	}
	return adBase + int(lenEA), int(lenAD), nil //nolint:gosec // both are bounded by room above
}

// resolve reads the file entry and fills size, kind and extents.
func (v *udfVolume) resolve(ctx context.Context, item *entry) error {
	if item.resolved {
		return nil
	}
	offset, err := v.blockOffset(item.icb)
	if err != nil {
		return err
	}
	buf := make([]byte, sectorSize)
	if err := v.src.readAt(ctx, buf, offset); err != nil {
		return err
	}
	adStart, adLen, err := adArea(buf)
	if err != nil {
		return fmt.Errorf("file entry at %d: %w", offset, err)
	}
	rawInfo := binary.LittleEndian.Uint64(buf[infoLengthOffset:])
	if rawInfo > math.MaxInt64 {
		return corrupt("udf information length %d", rawInfo)
	}
	info := int64(rawInfo)
	item.dir = buf[fileTypeOffset] == fileTypeDir
	extents, err := v.extents(ctx, adList{
		icb: item.icb, flags: buf[flagsOffset], data: buf[adStart : adStart+adLen], offset: offset + int64(adStart),
	})
	if err != nil {
		return err
	}
	item.size, item.extents = fitExtents(extents, info)
	if item.size != info {
		return corrupt("udf file shorter than its length (%d < %d)", item.size, info)
	}
	item.resolved = true
	return nil
}

// fitExtents trims the extents to the information length and returns the covered size.
func fitExtents(extents []Extent, info int64) (int64, []Extent) {
	var covered int64
	for index, extent := range extents {
		if covered+extent.Length >= info {
			extents[index].Length = info - covered
			return info, extents[:index+1]
		}
		covered += extent.Length
	}
	return covered, extents
}

// adList is the allocation descriptor area of a file entry: its address policy, bytes and image offset.
type adList struct {
	icb    udfICB
	flags  byte
	data   []byte
	offset int64
}

func (v *udfVolume) extents(ctx context.Context, list adList) ([]Extent, error) {
	ads, icb := list.data, list.icb
	kind := list.flags & allocTypeMask
	if kind == allocInline {
		return []Extent{{Offset: list.offset, Length: int64(len(ads))}}, nil
	}
	if kind != allocShort && kind != allocLong {
		return nil, corrupt("udf allocation type %d unsupported", kind)
	}
	var out []Extent
	for hop := 0; ; hop++ {
		var cont *longAD
		var err error
		out, cont, err = v.parseADs(out, ads, kind, icb.part)
		if err != nil {
			return nil, err
		}
		if cont == nil {
			return out, nil
		}
		if hop >= maxContinuations {
			return nil, corrupt("udf allocation continuation chain too long")
		}
		if ads, err = v.readContinuation(ctx, cont.icb); err != nil {
			return nil, err
		}
	}
}

func (v *udfVolume) readContinuation(ctx context.Context, icb udfICB) ([]byte, error) {
	offset, err := v.blockOffset(icb)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, sectorSize)
	if err := v.src.readAt(ctx, buf, offset); err != nil {
		return nil, err
	}
	if !tagOK(buf, tagAllocExt) {
		return nil, corrupt("udf allocation extent descriptor invalid")
	}
	length := int(binary.LittleEndian.Uint32(buf[20:]))
	if length < 0 || length > len(buf)-24 {
		return nil, corrupt("udf allocation extent length %d", length)
	}
	return buf[24 : 24+length], nil
}

// parseADs appends the recorded extents of one descriptor list; a continuation pointer is returned.
func (v *udfVolume) parseADs(out []Extent, ads []byte, kind byte, part uint16) ([]Extent, *longAD, error) {
	step := 8
	if kind == allocLong {
		step = 16
	}
	for pos := 0; pos+step <= len(ads); pos += step {
		raw := binary.LittleEndian.Uint32(ads[pos:])
		length, extType := raw&lengthMask, raw>>extentShift
		if length == 0 && extType == extentRecord {
			break
		}
		target := udfICB{lbn: binary.LittleEndian.Uint32(ads[pos+4:]), part: part}
		if kind == allocLong {
			target = parseLongAD(ads[pos:]).icb
		}
		if extType == extentCont {
			return out, &longAD{length: length, icb: target}, nil
		}
		if extType != extentRecord {
			return nil, nil, corrupt("udf extent type %d (allocated-but-unrecorded or unallocated/sparse) unsupported", extType)
		}
		var err error
		if out, err = v.appendExtent(out, target, length); err != nil {
			return nil, nil, err
		}
	}
	return out, nil, nil
}

func (v *udfVolume) appendExtent(out []Extent, target udfICB, length uint32) ([]Extent, error) {
	if len(out) >= maxExtents {
		return nil, corrupt("udf file has too many extents")
	}
	offset, err := v.blockOffset(target)
	if err != nil {
		return nil, err
	}
	extent := Extent{Offset: offset, Length: int64(length)}
	if err := v.src.checkExtent(extent); err != nil {
		return nil, err
	}
	return append(out, extent), nil
}

func (v *udfVolume) list(ctx context.Context, dir entry) ([]entry, error) {
	if err := v.resolve(ctx, &dir); err != nil {
		return nil, err
	}
	if !dir.dir {
		return nil, corrupt("udf %q is not a directory", dir.name)
	}
	body, err := v.src.readAll(ctx, dir)
	if err != nil {
		return nil, err
	}
	var out []entry
	for pos := 0; pos+fidHeaderSize <= len(body); {
		if binary.LittleEndian.Uint16(body[pos:]) == 0 { // zero padding after the last descriptor
			break
		}
		item, size, err := parseFileID(body[pos:])
		if err != nil {
			return nil, err
		}
		pos += size
		if item.name == "" {
			continue
		}
		if len(out) >= maxDirEntries {
			return nil, corrupt("udf directory has too many entries")
		}
		out = append(out, item)
	}
	return out, rejectDuplicates(out)
}

// parseFileID reads one File Identifier Descriptor; an empty name means "skip this entry".
func parseFileID(buf []byte) (entry, int, error) {
	if !tagOK(buf, tagFileID) {
		return entry{}, 0, corrupt("udf file identifier descriptor invalid")
	}
	chars := buf[18]
	nameLen := int(buf[19])
	implLen := int(binary.LittleEndian.Uint16(buf[36:]))
	size := (fidHeaderSize + implLen + nameLen + fidAlign - 1) &^ (fidAlign - 1)
	if size > len(buf) {
		return entry{}, 0, corrupt("udf file identifier overruns directory")
	}
	if chars&(fidDeleted|fidParent) != 0 || nameLen == 0 {
		return entry{}, size, nil
	}
	nameStart := fidHeaderSize + implLen
	name, err := decodeName(buf[nameStart : nameStart+nameLen])
	if err != nil {
		return entry{}, 0, err
	}
	if !validName(name) {
		return entry{}, size, nil
	}
	icb := parseLongAD(buf[20:]).icb
	return entry{name: name, dir: chars&fidDirectory != 0, icb: icb, id: icb.nodeID()}, size, nil
}

// decodeName handles OSTA compressed Unicode: 8 = one byte per rune, 16 = UTF-16BE.
func decodeName(raw []byte) (string, error) {
	switch raw[0] {
	case compressionBytes:
		runes := make([]rune, 0, len(raw)-1)
		for _, b := range raw[1:] {
			runes = append(runes, rune(b))
		}
		return string(runes), nil
	case compressionUnits:
		units := make([]uint16, 0, len(raw)/utf16UnitSize)
		for pos := 1; pos+utf16UnitSize <= len(raw); pos += utf16UnitSize {
			units = append(units, binary.BigEndian.Uint16(raw[pos:]))
		}
		return string(utf16.Decode(units)), nil
	}
	return "", corrupt("udf name compression id %d", raw[0])
}

package main

import (
	"encoding/binary"
	"fmt"
	"path"
	"slices"
	"strings"
)

// ISO 9660 image writer for the disc corpus: one level of folders below the root, upper-case
// 8.3 names with ";1" versions, path tables and both-endian fields, so operating systems
// mount the image and internal/adapters/disc/iso reads it through its ISO 9660 fallback. It
// writes files in name order without gaps, so a DVD title's VOBs lie back to back as on a
// real disc. Test-corpus code, not a mastering tool: no UDF, no Joliet, no Rock Ridge.
const (
	isoSystemAreaSectors = 16
	isoStandardID        = "CD001"
	isoDescriptorVersion = 1
	isoPrimaryType       = 1
	isoTerminatorType    = 255
	isoFileStructure     = 1
	isoVolumeIDLength    = 32
	isoDirectoryFlag     = 0x02
	isoRecordFixedSize   = 33
	isoVolumeSequence    = 1
	isoVersionSuffix     = ";1"
	// isoSelfName and isoParentName are the one-byte names of "." and "..".
	isoSelfName   = "\x00"
	isoParentName = "\x01"

	// Primary volume descriptor offsets (ECMA-119 8.4).
	pvdSystemIDOffset        = 8
	pvdVolumeIDOffset        = 40
	pvdVolumeSpaceOffset     = 80
	pvdSetSizeOffset         = 120
	pvdSequenceOffset        = 124
	pvdBlockSizeOffset       = 128
	pvdPathTableSizeOffset   = 132
	pvdLPathTableOffset      = 140
	pvdMPathTableOffset      = 148
	pvdRootRecordOffset      = 156
	pvdFileStructureOffset   = 881
	pvdIdentifierPadding     = ' '
	pvdIdentifierFieldsStart = 190
	pvdIdentifierFieldsEnd   = 813
	// The four volume dates follow the identifiers: 16 digits and a time-zone byte each.
	pvdDateFields    = 4
	pvdDateDigits    = 16
	pvdDateFieldSize = pvdDateDigits + 1

	// Directory record offsets (ECMA-119 9.1).
	recordExtentOffset   = 2
	recordSizeOffset     = 10
	recordDateOffset     = 18
	recordFlagsOffset    = 25
	recordSequenceOffset = 28
	recordNameLenOffset  = 32

	// isoRecordingYear is the fixed recording date (years since 1900, month, day), so
	// repeated runs write identical images.
	isoRecordingYear  = 120
	isoRecordingMonth = 1
	isoRecordingDay   = 1

	// pathTableFixedSize is the name length, extended attribute length, extent and parent.
	pathTableFixedSize = 8
	// Path table entry offsets (ECMA-119 9.4); byte 1 is the extended attribute length (0).
	pathTableNameLenOffset = 0
	pathTableExtentOffset  = 2
	pathTableParentOffset  = 6
	isoRootParent          = 1
	evenMask               = 1

	// isoFirstDataSector follows the descriptors (16, 17) and the two path tables (18, 19).
	isoLPathTableSector = 18
	isoMPathTableSector = 19
	isoFirstDataSector  = 20
)

// assembledFile is one file written in Go (a disc tree file, a photo); name is
// slash-separated and relative to the disc root or the assembly folder.
type assembledFile struct {
	name string
	data []byte
}

// isoDirectory is one folder of the image and its files.
type isoDirectory struct {
	name   string
	files  []assembledFile
	sector int
	size   int
}

// isoImage writes an ISO 9660 image of files, which may lie in the root or in one level of
// folders. volumeID must be a valid d-character identifier.
func isoImage(volumeID string, files []assembledFile) ([]byte, error) {
	root, folders, err := isoTree(files)
	if err != nil {
		return nil, err
	}

	sector := isoFirstDataSector
	for _, dir := range append([]*isoDirectory{root}, folders...) {
		dir.sector, dir.size = sector, dvdSectorSize
		sector++
	}

	dataSectors := map[string]int{}
	for _, dir := range append([]*isoDirectory{root}, folders...) {
		for _, file := range dir.files {
			dataSectors[file.name] = sector
			sector += sectorsFor(len(file.data))
		}
	}

	image := make([]byte, sector*dvdSectorSize)
	pathTable, pathTableSize := isoPathTables(root, folders)
	copy(image[isoLPathTableSector*dvdSectorSize:], pathTable.little)
	copy(image[isoMPathTableSector*dvdSectorSize:], pathTable.big)
	copy(image[isoSystemAreaSectors*dvdSectorSize:], primaryDescriptor(volumeID, sector, pathTableSize, root))
	copy(image[(isoSystemAreaSectors+1)*dvdSectorSize:], descriptorHeader(isoTerminatorType))

	for _, dir := range append([]*isoDirectory{root}, folders...) {
		// Each directory was given one sector; a disc folder lists a dozen files at most.
		body := directoryBody(dir, root, folders, dataSectors)
		if len(body) > dvdSectorSize {
			return nil, fmt.Errorf("iso image: directory %q needs more than one sector", dir.name)
		}

		copy(image[dir.sector*dvdSectorSize:], body)

		for _, file := range dir.files {
			copy(image[dataSectors[file.name]*dvdSectorSize:], file.data)
		}
	}

	return image, nil
}

// isoTree groups files into the root and its folders, both in name order.
func isoTree(files []assembledFile) (*isoDirectory, []*isoDirectory, error) {
	root := &isoDirectory{}
	byName := map[string]*isoDirectory{}

	var folders []*isoDirectory

	for _, file := range files {
		folder, base := path.Split(file.name)
		folder = strings.TrimSuffix(folder, "/")

		if strings.Contains(folder, "/") || !isISOName(base) || (folder != "" && !isISOName(folder)) {
			return nil, nil, fmt.Errorf("iso image: %q is not an upper-case 8.3 name one level deep", file.name)
		}

		dir := root
		if folder != "" {
			if byName[folder] == nil {
				byName[folder] = &isoDirectory{name: folder}
				folders = append(folders, byName[folder])
			}

			dir = byName[folder]
		}

		dir.files = append(dir.files, file)
	}

	slices.SortFunc(folders, func(a, b *isoDirectory) int { return strings.Compare(a.name, b.name) })

	for _, dir := range append([]*isoDirectory{root}, folders...) {
		slices.SortFunc(dir.files, func(a, b assembledFile) int { return strings.Compare(a.name, b.name) })
	}

	return root, folders, nil
}

// isISOName accepts ISO 9660 level 1 d-characters: up to eight characters, an optional
// extension of up to three.
func isISOName(name string) bool {
	const maxBase, maxExtension = 8, 3

	base, extension, _ := strings.Cut(name, ".")

	return base != "" && len(base) <= maxBase && len(extension) <= maxExtension &&
		strings.Trim(base+extension, "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_") == ""
}

func sectorsFor(size int) int {
	return (size + dvdSectorSize - 1) / dvdSectorSize
}

func descriptorHeader(descriptorType byte) []byte {
	header := make([]byte, dvdSectorSize)
	header[0] = descriptorType
	copy(header[1:], isoStandardID)
	header[len(isoStandardID)+1] = isoDescriptorVersion

	return header
}

func primaryDescriptor(volumeID string, totalSectors, pathTableSize int, root *isoDirectory) []byte {
	pvd := descriptorHeader(isoPrimaryType)
	copy(pvd[pvdSystemIDOffset:pvdVolumeIDOffset], strings.Repeat(string(pvdIdentifierPadding), pvdVolumeIDOffset-pvdSystemIDOffset))
	copy(pvd[pvdVolumeIDOffset:], fmt.Sprintf("%-*s", isoVolumeIDLength, volumeID))

	for index := pvdIdentifierFieldsStart; index < pvdIdentifierFieldsEnd; index++ {
		pvd[index] = pvdIdentifierPadding
	}

	// Creation, modification, expiration and effective date: "not specified" is sixteen
	// ASCII zeros and a zero time-zone offset (ECMA-119 8.4.26.1).
	for field := range pvdDateFields {
		start := pvdIdentifierFieldsEnd + field*pvdDateFieldSize
		copy(pvd[start:], strings.Repeat("0", pvdDateDigits))
		pvd[start+pvdDateDigits] = 0
	}

	putBothEndian32(pvd[pvdVolumeSpaceOffset:], totalSectors)
	putBothEndian16(pvd[pvdSetSizeOffset:], isoVolumeSequence)
	putBothEndian16(pvd[pvdSequenceOffset:], isoVolumeSequence)
	putBothEndian16(pvd[pvdBlockSizeOffset:], dvdSectorSize)
	putBothEndian32(pvd[pvdPathTableSizeOffset:], pathTableSize)
	binary.LittleEndian.PutUint32(pvd[pvdLPathTableOffset:], isoLPathTableSector)
	binary.BigEndian.PutUint32(pvd[pvdMPathTableOffset:], isoMPathTableSector)
	copy(pvd[pvdRootRecordOffset:], directoryRecord(isoSelfName, root.sector, root.size, isoDirectoryFlag))
	pvd[pvdFileStructureOffset] = isoFileStructure

	return pvd
}

// directoryBody lists ".", ".." and the entries of dir; it always fits in one sector here.
func directoryBody(dir, root *isoDirectory, folders []*isoDirectory, dataSectors map[string]int) []byte {
	// Folders are one level deep, so the root is every directory's parent (and its own).
	body := directoryRecord(isoSelfName, dir.sector, dir.size, isoDirectoryFlag)
	body = append(body, directoryRecord(isoParentName, root.sector, root.size, isoDirectoryFlag)...)

	if dir == root {
		for _, folder := range folders {
			body = append(body, directoryRecord(folder.name, folder.sector, folder.size, isoDirectoryFlag)...)
		}
	}

	for _, file := range dir.files {
		body = append(body, directoryRecord(path.Base(file.name)+isoVersionSuffix, dataSectors[file.name], len(file.data), 0)...)
	}

	return body
}

// directoryRecord is one ECMA-119 directory record, padded to an even length.
func directoryRecord(name string, sector, size int, flags byte) []byte {
	length := isoRecordFixedSize + len(name)
	length += length & evenMask

	record := make([]byte, length)
	record[0] = byte(length) //nolint:gosec // names are at most 12 characters
	putBothEndian32(record[recordExtentOffset:], sector)
	putBothEndian32(record[recordSizeOffset:], size)
	record[recordDateOffset], record[recordDateOffset+1], record[recordDateOffset+2] =
		isoRecordingYear, isoRecordingMonth, isoRecordingDay
	record[recordFlagsOffset] = flags
	putBothEndian16(record[recordSequenceOffset:], isoVolumeSequence)
	record[recordNameLenOffset] = byte(len(name)) //nolint:gosec // names are at most 12 characters
	copy(record[isoRecordFixedSize:], name)

	return record
}

// isoPathTableData holds the little-endian (L) and big-endian (M) path tables.
type isoPathTableData struct {
	little, big []byte
}

// isoPathTables lists the root and its folders; every folder's parent is the root (entry 1).
func isoPathTables(root *isoDirectory, folders []*isoDirectory) (isoPathTableData, int) {
	var tables isoPathTableData

	entries := append([]*isoDirectory{root}, folders...)
	for _, dir := range entries {
		name := dir.name
		if dir == root {
			name = isoSelfName
		}

		tables.little = append(tables.little, pathTableEntry(name, dir.sector, binary.LittleEndian)...)
		tables.big = append(tables.big, pathTableEntry(name, dir.sector, binary.BigEndian)...)
	}

	return tables, len(tables.little)
}

func pathTableEntry(name string, sector int, order binary.ByteOrder) []byte {
	length := pathTableFixedSize + len(name)
	length += length & evenMask

	entry := make([]byte, length)
	entry[pathTableNameLenOffset] = byte(len(name))                //nolint:gosec // names are at most 12 characters
	order.PutUint32(entry[pathTableExtentOffset:], uint32(sector)) //nolint:gosec // small images
	order.PutUint16(entry[pathTableParentOffset:], isoRootParent)  // every folder hangs off the root
	copy(entry[pathTableFixedSize:], name)

	return entry
}

func putBothEndian32(field []byte, value int) {
	binary.LittleEndian.PutUint32(field, uint32(value))  //nolint:gosec // small images
	binary.BigEndian.PutUint32(field[4:], uint32(value)) //nolint:gosec // small images
}

func putBothEndian16(field []byte, value int) {
	binary.LittleEndian.PutUint16(field, uint16(value))  //nolint:gosec // small values
	binary.BigEndian.PutUint16(field[2:], uint16(value)) //nolint:gosec // small values
}

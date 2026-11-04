package iso

import (
	"context"
	"encoding/binary"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	isoFirstDescriptor  = 16
	isoMaxDescriptors   = 32
	isoRootRecordOffset = 156
	isoMinRecord        = 34
	isoNameLenOffset    = 32
	isoNameOffset       = 33
	isoFlagsOffset      = 25
	isoUnitSizeOffset   = 26
	isoGapSizeOffset    = 27
	isoFlagDir          = 0x02
	isoFlagMultiExtent  = 0x80
	isoTypePrimary      = 1
	isoTypeTerminator   = 255
)

// iso9660Volume reads the plain ISO 9660 tree (no Rock Ridge or Joliet; UDF covers long names).
type iso9660Volume struct {
	src      *source
	rootItem entry
}

func openISO9660(ctx context.Context, src *source) (volume, error) {
	for index := range int64(isoMaxDescriptors) {
		sector := isoFirstDescriptor + index
		if (sector+1)*sectorSize > src.size {
			break
		}
		buf, err := src.sector(ctx, sector)
		if err != nil {
			return nil, err
		}
		if string(buf[1:6]) != "CD001" || buf[0] == isoTypeTerminator {
			break
		}
		if buf[0] == isoTypePrimary {
			return newISO9660(src, buf[isoRootRecordOffset:isoRootRecordOffset+isoMinRecord])
		}
	}
	return nil, errNotISO
}

func newISO9660(src *source, record []byte) (volume, error) {
	item, err := parseRecord(src, record)
	if err != nil {
		return nil, err
	}
	if !item.dir {
		return nil, corrupt("iso9660 root is not a directory")
	}
	item.name = "."
	return &iso9660Volume{src: src, rootItem: item}, nil
}

func (v *iso9660Volume) root() entry { return v.rootItem }

func (v *iso9660Volume) resolve(context.Context, *entry) error { return nil }

// parseRecord converts one directory record (without its name) into a resolved entry.
func parseRecord(src *source, rec []byte) (entry, error) {
	if rec[isoUnitSizeOffset] != 0 || rec[isoGapSizeOffset] != 0 {
		return entry{}, corrupt("iso9660 interleaved files unsupported")
	}
	attrLen := int64(rec[1])
	extent := Extent{
		Offset: (int64(binary.LittleEndian.Uint32(rec[2:])) + attrLen) * sectorSize,
		Length: int64(binary.LittleEndian.Uint32(rec[10:])),
	}
	if err := src.checkExtent(extent); err != nil {
		return entry{}, err
	}
	return entry{
		dir: rec[isoFlagsOffset]&isoFlagDir != 0, size: extent.Length,
		extents: []Extent{extent}, resolved: true,
		id: nodeID{first: extent.Offset, second: extent.Length},
	}, nil
}

func (v *iso9660Volume) list(ctx context.Context, dir entry) ([]entry, error) {
	body, err := v.src.readAll(ctx, dir)
	if err != nil {
		return nil, err
	}
	listing := isoListing{versions: map[string]int{}, index: map[string]int{}}
	for pos := 0; pos < len(body); {
		length := int(body[pos])
		if length == 0 { // records never span sectors; zero pads to the next one
			pos = (pos/sectorSize + 1) * sectorSize
			continue
		}
		if length < isoMinRecord || pos+length > len(body) || isoNameOffset+int(body[pos+isoNameLenOffset]) > length {
			return nil, corrupt("iso9660 directory record at %d", pos)
		}
		rec := body[pos : pos+length]
		pos += length
		name, version, skip := recordName(rec)
		if skip {
			continue
		}
		item, err := parseRecord(v.src, rec)
		if err != nil {
			return nil, err
		}
		item.name = name
		if err := listing.add(item, version, rec[isoFlagsOffset]&isoFlagMultiExtent != 0); err != nil {
			return nil, err
		}
	}
	return listing.entries, rejectDuplicates(listing.entries)
}

// isoListing accumulates directory records: multi-extent files are merged and, when a name occurs
// with several ";N" versions, the highest version wins.
type isoListing struct {
	entries   []entry
	versions  map[string]int // name -> version of the kept entry
	index     map[string]int // name -> position in entries
	openMulti bool           // previous record continues in the next one
}

func (l *isoListing) add(item entry, version int, multi bool) error {
	continues := l.openMulti
	l.openMulti = multi
	if last := len(l.entries) - 1; continues && last >= 0 && l.entries[last].name == item.name {
		if len(l.entries[last].extents) >= maxExtents {
			return corrupt("iso9660 file %q has too many extents", item.name)
		}
		l.entries[last].extents = append(l.entries[last].extents, item.extents...)
		l.entries[last].size += item.size
		return nil
	}
	if position, seen := l.index[item.name]; seen {
		if version > l.versions[item.name] {
			l.entries[position], l.versions[item.name] = item, version
		}
		return nil
	}
	if len(l.entries) >= maxDirEntries {
		return corrupt("iso9660 directory has too many entries")
	}
	l.index[item.name], l.versions[item.name] = len(l.entries), version
	l.entries = append(l.entries, item)
	return nil
}

// recordName strips the ";N" version (returned) and a trailing dot; skip is set for ".", ".."
// and unusable names.
func recordName(rec []byte) (name string, version int, skip bool) {
	raw := string(rec[isoNameOffset : isoNameOffset+int(rec[isoNameLenOffset])])
	if cut := strings.IndexByte(raw, ';'); cut >= 0 {
		version, _ = strconv.Atoi(raw[cut+1:]) // unparsable versions count as 0
		raw = raw[:cut]
	}
	raw = strings.TrimSuffix(raw, ".")
	if !validName(raw) {
		return "", 0, true
	}
	return raw, version, false
}

func validName(name string) bool {
	if name == "" || name == "." || name == ".." || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if r == '/' || r == '\\' || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

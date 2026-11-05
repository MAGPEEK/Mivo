package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"slices"
)

// FFmpeg encodes HEVC but has no HEIF muxer (ADR 0021), so the generator wraps a raw HEVC
// stream from libx265 in a HEIF file itself (task M3-15g): one coded picture per image item,
// the parameter sets in an hvcC property, an ispe size per item and an Exif item. A grid
// image is a grid item that references one hidden hvc1 tile item per picture.

// HEVC NAL unit types (H.265 table 7-1).
const (
	nalVPS          = 32
	nalSPS          = 33
	nalPPS          = 34
	nalFirstNonVCL  = 32
	nalTypeShift    = 1
	nalTypeMask     = 0x3F
	nalHeaderSize   = 2
	firstSliceFlag  = 0x80
	startCodeLength = 3
	// profileTierLevelStart and profileTierLevelSize locate the general profile, tier and
	// level in the SPS payload: after the NAL header and the byte with sps_video_parameter_set_id.
	profileTierLevelStart = nalHeaderSize + 1
	profileTierLevelSize  = 12
	emulationPrevention   = 0x03
)

// hvcC fields that do not depend on the stream (ISO/IEC 14496-15 section 8.3.3.1).
const (
	hvcCVersion = 1
	// hvcCSegmentation is the reserved bits with min_spatial_segmentation_idc 0.
	hvcCSegmentation = 0xF000
	// hvcCParallelism is the reserved bits with parallelismType 0.
	hvcCParallelism = 0xFC
	// hvcCChroma420 is the reserved bits with chroma_format_idc 1 (4:2:0).
	hvcCChroma420 = 0xFD
	// hvcCBitDepth8 is the reserved bits with bit_depth_minus8 0.
	hvcCBitDepth8 = 0xF8
	// hvcCLayers is one temporal layer, temporal ID nested and 4-byte NAL lengths.
	hvcCLayers = 0x0F
	// hvcCArrayComplete marks a NAL array that holds every parameter set of its type.
	hvcCArrayComplete = 0x80
)

// HEIF item values.
const (
	itemTypeHEVC = "hvc1"
	itemTypeGrid = "grid"
	itemTypeExif = "Exif"
	// infeVersion2 has 16-bit item IDs; infeHidden marks a tile that is not shown alone.
	infeVersion2 = 2
	infeHidden   = 1
	// essentialProperty marks an ipma association a reader must understand.
	essentialProperty = 0x80
	// ilocSizes are 4-byte offsets and lengths and no base offset (iloc version 0).
	ilocSizes = 0x44
	// ftypMinorVersionSize, exifOffsetSize, protectionIndexSize and hdlrReservedSize are
	// zero fields of ftyp, the Exif item, infe and hdlr.
	ftypMinorVersionSize = 4
	exifOffsetSize       = 4
	protectionIndexSize  = 2
	hdlrPreDefinedSize   = 4
	hdlrReservedSize     = 12
	// gridExtraItems are the grid item and the Exif item besides the tiles; firstTileID
	// follows the grid item.
	gridExtraItems = 2
	firstTileID    = 2
	singleExifID   = 2
)

var errNoHEVCPicture = errors.New("no HEVC picture in the stream")

// hevcStream is a raw HEVC stream split into its parameter sets and coded pictures; each
// picture is its VCL NAL units, length-prefixed as HEIF items store them.
type hevcStream struct {
	vps, sps, pps []byte
	pictures      [][]byte
}

// parseHEVC reads an Annex B stream with one slice per picture and intra pictures only,
// as the generator's libx265 sources are. SEI and other non-VCL units are dropped.
func parseHEVC(data []byte) (hevcStream, error) {
	var stream hevcStream

	for _, unit := range splitAnnexB(data) {
		if len(unit) < nalHeaderSize+1 {
			continue
		}

		switch kind := (unit[0] >> nalTypeShift) & nalTypeMask; {
		case kind == nalVPS:
			stream.vps = cmpOr(stream.vps, unit)
		case kind == nalSPS:
			stream.sps = cmpOr(stream.sps, unit)
		case kind == nalPPS:
			stream.pps = cmpOr(stream.pps, unit)
		case kind < nalFirstNonVCL:
			stream.addSlice(unit)
		}
	}

	if stream.vps == nil || stream.sps == nil || stream.pps == nil {
		return hevcStream{}, fmt.Errorf("HEVC stream lacks a parameter set")
	}

	if len(stream.pictures) == 0 {
		return hevcStream{}, errNoHEVCPicture
	}

	return stream, nil
}

// cmpOr keeps the first parameter set of a type.
func cmpOr(current, candidate []byte) []byte {
	if current != nil {
		return current
	}

	return candidate
}

// addSlice starts a new picture at a slice with first_slice_segment_in_pic_flag.
func (stream *hevcStream) addSlice(unit []byte) {
	prefixed := binary.BigEndian.AppendUint32(nil, uint32(len(unit))) //nolint:gosec // fixture slices are small
	prefixed = append(prefixed, unit...)

	if unit[nalHeaderSize]&firstSliceFlag != 0 || len(stream.pictures) == 0 {
		stream.pictures = append(stream.pictures, prefixed)

		return
	}

	last := len(stream.pictures) - 1
	stream.pictures[last] = append(stream.pictures[last], prefixed...)
}

// splitAnnexB returns the NAL units between 3- or 4-byte start codes.
func splitAnnexB(data []byte) [][]byte {
	var units [][]byte

	startCode := []byte{0, 0, 1}
	position := bytes.Index(data, startCode)

	for position >= 0 {
		start := position + startCodeLength
		next := bytes.Index(data[start:], startCode)

		if next < 0 {
			units = append(units, data[start:])

			break
		}

		units = append(units, bytes.TrimRight(data[start:start+next], "\x00"))
		position = start + next
	}

	return units
}

// hvcC is the HEVCDecoderConfigurationRecord of the stream: profile, tier and level from
// the SPS and one array each for VPS, SPS and PPS.
func (stream hevcStream) hvcC() ([]byte, error) {
	sps := removeEmulationPrevention(stream.sps)
	if len(sps) < profileTierLevelStart+profileTierLevelSize {
		return nil, fmt.Errorf("SPS of %d bytes is too short", len(sps))
	}

	record := []byte{hvcCVersion}
	record = append(record, sps[profileTierLevelStart:profileTierLevelStart+profileTierLevelSize]...)
	record = binary.BigEndian.AppendUint16(record, hvcCSegmentation)
	record = append(record, hvcCParallelism, hvcCChroma420, hvcCBitDepth8, hvcCBitDepth8)
	record = binary.BigEndian.AppendUint16(record, 0)                      // avgFrameRate unspecified
	record = append(record, hvcCLayers, byte(len(stream.parameterSets()))) //nolint:gosec // three arrays

	for _, unit := range stream.parameterSets() {
		record = append(record, hvcCArrayComplete|(unit[0]>>nalTypeShift)&nalTypeMask)
		record = binary.BigEndian.AppendUint16(record, 1)
		record = binary.BigEndian.AppendUint16(record, uint16(len(unit))) //nolint:gosec // parameter sets are small
		record = append(record, unit...)
	}

	return record, nil
}

func (stream hevcStream) parameterSets() [][]byte {
	return [][]byte{stream.vps, stream.sps, stream.pps}
}

// removeEmulationPrevention turns a NAL unit into its RBSP (00 00 03 -> 00 00).
func removeEmulationPrevention(unit []byte) []byte {
	rbsp := make([]byte, 0, len(unit))
	zeros := 0

	for _, value := range unit {
		if zeros >= 2 && value == emulationPrevention {
			zeros = 0

			continue
		}

		if value == 0 {
			zeros++
		} else {
			zeros = 0
		}

		rbsp = append(rbsp, value)
	}

	return rbsp
}

// heifImage describes the HEIF file: one picture shown as is, or a grid of rows x columns
// tiles (one picture each, row by row) shown as a width x height canvas.
type heifImage struct {
	stream     hevcStream
	tileWidth  int
	tileHeight int
	rows       int
	columns    int
	exif       []byte
}

// heifItem is one item with its data in mdat and its property associations.
type heifItem struct {
	id         uint16
	kind       string
	hidden     bool
	data       []byte
	properties []byte
}

// Property indexes (1-based) in ipco: the hvcC, the tile size and the canvas size.
const (
	propertyHvcC   = 1
	propertyTile   = 2
	propertyCanvas = 3
)

// encodeHEIF writes ftyp, meta and mdat. A single picture is item 1; a grid is item 1 with
// tiles 2...n+1. The Exif item comes last and describes the primary item.
func encodeHEIF(image heifImage) ([]byte, error) {
	items, err := image.items()
	if err != nil {
		return nil, err
	}

	config, err := image.stream.hvcC()
	if err != nil {
		return nil, err
	}

	ftyp := isoBox("ftyp", []byte("heic"), make([]byte, ftypMinorVersionSize), []byte("mif1heic"))
	properties := isoBox("ipco", isoBox("hvcC", config),
		isoFullBox("ispe", 0, 0, sizePayload(image.tileWidth, image.tileHeight)),
		isoFullBox("ispe", 0, 0, sizePayload(image.tileWidth*image.columns, image.tileHeight*image.rows)))

	meta := image.meta(items, properties, 0)
	dataStart := len(ftyp) + len(meta) + isoBoxHeaderSize
	meta = image.meta(items, properties, dataStart)

	var mdat []byte
	for _, item := range items {
		mdat = append(mdat, item.data...)
	}

	return slices.Concat(ftyp, meta, isoBox("mdat", mdat)), nil
}

// items lists the image items and the Exif item.
func (image heifImage) items() ([]heifItem, error) {
	if image.rows*image.columns != len(image.stream.pictures) {
		return nil, fmt.Errorf("%dx%d grid for %d pictures", image.rows, image.columns, len(image.stream.pictures))
	}

	if len(image.stream.pictures) == 1 {
		return []heifItem{
			{id: 1, kind: itemTypeHEVC, data: image.stream.pictures[0], properties: []byte{essentialProperty | propertyHvcC, propertyTile}},
			{id: singleExifID, kind: itemTypeExif, data: exifItemData(image.exif)},
		}, nil
	}

	grid := []byte{0, 0, byte(image.rows - 1), byte(image.columns - 1)}               //nolint:gosec // small grids
	grid = binary.BigEndian.AppendUint16(grid, uint16(image.tileWidth*image.columns)) //nolint:gosec // fixture sizes
	grid = binary.BigEndian.AppendUint16(grid, uint16(image.tileHeight*image.rows))   //nolint:gosec // fixture sizes
	items := []heifItem{{id: 1, kind: itemTypeGrid, data: grid, properties: []byte{propertyCanvas}}}

	for index, picture := range image.stream.pictures {
		items = append(items, heifItem{
			id: uint16(index + firstTileID), kind: itemTypeHEVC, hidden: true, data: picture,
			properties: []byte{essentialProperty | propertyHvcC, propertyTile},
		})
	}

	return append(items, heifItem{id: uint16(len(items) + 1), kind: itemTypeExif, data: exifItemData(image.exif)}), nil //nolint:gosec // small grids
}

// exifItemData prefixes the TIFF stream with exif_tiff_header_offset 0.
func exifItemData(tiffStream []byte) []byte {
	return slices.Concat(make([]byte, exifOffsetSize), tiffStream)
}

// meta builds the meta box; dataStart is the file offset of the mdat payload.
func (image heifImage) meta(items []heifItem, properties []byte, dataStart int) []byte {
	var infos, associations, locations, references []byte

	offset, associated := dataStart, 0

	for _, item := range items {
		flags := 0
		if item.hidden {
			flags = infeHidden
		}

		infos = append(infos, isoFullBox("infe", infeVersion2, flags,
			binary.BigEndian.AppendUint16(nil, item.id), make([]byte, protectionIndexSize), []byte(item.kind), []byte{0})...)

		locations = binary.BigEndian.AppendUint16(locations, item.id)
		locations = append(locations, 0, 0, 0, 1)                                    // data_reference_index 0, one extent
		locations = binary.BigEndian.AppendUint32(locations, uint32(offset))         //nolint:gosec // small files
		locations = binary.BigEndian.AppendUint32(locations, uint32(len(item.data))) //nolint:gosec // small files
		offset += len(item.data)

		if len(item.properties) > 0 {
			associations = binary.BigEndian.AppendUint16(associations, item.id)
			associations = append(associations, byte(len(item.properties))) //nolint:gosec // two properties at most
			associations = append(associations, item.properties...)
			associated++
		}
	}

	primary, exif := items[0], items[len(items)-1]
	references = append(references, isoBox("cdsc", referencePayload(exif.id, primary.id))...)

	if primary.kind == itemTypeGrid {
		tiles := make([]uint16, 0, len(items)-gridExtraItems)
		for _, item := range items[1 : len(items)-1] {
			tiles = append(tiles, item.id)
		}

		references = append(references, isoBox("dimg", referencePayload(primary.id, tiles...))...)
	}

	return isoFullBox("meta", 0, 0,
		isoFullBox("hdlr", 0, 0, make([]byte, hdlrPreDefinedSize), []byte("pict"), make([]byte, hdlrReservedSize), []byte{0}),
		isoFullBox("pitm", 0, 0, binary.BigEndian.AppendUint16(nil, primary.id)),
		isoFullBox("iinf", 0, 0, binary.BigEndian.AppendUint16(nil, uint16(len(items))), infos), //nolint:gosec // few items
		isoFullBox("iref", 0, 0, references),
		isoBox("iprp", properties, isoFullBox("ipma", 0, 0,
			binary.BigEndian.AppendUint32(nil, uint32(associated)), associations)),
		isoFullBox("iloc", 0, 0, []byte{ilocSizes, 0}, binary.BigEndian.AppendUint16(nil, uint16(len(items))), locations), //nolint:gosec // few items
	)
}

// referencePayload is one iref entry: from_item_ID, reference_count, to_item_IDs.
func referencePayload(from uint16, to ...uint16) []byte {
	payload := binary.BigEndian.AppendUint16(nil, from)
	payload = binary.BigEndian.AppendUint16(payload, uint16(len(to))) //nolint:gosec // few references

	for _, id := range to {
		payload = binary.BigEndian.AppendUint16(payload, id)
	}

	return payload
}

func sizePayload(width, height int) []byte {
	payload := binary.BigEndian.AppendUint32(nil, uint32(width))  //nolint:gosec // fixture sizes
	return binary.BigEndian.AppendUint32(payload, uint32(height)) //nolint:gosec // fixture sizes
}

// isoBoxHeaderSize is the 32-bit size and the type of a box.
const isoBoxHeaderSize = 8

// isoBox writes a box with a 32-bit size.
func isoBox(kind string, payload ...[]byte) []byte {
	body := slices.Concat(payload...)
	header := binary.BigEndian.AppendUint32(nil, uint32(isoBoxHeaderSize+len(body))) //nolint:gosec // small boxes

	return slices.Concat(header, []byte(kind), body)
}

// isoFullBox writes a box with version and 24-bit flags.
func isoFullBox(kind string, version, flags int, payload ...[]byte) []byte {
	versionAndFlags := binary.BigEndian.AppendUint32(nil, uint32(version<<24|flags)) //nolint:gosec // small values

	return isoBox(kind, append([][]byte{versionAndFlags}, payload...)...)
}

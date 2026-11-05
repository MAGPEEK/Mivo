package disc

import (
	"fmt"

	"github.com/magpeek/mivo/internal/domain/media"
)

// Encryption detection is a heuristic over the first bytes of each main-title clip. It only
// reads the flags that CSS and AACS leave in the clear; there is no key handling and no
// decryption path. A false positive shows a disc as unavailable (disc_encrypted), which is
// preferred over a session that fails with garbled output.

// DVD (CSS): each 2048-byte sector is one MPEG-2 program stream pack. CSS scrambles the
// payload behind the PES header and sets PES_scrambling_control in that header.
const (
	// maxProbedSectors bounds the sectors read per VOB; the feature's first video packs follow
	// the navigation pack within a few sectors.
	maxProbedSectors = 128

	packStartCode      = 0xBA
	systemHeaderStream = 0xBB
	paddingStream      = 0xBE
	privateStream2     = 0xBF
	// packHeaderSize is the MPEG-2 pack header without stuffing; byte 13 holds the stuffing length.
	packHeaderSize     = 14
	packStuffingOffset = 13
	packStuffingMask   = 0x07
	// mpeg2PackMarker is the '01' prefix of the SCR field that marks an MPEG-2 (not MPEG-1) pack.
	mpeg2PackMarker     = 0x40
	mpeg2PackMarkerMask = 0xC0
	// pesFlagsOffset is the PES header byte with the '10' marker and PES_scrambling_control.
	pesFlagsOffset     = 6
	pesMarker          = 0x80
	pesMarkerMask      = 0xC0
	pesScramblingShift = 4
	pesScramblingMask  = 0x03
)

// Blu-ray (AACS): a BDAV stream is a run of 192-byte source packets, a 4-byte TP_extra_header
// (copy_permission_indicator in its top two bits) followed by a 188-byte TS packet. AACS
// encrypts 6144-byte aligned units except their first 16 bytes, so from the second packet on
// the TS sync bytes no longer read 0x47, and it sets copy_permission_indicator.
const (
	sourcePacketSize  = 192
	tpExtraHeaderSize = 4
	alignedUnitSize   = 6144
	// probedAlignedUnits bounds the bytes read per m2ts clip.
	probedAlignedUnits = 2
	tsSyncByte         = 0x47
	// tsScramblingOffset is the TS header byte whose top two bits are transport_scrambling_control.
	tsScramblingOffset = 3
	twoBitFieldShift   = 6
)

const (
	maxVOBProbe  = maxProbedSectors * dvdSectorSize
	maxM2TSProbe = probedAlignedUnits * alignedUnitSize

	aacsFolderName = "aacs"
)

// vobVerdict wraps media.ErrDiscEncrypted when the probed VOB prefix holds a CSS scrambled
// sector. ISO images reach it through the image's fs.FS, so no io.ReaderAt entry is needed.
func vobVerdict(data []byte) error {
	if scrambledSector(data) {
		return fmt.Errorf("CSS scrambled sector: %w", media.ErrDiscEncrypted)
	}

	return nil
}

// m2tsVerdict wraps media.ErrDiscEncrypted when the probed m2ts prefix loses its TS sync bytes
// or, on a disc with an AACS folder, carries copy permission or transport scrambling bits.
func m2tsVerdict(data []byte, aacsFolder bool) error {
	if reason := m2tsEncryption(data, aacsFolder); reason != "" {
		return fmt.Errorf("%s: %w", reason, media.ErrDiscEncrypted)
	}

	return nil
}

// scrambledSector reports whether any whole sector in data starts with an MPEG-2 pack whose
// first PES packet is an elementary stream with PES_scrambling_control set. Navigation packs
// (system header, private stream 2) and padding are never scrambled and are skipped.
func scrambledSector(data []byte) bool {
	for start := 0; start+dvdSectorSize <= len(data); start += dvdSectorSize {
		if packScrambled(data[start : start+dvdSectorSize]) {
			return true
		}
	}

	return false
}

// packScrambled inspects one whole sector. Its PES header starts at most 21 bytes in (14-byte
// pack header plus up to 7 stuffing bytes), so the flags byte always lies inside the sector.
func packScrambled(sector []byte) bool {
	if !hasStartCode(sector, 0) || sector[3] != packStartCode ||
		sector[4]&mpeg2PackMarkerMask != mpeg2PackMarker {
		return false
	}
	pes := packHeaderSize + int(sector[packStuffingOffset]&packStuffingMask)
	if !hasStartCode(sector, pes) {
		return false
	}
	switch sector[pes+3] {
	case systemHeaderStream, paddingStream, privateStream2:
		return false
	}
	flags := sector[pes+pesFlagsOffset]

	return flags&pesMarkerMask == pesMarker && (flags>>pesScramblingShift)&pesScramblingMask != 0
}

// hasStartCode reports whether data holds the MPEG start code prefix 00 00 01 and a stream id
// byte at offset.
func hasStartCode(data []byte, offset int) bool {
	return offset+3 < len(data) && data[offset] == 0 && data[offset+1] == 0 && data[offset+2] == 1
}

// m2tsEncryption returns why the whole source packets in data look encrypted, or "".
func m2tsEncryption(data []byte, aacsFolder bool) string {
	for start := 0; start+sourcePacketSize <= len(data); start += sourcePacketSize {
		packet := data[start : start+sourcePacketSize]
		if packet[tpExtraHeaderSize] != tsSyncByte {
			return "TS sync byte missing"
		}
		if !aacsFolder {
			continue
		}
		if packet[0]>>twoBitFieldShift != 0 {
			return "AACS copy permission set"
		}
		if packet[tpExtraHeaderSize+tsScramblingOffset]>>twoBitFieldShift != 0 {
			return "AACS transport scrambling set"
		}
	}

	return ""
}

// checkEncryption checks every clip of the selected title and fails with ErrDiscEncrypted
// on the first that looks protected. Read errors abort, since the clip is unplayable anyway.
func (files *fileTree) checkEncryption(layout media.Layout, clips []titleClip) error {
	aacsFolder := false
	if layout == media.LayoutBDMV {
		// The AACS folder sits next to BDMV, so it is only visible when the tree is rooted at
		// the disc root; the sync byte check alone still catches AACS encrypted units.
		_, found, err := files.childDir(".", aacsFolderName)
		if err != nil {
			return fmt.Errorf("check disc encryption: %w", err)
		}
		aacsFolder = found
	}

	for _, clip := range clips {
		if err := files.checkClip(layout, clip.path, aacsFolder); err != nil {
			return fmt.Errorf("check disc encryption %s: %w", clip.path, err)
		}
	}

	return nil
}

func (files *fileTree) checkClip(layout media.Layout, clipPath string, aacsFolder bool) error {
	if layout == media.LayoutVideoTS {
		data, err := files.prefix(clipPath, maxVOBProbe)
		if err != nil {
			return err
		}
		return vobVerdict(data)
	}

	data, err := files.prefix(clipPath, maxM2TSProbe)
	if err != nil {
		return err
	}

	return m2tsVerdict(data, aacsFolder)
}

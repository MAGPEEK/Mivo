package disc

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/fstest"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/domain/media"
)

// PES stream ids used by the DVD fixtures.
const (
	testVideoStream    = 0xE0
	testAudioStream    = 0xC0
	testPrivateStream1 = 0xBD
	testPaddingStream  = 0xBE
)

// dvdSector returns one MPEG-2 program stream pack whose first PES packet carries the given
// stream id and PES_scrambling_control value; stuffing adds pack stuffing bytes.
func dvdSector(streamID, scrambling, stuffing byte) []byte {
	sector := make([]byte, dvdSectorSize)
	copy(sector, []byte{0, 0, 1, packStartCode, 0x44})
	sector[packStuffingOffset] = 0xF8 | stuffing
	pes := packHeaderSize + int(stuffing)
	copy(sector[pes:], []byte{0, 0, 1, streamID, 0x07, 0xEC})
	sector[pes+pesFlagsOffset] = 0x80 | scrambling<<4

	return sector
}

// navSector returns a DVD navigation pack (system header first), which is never scrambled.
func navSector() []byte {
	sector := make([]byte, dvdSectorSize)
	copy(sector, []byte{0, 0, 1, packStartCode, 0x44})
	sector[packStuffingOffset] = 0xF8
	copy(sector[packHeaderSize:], []byte{0, 0, 1, systemHeaderStream, 0x00, 0x12, 0xFF})

	return sector
}

func sectors(parts ...[]byte) []byte {
	return bytes.Join(parts, nil)
}

func clearVOB() []byte {
	return sectors(navSector(), dvdSector(testVideoStream, 0, 0), dvdSector(testAudioStream, 0, 0))
}

func scrambledVOB() []byte {
	return sectors(navSector(), dvdSector(testVideoStream, 1, 0), dvdSector(testVideoStream, 1, 0))
}

func TestVOBVerdict(t *testing.T) {
	mpeg1Pack := dvdSector(testVideoStream, 1, 0)
	mpeg1Pack[4] = 0x21
	partial := dvdSector(testVideoStream, 1, 0)[:dvdSectorSize-1]
	noPES := dvdSector(testVideoStream, 1, 0)
	noPES[packHeaderSize+2] = 0

	cases := []struct {
		name      string
		data      []byte
		encrypted bool
	}{
		{"clear video and audio", clearVOB(), false},
		{"empty file", nil, false},
		{"no pack headers", make([]byte, 4*dvdSectorSize), false},
		{"scrambled video", scrambledVOB(), true},
		{"scrambled audio", sectors(navSector(), dvdSector(testAudioStream, 2, 0)), true},
		{"scrambled private stream 1", sectors(dvdSector(testPrivateStream1, 3, 0)), true},
		{"scrambled after pack stuffing", sectors(dvdSector(testVideoStream, 1, 3)), true},
		{"pack without PES start code", noPES, false},
		{"padding stream is ignored", sectors(dvdSector(testPaddingStream, 1, 0)), false},
		{"MPEG-1 pack is ignored", mpeg1Pack, false},
		{"partial sector is ignored", partial, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := vobVerdict(tc.data)
			assertEncrypted(t, err, tc.encrypted)
		})
	}
}

// m2tsPacket describes one 192-byte BDAV source packet.
type m2tsPacket struct {
	copyPermission, scrambling byte
	brokenSync                 bool
}

func buildM2TS(packets ...m2tsPacket) []byte {
	data := make([]byte, 0, len(packets)*sourcePacketSize)
	for _, packet := range packets {
		source := make([]byte, sourcePacketSize)
		source[0] = packet.copyPermission << 6
		source[tpExtraHeaderSize] = tsSyncByte
		if packet.brokenSync {
			source[tpExtraHeaderSize] = 0x9C
		}
		source[tpExtraHeaderSize+tsScramblingOffset] = packet.scrambling<<6 | 0x10
		data = append(data, source...)
	}

	return data
}

func clearM2TS() []byte {
	return buildM2TS(m2tsPacket{}, m2tsPacket{}, m2tsPacket{})
}

func TestM2TSVerdict(t *testing.T) {
	aacsUnit := buildM2TS(m2tsPacket{copyPermission: 3}, m2tsPacket{brokenSync: true}, m2tsPacket{brokenSync: true})
	trailingGarbage := append(clearM2TS(), 0x00, 0x01, 0x02)

	cases := []struct {
		name      string
		data      []byte
		aacs      bool
		encrypted bool
	}{
		{"clear without AACS", clearM2TS(), false, false},
		{"clear with AACS", clearM2TS(), true, false},
		{"empty file", nil, true, false},
		{"shorter than one packet", make([]byte, sourcePacketSize-1), false, false},
		{"trailing partial packet is ignored", trailingGarbage, false, false},
		{"copy permission without AACS", buildM2TS(m2tsPacket{copyPermission: 1}), false, false},
		{"copy permission with AACS", buildM2TS(m2tsPacket{}, m2tsPacket{copyPermission: 1}), true, true},
		{"scrambling without AACS", buildM2TS(m2tsPacket{scrambling: 2}), false, false},
		{"scrambling with AACS", buildM2TS(m2tsPacket{scrambling: 2}), true, true},
		{"encrypted aligned unit without AACS folder", aacsUnit, false, true},
		{"broken first sync byte", buildM2TS(m2tsPacket{brokenSync: true}), false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := m2tsVerdict(tc.data, tc.aacs)
			assertEncrypted(t, err, tc.encrypted)
		})
	}
}

func TestEncryptionCheckReadsOnlyAPrefix(t *testing.T) {
	beyondProbe := make([][]byte, 0, maxProbedSectors+1)
	for range maxProbedSectors {
		beyondProbe = append(beyondProbe, dvdSector(testVideoStream, 0, 0))
	}
	beyondProbe = append(beyondProbe, dvdSector(testVideoStream, 1, 0))

	files := newFileTree(context.Background(), videoTSTree(sectors(beyondProbe...)))
	if err := files.checkEncryption(media.LayoutVideoTS, []titleClip{{path: "VIDEO_TS/VTS_01_1.VOB"}}); err != nil {
		t.Fatalf("scrambled sector beyond the probe: error = %v, want nil", err)
	}
	if spent := int64(maxReadBudget) - files.budget; spent != maxVOBProbe {
		t.Fatalf("read %d bytes, want %d", spent, maxVOBProbe)
	}
}

func TestEncryptionCheckTreeRootedAtBDMV(t *testing.T) {
	// The AACS folder next to BDMV is invisible here, so CPI bits alone do not count.
	tree := fstest.MapFS{
		"index.bdmv":          sized(1),
		"PLAYLIST/00000.mpls": file(buildMPLS(item("00001", 60))),
		"STREAM/00001.m2ts":   file(buildM2TS(m2tsPacket{copyPermission: 3}, m2tsPacket{})),
	}
	_, err := ResolveTree(context.Background(), tree, media.LayoutBDMV)
	assertEncrypted(t, err, false)
}

func assertEncrypted(t *testing.T, err error, want bool) {
	t.Helper()
	if want {
		if !errors.Is(err, media.ErrDiscEncrypted) {
			t.Fatalf("error = %v, want %v", err, media.ErrDiscEncrypted)
		}
		if code, _ := media.DiscErrorCode(err); code != "disc_encrypted" {
			t.Fatalf("code = %q, want disc_encrypted", code)
		}
		return
	}
	if err != nil {
		t.Fatalf("error = %v, want nil", err)
	}
}

func bdmvTree(stream []byte, extra fstest.MapFS) fstest.MapFS {
	tree := fstest.MapFS{
		"BDMV/index.bdmv":          sized(1),
		"BDMV/PLAYLIST/00000.mpls": file(buildMPLS(item("00001", 60), item("00002", 60))),
		"BDMV/STREAM/00001.m2ts":   file(clearM2TS()),
		"BDMV/STREAM/00002.m2ts":   file(stream),
	}
	for name, entry := range extra {
		tree[name] = entry
	}

	return tree
}

func videoTSTree(vob []byte) fstest.MapFS {
	return fstest.MapFS{"VIDEO_TS/VIDEO_TS.IFO": sized(1), "VIDEO_TS/VTS_01_1.VOB": file(vob)}
}

func TestResolveTreeDetectsEncryption(t *testing.T) {
	aacsFolder := fstest.MapFS{"AACS/Unit_Key_RO.inf": sized(1)}
	cases := []struct {
		name      string
		tree      fstest.MapFS
		layout    media.Layout
		encrypted bool
	}{
		{"clear Blu-ray without AACS", bdmvTree(clearM2TS(), nil), media.LayoutBDMV, false},
		{"clear Blu-ray with AACS folder", bdmvTree(clearM2TS(), aacsFolder), media.LayoutBDMV, false},
		{"copy permission without AACS folder", bdmvTree(buildM2TS(m2tsPacket{copyPermission: 1}), nil), media.LayoutBDMV, false},
		{"copy permission with AACS folder", bdmvTree(buildM2TS(m2tsPacket{copyPermission: 1}), aacsFolder), media.LayoutBDMV, true},
		{"broken sync in second clip", bdmvTree(buildM2TS(m2tsPacket{}, m2tsPacket{brokenSync: true}), nil), media.LayoutBDMV, true},
		{"clear DVD", videoTSTree(clearVOB()), media.LayoutVideoTS, false},
		{"CSS scrambled DVD", videoTSTree(scrambledVOB()), media.LayoutVideoTS, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, err := ResolveTree(context.Background(), tc.tree, tc.layout)
			assertEncrypted(t, err, tc.encrypted)
			if !tc.encrypted && len(title) == 0 {
				t.Fatal("clear disc returned no title")
			}
		})
	}
}

func TestResolveImageDetectsEncryption(t *testing.T) {
	img := fakeImage{
		MapFS:   bdmvTree(buildM2TS(m2tsPacket{scrambling: 1}), fstest.MapFS{"AACS/MKB_RO.inf": sized(1)}),
		extents: map[string][]iso.Extent{"BDMV/STREAM/00001.m2ts": {{Offset: 1 << 20, Length: 576}}},
	}
	_, err := ResolveImage(context.Background(), img)
	assertEncrypted(t, err, true)
}

func TestEncryptionCheckHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	files := newFileTree(ctx, videoTSTree(scrambledVOB()))
	cancel()
	err := files.checkEncryption(media.LayoutVideoTS, []titleClip{{path: "VIDEO_TS/VTS_01_1.VOB", duration: time.Second}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestEncryptionCheckBudget(t *testing.T) {
	files := newFileTree(context.Background(), videoTSTree(clearVOB()))
	files.budget = dvdSectorSize
	err := files.checkEncryption(media.LayoutVideoTS, []titleClip{{path: "VIDEO_TS/VTS_01_1.VOB"}})
	if !errors.Is(err, media.ErrDiscUnsupported) {
		t.Fatalf("error = %v, want %v", err, media.ErrDiscUnsupported)
	}
}

// read() may overshoot the budget by one byte when it detects an oversized file; the check
// must then report the exhausted budget instead of probing nothing.
func TestEncryptionCheckOvershotBudget(t *testing.T) {
	files := newFileTree(context.Background(), videoTSTree(scrambledVOB()))
	files.budget = -1
	err := files.checkEncryption(media.LayoutVideoTS, []titleClip{{path: "VIDEO_TS/VTS_01_1.VOB"}})
	if !errors.Is(err, media.ErrDiscUnsupported) {
		t.Fatalf("error = %v, want %v", err, media.ErrDiscUnsupported)
	}
}

func TestEncryptionCheckMissingClip(t *testing.T) {
	files := newFileTree(context.Background(), fstest.MapFS{})
	err := files.checkEncryption(media.LayoutBDMV, []titleClip{{path: "BDMV/STREAM/00001.m2ts"}})
	if err == nil || errors.Is(err, media.ErrDiscEncrypted) {
		t.Fatalf("error = %v, want a read error", err)
	}
}

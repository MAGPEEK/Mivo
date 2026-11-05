package disc

import (
	"encoding/binary"
	"errors"
	"testing"
	"time"
)

func TestParsePlaylist(t *testing.T) {
	t.Parallel()

	longItem := testItem{clip: "00001", codec: "M2TS", in: 90000, out: 90000 + 45000*125}
	cases := []struct {
		name string
		data []byte
		want playlist
	}{
		{"single item", buildMPLS(longItem), playlist{{clipID: "00001", in: 90000, out: 90000 + 45000*125}}},
		{"several items", buildMPLS(item("00002", 10), item("00003", 20)), playlist{
			{clipID: "00002", out: 450000}, {clipID: "00003", out: 900000},
		}},
		{
			"multi-angle uses angle 1", buildMPLS(testItem{clip: "00007", codec: "M2TS", out: 45000, angles: []string{"00008", "00009"}}),
			playlist{{clipID: "00007", out: 45000}},
		},
		{"FMTS codec", buildMPLS(testItem{clip: "00004", codec: "FMTS", out: 45000}), playlist{{clipID: "00004", out: 45000}}},
		{"zero items", buildMPLS(), playlist{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parsePlaylist(tc.data)
			if err != nil {
				t.Fatalf("parsePlaylist: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("items = %d, want %d", len(got), len(tc.want))
			}
			for index := range got {
				if got[index] != tc.want[index] {
					t.Errorf("item %d = %+v, want %+v", index, got[index], tc.want[index])
				}
			}
		})
	}
}

func TestParsePlaylistRejectsMalformed(t *testing.T) {
	t.Parallel()

	valid := buildMPLS(item("00001", 60))
	mutate := func(change func([]byte) []byte) []byte {
		return change(append([]byte(nil), valid...))
	}
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"bad magic", mutate(func(data []byte) []byte { copy(data, "XPLS"); return data })},
		{"truncated header", valid[:12]},
		{"playlist start beyond data", mutate(func(data []byte) []byte {
			binary.BigEndian.PutUint32(data[8:], 0xFFFFFFFF)
			return data
		})},
		{"truncated play item", valid[:len(valid)-20]},
		{"item count larger than data", mutate(func(data []byte) []byte {
			binary.BigEndian.PutUint16(data[testPlaylistStart+6:], 0xFFFF)
			return data
		})},
		{"item length too short", mutate(func(data []byte) []byte {
			binary.BigEndian.PutUint16(data[testPlaylistStart+10:], 4)
			return data
		})},
		{"traversal clip name", buildMPLS(testItem{clip: "../..", codec: "M2TS", out: 45000})},
		{"non-digit clip name", buildMPLS(testItem{clip: "0000a", codec: "M2TS", out: 45000})},
		{"unknown codec", buildMPLS(testItem{clip: "00001", codec: "XXXX", out: 45000})},
		{"out before in", buildMPLS(testItem{clip: "00001", codec: "M2TS", in: 45000, out: 1})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, err := parsePlaylist(tc.data); !errors.Is(err, errMalformed) {
				t.Fatalf("parsePlaylist = %v, %v; want errMalformed", got, err)
			}
		})
	}
}

func TestPlaylistDurationAndLoop(t *testing.T) {
	t.Parallel()

	list := playlist{{clipID: "00001", in: 45000, out: 45000 * 61}, {clipID: "00002", out: 22500}}
	if got, want := list.duration(), 60*time.Second+500*time.Millisecond; got != want {
		t.Errorf("duration = %v, want %v", got, want)
	}
	if list.repeatsClip() {
		t.Error("distinct clips reported as loop")
	}
	if !append(list, playItem{clipID: "00001"}).repeatsClip() {
		t.Error("repeated clip not reported as loop")
	}
}

func TestParseTitleSetDuration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		data []byte
		want time.Duration
	}{
		{
			"one PGC at 25 fps", buildIFO(pgcTime{1, 45, 30, 12, 1}.bcd()),
			time.Hour + 45*time.Minute + 30*time.Second + 12*time.Second/25,
		},
		{
			"longest of several PGCs", buildIFO(pgcTime{0, 5, 0, 0, 1}.bcd(), pgcTime{2, 1, 2, 0, 3}.bcd(), pgcTime{0, 0, 3, 0, 1}.bcd()),
			2*time.Hour + time.Minute + 2*time.Second,
		},
		{"30 fps frames", buildIFO(pgcTime{0, 0, 1, 15, 3}.bcd()), time.Second + 500*time.Millisecond},
		{"invalid PGC skipped", buildIFO([4]byte{0xAA, 0, 0, 0}, pgcTime{0, 1, 0, 0, 1}.bcd()), time.Minute},
		{"zero time without rate", buildIFO([4]byte{0, 0, 0x10, 0}), 10 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseTitleSetDuration(tc.data)
			if err != nil {
				t.Fatalf("parseTitleSetDuration: %v", err)
			}
			if got != tc.want {
				t.Errorf("duration = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestParseTitleSetDurationRejectsMalformed(t *testing.T) {
	t.Parallel()

	valid := buildIFO(pgcTime{1, 0, 0, 0, 1}.bcd())
	mutate := func(change func([]byte)) []byte {
		data := append([]byte(nil), valid...)
		change(data)
		return data
	}
	cases := []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"bad magic", mutate(func(data []byte) { copy(data, "DVDVIDEO-VMG") })},
		{"PGCIT beyond data", mutate(func(data []byte) {
			binary.BigEndian.PutUint32(data[ifoPGCITSectorOffset:], 0xFFFFFFFF)
		})},
		{"no PGCs", buildIFO()},
		{"PGC count larger than data", mutate(func(data []byte) {
			binary.BigEndian.PutUint16(data[dvdSectorSize:], 0xFFFF)
			binary.BigEndian.PutUint32(data[dvdSectorSize+pgcitHeaderSize+4:], 0xFFFFFFF0)
		})},
		{"invalid BCD only", buildIFO([4]byte{0x0A, 0, 0, 0})},
		{"frames with invalid rate", buildIFO([4]byte{0, 0, 0, 0x85})},
		{"minutes out of range", buildIFO(pgcTime{0, 61, 0, 0, 1}.bcd())},
		{"truncated PGC", valid[:len(valid)-14]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, err := parseTitleSetDuration(tc.data); !errors.Is(err, errMalformed) {
				t.Fatalf("parseTitleSetDuration = %v, %v; want errMalformed", got, err)
			}
		})
	}
}

func FuzzParsePlaylist(f *testing.F) {
	f.Add(buildMPLS(item("00001", 60), item("00002", 30)))
	f.Add(buildMPLS(testItem{clip: "00007", codec: "M2TS", out: 45000, angles: []string{"00008"}}))
	f.Add([]byte("MPLS0200"))
	f.Fuzz(func(t *testing.T, data []byte) {
		list, err := parsePlaylist(data)
		if err != nil {
			return
		}
		for _, entry := range list {
			if !isClipID(entry.clipID) || entry.out < entry.in {
				t.Fatalf("accepted invalid item %+v", entry)
			}
		}
		if list.duration() < 0 {
			t.Fatalf("negative duration %v", list.duration())
		}
	})
}

// maxPGCPlaybackTime bounds every decodable BCD time (99:59:59 plus frames).
const maxPGCPlaybackTime = 100 * time.Hour

func FuzzParseTitleSetDuration(f *testing.F) {
	f.Add(buildIFO(pgcTime{1, 30, 0, 0, 1}.bcd(), pgcTime{0, 2, 0, 5, 3}.bcd()))
	f.Add(buildIFO())
	f.Add([]byte(vtsMagic))
	f.Fuzz(func(t *testing.T, data []byte) {
		duration, err := parseTitleSetDuration(data)
		if err == nil && (duration < 0 || duration > maxPGCPlaybackTime) {
			t.Fatalf("implausible duration %v", duration)
		}
	})
}

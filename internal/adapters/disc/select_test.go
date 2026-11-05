package disc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/media"
)

func clipAt(path string, duration time.Duration) media.DiscClip {
	return media.DiscClip{Path: path, Duration: duration}
}

func assertTitle(t *testing.T, got, want media.DiscTitle) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("title = %+v, want %+v", got, want)
	}
}

// TestResolveTreeBDMVCarriesTheSpanStart: a play item's IN_time becomes the clip's Start, so
// the player can cut the clip to the playlist's span.
func TestResolveTreeBDMVCarriesTheSpanStart(t *testing.T) {
	t.Parallel()

	const inTicks = 3 * mplsTicksPerSecond
	tree := fstest.MapFS{
		"BDMV/STREAM/00001.m2ts": sized(10),
		"BDMV/PLAYLIST/00000.mpls": file(buildMPLS(
			testItem{clip: "00001", codec: "M2TS", in: inTicks, out: inTicks + 90*mplsTicksPerSecond})),
	}

	got, err := ResolveTree(context.Background(), tree, media.LayoutBDMV)
	if err != nil {
		t.Fatalf("ResolveTree: %v", err)
	}
	assertTitle(t, got, media.DiscTitle{{Path: "BDMV/STREAM/00001.m2ts", Start: 3 * time.Second, Duration: 90 * time.Second}})
}

func TestResolveTreeBDMV(t *testing.T) {
	t.Parallel()

	streams := fstest.MapFS{
		"BDMV/index.bdmv":         sized(8),
		"BDMV/STREAM/00001.m2ts":  sized(10),
		"BDMV/STREAM/00002.m2ts":  sized(10),
		"BDMV/STREAM/00003.m2ts":  sized(10),
		"BDMV/STREAM/00004.m2ts":  sized(10),
		"BDMV/STREAM/00005.m2ts":  sized(10),
		"BDMV/STREAM/00099.m2ts":  sized(10),
		"BDMV/CLIPINF/00001.clpi": sized(4),
	}
	with := func(playlists map[string][]byte) fstest.MapFS {
		tree := fstest.MapFS{}
		for name, mapped := range streams {
			tree[name] = mapped
		}
		for name, data := range playlists {
			tree["BDMV/PLAYLIST/"+name] = file(data)
		}
		return tree
	}
	cases := []struct {
		name string
		tree fstest.MapFS
		want media.DiscTitle
	}{
		{"longest of several playlists", with(map[string][]byte{
			"00000.mpls": buildMPLS(item("00099", 30)),
			"00001.mpls": buildMPLS(item("00001", 3000), item("00002", 2000)),
			"00002.mpls": buildMPLS(item("00003", 600)),
		}), media.DiscTitle{
			clipAt("BDMV/STREAM/00001.m2ts", 3000*time.Second),
			clipAt("BDMV/STREAM/00002.m2ts", 2000*time.Second),
		}},
		{"loop playlist with repeated clips is skipped", with(map[string][]byte{
			"00000.mpls": buildMPLS(item("00099", 900), item("00099", 900), item("00099", 900)),
			"00001.mpls": buildMPLS(item("00003", 1200)),
		}), media.DiscTitle{clipAt("BDMV/STREAM/00003.m2ts", 1200*time.Second)}},
		{"obfuscated short playlists", obfuscated(with), media.DiscTitle{
			clipAt("BDMV/STREAM/00004.m2ts", 5400*time.Second),
			clipAt("BDMV/STREAM/00005.m2ts", 1800*time.Second),
		}},
		{"playlist with missing clip is skipped", with(map[string][]byte{
			"00000.mpls": buildMPLS(item("00001", 9000), item("00042", 10)),
			"00001.mpls": buildMPLS(item("00002", 100)),
		}), media.DiscTitle{clipAt("BDMV/STREAM/00002.m2ts", 100*time.Second)}},
		{"unparsable playlist is skipped", with(map[string][]byte{
			"00000.mpls": []byte("garbage"),
			"00001.mpls": buildMPLS(item("00002", 100)),
		}), media.DiscTitle{clipAt("BDMV/STREAM/00002.m2ts", 100*time.Second)}},
		{"tie keeps the lower playlist number", with(map[string][]byte{
			"00800.mpls": buildMPLS(item("00002", 100)),
			"00010.mpls": buildMPLS(item("00001", 100)),
		}), media.DiscTitle{clipAt("BDMV/STREAM/00001.m2ts", 100*time.Second)}},
		{"multi-angle playlist plays angle 1", with(map[string][]byte{
			"00000.mpls": buildMPLS(testItem{clip: "00001", codec: "M2TS", out: 45000 * 70, angles: []string{"00002"}}),
		}), media.DiscTitle{clipAt("BDMV/STREAM/00001.m2ts", 70*time.Second)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveTree(context.Background(), tc.tree, media.LayoutBDMV)
			if err != nil {
				t.Fatalf("ResolveTree: %v", err)
			}
			assertTitle(t, got, tc.want)
		})
	}
}

// obfuscated builds a disc with many short decoy playlists around the real feature.
func obfuscated(with func(map[string][]byte) fstest.MapFS) fstest.MapFS {
	playlists := map[string][]byte{}
	for number := range 200 {
		playlists[fmt.Sprintf("%05d.mpls", number+100)] = buildMPLS(item("00001", 20), item("00002", 25))
	}
	playlists["00250.mpls"] = buildMPLS(item("00004", 5400), item("00005", 1800))
	return with(playlists)
}

func TestResolveTreeBDMVFolderVariants(t *testing.T) {
	t.Parallel()

	playlist := buildMPLS(item("00001", 60))
	cases := []struct {
		name string
		tree fstest.MapFS
		want string
	}{
		{"root is the BDMV folder", fstest.MapFS{
			"index.bdmv": sized(1), "PLAYLIST/00000.mpls": file(playlist), "STREAM/00001.m2ts": sized(1),
		}, "STREAM/00001.m2ts"},
		{"lower-case names", fstest.MapFS{
			"bdmv/playlist/00000.MPLS": file(playlist), "bdmv/stream/00001.M2TS": sized(1),
		}, "bdmv/stream/00001.M2TS"},
		{"AVCHD short extensions", fstest.MapFS{
			"BDMV/PLAYLIST/00000.MPL": file(playlist), "BDMV/STREAM/00001.MTS": sized(1),
		}, "BDMV/STREAM/00001.MTS"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveTree(context.Background(), tc.tree, media.LayoutBDMV)
			if err != nil {
				t.Fatalf("ResolveTree: %v", err)
			}
			assertTitle(t, got, media.DiscTitle{clipAt(tc.want, time.Minute)})
		})
	}
}

func TestResolveTreeNoTitle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		tree   fstest.MapFS
		layout media.Layout
	}{
		{"BDMV without playlists", fstest.MapFS{"BDMV/index.bdmv": sized(1), "BDMV/STREAM/00001.m2ts": sized(1)}, media.LayoutBDMV},
		{"BDMV only with missing clips", fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file(buildMPLS(item("00001", 60))),
		}, media.LayoutBDMV},
		{"BDMV only with empty clips", fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file(buildMPLS(item("00001", 60))), "BDMV/STREAM/00001.m2ts": sized(0),
		}, media.LayoutBDMV},
		{"BDMV only unparsable playlists", fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file([]byte("MPLS")), "BDMV/STREAM/00001.m2ts": sized(1),
		}, media.LayoutBDMV},
		{"BDMV only empty playlists", fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file(buildMPLS()), "BDMV/STREAM/00001.m2ts": sized(1),
		}, media.LayoutBDMV},
		{"VIDEO_TS without title VOBs", fstest.MapFS{
			"VIDEO_TS/VIDEO_TS.IFO": sized(1), "VIDEO_TS/VTS_01_0.VOB": sized(100),
		}, media.LayoutVideoTS},
		{"VIDEO_TS with a gap only", fstest.MapFS{
			"VIDEO_TS/VTS_01_1.VOB": sized(100), "VIDEO_TS/VTS_01_3.VOB": sized(100),
		}, media.LayoutVideoTS},
		{"VIDEO_TS with empty VOBs", fstest.MapFS{"VIDEO_TS/VTS_01_1.VOB": sized(0)}, media.LayoutVideoTS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ResolveTree(context.Background(), tc.tree, tc.layout)
			if code, _ := media.DiscErrorCode(err); code != "disc_no_title" {
				t.Fatalf("err = %v, want disc_no_title", err)
			}
		})
	}
}

func TestResolveTreeVideoTS(t *testing.T) {
	t.Parallel()

	const vob = 1000
	feature := buildIFO(pgcTime{0, 1, 40, 0, 1}.bcd())
	cases := []struct {
		name string
		tree fstest.MapFS
		want media.DiscTitle
	}{
		{"largest title set with IFO duration split by size", fstest.MapFS{
			"VIDEO_TS/VIDEO_TS.IFO": sized(1),
			"VIDEO_TS/VTS_01_0.VOB": sized(9 * vob), "VIDEO_TS/VTS_01_1.VOB": sized(vob),
			"VIDEO_TS/VTS_02_0.IFO": file(feature), "VIDEO_TS/VTS_02_0.VOB": sized(vob),
			"VIDEO_TS/VTS_02_1.VOB": sized(3 * vob), "VIDEO_TS/VTS_02_2.VOB": sized(vob),
		}, media.DiscTitle{
			clipAt("VIDEO_TS/VTS_02_1.VOB", 75*time.Second), clipAt("VIDEO_TS/VTS_02_2.VOB", 25*time.Second),
		}},
		{"BUP when IFO is unreadable", fstest.MapFS{
			"VIDEO_TS/VTS_01_0.IFO": file([]byte("broken")), "VIDEO_TS/VTS_01_0.BUP": file(feature),
			"VIDEO_TS/VTS_01_1.VOB": sized(vob),
		}, media.DiscTitle{clipAt("VIDEO_TS/VTS_01_1.VOB", 100*time.Second)}},
		{"unknown duration without IFO", fstest.MapFS{
			"VIDEO_TS/VTS_01_1.VOB": sized(vob), "VIDEO_TS/VTS_01_2.VOB": sized(vob),
		}, media.DiscTitle{clipAt("VIDEO_TS/VTS_01_1.VOB", 0), clipAt("VIDEO_TS/VTS_01_2.VOB", 0)}},
		{"set with a gap is skipped", fstest.MapFS{
			"VIDEO_TS/VTS_01_1.VOB": sized(vob), "VIDEO_TS/VTS_01_3.VOB": sized(9 * vob),
			"VIDEO_TS/VTS_02_1.VOB": sized(vob),
		}, media.DiscTitle{clipAt("VIDEO_TS/VTS_02_1.VOB", 0)}},
		{"tie keeps the lower set number", fstest.MapFS{
			"VIDEO_TS/VTS_03_1.VOB": sized(vob), "VIDEO_TS/VTS_02_1.VOB": sized(vob),
		}, media.DiscTitle{clipAt("VIDEO_TS/VTS_02_1.VOB", 0)}},
		{"root is the VIDEO_TS folder, lower case", fstest.MapFS{
			"video_ts.ifo": sized(1), "vts_01_1.vob": sized(vob), "vts_00_1.vob": sized(9 * vob),
			"VTS_01_1.VOB.part": sized(9 * vob),
		}, media.DiscTitle{clipAt("vts_01_1.vob", 0)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ResolveTree(context.Background(), tc.tree, media.LayoutVideoTS)
			if err != nil {
				t.Fatalf("ResolveTree: %v", err)
			}
			assertTitle(t, got, tc.want)
		})
	}
}

func TestResolveTreeRejectsNonFolderLayouts(t *testing.T) {
	t.Parallel()

	for _, layout := range []media.Layout{media.LayoutFile, media.LayoutISO, "bogus"} {
		if _, err := ResolveTree(context.Background(), fstest.MapFS{}, layout); !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("layout %q: err = %v, want ErrInvalid", layout, err)
		}
	}
}

func TestResolveTreeHonoursCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tree := fstest.MapFS{"BDMV/PLAYLIST/00000.mpls": file(buildMPLS(item("00001", 1))), "BDMV/STREAM/00001.m2ts": sized(1)}
	if _, err := ResolveTree(ctx, tree, media.LayoutBDMV); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

func TestLimits(t *testing.T) {
	t.Parallel()

	t.Run("oversized playlist is skipped", func(t *testing.T) {
		t.Parallel()
		huge := append(buildMPLS(item("00001", 9000)), make([]byte, maxPlaylistSize)...)
		tree := fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file(huge), "BDMV/PLAYLIST/00001.mpls": file(buildMPLS(item("00002", 5))),
			"BDMV/STREAM/00001.m2ts": sized(1), "BDMV/STREAM/00002.m2ts": sized(1),
		}
		got, err := ResolveTree(context.Background(), tree, media.LayoutBDMV)
		if err != nil {
			t.Fatalf("ResolveTree: %v", err)
		}
		assertTitle(t, got, media.DiscTitle{clipAt("BDMV/STREAM/00002.m2ts", 5*time.Second)})
	})
	t.Run("too many directory entries", func(t *testing.T) {
		t.Parallel()
		tree := fstest.MapFS{}
		for number := range maxDirEntries + 1 {
			tree[fmt.Sprintf("BDMV/STREAM/%05d.m2ts", number)] = sized(0)
		}
		_, err := ResolveTree(context.Background(), tree, media.LayoutBDMV)
		if !errors.Is(err, media.ErrDiscUnsupported) {
			t.Fatalf("err = %v, want disc_unsupported", err)
		}
	})
	t.Run("read budget exhausted", func(t *testing.T) {
		t.Parallel()
		tree := fstest.MapFS{"BDMV/STREAM/00001.m2ts": sized(1)}
		for number := range 4 {
			tree[fmt.Sprintf("BDMV/PLAYLIST/%05d.mpls", number)] = file(buildMPLS(item("00001", 5)))
		}
		files := newFileTree(context.Background(), tree)
		files.budget = 100
		if _, err := selectBDMV(files); !errors.Is(err, media.ErrDiscUnsupported) {
			t.Fatalf("err = %v, want disc_unsupported", err)
		}
	})
}

func TestSplitDuration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		total   time.Duration
		weights []int64
		want    []time.Duration
	}{
		{"proportional with exact remainder", 10 * time.Second, []int64{1, 1, 1}, []time.Duration{3333333333, 3333333333, 3333333334}},
		{"zero total", 0, []int64{5, 5}, []time.Duration{0, 0}},
		{"zero weights", time.Second, []int64{0, 0}, []time.Duration{0, time.Second}},
		{"huge values", 100 * time.Hour, []int64{1 << 40, 1 << 40}, []time.Duration{50 * time.Hour, 50 * time.Hour}},
		{"empty", time.Second, nil, []time.Duration{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := splitDuration(tc.total, tc.weights)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for index := range got {
				if got[index] != tc.want[index] {
					t.Errorf("share %d = %v, want %v", index, got[index], tc.want[index])
				}
			}
		})
	}
}

func FuzzResolveTreeBDMV(f *testing.F) {
	f.Add(buildMPLS(item("00001", 60), item("00002", 30)))
	f.Add(buildMPLS(item("00001", 60), item("00001", 30)))
	f.Fuzz(func(t *testing.T, data []byte) {
		tree := fstest.MapFS{
			"BDMV/PLAYLIST/00000.mpls": file(data),
			"BDMV/STREAM/00001.m2ts":   sized(1), "BDMV/STREAM/00002.m2ts": sized(1),
		}
		title, err := ResolveTree(context.Background(), tree, media.LayoutBDMV)
		if err != nil {
			if !errors.Is(err, media.ErrDiscNoTitle) {
				t.Fatalf("unexpected error %v", err)
			}
			return
		}
		for _, clip := range title {
			if clip.Path != "BDMV/STREAM/00001.m2ts" && clip.Path != "BDMV/STREAM/00002.m2ts" {
				t.Fatalf("clip outside the indexed streams: %q", clip.Path)
			}
		}
	})
}

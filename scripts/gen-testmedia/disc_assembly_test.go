package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/magpeek/mivo/internal/adapters/disc"
	"github.com/magpeek/mivo/internal/adapters/disc/iso"
	"github.com/magpeek/mivo/internal/domain/media"
)

// The disc writers are checked against the real readers of internal/adapters/disc: what
// the generator writes must be what title selection and the encryption check expect.

func fakeDVDSources() [][]byte {
	return [][]byte{fakeProgramStream(), fakeProgramStream()}
}

func fakeBluraySources() [][]byte {
	return [][]byte{fakeSourcePacketStream(), fakeSourcePacketStream(), fakeSourcePacketStream()}
}

// mapTree turns disc files into an in-memory tree rooted at the folder holding the disc.
func mapTree(files []discFile) fstest.MapFS {
	tree := fstest.MapFS{}
	for _, file := range files {
		tree[file.name] = &fstest.MapFile{Data: file.data}
	}

	return tree
}

func mustBuild(t *testing.T, build func([][]byte) ([]discFile, error), sources [][]byte) []discFile {
	t.Helper()

	files, err := build(sources)
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	return files
}

func TestDiscTreesResolveToTheCatalogTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tree   []discFile
		layout media.Layout
		want   expectedDiscTitle
	}{
		{
			name: "DVD", tree: mustBuild(t, dvdTree, fakeDVDSources()), layout: media.LayoutVideoTS,
			want: expectedDiscTitle{Clips: prefixed(videoTSFolder, dvdTitleClips()), DurationSeconds: dvdMainDuration.Seconds()},
		},
		{
			name: "Blu-ray", tree: mustBuild(t, blurayTree, fakeBluraySources()), layout: media.LayoutBDMV,
			want: expectedDiscTitle{
				Clips: prefixed(bdmvFolder, blurayTitleClips()), DurationSeconds: (blurayPartOne + blurayPartTwo).Seconds(),
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			title, err := disc.ResolveTree(t.Context(), mapTree(test.tree), test.layout)
			if err != nil {
				t.Fatalf("ResolveTree: %v", err)
			}

			var clips []string
			for _, clip := range title {
				clips = append(clips, clip.Path)
			}

			if !slices.Equal(clips, test.want.Clips) || title.Duration().Seconds() != test.want.DurationSeconds {
				t.Errorf("title %q for %s, want %q for %gs", clips, title.Duration(), test.want.Clips, test.want.DurationSeconds)
			}
		})
	}
}

func prefixed(prefix string, names []string) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, prefix+name)
	}

	return result
}

func TestProtectedStandInsAreDetectedAsEncrypted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		tree   fstest.MapFS
		layout media.Layout
	}{
		{name: "CSS", tree: mapTree(mustBuild(t, scrambledDVDTree, fakeDVDSources())), layout: media.LayoutVideoTS},
		{name: "AACS from the disc root", tree: mapTree(mustBuild(t, protectedBlurayTree, fakeBluraySources())), layout: media.LayoutBDMV},
		{name: "AACS from the BDMV folder", tree: bdmvSubtree(t), layout: media.LayoutBDMV},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if _, err := disc.ResolveTree(t.Context(), test.tree, test.layout); !errors.Is(err, media.ErrDiscEncrypted) {
				t.Errorf("ResolveTree = %v, want %v", err, media.ErrDiscEncrypted)
			}
		})
	}
}

// bdmvSubtree is the protected Blu-ray seen from its BDMV folder, where the AACS folder next
// to it is out of sight and only the unit marks remain.
func bdmvSubtree(t *testing.T) fstest.MapFS {
	t.Helper()

	tree := fstest.MapFS{}

	for _, file := range mustBuild(t, protectedBlurayTree, fakeBluraySources()) {
		if name, ok := strings.CutPrefix(file.name, bdmvFolder); ok {
			tree[name] = &fstest.MapFile{Data: file.data}
		}
	}

	return tree
}

func TestDVDImageResolvesToOneContiguousExtent(t *testing.T) {
	t.Parallel()

	files := mustBuild(t, dvdImage("image.iso"), fakeDVDSources())
	if len(files) != 1 || files[0].name != "image.iso" {
		t.Fatalf("files = %d, want only image.iso", len(files))
	}

	data := files[0].data

	image, err := iso.Open(t.Context(), bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("iso.Open: %v", err)
	}

	title, err := disc.ResolveImage(t.Context(), image)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}

	tree := mustBuild(t, dvdTree, fakeDVDSources())

	var want []byte

	for _, file := range tree {
		if slices.Contains(dvdTitleClips(), path.Base(file.name)) {
			want = append(want, file.data...)
		}
	}

	if len(title) != 1 || title.Duration() != dvdMainDuration ||
		!bytes.Equal(data[title[0].Offset:title[0].Offset+title[0].Length], want) {
		t.Errorf("title = %+v, want one extent with the feature VOBs lasting %s", title, dvdMainDuration)
	}

	pvd := data[isoSystemAreaSectors*dvdSectorSize:]
	if size := binary.LittleEndian.Uint32(pvd[pvdVolumeSpaceOffset:]); int(size)*dvdSectorSize != len(data) {
		t.Errorf("volume space %d sectors, image %d bytes", size, len(data))
	}

	unset := append([]byte(strings.Repeat("0", pvdDateDigits)), 0)
	if dates := pvd[pvdIdentifierFieldsEnd : pvdIdentifierFieldsEnd+pvdDateFields*pvdDateFieldSize]; !bytes.Equal(dates, bytes.Repeat(unset, pvdDateFields)) {
		t.Errorf("volume dates = %q, want four unset dates", dates)
	}
}

func TestDiscBuildersRefuseUnusableSources(t *testing.T) {
	t.Parallel()

	tiny := []byte("fake media")

	tests := []struct {
		name    string
		build   func([][]byte) ([]discFile, error)
		sources [][]byte
	}{
		{name: "VOB below two sectors", build: dvdTree, sources: [][]byte{tiny, tiny}},
		{name: "VOB without packs", build: scrambledDVDTree, sources: [][]byte{make([]byte, 2*dvdSectorSize), tiny}},
		{name: "clip that is no m2ts", build: blurayTree, sources: [][]byte{tiny, tiny, tiny}},
		{name: "image of a broken DVD", build: dvdImage("image.iso"), sources: [][]byte{tiny, tiny}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if files, err := test.build(test.sources); err == nil {
				t.Errorf("build = %d files, want an error", len(files))
			}
		})
	}

	if _, err := markAACSUnits(make([]byte, sourcePacketSize)); !errors.Is(err, errNoSourcePackets) {
		t.Errorf("markAACSUnits of one packet = %v, want %v", err, errNoSourcePackets)
	}

	if _, err := protectedBlurayTree([][]byte{tiny, tiny, tiny}); err == nil {
		t.Error("protectedBlurayTree accepted clips that are no m2ts")
	}
}

func TestISOTreeAcceptsOnlyLevelOneNames(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]bool{
		"VIDEO_TS": true, "VTS_01_1.VOB": true, "A.B": true,
		"": false, "video_ts": false, "TOOLONGNAME": false, "FILE.EXTN": false, "SPACE NAME": false,
	} {
		if got := isISOName(name); got != want {
			t.Errorf("isISOName(%q) = %t, want %t", name, got, want)
		}
	}

	for _, name := range []string{"A/B/C.TXT", "lower.txt", "DIR/lower.txt"} {
		if _, err := isoImage(isoVolumeID, []discFile{{name: name, data: []byte("x")}}); err == nil {
			t.Errorf("isoImage accepted %q", name)
		}
	}

	// Two sectors' worth of directory records do not fit the one sector a folder gets.
	const crowdedFolder = 100

	var crowded []discFile
	for index := range crowdedFolder {
		crowded = append(crowded, discFile{name: fmt.Sprintf("DIR/F%05d.TXT", index), data: []byte("x")})
	}

	if _, err := isoImage(isoVolumeID, crowded); err == nil {
		t.Error("isoImage accepted a folder that needs more than one directory sector")
	}
}

func TestPlaybackTimeIsBCD(t *testing.T) {
	t.Parallel()

	got := playbackTimeBCD(time.Hour + 23*time.Minute + 45*time.Second)
	if want := []byte{0x01, 0x23, 0x45, pal25FPSFlag}; !bytes.Equal(got, want) {
		t.Errorf("playbackTimeBCD = % x, want % x", got, want)
	}
}

// TestPGSSubtitlesAreWellFramed walks the segments: each starts with the magic, its length
// field matches, and every cue opens and closes a display set with an end segment.
func TestPGSSubtitlesAreWellFramed(t *testing.T) {
	t.Parallel()

	const segmentHeader = 13

	stream := pgsSubtitles(pgsFrame{width: blurayWidth, height: blurayHeight}, discSubtitleCues())

	var types []byte

	for offset := 0; offset < len(stream); {
		if offset+segmentHeader > len(stream) || string(stream[offset:offset+2]) != pgsMagic {
			t.Fatalf("no segment header at %d", offset)
		}

		types = append(types, stream[offset+10])
		offset += segmentHeader + int(binary.BigEndian.Uint16(stream[offset+11:]))
	}

	shown := []byte{pgsSegmentComposition, pgsSegmentWindow, pgsSegmentPalette, pgsSegmentObject, pgsSegmentEnd}
	cleared := []byte{pgsSegmentComposition, pgsSegmentWindow, pgsSegmentEnd}
	want := slices.Concat(shown, cleared, shown, cleared)

	if !bytes.Equal(types, want) {
		t.Errorf("segment types % x, want % x", types, want)
	}
}

func TestAssembleDiscSkipsWithoutSourcesAndHonoursTheFilter(t *testing.T) {
	t.Parallel()

	gen := &generator{outDir: t.TempDir(), progress: &strings.Builder{}}
	assembly := discAssemblies()[0]

	only, err := parseCaseFilter("library-movies-*")
	if err != nil {
		t.Fatal(err)
	}

	if entry := gen.assembleDisc(t.Context(), assembly, only); entry.Status != statusSkipped || entry.SkipKind != skipNotSelected {
		t.Errorf("unselected disc = %s/%s, want skipped as not selected", entry.Status, entry.SkipKind)
	}

	entry := gen.assembleDisc(t.Context(), assembly, caseFilter{})
	if entry.Status != statusSkipped || entry.SkipKind != skipMissingInput || !strings.Contains(entry.Reason, dvdMainSource) {
		t.Errorf("disc without sources = %+v, want skipped for the missing %s", entry, dvdMainSource)
	}

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()

	if entry := gen.assembleDisc(cancelled, assembly, caseFilter{}); entry.SkipKind != skipInterrupted {
		t.Errorf("interrupted disc = %+v, want skipped as interrupted", entry)
	}
}

func TestAssembleDiscRebuildsWithForceAndKeepsOtherwise(t *testing.T) {
	t.Parallel()

	gen := &generator{outDir: t.TempDir(), progress: &strings.Builder{}}
	assembly := discAssemblies()[0]

	for _, source := range assembly.sources {
		if err := writeFileAtomicallyInNewDirectory(gen.path(source), fakeProgramStream()); err != nil {
			t.Fatal(err)
		}
	}

	if entry := gen.assembleDisc(t.Context(), assembly, caseFilter{}); entry.Status != statusGenerated || entry.kept {
		t.Fatalf("first assembly = %+v, want built", entry)
	}

	if entry := gen.assembleDisc(t.Context(), assembly, caseFilter{}); !entry.kept {
		t.Errorf("second assembly = %+v, want kept", entry)
	}

	gen.force = true
	ifo := gen.path(path.Join(discsLibraryPath, path.Dir(assembly.disc.Path), videoTSFolder+"VTS_01_0.IFO"))

	if err := os.WriteFile(ifo, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}

	if entry := gen.assembleDisc(t.Context(), assembly, caseFilter{}); entry.Status != statusGenerated || entry.kept {
		t.Errorf("forced assembly = %+v, want rebuilt", entry)
	}

	if data, err := os.ReadFile(filepath.Clean(ifo)); err != nil || !bytes.HasPrefix(data, []byte(vtsMagic)) {
		t.Errorf("the forced run left the IFO as %q (%v)", data, err)
	}
}

// TestWriteDiscRefusesFilesTheCatalogDoesNotList checks the names before anything is
// removed or written, so a builder bug never leaves a half-replaced disc.
func TestWriteDiscRefusesFilesTheCatalogDoesNotList(t *testing.T) {
	t.Parallel()

	gen := &generator{outDir: t.TempDir()}
	assembly := discAssemblies()[0]
	assembly.build = func([][]byte) ([]discFile, error) {
		return []discFile{{name: "VIDEO_TS/EXTRA.VOB", data: []byte("x")}}, nil
	}

	if err := gen.writeDisc(assembly, nil); err == nil {
		t.Fatal("writeDisc accepted a file the catalog does not list")
	}

	if entries, err := os.ReadDir(gen.outDir); err != nil || len(entries) != 0 {
		t.Errorf("writeDisc left %d entries (%v), want none", len(entries), err)
	}
}

func TestDiscCatalogNamesMatchTheBuilders(t *testing.T) {
	t.Parallel()

	for _, assembly := range discAssemblies() {
		sources := fakeDVDSources()
		if assembly.disc.Layout == discLayoutBDMV {
			sources = fakeBluraySources()
		}

		var names []string

		for _, file := range mustBuild(t, assembly.build, sources) {
			names = append(names, path.Join(path.Dir(assembly.disc.Path), file.name))
		}

		slices.Sort(names)

		if !slices.Equal(names, assembly.disc.Files) {
			t.Errorf("%s writes %q, the catalog lists %q", assembly.name(), names, assembly.disc.Files)
		}
	}

	var summary strings.Builder

	writeDiscSummary(&summary, expectedDiscs{})

	if summary.Len() != 0 {
		t.Errorf("an empty disc library prints %q", summary.String())
	}
}

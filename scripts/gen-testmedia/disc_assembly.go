package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
)

// Disc folder contents, relative to the folder that holds VIDEO_TS or BDMV.
const (
	videoTSFolder   = "VIDEO_TS/"
	bdmvFolder      = "BDMV/"
	playlistFolder  = bdmvFolder + "PLAYLIST/"
	clipInfoFolder  = bdmvFolder + "CLIPINF/"
	streamFolder    = bdmvFolder + "STREAM/"
	aacsStandInFile = "AACS/STAND-IN.TXT"
	// aacsStandInText explains the folder to anyone who finds it; real AACS folders hold
	// key and certificate files, which the stand-in never imitates.
	aacsStandInText = "Generated test fixture. This folder only marks the disc as AACS protected for the\n" +
		"encryption check; it holds no keys and nothing on this disc is encrypted.\n"
	// isoVolumeID names the DVD image volume.
	isoVolumeID = "MOVIE"
	// discAssemblyPrefix starts the case names of assembled discs.
	discAssemblyPrefix = libraryCasePrefix + discsLibraryPath + "-"
)

var errMissingSource = errors.New("missing source")

// discAssembly writes one disc of the catalog from the staging outputs of source cases.
type discAssembly struct {
	disc    expectedDisc
	sources []string
	// build returns the disc files, named relative to the folder of disc.Path, from the
	// source contents in the order of sources.
	build func(sources [][]byte) ([]discFile, error)
}

// discAssemblies pairs every catalog disc with its sources and builder.
func discAssemblies() []discAssembly {
	dvdSources := []string{dvdMainSource, dvdBonusSource}
	bluraySources := []string{blurayPartOneSrc, blurayPartTwoSrc, blurayTrailerSrc}

	var assemblies []discAssembly

	for _, disc := range discCatalog().Items {
		assembly := discAssembly{disc: disc, sources: bluraySources, build: blurayTree}

		switch {
		case disc.Layout == discLayoutISO:
			assembly.sources, assembly.build = dvdSources, dvdImage(path.Base(disc.Path))
		case disc.Layout == discLayoutVideoTS && disc.Protection == protectionCSS:
			assembly.sources, assembly.build = dvdSources, scrambledDVDTree
		case disc.Layout == discLayoutVideoTS:
			assembly.sources, assembly.build = dvdSources, dvdTree
		case disc.Protection == protectionAACS:
			assembly.build = protectedBlurayTree
		}

		assemblies = append(assemblies, assembly)
	}

	return assemblies
}

// name is the case name of the assembled disc, e.g. "library-discs-movie-2019-video-ts".
func (assembly discAssembly) name() string {
	return discAssemblyPrefix + slug(fmt.Sprintf("%s %d %s", assembly.disc.Title, assembly.disc.Year, assembly.disc.Layout))
}

// asCase describes the assembly for the manifest: Output is the first disc file and Copies
// are the others, so the case lists every file it writes. The runner never builds it.
func (assembly discAssembly) asCase() mediaCase {
	files := make([]string, 0, len(assembly.disc.Files))
	for _, file := range assembly.disc.Files {
		files = append(files, path.Join(discsLibraryPath, file))
	}

	return mediaCase{Name: assembly.name(), Category: categoryDiscs, Output: files[0], Copies: files[1:]}
}

// discAssemblyCases returns the manifest cases of every assembly.
func discAssemblyCases() []mediaCase {
	var cases []mediaCase
	for _, assembly := range discAssemblies() {
		cases = append(cases, assembly.asCase())
	}

	return cases
}

// assembleDiscs writes every disc after the source cases ran. Like an FFmpeg case, a disc is
// kept when its files exist (unless -force), left alone when -only does not select it, and
// skipped when a source is missing.
func (g *generator) assembleDiscs(ctx context.Context, filter caseFilter) []manifestEntry {
	assemblies := discAssemblies()
	entries := make([]manifestEntry, 0, len(assemblies))

	for _, assembly := range assemblies {
		entry := g.assembleDisc(ctx, assembly, filter)
		g.report(entry)
		entries = append(entries, entry)
	}

	return entries
}

func (g *generator) assembleDisc(ctx context.Context, assembly discAssembly, filter caseFilter) manifestEntry {
	c := assembly.asCase()

	switch {
	case !filter.matches(c.Name):
		return g.keptOrSkippedDisc(c, skipNotSelected, fmt.Sprintf("not selected by -only %q", filter))
	case ctx.Err() != nil:
		return g.keptOrSkippedDisc(c, skipInterrupted, interruptedReason)
	case g.discComplete(c) && !g.force:
		return newKeptEntry(c, nil)
	}

	sources, err := g.readSources(assembly.sources)
	if err != nil {
		return g.keptOrSkippedDisc(c, skipMissingInput, err.Error())
	}

	if err := g.writeDisc(assembly, sources); err != nil {
		return newEntry(c, statusFailed, err.Error())
	}

	return newBuiltEntry(c)
}

// keptOrSkippedDisc lists a disc of an earlier run as kept and skips it otherwise.
func (g *generator) keptOrSkippedDisc(c mediaCase, kind skipKind, reason string) manifestEntry {
	if g.discComplete(c) {
		return newKeptEntry(c, nil)
	}

	return newSkippedEntry(c, kind, reason)
}

func (g *generator) discComplete(c mediaCase) bool {
	return !slices.ContainsFunc(c.files(), func(file string) bool { return !isNonEmptyFile(g.path(file)) })
}

func (g *generator) readSources(sources []string) ([][]byte, error) {
	contents := make([][]byte, 0, len(sources))

	for _, source := range sources {
		data, err := os.ReadFile(g.path(source))
		if err != nil || len(data) == 0 {
			return nil, fmt.Errorf("%w %s: the source case was not built", errMissingSource, source)
		}

		contents = append(contents, data)
	}

	return contents, nil
}

// writeDisc builds the disc and writes its files. The builder must produce exactly the
// catalog's files, so the manifest never lists a file that was not written. The old files are
// removed before the first new one is written: a run that stops halfway leaves an incomplete
// disc, which the next run rebuilds, never a mix of old and new files that counts as complete.
func (g *generator) writeDisc(assembly discAssembly, sources [][]byte) error {
	files, err := assembly.build(sources)
	if err != nil {
		return fmt.Errorf("assemble %s: %w", assembly.disc.Path, err)
	}

	folder := path.Join(discsLibraryPath, path.Dir(assembly.disc.Path))
	targets := make([]string, 0, len(files))

	for _, file := range files {
		targets = append(targets, path.Join(folder, file.name))
	}

	if want := assembly.asCase().files(); !slices.Equal(sortedCopy(targets), sortedCopy(want)) {
		return fmt.Errorf("assemble %s: builds %v, the catalog lists %v", assembly.disc.Path, targets, want)
	}

	for _, target := range targets {
		if err := os.Remove(g.path(target)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("assemble %s: remove the old %s: %w", assembly.disc.Path, target, err)
		}
	}

	for index, file := range files {
		if err := writeFileAtomicallyInNewDirectory(g.path(targets[index]), file.data); err != nil {
			return fmt.Errorf("assemble %s: %w", assembly.disc.Path, err)
		}
	}

	return nil
}

func sortedCopy(values []string) []string {
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	return sorted
}

// dvdFileNames are the files of a generated VIDEO_TS folder: the VMG and two title sets,
// each IFO with its BUP copy. Title set 1 is the feature in two VOBs, set 2 a bonus.
func dvdFileNames() []string {
	return []string{
		videoTSFolder + "VIDEO_TS.BUP", videoTSFolder + "VIDEO_TS.IFO",
		videoTSFolder + "VTS_01_0.BUP", videoTSFolder + "VTS_01_0.IFO",
		videoTSFolder + "VTS_01_1.VOB", videoTSFolder + "VTS_01_2.VOB",
		videoTSFolder + "VTS_02_0.BUP", videoTSFolder + "VTS_02_0.IFO", videoTSFolder + "VTS_02_1.VOB",
	}
}

// dvdTree writes the VIDEO_TS folder from the main and the bonus program streams.
func dvdTree(sources [][]byte) ([]discFile, error) {
	main, bonus := sources[0], sources[1]

	partOne, partTwo, err := splitVOB(main)
	if err != nil {
		return nil, err
	}

	vmg, mainIFO, bonusIFO := vmgIFO(), vtsIFO(dvdMainDuration), vtsIFO(dvdBonusDuration)

	return []discFile{
		{name: videoTSFolder + "VIDEO_TS.IFO", data: vmg},
		{name: videoTSFolder + "VIDEO_TS.BUP", data: vmg},
		{name: videoTSFolder + "VTS_01_0.IFO", data: mainIFO},
		{name: videoTSFolder + "VTS_01_0.BUP", data: mainIFO},
		{name: videoTSFolder + "VTS_01_1.VOB", data: partOne},
		{name: videoTSFolder + "VTS_01_2.VOB", data: partTwo},
		{name: videoTSFolder + "VTS_02_0.IFO", data: bonusIFO},
		{name: videoTSFolder + "VTS_02_0.BUP", data: bonusIFO},
		{name: videoTSFolder + "VTS_02_1.VOB", data: bonus},
	}, nil
}

// scrambledDVDTree is the DVD with CSS scrambling flags in every pack of the feature VOBs.
func scrambledDVDTree(sources [][]byte) ([]discFile, error) {
	files, err := dvdTree(sources)
	if err != nil {
		return nil, err
	}

	for index, file := range files {
		if !slices.Contains(dvdTitleClips(), path.Base(file.name)) {
			continue
		}

		if files[index].data, err = flagCSSSectors(file.data); err != nil {
			return nil, fmt.Errorf("%s: %w", file.name, err)
		}
	}

	return files, nil
}

// dvdImage packs the DVD folder into an ISO 9660 image named imageName.
func dvdImage(imageName string) func([][]byte) ([]discFile, error) {
	return func(sources [][]byte) ([]discFile, error) {
		files, err := dvdTree(sources)
		if err != nil {
			return nil, err
		}

		image, err := isoImage(isoVolumeID, files)
		if err != nil {
			return nil, err
		}

		return []discFile{{name: imageName, data: image}}, nil
	}
}

// blurayFileNames are the files of a generated BDMV folder.
func blurayFileNames() []string {
	names := []string{bdmvFolder + "index.bdmv", bdmvFolder + "MovieObject.bdmv"}
	for _, playlist := range blurayPlaylistNames() {
		names = append(names, playlistFolder+playlist+".mpls")
	}

	for _, clip := range []string{blurayClipPartOne, blurayClipPartTwo, blurayClipTrailer} {
		names = append(names, clipInfoFolder+clip+".clpi", streamFolder+clip+".m2ts")
	}

	return names
}

// blurayPlaylistNames are the playlists in the order blurayPlaylists returns them.
func blurayPlaylistNames() []string { return []string{"00000", "00001", "00002", "00003"} }

// blurayPlaylists are a menu loop that repeats the trailer and is longer than the feature,
// the feature, the trailer alone and a long playlist whose clip is missing. Title selection
// must skip the loop and the broken playlist and pick 00001.
func blurayPlaylists() [][]byte {
	trailer := playlistItem{clip: blurayClipTrailer, length: blurayTrailer}

	return [][]byte{
		moviePlaylist(slices.Repeat([]playlistItem{trailer}, blurayDecoyRepeats)...),
		moviePlaylist(
			playlistItem{clip: blurayClipPartOne, length: blurayPartOne},
			playlistItem{clip: blurayClipPartTwo, length: blurayPartTwo},
		),
		moviePlaylist(trailer),
		moviePlaylist(playlistItem{clip: blurayClipMissing, length: blurayMissingLength}),
	}
}

// blurayTree writes the BDMV folder from the clips 00001, 00002 and 00003.
func blurayTree(sources [][]byte) ([]discFile, error) {
	files := []discFile{
		{name: bdmvFolder + "index.bdmv", data: bdmvStub(indexMagic)},
		{name: bdmvFolder + "MovieObject.bdmv", data: bdmvStub(movieObjectMagic)},
	}

	for index, playlist := range blurayPlaylists() {
		files = append(files, discFile{name: playlistFolder + blurayPlaylistNames()[index] + ".mpls", data: playlist})
	}

	for index, clip := range []string{blurayClipPartOne, blurayClipPartTwo, blurayClipTrailer} {
		if !isSourcePacketStream(sources[index]) {
			return nil, fmt.Errorf("clip %s: %w", clip, errNoSourcePackets)
		}

		files = append(files,
			discFile{name: clipInfoFolder + clip + ".clpi", data: bdmvStub(clipInfoMagic)},
			discFile{name: streamFolder + clip + ".m2ts", data: sources[index]},
		)
	}

	return files, nil
}

// protectedBlurayTree is the Blu-ray with an AACS folder and AACS marks in the feature clips.
func protectedBlurayTree(sources [][]byte) ([]discFile, error) {
	files, err := blurayTree(sources)
	if err != nil {
		return nil, err
	}

	for index, file := range files {
		if !slices.ContainsFunc(blurayTitleClips(), func(clip string) bool { return strings.HasSuffix(file.name, clip) }) {
			continue
		}

		if files[index].data, err = markAACSUnits(file.data); err != nil {
			return nil, fmt.Errorf("%s: %w", file.name, err)
		}
	}

	return append(files, discFile{name: aacsStandInFile, data: []byte(aacsStandInText)}), nil
}

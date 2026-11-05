package main

import (
	"context"
	"fmt"
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

// discAssembly writes one disc of the catalog from the staging outputs of source cases.
type discAssembly struct {
	disc    expectedDisc
	sources []string
	// build returns the disc files, named relative to the folder of disc.Path, from the
	// source contents in the order of sources.
	build func(sources [][]byte) ([]assembledFile, error)
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

// generic is the disc as a file assembly whose files lie in the folder of disc.Path.
func (assembly discAssembly) generic() fileAssembly {
	folder := path.Dir(assembly.disc.Path)
	files := make([]string, 0, len(assembly.disc.Files))

	for _, file := range assembly.disc.Files {
		files = append(files, strings.TrimPrefix(file, folder+"/"))
	}

	return fileAssembly{
		name: assembly.name(), category: categoryDiscs, label: assembly.disc.Path,
		folder: path.Join(discsLibraryPath, folder), files: files,
		sources: assembly.sources, build: assembly.build,
	}
}

// discAssemblyCases returns the manifest cases of every assembly.
func discAssemblyCases() []mediaCase {
	var cases []mediaCase
	for _, assembly := range discAssemblies() {
		cases = append(cases, assembly.generic().asCase())
	}

	return cases
}

// assembleDiscs writes every disc after the source cases ran.
func (g *generator) assembleDiscs(ctx context.Context, filter caseFilter) []manifestEntry {
	assemblies := discAssemblies()
	generic := make([]fileAssembly, 0, len(assemblies))

	for _, assembly := range assemblies {
		generic = append(generic, assembly.generic())
	}

	return g.assembleAll(ctx, generic, filter)
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
func dvdTree(sources [][]byte) ([]assembledFile, error) {
	main, bonus := sources[0], sources[1]

	partOne, partTwo, err := splitVOB(main)
	if err != nil {
		return nil, err
	}

	vmg, mainIFO, bonusIFO := vmgIFO(), vtsIFO(dvdMainDuration), vtsIFO(dvdBonusDuration)

	return []assembledFile{
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
func scrambledDVDTree(sources [][]byte) ([]assembledFile, error) {
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
func dvdImage(imageName string) func([][]byte) ([]assembledFile, error) {
	return func(sources [][]byte) ([]assembledFile, error) {
		files, err := dvdTree(sources)
		if err != nil {
			return nil, err
		}

		image, err := isoImage(isoVolumeID, files)
		if err != nil {
			return nil, err
		}

		return []assembledFile{{name: imageName, data: image}}, nil
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
func blurayTree(sources [][]byte) ([]assembledFile, error) {
	files := []assembledFile{
		{name: bdmvFolder + "index.bdmv", data: bdmvStub(indexMagic)},
		{name: bdmvFolder + "MovieObject.bdmv", data: bdmvStub(movieObjectMagic)},
	}

	for index, playlist := range blurayPlaylists() {
		files = append(files, assembledFile{name: playlistFolder + blurayPlaylistNames()[index] + ".mpls", data: playlist})
	}

	for index, clip := range []string{blurayClipPartOne, blurayClipPartTwo, blurayClipTrailer} {
		if !isSourcePacketStream(sources[index]) {
			return nil, fmt.Errorf("clip %s: %w", clip, errNoSourcePackets)
		}

		files = append(files,
			assembledFile{name: clipInfoFolder + clip + ".clpi", data: bdmvStub(clipInfoMagic)},
			assembledFile{name: streamFolder + clip + ".m2ts", data: sources[index]},
		)
	}

	return files, nil
}

// protectedBlurayTree is the Blu-ray with an AACS folder and AACS marks in the feature clips.
func protectedBlurayTree(sources [][]byte) ([]assembledFile, error) {
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

	return append(files, assembledFile{name: aacsStandInFile, data: []byte(aacsStandInText)}), nil
}

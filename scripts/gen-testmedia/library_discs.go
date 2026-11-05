package main

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"
)

// The disc corpus (task M3-16i, ADR 0029): licence-free DVD and Blu-ray fixtures in a library
// root of their own. FFmpeg encodes the streams into a staging folder outside every library
// root; the generator then writes the disc structures (IFO, MPLS, index files, ISO 9660)
// around them in Go. Protected discs are stand-ins that only carry the flags the encryption
// check reads; nothing is encrypted and there are no keys.
const (
	categoryDiscs = "discs"
	// discsLibraryPath is the disc library root below the -library directory.
	discsLibraryPath = "discs"
	// discSourcesPath holds the FFmpeg outputs the discs are assembled from; it lies outside
	// every library root, so a scan never lists these files.
	discSourcesPath = ".disc-sources"

	// Layouts and the error code as internal/domain/media names them.
	discLayoutVideoTS  = "video_ts"
	discLayoutBDMV     = "bdmv"
	discLayoutISO      = "iso"
	discErrorEncrypted = "disc_encrypted"
	// Protection stand-ins: CSS sector flags, AACS folder and unit marks.
	protectionCSS  = "css"
	protectionAACS = "aacs"

	// DVD sources: a PAL title that is split into two VOBs, and a short bonus title set.
	dvdWidth         = 720
	dvdHeight        = 576
	dvdFrameRate     = 25
	dvdMainDuration  = 6 * time.Second
	dvdBonusDuration = 2 * time.Second
	// dvdGOPLength is the PAL DVD GOP of 12 frames.
	dvdGOPLength = "12"
	// Blu-ray sources: two clips of the main playlist and a trailer clip.
	blurayWidth          = 1280
	blurayHeight         = 720
	blurayFrameRate      = 24
	blurayPartOne        = 4 * time.Second
	blurayPartTwo        = 3 * time.Second
	blurayTrailer        = 2 * time.Second
	blurayGOPLength      = "24"
	videoBitrate         = "1500k"
	blurayClipPartOne    = "00001"
	blurayClipPartTwo    = "00002"
	blurayClipTrailer    = "00003"
	blurayClipMissing    = "00009"
	blurayDecoyRepeats   = 4
	blurayMissingLength  = time.Minute
	subtitleFileName     = "subtitles.sup"
	subtitleDemuxer      = "sup"
	discVideoCodecDVD    = "mpeg2video"
	discVideoCodecBluray = "h264"
	discAudioCodec       = "ac3"
	discSubtitleDVD      = "dvd_subtitle"
	discSubtitleBluray   = "hdmv_pgs_subtitle"
)

// Staging outputs of the source cases, relative to the -library directory.
const (
	dvdMainSource    = discSourcesPath + "/dvd-main.vob"
	dvdBonusSource   = discSourcesPath + "/dvd-bonus.vob"
	blurayPartOneSrc = discSourcesPath + "/bluray-" + blurayClipPartOne + ".m2ts"
	blurayPartTwoSrc = discSourcesPath + "/bluray-" + blurayClipPartTwo + ".m2ts"
	blurayTrailerSrc = discSourcesPath + "/bluray-" + blurayClipTrailer + ".m2ts"
)

// The subtitle cues end before the shortest subtitled clip (3 s) does.
const (
	firstCueStart  = 500 * time.Millisecond
	firstCueEnd    = 1500 * time.Millisecond
	secondCueStart = 2000 * time.Millisecond
	secondCueEnd   = 2800 * time.Millisecond
)

// Invented years of the disc titles.
const (
	movieYear            = 2019
	featureYear          = 2021
	imageMovieYear       = 2020
	scrambledMovieYear   = 2018
	protectedFeatureYear = 2022
)

func discSubtitleCues() []bitmapCue {
	return []bitmapCue{{start: firstCueStart, end: firstCueEnd}, {start: secondCueStart, end: secondCueEnd}}
}

// expectedDiscs is the disc library root below the -library directory.
type expectedDiscs struct {
	// Path is the library root relative to the -library directory; disc paths are relative
	// to it.
	Path  string         `json:"path"`
	Items []expectedDisc `json:"items"`
}

// expectedDisc is one disc a scan must detect and what resolving its main title gives.
type expectedDisc struct {
	Title string `json:"title"`
	Year  int    `json:"year"`
	// Path is the disc as the scan reports it: the VIDEO_TS or BDMV folder, or the image.
	Path string `json:"path"`
	// Layout is "video_ts", "bdmv" or "iso".
	Layout    string            `json:"layout"`
	MainTitle expectedDiscTitle `json:"mainTitle"`
	// ErrorCode is the disc error code resolving the disc fails with; empty when it plays.
	ErrorCode string `json:"errorCode,omitempty"`
	// Protection names the stand-in flags of a protected disc: "css" or "aacs".
	Protection string `json:"protection,omitempty"`
	// Streams is what probing the main title reports; nil for a disc that fails.
	Streams *expectedDiscStreams `json:"streams,omitempty"`
	// Files are every file of the disc, relative to the library root.
	Files []string `json:"files"`
}

// expectedDiscTitle is the main title selection.
type expectedDiscTitle struct {
	// Clips are relative to Path for a folder disc and to the image root for an image.
	Clips []string `json:"clips"`
	// DurationSeconds is the length the IFO or MPLS gives the title.
	DurationSeconds float64 `json:"durationSeconds"`
}

// expectedDiscStreams are the codecs (ffprobe codec_name) and frame size of the main title.
type expectedDiscStreams struct {
	Video     string   `json:"video"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	Audio     []string `json:"audio"`
	Subtitles []string `json:"subtitles"`
}

// dvdTitleClips and blurayTitleClips are the main title clips of the generated discs.
func dvdTitleClips() []string { return []string{"VTS_01_1.VOB", "VTS_01_2.VOB"} }

func blurayTitleClips() []string {
	return []string{"STREAM/" + blurayClipPartOne + ".m2ts", "STREAM/" + blurayClipPartTwo + ".m2ts"}
}

func dvdStreams() *expectedDiscStreams {
	return &expectedDiscStreams{
		Video: discVideoCodecDVD, Width: dvdWidth, Height: dvdHeight,
		Audio: []string{discAudioCodec}, Subtitles: []string{discSubtitleDVD},
	}
}

func blurayStreams() *expectedDiscStreams {
	return &expectedDiscStreams{
		Video: discVideoCodecBluray, Width: blurayWidth, Height: blurayHeight,
		Audio: []string{discAudioCodec}, Subtitles: []string{discSubtitleBluray},
	}
}

// discCatalog is the expected disc library: a clear DVD folder, a clear Blu-ray folder, an
// ISO image of the DVD, and a CSS and an AACS stand-in. Titles are invented.
func discCatalog() expectedDiscs {
	dvdMain := expectedDiscTitle{Clips: dvdTitleClips(), DurationSeconds: dvdMainDuration.Seconds()}
	blurayMain := expectedDiscTitle{Clips: blurayTitleClips(), DurationSeconds: (blurayPartOne + blurayPartTwo).Seconds()}
	imageMain := expectedDiscTitle{DurationSeconds: dvdMainDuration.Seconds()}

	for _, clip := range dvdTitleClips() {
		imageMain.Clips = append(imageMain.Clips, "VIDEO_TS/"+clip)
	}

	items := []expectedDisc{
		{
			Title: "Movie", Year: movieYear, Path: "Movie (2019)/VIDEO_TS", Layout: discLayoutVideoTS,
			MainTitle: dvdMain, Streams: dvdStreams(),
		},
		{
			Title: "Feature", Year: featureYear, Path: "Feature (2021)/BDMV", Layout: discLayoutBDMV,
			MainTitle: blurayMain, Streams: blurayStreams(),
		},
		{
			Title: "Image Movie", Year: imageMovieYear, Path: "Image Movie (2020)/Image Movie (2020).iso", Layout: discLayoutISO,
			MainTitle: imageMain, Streams: dvdStreams(),
		},
		{
			Title: "Scrambled Movie", Year: scrambledMovieYear, Path: "Scrambled Movie (2018)/VIDEO_TS", Layout: discLayoutVideoTS,
			MainTitle: dvdMain, ErrorCode: discErrorEncrypted, Protection: protectionCSS,
		},
		{
			Title: "Protected Feature", Year: protectedFeatureYear, Path: "Protected Feature (2022)/BDMV", Layout: discLayoutBDMV,
			MainTitle: blurayMain, ErrorCode: discErrorEncrypted, Protection: protectionAACS,
		},
	}

	for index := range items {
		items[index].Files = discFileNames(items[index])
	}

	return expectedDiscs{Path: discsLibraryPath, Items: items}
}

// discFileNames lists the files the assembly writes for a disc, relative to the library root.
func discFileNames(disc expectedDisc) []string {
	folder := path.Dir(disc.Path)

	var names []string

	switch {
	case disc.Layout == discLayoutISO:
		names = []string{path.Base(disc.Path)}
	case disc.Layout == discLayoutVideoTS:
		names = dvdFileNames()
	case disc.Protection == protectionAACS:
		names = append(blurayFileNames(), aacsStandInFile)
	default:
		names = blurayFileNames()
	}

	files := make([]string, 0, len(names))
	for _, name := range names {
		files = append(files, path.Join(folder, name))
	}

	slices.Sort(files)

	return files
}

// discSourceCases are the FFmpeg runs whose outputs the discs are assembled from.
func discSourceCases() []mediaCase {
	return []mediaCase{
		dvdSourceCase("main", dvdMainSource, dvdMainDuration, true),
		dvdSourceCase("bonus", dvdBonusSource, dvdBonusDuration, false),
		bluraySourceCase(blurayClipPartOne, blurayPartOneSrc, blurayPartOne, true),
		bluraySourceCase(blurayClipPartTwo, blurayPartTwoSrc, blurayPartTwo, true),
		bluraySourceCase(blurayClipTrailer, blurayTrailerSrc, blurayTrailer, false),
	}
}

// dvdSourceCase is an MPEG-2/AC-3 program stream from the dvd muxer (2048-byte packs with
// navigation packets); with subtitles it carries the PGS cues re-encoded as dvd_subtitle.
func dvdSourceCase(name, output string, duration time.Duration, subtitles bool) mediaCase {
	c := mediaCase{
		Name: discSourceCaseName("dvd-" + name), Category: categoryDiscs, Output: output,
		Source: sourceSpec{
			Width: dvdWidth, Height: dvdHeight, FrameRate: dvdFrameRate,
			SampleRate: defaultSampleRate, Channels: stereoChannels, Duration: duration,
		},
		Muxer: "dvd", Encoders: []string{discVideoCodecDVD, discAudioCodec},
		Args: slices.Concat(simpleVideoArgs(discVideoCodecDVD), []string{"-g", dvdGOPLength, "-b:v", videoBitrate},
			audioArgs(discAudioCodec)),
		Expect: expect("vob", probeMPEGPS, mpeg2Video(), audioOf(discAudioCodec)),
	}

	if subtitles {
		c.Inputs = []textInput{subtitleInput(pgsFrame{width: dvdWidth, height: dvdHeight})}
		c.Encoders = append(c.Encoders, "dvdsub")
		c.Args = append(c.Args, "-c:s", "dvdsub")
		c.Expect.Subtitles = []subtitleExpectation{{Codec: discSubtitleDVD}}
	}

	return c
}

// bluraySourceCase is an H.264/AC-3 clip in 192-byte source packets (m2ts mode); with
// subtitles it carries the PGS cues as they are.
func bluraySourceCase(clip, output string, duration time.Duration, subtitles bool) mediaCase {
	c := mediaCase{
		Name: discSourceCaseName("bluray-" + clip), Category: categoryDiscs, Output: output,
		Source: sourceSpec{
			Width: blurayWidth, Height: blurayHeight, FrameRate: blurayFrameRate,
			SampleRate: defaultSampleRate, Channels: stereoChannels, Duration: duration,
		},
		Muxer: "mpegts", Encoders: []string{"libx264", discAudioCodec},
		Args: slices.Concat(x264Args(), []string{"-g", blurayGOPLength, "-b:v", videoBitrate},
			audioArgs(discAudioCodec), []string{"-mpegts_m2ts_mode", "1"}),
		Expect: expect("m2ts", probeMPEGTS, h264Video(), audioOf(discAudioCodec)),
	}

	if subtitles {
		c.Inputs = []textInput{subtitleInput(pgsFrame{width: blurayWidth, height: blurayHeight})}
		c.Args = append(c.Args, "-c:s", "copy")
		c.Expect.Subtitles = []subtitleExpectation{{Codec: discSubtitleBluray}}
	}

	return c
}

// subtitleInput is the generated PGS stream; textInput carries bytes, not only text.
func subtitleInput(frame pgsFrame) textInput {
	return textInput{
		FileName: subtitleFileName, Format: subtitleDemuxer, Role: roleSubtitle,
		Content: string(pgsSubtitles(frame, discSubtitleCues())),
	}
}

func discSourceCaseName(name string) string {
	return libraryCasePrefix + discsLibraryPath + "-source-" + name
}

// writeDiscSummary adds one line per disc, e.g. "bdmv  Feature (2021): 7 s main title".
func writeDiscSummary(text *strings.Builder, discs expectedDiscs) {
	if len(discs.Items) == 0 {
		return
	}

	fmt.Fprintf(text, "  %s/ (%d discs)\n", discs.Path, len(discs.Items))

	for _, disc := range discs.Items {
		details := fmt.Sprintf("%g s main title", disc.MainTitle.DurationSeconds)
		if disc.ErrorCode != "" {
			details = fmt.Sprintf("%s stand-in, %s", disc.Protection, disc.ErrorCode)
		}

		fmt.Fprintf(text, "    %-8s %s (%d): %s\n", disc.Layout, disc.Title, disc.Year, details)
	}
}

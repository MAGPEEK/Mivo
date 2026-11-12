// Command gen-testmedia generates the rights-free test media corpus (ADR 0021).
//
// It builds short video clips, audio files, images and subtitle files (sidecar, embedded and
// multi-track) from FFmpeg lavfi sources and text it writes itself, in every format the local
// FFmpeg can produce. It skips cases it cannot build and writes <out>/manifest.json, which
// lists every case with its status, expected properties and the known gaps FFmpeg cannot
// produce at all. Run it from the repository root:
//
//	go run ./scripts/gen-testmedia [-out testdata/media] [-library testdata/library]
//	                               [-ffmpeg ffmpeg] [-force] [-only <glob>[,<glob>...]] [-jobs N]
//
// # Feature tags and the milestone-4 inputs
//
// A manifest entry may carry "tags" (task M4-03) that name what the case is an input for, so
// tests select by tag instead of by file name: "textsub", "styledsub", "imagesub",
// "multiaudio", "commentary", "surround", "hires", "dsd", "tonemap" and "long". Known gaps
// are tagged as well (both "dsd" cases and the raw PGS, VobSub and DVB cases are gaps), so
// check the status of an entry. The track of a stream may carry a "title".
//
// The categories "tracks" and "hdr" hold the clips M4-03 added: Matroska files with a styled
// ASS track before plain SRT, with English stereo, German 5.1 and an English "Commentary"
// audio track, with a PGS and with a dvd_subtitle track, and a 20 s HDR10 clip. The bitmap
// subtitles come from the PGS stream the generator writes itself (see the disc corpus
// below). The playback matrix of ADR 0025 (internal/adapters/corpustest) covers the
// categories "video", "subtitle" and "multitrack" only; the tasks that implement subtitle
// and track selection add these clips to it.
//
// # Library tree
//
// Unless -library is "", it also writes a library tree for the milestone-3 scan (task
// M3-17, ADR 0027): <library>/movies and <library>/shows with folder and loose movies,
// versions, editions, parts, extras, samples, shows with seasons, specials, multi-episode
// files, absolutely numbered episodes (UX DT-24: "Dragon Ball Super - 001" … "- 131"), Kodi
// NFO files (UTF-8, ISO-8859-1, URL-only, malformed) and local and embedded artwork. Names of
// real movies and shows are only file names; all content is generated. Every video is the
// same 2 s H.264/AAC clip; files of one item share one FFmpeg run and are byte copies.
// <library>/music (task M3-13f) holds a tagged two-disc FLAC album with embedded covers, a
// tagged MP3 album, an M4A album with cover.jpg in its folder, a "Various Artists"
// compilation and an untagged FLAC album; every track is a 2 s sine tone with its own tags,
// all names are invented. <library>/discs (task M3-16i, ADR 0029) holds a clear DVD
// ("Movie (2019)/VIDEO_TS": a 6 s PAL feature split into VTS_01_1.VOB and VTS_01_2.VOB plus
// a 2 s bonus title set, IFO and BUP files), a clear Blu-ray ("Feature (2021)/BDMV": the
// 4 s + 3 s feature playlist, a longer menu loop that repeats the trailer clip and a
// playlist whose clip is missing), an ISO 9660 image of the DVD and two protected stand-ins:
// a DVD whose feature packs carry the CSS PES_scrambling_control flag and a Blu-ray with an
// AACS folder whose feature clips carry copy_permission_indicator and lost sync bytes in
// their first aligned units. The stand-ins hold only these flags; nothing is encrypted and
// there are no keys. FFmpeg encodes the streams into <library>/.disc-sources (outside every
// library root); the generator writes the IFO, MPLS, index and image structures around
// them. The feature titles carry real bitmap subtitles: FFmpeg cannot turn text into bitmap
// subtitles and has no subtitle source, so the generator writes a raw PGS stream (one
// rectangle per cue) that the Blu-ray clips copy as hdmv_pgs_subtitle and the DVD re-encodes
// with dvdsub (bitmap to bitmap) as dvd_subtitle. Library case names start with "library-",
// so -only 'library-*' builds the tree alone; disc cases start with "library-discs-".
// <library>/audiobooks (task M3-14f) holds an M4B with three chapters and an embedded cover
// ("Ada Inkwell/The Clockwork Orchard.m4b"), a folder book of three MP3 parts on two discs
// whose disc and track tags contradict the file-name order and whose cover is cover.jpg,
// and a shared folder with a single MP3 book (artist tag only, no narrator, no cover) next
// to a chapterless M4B whose cover is the image named after it. The MP3 parts of the folder
// book carry an explicit narrator (ID3 TXXX "narrator"); the M4B files carry only a
// composer: the chaptered one labels it as the narrator ("Narrated by …", M3-14g), the
// chapterless one does not, so it expects none. All names are invented. Audiobook cases
// start with "library-audiobooks-".
// <library>/photos (task M3-15g, ADR 0029) holds photos with EXIF written by the generator
// (FFmpeg writes none): JPEGs with camera, lens, exposure, capture time and the Eiffel Tower
// as the only GPS position, the nested album "Holidays 2024/Lake Shore" with a portrait
// (orientation 6), a RAW+JPEG pair (a DNG stand-in with a JPEG preview) and a short MP4, a
// lone DNG (orientation 8) and a RAW name over unreadable bytes, PNG, WebP, GIF, an RGB TIFF,
// a JPEG without EXIF and a corrupt one (both with a fixed modification time), one JPEG per
// orientation 1-8, and, when FFmpeg has the encoders, a HEIC picture, a 2x2 HEIC grid (Go
// wraps libx265 streams from <library>/.photo-sources in HEIF) and an AVIF. Cameras and
// names are invented. Photo cases start with "library-photos-"; "library-photos-source-*" are
// the HEVC runs.
// <library>/subs (task M4-03) holds "Movie (2020).mkv" with external subtitles for the
// sidecar discovery: ".en.srt", ".de.forced.srt" and ".fr.sdh.ass" next to it and
// "Subs/Movie (2020).es.srt". Its case is "library-subs-movie-2020".
//
// <library>/manifest.json describes the expected catalog (what a scan must produce) and the
// build result. Paths are relative to the library root (Path of the library) unless noted,
// use forward slashes and keep the raw file-system form (one folder name is Unicode NFD);
// titles and NFO text are NFC. Schema version 1:
//
//	schemaVersion  1
//	ffmpegVersion  version of the FFmpeg that ran
//	libraries[]    kind ("movies"|"shows"), path (below <library>), items[]:
//	  kind           "movie"|"show"
//	  title, year    expected catalog values after the merge (NFO before file name); year
//	                 is omitted when there is none
//	  path           movie or show folder, file of a loose movie, "" for a show whose files
//	                 lie directly in the library root
//	  versions[]     movies: label (file-name suffix or edition, "" for none), parts[]
//	                 (files in part order)
//	  extras[]       type ("trailer"|"featurette"), file
//	  ignored[]      files the scan must not list (samples)
//	  nfo            file, format ("xml"|"url"|"malformed"), encoding ("UTF-8"|
//	                 "ISO-8859-1", XML only), fields: title, originalTitle, year, plot,
//	                 aired, season, episode, genres[], providerIds{tmdb,imdb,tvdb};
//	                 fields is empty for a malformed file
//	  artwork[]      kind ("poster"|"backdrop"|"thumb"|"logo"), source ("local"|
//	                 "embedded"), file (the image, or the video embedding it)
//	  seasons[]      shows, ascending number (0 = specials): number, path ("" without a
//	                 season folder), artwork[], episodes[]: number, numberEnd (multi-episode
//	                 file), absoluteNumber (absolutely numbered files), files[], nfo, artwork[]
//	music          path (below <library>), albums[] (added in M3-13f without a version bump;
//	               kept out of libraries[], whose consumers compare movie and show items):
//	  albumArtist, title, year, compilation, path (album folder), container
//	                 ("flac"|"mp3"|"m4a")
//	  tracks[]       file, artist, album, disc, discTotal, track, trackTotal, title, tagged
//	                 (false: no tags, a scan reads the layout "Artist/Album/[CD n/]NN Title"),
//	                 coverSource ("embedded"|"folder"|"none"), coverFile (folder image, or
//	                 the track itself when embedded)
//	discs          path (below <library>), items[] (added in M3-16i without a version bump,
//	               kept out of libraries[] like music):
//	  title, year    invented title and year of the folder that holds the disc
//	  path           the disc as a scan reports it: the VIDEO_TS or BDMV folder, or the image
//	  layout         "video_ts"|"bdmv"|"iso"
//	  mainTitle      clips[] (relative to path; inside the image for "iso") and
//	                 durationSeconds (from the IFO or MPLS) of the selected main title
//	  errorCode      "disc_encrypted" for the stand-ins, omitted for discs that play
//	  protection     "css"|"aacs" for the stand-ins
//	  streams        probe of the main title: video, width, height, audio[], subtitles[]
//	                 (ffprobe codec names); omitted for the stand-ins
//	  files[]        every file of the disc, relative to the library root
//	audiobooks     path (below <library>), books[] (added in M3-14f without a version bump,
//	               kept out of libraries[] like music), in catalog order:
//	  title, author  expected book title (album tag) and author
//	  authorTag      "album_artist" (with an equal artist tag) or "artist"
//	  composer       raw composer tag of the parts, omitted without one
//	  narrator       narrator a scan must produce (explicit narrator tag, ID3 TXXX, or a
//	                 composer starting "Narrated by "); an unlabelled composer is none;
//	                 omitted without one
//	  path           book folder; standsAlone marks a one-file book sharing it (M4B)
//	  container      "m4b"|"mp3"
//	  coverSource    "embedded"|"folder" (image file)|"none"; coverFile (image, or the part)
//	  parts[]        play order: file, title, disc, discTotal, track, trackTotal (omitted
//	                 without tags), durationSeconds (nominal; encoders pad a little)
//	  chapters[]     play order: file, title, startSeconds, endSeconds (relative to the
//	                 file), embedded (false: the whole-file chapter a scan derives; its end
//	                 is the probed length)
//	photos         path (below <library>), photos[] (added in M3-15g without a version bump,
//	               kept out of libraries[] like music), one per file in catalog order:
//	  file, album    file and its folder (the album a scan creates), relative to the root
//	  format         "jpeg"|"png"|"webp"|"gif"|"tiff"|"dng"|"nef"|"heic"|"avif"|"mp4"
//	  alternateOf    primary file of a RAW+JPEG pair (the RAW is filed under it)
//	  optional       HEIC and AVIF: only built when FFmpeg has the encoder
//	  metadata       photometa ReadInfo outcome: "read"|"unsupported"|"corrupt"
//	  exif           the Info it returns: takenAt (wall clock "2006-01-02T15:04:05", no
//	                 zone), make, model, lens, exposureSeconds, fNumber, focalLengthMM, iso,
//	                 orientation, gps{latitude, longitude}, width, height; zero values and a
//	                 zero Info are omitted
//	  takenAtSource  "exif"|"file" (the modification time stands in)
//	  modifiedAt     modification time the generator sets (RFC 3339 UTC), omitted for
//	                 FFmpeg outputs
//	  thumbnailState "ready"|"unsupported" as a scan stores it
//	  renders        whether the photo renderer produces a thumbnail; displayWidth and
//	                 displayHeight are the upright size of a still (orientations 5-8 swap)
//	subtitles      path (below <library>), movies[] (added in M4-03 without a version bump,
//	               kept out of libraries[] like music):
//	  title, year    expected catalog values of the loose movie
//	  file           the movie file, a plain clip without embedded subtitles
//	  sidecars[]     file (next to the movie or in "Subs/"), language (the ISO 639-1 code of
//	                 the file name), forced (".forced"), hearingImpaired (".sdh"), format
//	                 ("srt"|"ass")
//	cases[]        name, category ("artwork"|"movies"|"shows"|"music"|"audiobooks"|"discs"|
//	               "photos"|"subs"), status
//	               ("generated"|"skipped"|"failed"), skipKind, reason, files[] (relative to
//	               <library>); "library-discs-source-*" cases are the FFmpeg runs, the other
//	               "library-discs-*" cases the assembled discs
//
// The catalog is static; consumers check that every case is generated (or skip with the
// reason) before they compare a scan with it.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
)

const (
	exitOK = 0
	// exitFailure signals that a case failed or FFmpeg could not be used.
	exitFailure = 1
	// exitUsage matches the exit code of the flag package for invalid command lines.
	exitUsage = 2

	defaultOutDir = "testdata/media"
	// defaultLibraryDir stays apart from defaultOutDir: tests and the smoke script use
	// testdata/media as one movies library and would change their counts otherwise.
	defaultLibraryDir = "testdata/library"
	defaultFFmpeg     = "ffmpeg"
)

// errCasesFailed reports that at least one buildable case failed; details are in the manifest.
var errCasesFailed = errors.New("at least one case failed, see the manifest for reasons")

// errInterrupted reports a run stopped by Ctrl+C or SIGTERM; the manifest lists what was done.
var errInterrupted = errors.New("generation interrupted, run again to finish the corpus")

func main() {
	ctx, stop := interruptContext(context.Background())
	code := run(ctx, os.Args[1:], os.Stdout, os.Stderr)

	stop()
	os.Exit(code)
}

// interruptContext returns a context that the first Ctrl+C or SIGTERM cancels. The run then
// stops its FFmpeg processes and still writes the manifest. Once the context is done, the
// signal handlers are released, so a second Ctrl+C ends the program at once with the default
// behaviour instead of being swallowed while the run winds down.
func interruptContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
	context.AfterFunc(ctx, stop)

	return ctx, stop
}

// options holds the parsed command line.
type options struct {
	outDir string
	// libraryDir receives the library tree; "" skips it.
	libraryDir string
	ffmpeg     string
	force      bool
	only       caseFilter
	jobs       int
}

// run executes the command with the given arguments and returns the process exit code.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	opts, err := parseOptions(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return exitOK
	}

	if err != nil {
		return exitUsage
	}

	if err := generateCorpus(ctx, opts, execRunner{executable: opts.ffmpeg}, stdout); err != nil {
		// A failed write to stderr cannot be reported anywhere else.
		_, _ = fmt.Fprintf(stderr, "gen-testmedia: %v\n", err)

		return exitFailure
	}

	return exitOK
}

func parseOptions(args []string, stderr io.Writer) (options, error) {
	opts := options{}

	var only string

	flags := flag.NewFlagSet("gen-testmedia", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&opts.outDir, "out", defaultOutDir, "output directory for media files and manifest.json")
	flags.StringVar(&opts.libraryDir, "library", defaultLibraryDir,
		"output directory for the library tree (movies/, shows/) and its manifest.json; \"\" skips it")
	flags.StringVar(&opts.ffmpeg, "ffmpeg", defaultFFmpeg, "ffmpeg executable (name on PATH or path)")
	flags.BoolVar(&opts.force, "force", false, "regenerate files that already exist")
	flags.StringVar(&only, "only", "",
		"only build cases whose name matches one of these comma-separated globs,\n"+
			"e.g. 'video-mkv-*' or 'video-mp4-*,video-webm-vp9-opus'")
	flags.IntVar(&opts.jobs, "jobs", runtime.NumCPU(), "number of parallel ffmpeg processes (minimum 1)")

	if err := flags.Parse(args); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	if flags.NArg() > 0 {
		return options{}, usageError(flags, fmt.Errorf("unexpected argument %q", flags.Arg(0)))
	}

	filter, err := parseCaseFilter(only)
	if err != nil {
		return options{}, usageError(flags, err)
	}

	opts.only = filter
	opts.jobs = max(opts.jobs, 1)

	return opts, nil
}

// usageError prints err and the usage text and returns err.
func usageError(flags *flag.FlagSet, err error) error {
	// A failed write to stderr cannot be reported anywhere else.
	_, _ = fmt.Fprintln(flags.Output(), err)
	flags.Usage()

	return err
}

// generateCorpus detects what FFmpeg can build, generates the cases and writes the manifest.
func generateCorpus(ctx context.Context, opts options, ffmpeg commandRunner, stdout io.Writer) error {
	caps, err := detectCapabilities(ctx, ffmpeg)
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", errInterrupted, ctx.Err())
	}

	if err != nil {
		return fmt.Errorf("use ffmpeg %q (install FFmpeg or pass -ffmpeg): %w", opts.ffmpeg, err)
	}

	run := session{opts: opts, caps: caps, ffmpeg: ffmpeg, stdout: stdout}

	// An interrupted run still writes the manifests, so they match the files on disk.
	media, err := run.generateMedia(ctx)
	if err != nil {
		return err
	}

	failed := media.count(statusFailed)

	if opts.libraryDir != "" {
		library, err := run.generateLibrary(ctx)
		if err != nil {
			return err
		}

		failed += library.count(statusFailed)
	}

	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", errInterrupted, ctx.Err())
	}

	if failed > 0 {
		return errCasesFailed
	}

	return nil
}

// session is one run of the program with detected FFmpeg capabilities.
type session struct {
	opts   options
	caps   capabilities
	ffmpeg commandRunner
	stdout io.Writer
}

// generateMedia builds the playback corpus into s.opts.outDir and writes its manifest and
// summary.
func (s session) generateMedia(ctx context.Context) (manifest, error) {
	gen := generator{
		ffmpeg: s.ffmpeg, outDir: s.opts.outDir, force: s.opts.force, jobs: s.opts.jobs, progress: s.stdout,
		previous: loadPreviousEncoders(s.opts.outDir, s.stdout),
	}
	result := newManifest(s.caps.version, gen.generateAll(ctx, planCases(allCases(), s.caps, s.opts.only)))

	if err := writeManifest(s.opts.outDir, result); err != nil {
		return manifest{}, err
	}

	// The summary is informational; the manifest is the authoritative result.
	_ = writeSummary(s.stdout, result, filepath.Join(s.opts.outDir, manifestFileName))

	return result, nil
}

// loadPreviousEncoders reads the encoders of the files the earlier runs built. An unusable
// manifest is not fatal: the files it described are still kept, just listed without
// encoders, and the note says why.
func loadPreviousEncoders(outDir string, progress io.Writer) previousEncoders {
	previous, err := readPreviousEncoders(outDir)
	if err != nil {
		// Progress output is informational; the manifest is the authoritative result.
		_, _ = fmt.Fprintf(progress, "note: ignoring the previous manifest, kept files are listed without encoders: %v\n", err)
	}

	return previous
}

package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// catalogFiles lists every file the catalog names, relative to the -library directory. It
// walks the catalog independently of contentOf, so the tests do not trust the derivation.
func catalogFiles(catalog []expectedLibrary, music expectedMusic, discs expectedDiscs, audiobooks expectedAudiobooks) []string {
	var files []string

	for _, library := range catalog {
		for _, item := range library.Items {
			for _, file := range itemCatalogFiles(item) {
				files = append(files, path.Join(library.Path, file))
			}
		}
	}

	files = append(files, musicCatalogFiles(music)...)
	files = append(files, audiobookCatalogFiles(audiobooks)...)

	for _, disc := range discs.Items {
		for _, file := range disc.Files {
			files = append(files, path.Join(discs.Path, file))
		}
	}

	return files
}

// musicCatalogFiles lists the tracks and the folder covers (once per album).
func musicCatalogFiles(music expectedMusic) []string {
	var files []string

	for _, album := range music.Albums {
		for index, track := range album.Tracks {
			files = append(files, path.Join(music.Path, track.File))

			if index == 0 && track.CoverSource == coverSourceFolder {
				files = append(files, path.Join(music.Path, track.CoverFile))
			}
		}
	}

	return files
}

// itemCatalogFiles lists the files of one item, its seasons and episodes, relative to the
// library root.
func itemCatalogFiles(item expectedItem) []string {
	files := slices.Concat(item.Ignored, localArtworkFiles(item.Artwork), nfoFiles(item.NFO))

	for _, version := range item.Versions {
		files = append(files, version.Parts...)
	}

	for _, extra := range item.Extras {
		files = append(files, extra.File)
	}

	for _, season := range item.Seasons {
		files = append(files, localArtworkFiles(season.Artwork)...)

		for _, episode := range season.Episodes {
			files = slices.Concat(files, episode.Files, localArtworkFiles(episode.Artwork), nfoFiles(episode.NFO))
		}
	}

	return files
}

// localArtworkFiles skips embedded artwork: it names the video, which is listed already.
func localArtworkFiles(artwork []expectedArtwork) []string {
	var files []string

	for _, art := range artwork {
		if art.Source == artworkSourceLocal {
			files = append(files, art.File)
		}
	}

	return files
}

func nfoFiles(nfo *expectedNFO) []string {
	if nfo == nil {
		return nil
	}

	return []string{nfo.File}
}

// catalogNFO is an NFO of the catalog with the root element its file must have.
type catalogNFO struct {
	root string
	nfo  expectedNFO
}

// catalogNFOs maps the NFO files of the catalog (relative to the -library directory) to
// their expectations.
func catalogNFOs() map[string]catalogNFO {
	nfos := map[string]catalogNFO{}

	add := func(library expectedLibrary, root string, nfo *expectedNFO) {
		if nfo != nil {
			nfos[path.Join(library.Path, nfo.File)] = catalogNFO{root: root, nfo: *nfo}
		}
	}

	for _, library := range libraryCatalog() {
		for _, item := range library.Items {
			add(library, nfoRootOf(library.Kind), item.NFO)

			for _, season := range item.Seasons {
				for _, episode := range season.Episodes {
					add(library, nfoRootEpisode, episode.NFO)
				}
			}
		}
	}

	return nfos
}

func libraryCaseByName(t *testing.T, name string) mediaCase {
	t.Helper()

	for _, c := range libraryCases() {
		if c.Name == name {
			return c
		}
	}

	t.Fatalf("no library case %q", name)

	return mediaCase{}
}

func libraryItem(t *testing.T, libraryKind, title string) expectedItem {
	t.Helper()

	for _, library := range libraryCatalog() {
		for _, item := range library.Items {
			if library.Kind == libraryKind && item.Title == title {
				return item
			}
		}
	}

	t.Fatalf("no %s item %q", libraryKind, title)

	return expectedItem{}
}

func TestLibraryCasesHaveUniqueNamesAndPaths(t *testing.T) {
	t.Parallel()

	names := map[string]bool{}
	owners := map[string]string{}

	for _, c := range libraryCases() {
		if names[c.Name] {
			t.Errorf("case name %s is used twice", c.Name)
		}

		names[c.Name] = true

		if !strings.HasPrefix(c.Name, libraryCasePrefix) || c.Muxer == "" || len(c.Encoders) == 0 || c.isGap() {
			t.Errorf("%s: library cases need the prefix, a muxer and encoders and are never gaps", c.Name)
		}

		if !slices.Contains([]string{categoryArtwork, libraryKindMovies, libraryKindShows, categoryMusic, categoryAudiobooks, categoryDiscs}, c.Category) {
			t.Errorf("%s: unexpected category %q", c.Name, c.Category)
		}

		for _, file := range c.files() {
			checkLibraryPath(t, c.Name, file)

			if owner, seen := owners[file]; seen {
				t.Errorf("%s and %s both write %s", owner, c.Name, file)
			}

			owners[file] = c.Name
		}
	}
}

func checkLibraryPath(t *testing.T, name, file string) {
	t.Helper()

	roots := []string{
		moviesLibraryPath, showsLibraryPath, musicLibraryPath, audiobooksLibraryPath, discsLibraryPath, discSourcesPath,
	}
	inLibrary := slices.ContainsFunc(roots, func(root string) bool {
		return strings.HasPrefix(file, root+"/")
	})
	if !inLibrary || path.Clean(file) != file || strings.Contains(file, `\`) || path.Ext(file) == "" {
		t.Errorf("%s: %q must be a clean path below a library root or the disc sources, with forward slashes and an extension", name, file)
	}
}

// TestLibraryCatalogIsBackedByCases checks both directions: every file of the manifest is
// written by a case, and no case writes a file the manifest does not describe. The disc
// sources are staging files outside every library root, so the catalog does not list them.
func TestLibraryCatalogIsBackedByCases(t *testing.T) {
	t.Parallel()

	written := map[string]bool{}

	for _, c := range slices.Concat(libraryCases(), discAssemblyCases()) {
		for _, file := range c.files() {
			written[file] = !strings.HasPrefix(file, discSourcesPath+"/")
		}
	}

	described := map[string]bool{}

	for _, file := range catalogFiles(libraryCatalog(), musicCatalog(), discCatalog(), audiobookCatalog()) {
		if described[file] {
			t.Errorf("the catalog names %s twice", file)
		}

		described[file] = true

		if !written[file] {
			t.Errorf("no case writes %s", file)
		}
	}

	for file := range written {
		if !described[file] && written[file] {
			t.Errorf("a case writes %s, which the catalog does not describe", file)
		}
	}
}

func TestLibraryCopiesShareOneFFmpegRunPerItem(t *testing.T) {
	t.Parallel()

	cases := libraryCases()
	files := 0

	for _, c := range cases {
		files += 1 + len(c.Copies)

		for _, copyOutput := range c.Copies {
			if path.Ext(copyOutput) != path.Ext(c.Output) {
				t.Errorf("%s: copy %s has another format than %s", c.Name, copyOutput, c.Output)
			}
		}
	}

	dragonBall := libraryCaseByName(t, "library-shows-dragon-ball-super")
	if dragonBall.Output != "shows/Dragon Ball Super/Dragon Ball Super - 001.mkv" || len(dragonBall.Copies) != dragonBallSuperEpisodes-1 {
		t.Errorf("Dragon Ball Super: output %s with %d copies, want episode 001 and %d copies",
			dragonBall.Output, len(dragonBall.Copies), dragonBallSuperEpisodes-1)
	}

	// One run per artwork shape (3), movie (13, two of them embed covers) and show (4) builds
	// 8 images, 19 movie files and 138 episode files; the music tree adds one run for the
	// album cover and one per track (14), because every track carries its own tags, and the
	// audiobook tree one for the book cover (and its copy) and one per part (6). The
	// discs are assembled from five source runs (two DVD title sets, three Blu-ray clips).
	const wantRuns, wantFiles = 47, 193
	if len(cases) != wantRuns || files != wantFiles {
		t.Errorf("%d FFmpeg runs for %d media files, want %d runs for %d files", len(cases), files, wantRuns, wantFiles)
	}
}

// TestDragonBallSuperIsOneShowWithOneSeason pins the DT-24 regression case: 131 absolutely
// numbered files are one show, one season and 131 episodes, never 130 seasons.
func TestDragonBallSuperIsOneShowWithOneSeason(t *testing.T) {
	t.Parallel()

	show := libraryItem(t, libraryKindShows, "Dragon Ball Super")
	if show.Kind != itemKindShow || show.Path != dragonBallSuperFolder || len(show.Seasons) != 1 {
		t.Fatalf("show = %s at %q with %d seasons, want one show with one season", show.Kind, show.Path, len(show.Seasons))
	}

	season := show.Seasons[0]
	if season.Number != 1 || len(season.Episodes) != dragonBallSuperEpisodes {
		t.Fatalf("season %d with %d episodes, want season 1 with %d", season.Number, len(season.Episodes), dragonBallSuperEpisodes)
	}

	for index, episode := range season.Episodes {
		number := index + 1
		want := []string{fmt.Sprintf("Dragon Ball Super/Dragon Ball Super - %03d.mkv", number)}

		if episode.Number != number || episode.AbsoluteNumber != number || !slices.Equal(episode.Files, want) {
			t.Errorf("episode %d = %+v, want number and absolute number %d in %q", number, episode, number, want)
		}
	}

	anime := libraryItem(t, libraryKindShows, "Dragon Ball Super Fansub")
	episode := anime.Seasons[0].Episodes[0]

	if anime.Path == show.Path || episode.Number != animeEpisodeNumber || episode.AbsoluteNumber != animeEpisodeNumber ||
		episode.Files[0] != "Dragon Ball Super Fansub/[Group] Dragon Ball Super - 130 [1080p] [ABCD1234].mkv" {
		t.Errorf("anime-style show = %+v", anime)
	}
}

func TestLibraryCatalogCoversTheMovieLayouts(t *testing.T) {
	t.Parallel()

	bunny := libraryItem(t, libraryKindMovies, "Big Buck Bunny")
	if labels := versionLabels(bunny); !slices.Equal(labels, []string{"1080p", "4K"}) {
		t.Errorf("version labels = %q", labels)
	}

	wantExtras := []expectedExtra{
		{Type: extraTrailer, File: "Big Buck Bunny (2008)/Trailers/Big Buck Bunny Trailer.mkv"},
		{Type: extraFeaturette, File: "Big Buck Bunny (2008)/Big Buck Bunny (2008)-featurette.mkv"},
	}
	if !reflect.DeepEqual(bunny.Extras, wantExtras) || !slices.Equal(bunny.Ignored, []string{"Big Buck Bunny (2008)/sample.mkv"}) {
		t.Errorf("extras = %+v, ignored = %q", bunny.Extras, bunny.Ignored)
	}

	if kinds := artworkKinds(bunny.Artwork, artworkSourceLocal); !slices.Equal(kinds, []string{"poster", "backdrop", "logo"}) {
		t.Errorf("local artwork = %q", kinds)
	}

	if labels := versionLabels(libraryItem(t, libraryKindMovies, "Blade Runner")); !slices.Equal(labels, []string{"", "Director's Cut"}) {
		t.Errorf("edition labels = %q", labels)
	}

	if parts := libraryItem(t, libraryKindMovies, "Kill Bill").Versions[0].Parts; len(parts) != 2 ||
		!strings.HasSuffix(parts[0], " - cd1.mkv") || !strings.HasSuffix(parts[1], " - cd2.mkv") {
		t.Errorf("parts = %q", parts)
	}

	for _, title := range []string{"Sintel", "Tears of Steel", "Elephants Dream"} {
		if item := libraryItem(t, libraryKindMovies, title); path.Dir(item.Path) != "Blender Open Movies" || path.Ext(item.Path) != ".mkv" {
			t.Errorf("loose movie %s at %q", title, item.Path)
		}
	}
}

func TestLibraryCatalogCoversNFOVariantsAndUnicode(t *testing.T) {
	t.Parallel()

	nfoOf := func(title string) expectedNFO { return *libraryItem(t, libraryKindMovies, title).NFO }

	if nfo := nfoOf("Cosmos Laundromat: First Cycle"); path.Base(nfo.File) != "movie.nfo" || nfo.Fields.Title == "Cosmos Laundromat" {
		t.Errorf("the movie.nfo title must differ from the folder name: %+v", nfo)
	}

	if nfo := nfoOf("Spring"); nfo.Format != nfoFormatURL || nfo.Fields.ProviderIDs[providerTMDB] == "" {
		t.Errorf("URL NFO = %+v", nfo)
	}

	if nfo := nfoOf("Das Boot: Größenwahn unter Wasser"); nfo.Encoding != encodingLatin1 {
		t.Errorf("Latin-1 NFO = %+v", nfo)
	}

	if nfo := nfoOf("Caminandes"); nfo.Format != nfoFormatMalformed || !reflect.DeepEqual(nfo.Fields, nfoFields{}) {
		t.Errorf("malformed NFO = %+v", nfo)
	}

	amelie := libraryItem(t, libraryKindMovies, "Am\u00e9lie")
	if amelie.Path != "Ame\u0301lie (2001)" || !utf8.ValidString(amelie.Path) {
		t.Errorf("Amélie must keep its NFD folder name, got %q", amelie.Path)
	}
}

func TestLibraryCatalogCoversEmbeddedArtwork(t *testing.T) {
	t.Parallel()

	for title, extension := range map[string]string{"Glass Half": ".mkv", "Night of the Living Dead": extensionMP4} {
		item := libraryItem(t, libraryKindMovies, title)
		embedded := slices.IndexFunc(item.Artwork, func(art expectedArtwork) bool {
			return art.Source == artworkSourceEmbedded && art.Kind == artworkPoster && path.Ext(art.File) == extension
		})

		if embedded < 0 || len(artworkKinds(item.Artwork, artworkSourceLocal)) != 0 {
			t.Errorf("%s: want an embedded poster in a %s file and no local artwork, got %+v", title, extension, item.Artwork)
		}
	}
}

func TestLibraryCatalogCoversTheShowLayouts(t *testing.T) {
	t.Parallel()

	pioneer := libraryItem(t, libraryKindShows, "Pioneer One")

	numbers := make([]int, 0, len(pioneer.Seasons))
	for _, season := range pioneer.Seasons {
		numbers = append(numbers, season.Number)
	}

	if !slices.Equal(numbers, []int{0, 1, 2}) || !strings.HasSuffix(pioneer.Seasons[0].Path, "/Specials") {
		t.Errorf("seasons = %v, specials at %q", numbers, pioneer.Seasons[0].Path)
	}

	if ids := pioneer.NFO.Fields.ProviderIDs; path.Base(pioneer.NFO.File) != "tvshow.nfo" || len(ids) != 3 {
		t.Errorf("tvshow.nfo = %+v", pioneer.NFO)
	}

	season1 := pioneer.Seasons[1]
	if season1.Artwork[0].File != "Pioneer One (2010)/season01-poster.jpg" {
		t.Errorf("season poster = %+v", season1.Artwork)
	}

	multi, withMetadata := season1.Episodes[0], season1.Episodes[1]
	if multi.Number != 1 || multi.NumberEnd != 2 || !strings.HasSuffix(multi.Files[0], "S01E01-E02.mkv") {
		t.Errorf("multi-episode = %+v", multi)
	}

	if withMetadata.NFO == nil || !strings.HasSuffix(withMetadata.Artwork[0].File, "S01E03-thumb.jpg") ||
		withMetadata.Artwork[0].Kind != artworkThumb {
		t.Errorf("episode with NFO and thumb = %+v", withMetadata)
	}

	if root := libraryItem(t, libraryKindShows, "Star Wreck"); root.Path != "" || strings.Contains(root.Seasons[0].Episodes[0].Files[0], "/") {
		t.Errorf("root show = %+v", root)
	}
}

func versionLabels(item expectedItem) []string {
	labels := make([]string, 0, len(item.Versions))
	for _, version := range item.Versions {
		labels = append(labels, version.Label)
	}

	return labels
}

func artworkKinds(artwork []expectedArtwork, source string) []string {
	var kinds []string

	for _, art := range artwork {
		if art.Source == source {
			kinds = append(kinds, art.Kind)
		}
	}

	return kinds
}

func TestArtworkCasesMatchTheirFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		size      string
		extension string
		encoder   string
	}{
		{name: "library-artwork-poster", size: "600x900", extension: extensionJPEG, encoder: "mjpeg"},
		{name: "library-artwork-backdrop", size: "1280x720", extension: extensionJPEG, encoder: "mjpeg"},
		{name: "library-artwork-logo", size: "800x310", extension: extensionPNG, encoder: "png"},
	}

	for _, test := range tests {
		c := libraryCaseByName(t, test.name)
		args := strings.Join(buildArgs(c, "", "out.partial"), " ")

		if !strings.Contains(args, "testsrc2=size="+test.size) || !slices.Equal(c.Encoders, []string{test.encoder}) ||
			c.Source.Frames != stillFrames {
			t.Errorf("%s: args %s", test.name, args)
		}

		for _, file := range c.files() {
			if path.Ext(file) != test.extension {
				t.Errorf("%s writes %s, want %s files", test.name, file, test.extension)
			}
		}
	}

	if logo := strings.Join(libraryCaseByName(t, "library-artwork-logo").Args, " "); !strings.Contains(logo, "-pix_fmt rgba") ||
		!strings.Contains(logo, "colorchannelmixer=aa=") {
		t.Errorf("the logo needs a real alpha channel: %s", logo)
	}
}

func TestCoverCasesEmbedThePosterImage(t *testing.T) {
	t.Parallel()

	poster := libraryCaseByName(t, "library-artwork-poster").Output

	matroska := libraryCaseByName(t, "library-movies-glass-half-2015-cover")
	mp4 := libraryCaseByName(t, "library-movies-night-of-the-living-dead-1968-cover")

	if matroska.Cover != poster || mp4.Cover != poster {
		t.Fatalf("covers %q and %q, want the poster %q", matroska.Cover, mp4.Cover, poster)
	}

	wantMatroska := "-attach /lib/poster.jpg -metadata:s:t:0 mimetype=image/jpeg -metadata:s:t:0 filename=cover.jpg " +
		"-t 2 -f matroska out.partial"
	matroska.Cover = "/lib/poster.jpg"

	if args := strings.Join(buildArgs(matroska, "", "out.partial"), " "); !strings.HasSuffix(args, wantMatroska) {
		t.Errorf("Matroska cover args = %s", args)
	}

	mp4.Cover = "/lib/poster.jpg"
	args := strings.Join(buildArgs(mp4, "", "out.partial"), " ")

	for _, want := range []string{
		"duration=2 -i /lib/poster.jpg -map 0:v:0 -map 1:a:0 -ac 2 -map 2:v:0 ",
		"-c:v:0 libx264 -preset:v:0 veryfast -profile:v:0 high -pix_fmt:v:0 yuv420p -c:a aac " +
			"-c:v:1 copy -disposition:v:1 attached_pic -t 2 -f mp4 out.partial",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("MP4 cover args lack %q:\n%s", want, args)
		}
	}
}

func TestItemCasesSplitContainersAndAttachNFOsToTheFirstCase(t *testing.T) {
	t.Parallel()

	item := itemFiles{
		name: "library-movies-x", category: libraryKindMovies,
		content: itemContent{
			videos:   []string{"movies/x/a.mkv", "movies/x/b.mp4", "movies/x/c.mkv"},
			embedded: []string{"movies/x/d.mp4"},
			nfos:     []sidecarFile{{Output: "movies/x/movie.nfo", Content: "nfo"}},
		},
	}

	cases := itemCases(item, "movies/poster.jpg")

	got := make([]string, 0, len(cases))
	for _, c := range cases {
		got = append(got, fmt.Sprintf("%s %s %s copies=%q cover=%q sidecars=%d",
			c.Name, c.Muxer, c.Output, c.Copies, c.Cover, len(c.Sidecars)))
	}

	want := []string{
		`library-movies-x matroska movies/x/a.mkv copies=["movies/x/c.mkv"] cover="" sidecars=1`,
		`library-movies-x-mp4 mp4 movies/x/b.mp4 copies=[] cover="" sidecars=0`,
		`library-movies-x-cover mp4 movies/x/d.mp4 copies=[] cover="movies/poster.jpg" sidecars=0`,
	}

	if !slices.Equal(got, want) {
		t.Errorf("cases =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestSlug(t *testing.T) {
	t.Parallel()

	for text, want := range map[string]string{
		"Big Buck Bunny 2008":                    "big-buck-bunny-2008",
		"Cosmos Laundromat: First Cycle 2015":    "cosmos-laundromat-first-cycle-2015",
		"Am\u00e9lie 2001":                       "am\u00e9lie-2001",
		"  --Das Boot: Größenwahn unter Wasser ": "das-boot-größenwahn-unter-wasser",
	} {
		if got := slug(text); got != want {
			t.Errorf("slug(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestRenderMovieNFO(t *testing.T) {
	t.Parallel()

	nfo := expectedNFO{
		Format: nfoFormatXML, Encoding: encodingUTF8,
		Fields: nfoFields{
			Title: "A & B <C>", OriginalTitle: "AB", Year: 2015, Plot: `Say "hi"`, Genres: []string{"Animation"},
			ProviderIDs: map[string]string{providerIMDb: "tt1", providerTMDB: "2"},
		},
	}

	const want = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<movie>
  <title>A &amp; B &lt;C&gt;</title>
  <originaltitle>AB</originaltitle>
  <year>2015</year>
  <plot>Say &quot;hi&quot;</plot>
  <genre>Animation</genre>
  <uniqueid type="tmdb" default="true">2</uniqueid>
  <uniqueid type="imdb" default="false">tt1</uniqueid>
</movie>
`

	if got := renderNFO(nfoRootMovie, nfo); got != want {
		t.Errorf("NFO =\n%s\nwant\n%s", got, want)
	}
}

// nfoDocument is what a Kodi reader takes from the rendered XML.
type nfoDocument struct {
	XMLName       xml.Name
	Title         string   `xml:"title"`
	OriginalTitle string   `xml:"originaltitle"`
	Year          int      `xml:"year"`
	Season        int      `xml:"season"`
	Episode       int      `xml:"episode"`
	Aired         string   `xml:"aired"`
	Plot          string   `xml:"plot"`
	Genres        []string `xml:"genre"`
	UniqueIDs     []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`
}

// decodeNFO parses rendered XML like a reader would, including ISO-8859-1 files.
func decodeNFO(content string) (nfoDocument, error) {
	decoder := xml.NewDecoder(strings.NewReader(content))
	decoder.CharsetReader = func(charset string, input io.Reader) (io.Reader, error) {
		if !strings.EqualFold(charset, encodingLatin1) {
			return nil, fmt.Errorf("unexpected charset %s", charset)
		}

		data, err := io.ReadAll(input)
		if err != nil {
			return nil, err
		}

		runes := make([]rune, 0, len(data))
		for _, value := range data {
			runes = append(runes, rune(value))
		}

		return strings.NewReader(string(runes)), nil
	}

	var document nfoDocument
	err := decoder.Decode(&document)

	return document, err
}

func (d nfoDocument) fields() nfoFields {
	fields := nfoFields{
		Title: d.Title, OriginalTitle: d.OriginalTitle, Year: d.Year, Season: d.Season, Episode: d.Episode,
		Aired: d.Aired, Plot: d.Plot, Genres: d.Genres,
	}

	for _, id := range d.UniqueIDs {
		if fields.ProviderIDs == nil {
			fields.ProviderIDs = map[string]string{}
		}

		fields.ProviderIDs[id.Type] = id.Value
	}

	return fields
}

// TestLibraryNFOFilesYieldTheirManifestFields renders every NFO of the catalog and reads it
// back: XML gives exactly the manifest fields, a URL file the TMDB ID, a malformed file
// nothing, and the Latin-1 file is single-byte encoded.
func TestLibraryNFOFilesYieldTheirManifestFields(t *testing.T) {
	t.Parallel()

	nfos := catalogNFOs()
	if len(nfos) != 6 {
		t.Fatalf("%d NFO files, want 6", len(nfos))
	}

	rendered := 0

	for _, c := range libraryCases() {
		for _, sidecar := range c.Sidecars {
			want, found := nfos[sidecar.Output]
			if !found {
				t.Errorf("%s: sidecar %s is no catalog NFO", c.Name, sidecar.Output)

				continue
			}

			checkRenderedNFO(t, sidecar.Output, want, sidecar.Content)
			rendered++
		}
	}

	if rendered != len(nfos) {
		t.Errorf("%d NFO files rendered, want %d", rendered, len(nfos))
	}
}

func checkRenderedNFO(t *testing.T, file string, want catalogNFO, content string) {
	t.Helper()

	root, nfo := want.root, want.nfo

	switch nfo.Format {
	case nfoFormatURL:
		if content != tmdbMovieURL+nfo.Fields.ProviderIDs[providerTMDB]+"\n" {
			t.Errorf("%s: URL NFO = %q", file, content)
		}
	case nfoFormatMalformed:
		if _, err := decodeNFO(content); err == nil {
			t.Errorf("%s: the malformed NFO parses", file)
		}
	default:
		document, err := decodeNFO(content)
		if err != nil {
			t.Fatalf("%s: %v", file, err)
		}

		if document.XMLName.Local != root || !reflect.DeepEqual(document.fields(), nfo.Fields) {
			t.Errorf("%s: <%s> %+v, want <%s> %+v", file, document.XMLName.Local, document.fields(), root, nfo.Fields)
		}

		if nfo.Encoding == encodingLatin1 && (utf8.ValidString(content) || !strings.Contains(content, "\xf6")) {
			t.Errorf("%s: not ISO-8859-1 encoded: %q", file, content)
		}
	}
}

// TestLibraryCatalogYears pins the years the catalog derives from the names.
func TestLibraryCatalogYears(t *testing.T) {
	t.Parallel()

	want := map[string]int{
		"Big Buck Bunny": 2008, "Blade Runner": 1982, "Kill Bill": 2003, "Cosmos Laundromat: First Cycle": 2015,
		"Spring": 2019, "Das Boot: Größenwahn unter Wasser": 1981, "Caminandes": 2013, "Amélie": 2001,
		"Sintel": 2010, "Tears of Steel": 2012, "Elephants Dream": 2006, "Glass Half": 2015,
		"Night of the Living Dead": 1968, "Pioneer One": 2010, "Star Wreck": 0, "Dragon Ball Super": 0,
		"Dragon Ball Super Fansub": 0,
	}

	got := map[string]int{}

	for _, library := range libraryCatalog() {
		for _, item := range library.Items {
			got[item.Title] = item.Year

			if item.NFO != nil && item.NFO.Fields.Year != 0 && item.NFO.Fields.Year != item.Year {
				t.Errorf("%s: NFO year %d differs from %d", item.Title, item.NFO.Fields.Year, item.Year)
			}
		}
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("years = %v, want %v", got, want)
	}
}

func TestYearOf(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]int{
		"Movie (2010)": 2010, "Movie (2010).mp4": 2010, "Show": 0, "Movie (20)": 0, "Movie (abcd)": 0,
		"Movie (2010": 0, "(1999)": 1999,
	} {
		if got := yearOf(name); got != want {
			t.Errorf("yearOf(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestToLatin1ReplacesRunesOutsideTheCharset(t *testing.T) {
	t.Parallel()

	if got := toLatin1("Größe – ok"); got != "Gr\xf6\xdfe ? ok" {
		t.Errorf("toLatin1 = %q", got)
	}
}

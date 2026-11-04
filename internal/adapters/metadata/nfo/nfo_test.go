package nfo_test

import (
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/metadata/nfo"
	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

func date(t *testing.T, text string) metadata.Date {
	t.Helper()

	parsed, err := metadata.ParseDate(text)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", text, err)
	}

	return parsed
}

func ids(pairs ...string) []metadata.ProviderID {
	var result []metadata.ProviderID

	for index := 0; index+1 < len(pairs); index += 2 {
		result = append(result, metadata.ProviderID{Provider: metadata.Provider(pairs[index]), Value: pairs[index+1]})
	}

	return result
}

func actor(name, character string, order int) metadata.Credit {
	return metadata.Credit{Name: name, Role: metadata.RoleActor, Character: character, Order: order}
}

func crew(name string, role metadata.Role, order int) metadata.Credit {
	return metadata.Credit{Name: name, Role: role, Order: order}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()

	data, err := fs.ReadFile(os.DirFS("testdata"), name)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	return data
}

type fixtureCase struct {
	file     string
	kind     nfo.Kind
	want     []metadata.Record
	warnings []string
}

func TestParseFixtures(t *testing.T) {
	t.Parallel()

	for _, test := range fixtureCases(t) {
		t.Run(test.file, func(t *testing.T) {
			t.Parallel()

			result, err := nfo.Parse(readFixture(t, test.file), test.kind)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if !reflect.DeepEqual(result.Records, test.want) {
				t.Errorf("records\n got %+v\nwant %+v", result.Records, test.want)
			}

			if !reflect.DeepEqual(result.Warnings, test.warnings) {
				t.Errorf("warnings = %v, want %v", result.Warnings, test.warnings)
			}
		})
	}
}

func fixtureCases(t *testing.T) []fixtureCase {
	t.Helper()

	cases := kodiCases(t)
	cases = append(cases, exporterCases(t)...)
	cases = append(cases, urlCases()...)

	return append(cases, textCases(t)...)
}

func kodiCases(t *testing.T) []fixtureCase {
	t.Helper()

	return []fixtureCase{
		{file: "kodi_movie.nfo", kind: nfo.KindMovie, want: []metadata.Record{{
			Title: "Big Buck Bunny", SortTitle: "Bunny, Big Buck", OriginalTitle: "Big Buck Bunny",
			Year: 2008, ReleaseDate: date(t, "2008-05-20"),
			Summary: "A giant rabbit takes revenge.\nSecond paragraph.", Tagline: "One big rabbit",
			ContentRating: "Rated G", Genres: []string{"Animation", "Comedy"},
			Studios: []string{"Blender Foundation"}, Countries: []string{"Netherlands"},
			Collections: []string{"Blender Open Movies"},
			People: []metadata.Credit{
				crew("Sacha Goedegebure", metadata.RoleDirector, 0),
				crew("Sacha Goedegebure", metadata.RoleWriter, 0),
				actor("Big Buck", "Bunny", 0), actor("Frank", "Flying Squirrel", 1),
			},
			ProviderIDs: ids("tmdb", "10378", "imdb", "tt1254207"),
		}}},
		{file: "kodi_tvshow.nfo", kind: nfo.KindShow, want: []metadata.Record{{
			Title: "Pioneer One", Year: 2010, ReleaseDate: date(t, "2010-06-16"),
			Summary: "An object in the sky over North America.", ContentRating: "TV-14",
			Genres: []string{"Drama", "Science Fiction"}, Studios: []string{"VODO"},
			People:      []metadata.Credit{actor("James Rotenberg", "Tom Taylor", 0)},
			ProviderIDs: ids("tmdb", "30992", "imdb", "tt1748166", "tvdb", "170551"),
		}}},
		{file: "kodi_season.nfo", kind: nfo.KindSeason, want: []metadata.Record{{
			Title: "Season One", Summary: "The first season.", Year: 2010,
			ReleaseDate: date(t, "2010-06-16"), ProviderIDs: ids("tvdb", "417671"),
		}}},
		{file: "kodi_episode.nfo", kind: nfo.KindEpisode, want: []metadata.Record{{
			Title: "Earthfall", Summary: "Agents investigate the crash.", Year: 2010,
			ReleaseDate: date(t, "2010-06-16"),
			People: []metadata.Credit{
				crew("Bracey Smith", metadata.RoleDirector, 0),
				crew("Josh Bernhard", metadata.RoleWriter, 0),
				crew("Bracey Smith", metadata.RoleWriter, 1),
				actor("Jack Haley", "Taylor", 0),
			},
			ProviderIDs: ids("tvdb", "2363431"),
		}}},
		{file: "multi_episode.nfo", kind: nfo.KindEpisode, want: []metadata.Record{
			{Title: "Earthfall", Year: 2010, ReleaseDate: date(t, "2010-06-16"), ProviderIDs: ids("tvdb", "2363431")},
			{Title: "The Man from Mars", Year: 2010, ReleaseDate: date(t, "2010-12-13"), ProviderIDs: ids("tvdb", "2363432")},
		}},
	}
}

func exporterCases(t *testing.T) []fixtureCase {
	t.Helper()

	return []fixtureCase{
		{file: "jellyfin_movie.nfo", kind: nfo.KindMovie, want: []metadata.Record{{
			Title: "Sintel", OriginalTitle: "Sintel", Year: 2010, ReleaseDate: date(t, "2010-09-27"),
			Summary: "A girl and a dragon.", ContentRating: "NR",
			Genres: []string{"Animation", "Fantasy"}, Studios: []string{"Blender Foundation"},
			Collections: []string{"Blender Open Movies"},
			People: []metadata.Credit{
				crew("Colin Levy", metadata.RoleDirector, 0),
				crew("Esther Wouda", metadata.RoleWriter, 0),
				actor("Halina Reijn", "Sintel", 0),
			},
			ProviderIDs: ids("tmdb", "45745", "imdb", "tt1727587"),
		}}},
		{file: "jellyfin_episode.nfo", kind: nfo.KindEpisode, want: []metadata.Record{{
			Title: "Pilot", Year: 2010, ReleaseDate: date(t, "2010-06-16"),
			Summary: "The crew lands.", ProviderIDs: ids("tvdb", "2363431"),
		}}},
		{file: "plex_exporter_movie.nfo", kind: nfo.KindMovie, want: []metadata.Record{{
			Title: "Elephants Dream", SortTitle: "Elephants Dream", Year: 2006,
			ReleaseDate: date(t, "2006-03-24"), ContentRating: "DE:FSK 6",
			Genres: []string{"Animation"}, Studios: []string{"Orange Open Movie Project"},
			Countries: []string{"Netherlands"}, Collections: []string{"Blender Open Movies"},
			People: []metadata.Credit{
				crew("Bassam Kurdali", metadata.RoleDirector, 0),
				actor("Cas Jansen", "Emo", 2), actor("Tygo Gernandt", "Proog", 1),
			},
			ProviderIDs: ids("tmdb", "9761", "imdb", "tt0807840"),
		}}},
		{file: "plex_exporter_show.nfo", kind: nfo.KindShow, want: []metadata.Record{{
			Title: "Pioneer One", Year: 2010, Genres: []string{"Drama"}, Studios: []string{"VODO"},
			ProviderIDs: ids("tvdb", "170551"),
		}}},
	}
}

func urlCases() []fixtureCase {
	return []fixtureCase{
		{file: "url_only_tmdb.nfo", kind: nfo.KindMovie, want: []metadata.Record{{ProviderIDs: ids("tmdb", "1000002")}}},
		{file: "url_only_tmdb_show.nfo", kind: nfo.KindShow, want: []metadata.Record{{ProviderIDs: ids("tmdb", "30992")}}},
		{file: "url_only_imdb.nfo", kind: nfo.KindMovie, want: []metadata.Record{{ProviderIDs: ids("imdb", "tt0807840")}}},
		{file: "url_only_tvdb_query.nfo", kind: nfo.KindShow, want: []metadata.Record{{ProviderIDs: ids("tvdb", "170551")}}},
		{file: "url_only_tvdb_series.nfo", kind: nfo.KindShow, want: []metadata.Record{{ProviderIDs: ids("tvdb", "170551")}}},
		{file: "xml_with_url.nfo", kind: nfo.KindMovie, want: []metadata.Record{{
			Title: "Spring", ProviderIDs: ids("tmdb", "1000002", "imdb", "tt9999999"),
		}}},
	}
}

func textCases(t *testing.T) []fixtureCase {
	t.Helper()

	return []fixtureCase{
		{file: "latin1.nfo", kind: nfo.KindMovie, want: []metadata.Record{{
			Title: "Das Boot: Größenwahn unter Wasser", Summary: "Ärger an Bord.", Year: 1981,
		}}},
		{file: "windows1252.nfo", kind: nfo.KindMovie, want: []metadata.Record{{Title: "Café “Noir” €"}}},
		{file: "bom_utf8.nfo", kind: nfo.KindMovie, want: []metadata.Record{{Title: "Amélie"}}},
		{file: "nfd_text.nfo", kind: nfo.KindMovie, want: []metadata.Record{{Title: "Amélie", Genres: []string{"Comédie"}}}},
		{file: "entities.nfo", kind: nfo.KindMovie, want: []metadata.Record{{Title: "Tom & Jerry – \"Live\" é"}}},
		{file: "unknown_elements.nfo", kind: nfo.KindMovie, want: []metadata.Record{{Title: "Known", Tagline: "Case does not matter"}}},
		{file: "default_uniqueid.nfo", kind: nfo.KindMovie, want: []metadata.Record{{Title: "Defaults", ProviderIDs: ids("tmdb", "333")}}},
		{file: "outline_fallback.nfo", kind: nfo.KindMovie, want: []metadata.Record{{
			Title: "Padded   Title", Summary: "Only the outline.", Year: 2019, ReleaseDate: date(t, "2019-04-02"),
		}}},
		{
			file: "invalid_values.nfo", kind: nfo.KindMovie,
			want: []metadata.Record{{
				Title: "Bad IDs", People: []metadata.Credit{actor("Someone", "", 0)},
				ProviderIDs: ids("tvdb", "170551"),
			}},
			warnings: []string{nfo.WarningInvalidDate, nfo.WarningInvalidYear, nfo.WarningInvalidOrder, nfo.WarningInvalidProviderID},
		},
	}
}

func TestParseRejectsUnusableFiles(t *testing.T) {
	t.Parallel()

	tests := []struct {
		file   string
		kind   nfo.Kind
		secret string
	}{
		{file: "malformed_truncated.nfo", kind: nfo.KindMovie, secret: "Never read"},
		{file: "malformed_mismatched.nfo", kind: nfo.KindMovie, secret: "Secret"},
		{file: "malformed_second_root.nfo", kind: nfo.KindEpisode, secret: "broken"},
		{file: "invalid_utf8.nfo", kind: nfo.KindMovie, secret: "Caf"},
		{file: "external_entity.nfo", kind: nfo.KindMovie, secret: "passwd"},
		{file: "utf16_declared.nfo", kind: nfo.KindMovie, secret: "UTF-16"},
		{file: "empty.nfo", kind: nfo.KindMovie},
		{file: "whitespace_only.nfo", kind: nfo.KindMovie},
		{file: "url_only_unknown.nfo", kind: nfo.KindMovie, secret: "words"},
		{file: "wrong_root.nfo", kind: nfo.KindMovie, secret: "tvshow"},
	}

	for _, test := range tests {
		t.Run(test.file, func(t *testing.T) {
			t.Parallel()

			_, err := nfo.Parse(readFixture(t, test.file), test.kind)
			assertUnsupported(t, err, test.secret)
		})
	}
}

func assertUnsupported(t *testing.T, err error, secret string) {
	t.Helper()

	if !errors.Is(err, domain.ErrUnsupported) {
		t.Fatalf("error = %v, want ErrUnsupported", err)
	}

	if secret != "" && strings.Contains(err.Error(), secret) {
		t.Errorf("error %q leaks file content %q", err, secret)
	}
}

func TestParseLimits(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"oversized":     "<movie><plot>" + strings.Repeat("a", nfo.MaxInputBytes) + "</plot></movie>",
		"deep nesting":  "<movie>" + strings.Repeat("<a>", 40) + strings.Repeat("</a>", 40) + "</movie>",
		"many elements": "<movie>" + strings.Repeat("<genre>x</genre>", 30000) + "</movie>",
		"unclosed":      "<movie><title>x</title>",
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := nfo.Parse([]byte(input), nfo.KindMovie)
			assertUnsupported(t, err, "")
		})
	}
}

func TestParseAcceptsInputAtTheSizeLimit(t *testing.T) {
	t.Parallel()

	prefix, suffix := "<movie><title>Edge</title><plot>", "</plot></movie>"
	input := prefix + strings.Repeat("a", nfo.MaxInputBytes-len(prefix)-len(suffix)) + suffix

	result, err := nfo.Parse([]byte(input), nfo.KindMovie)
	if err != nil || result.Records[0].Title != "Edge" {
		t.Fatalf("Parse = %+v, %v", result.Records, err)
	}
}

func TestParseUnknownKind(t *testing.T) {
	t.Parallel()

	_, err := nfo.Parse([]byte("<movie/>"), nfo.Kind("album"))
	if !errors.Is(err, domain.ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestParseReadsOnlyTheFirstRootForNonEpisodes(t *testing.T) {
	t.Parallel()

	result, err := nfo.Parse(readFixture(t, "multi_episode.nfo"), nfo.KindEpisode)
	if err != nil || len(result.Records) != 2 {
		t.Fatalf("episode: %d records, %v", len(result.Records), err)
	}

	input := "<movie><title>First</title></movie><movie><title>Second</title></movie>"

	result, err = nfo.Parse([]byte(input), nfo.KindMovie)
	if err != nil || len(result.Records) != 1 || result.Records[0].Title != "First" {
		t.Fatalf("movie: %+v, %v", result.Records, err)
	}
}

func TestParseInlineVariants(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		kind  nfo.Kind
		want  metadata.Record
	}{
		{"crlf url", "https://www.imdb.com/title/tt0807840\r\n", nfo.KindMovie, metadata.Record{ProviderIDs: ids("imdb", "tt0807840")}},
		{"url without scheme", "www.themoviedb.org/movie/603", nfo.KindMovie, metadata.Record{ProviderIDs: ids("tmdb", "603")}},
		{"legacy numeric id of a show", "<tvshow><id>170551</id></tvshow>", nfo.KindShow, metadata.Record{ProviderIDs: ids("tvdb", "170551")}},
		{"legacy numeric id of a movie", "<movie><id>603</id></movie>", nfo.KindMovie, metadata.Record{ProviderIDs: ids("tmdb", "603")}},
		{"single-quoted latin1 declaration", "<?xml version='1.0' encoding='latin1'?><movie><title>\xe9</title></movie>", nfo.KindMovie, metadata.Record{Title: "é"}},
		{"declaration without encoding", "<?xml version=\"1.0\"?><movie><title>x</title></movie>", nfo.KindMovie, metadata.Record{Title: "x"}},
		{"date with time", "<movie><aired>2001-02-03T04:05:06</aired></movie>", nfo.KindMovie, metadata.Record{Year: 2001, ReleaseDate: date(t, "2001-02-03")}},
		{"year wins over date", "<movie><year>1999</year><premiered>2001-02-03</premiered></movie>", nfo.KindMovie, metadata.Record{Year: 1999, ReleaseDate: date(t, "2001-02-03")}},
		{"text set", "<movie><set> Saga </set></movie>", nfo.KindMovie, metadata.Record{Collections: []string{"Saga"}}},
		{"unrelated url lines", "https://example.com/movie/1 https://www.imdb.com/name/nm1\nhttps://thetvdb.com/series/pioneer-one", nfo.KindMovie, metadata.Record{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := nfo.Parse([]byte(test.input), test.kind)
			if test.want.ProviderIDs == nil && strings.HasPrefix(test.input, "http") {
				assertUnsupported(t, err, "")

				return
			}

			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if !reflect.DeepEqual(result.Records, []metadata.Record{test.want}) {
				t.Errorf("got %+v, want %+v", result.Records, test.want)
			}
		})
	}
}

// TestParseMatchesTheTestLibraryMalformedFile uses the exact text gen-testmedia writes for
// the malformed movie of the M3-17 library.
func TestParseMatchesTheTestLibraryMalformedFile(t *testing.T) {
	t.Parallel()

	const malformed = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<movie>\n  <title>Never read</title>\n  <plot>The file ends before"

	_, err := nfo.Parse([]byte(malformed), nfo.KindMovie)
	assertUnsupported(t, err, "Never read")
}

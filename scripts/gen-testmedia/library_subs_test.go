package main

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestSubtitleLibraryHasTheSidecarSet(t *testing.T) {
	t.Parallel()

	c := libraryCaseByName(t, "library-subs-movie-2020")

	want := []string{
		"subs/Movie (2020).mkv",
		"subs/Movie (2020).en.srt",
		"subs/Movie (2020).de.forced.srt",
		"subs/Movie (2020).fr.sdh.ass",
		"subs/Subs/Movie (2020).es.srt",
	}
	if got := c.files(); !slices.Equal(got, want) {
		t.Errorf("files = %q, want %q", got, want)
	}

	if c.Category != categorySubtitles || c.Muxer != matroskaMuxer || len(c.Inputs) != 0 {
		t.Errorf("the movie must be the plain clip without embedded subtitles: %+v", c)
	}

	for _, sidecar := range c.Sidecars {
		isASS := strings.HasSuffix(sidecar.Output, ".ass")
		if isASS != strings.HasPrefix(sidecar.Content, "[Script Info]") || isASS == strings.HasPrefix(sidecar.Content, "1\n00:00:00,000 --> ") {
			t.Errorf("%s: content does not match the extension:\n%s", sidecar.Output, sidecar.Content)
		}

		if wantCodec := map[bool]string{true: codecASS, false: codecSubRip}[isASS]; sidecar.Codec != wantCodec {
			t.Errorf("%s: codec %s, want %s", sidecar.Output, sidecar.Codec, wantCodec)
		}
	}

	if sdh := c.Sidecars[2].Content; !strings.Contains(sdh, `[une porte claque]\NSous-titre un`) {
		t.Errorf("the SDH sidecar lacks its sound description:\n%s", sdh)
	}
}

func TestSubtitleCatalogJSONShape(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(subtitleCatalog())
	if err != nil {
		t.Fatal(err)
	}

	const want = `{"path":"subs","movies":[{"title":"Movie","year":2020,"file":"Movie (2020).mkv","sidecars":[` +
		`{"file":"Movie (2020).en.srt","language":"en","forced":false,"hearingImpaired":false,"format":"srt"},` +
		`{"file":"Movie (2020).de.forced.srt","language":"de","forced":true,"hearingImpaired":false,"format":"srt"},` +
		`{"file":"Movie (2020).fr.sdh.ass","language":"fr","forced":false,"hearingImpaired":true,"format":"ass"},` +
		`{"file":"Subs/Movie (2020).es.srt","language":"es","forced":false,"hearingImpaired":false,"format":"srt"}]}]}`
	if string(data) != want {
		t.Errorf("subtitle catalog JSON =\n%s\nwant\n%s", data, want)
	}
}

func TestWriteSubtitleSummary(t *testing.T) {
	t.Parallel()

	var text strings.Builder

	writeSubtitleSummary(&text, expectedSubtitles{})

	if text.Len() != 0 {
		t.Errorf("summary of an empty library = %q", text.String())
	}

	writeSubtitleSummary(&text, subtitleCatalog())

	const want = "  subs/ (1 movies with sidecar subtitles)\n    Movie (2020): en, de (forced), fr (sdh), es\n"
	if text.String() != want {
		t.Errorf("summary = %q, want %q", text.String(), want)
	}
}

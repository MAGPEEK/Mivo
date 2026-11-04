package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestClassifyFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantClass library.FileClass
		wantOK    bool
	}{
		{name: "movie.mkv", wantClass: library.FileClassVideo, wantOK: true},
		{name: "MOVIE.MP4", wantClass: library.FileClassVideo, wantOK: true},
		{name: "VTS_01_1.VOB", wantClass: library.FileClassVideo, wantOK: true},
		{name: "movie.nfo", wantClass: library.FileClassNFO, wantOK: true},
		{name: "tvshow.NFO", wantClass: library.FileClassNFO, wantOK: true},
		{name: "poster.jpg", wantClass: library.FileClassImage, wantOK: true},
		{name: "folder.JPEG", wantClass: library.FileClassImage, wantOK: true},
		{name: "fanart.png", wantClass: library.FileClassImage, wantOK: true},
		{name: "clearlogo.WebP", wantClass: library.FileClassImage, wantOK: true},
		{name: "movie.tbn", wantClass: library.FileClassImage, wantOK: true},
		{name: "Über Film-poster.jpg", wantClass: library.FileClassImage, wantOK: true},
		{name: "subtitle.srt", wantOK: false},
		{name: "song.mp3", wantOK: false},
		{name: "sample.m4a", wantOK: false},
		{name: "cover.gif", wantOK: false},
		{name: "photo.heic", wantOK: false},
		{name: "movie.nfo.bak", wantOK: false},
		{name: "poster.jpg.part", wantOK: false},
		{name: "nfo", wantOK: false},
		{name: "noextension", wantOK: false},
		{name: "", wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			class, ok := library.ClassifyFile(test.name)
			if class != test.wantClass || ok != test.wantOK {
				t.Errorf("ClassifyFile(%q) = %q, %v, want %q, %v", test.name, class, ok, test.wantClass, test.wantOK)
			}
		})
	}
}

func TestClassifyFileAgreesWithTheVideoPolicy(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"a.mkv", "a.MXF", "a.rmvb", "a.3g2", "a.srt", "a.nfo", "a.jpg", "a", ""} {
		class, ok := library.ClassifyFile(name)
		isVideo := ok && class == library.FileClassVideo

		if isVideo != library.IsVideoFileName(name) {
			t.Errorf("ClassifyFile(%q) video = %v, IsVideoFileName = %v", name, isVideo, library.IsVideoFileName(name))
		}
	}
}

func TestParseFileClass(t *testing.T) {
	t.Parallel()

	for _, want := range []library.FileClass{library.FileClassVideo, library.FileClassNFO, library.FileClassImage} {
		got, err := library.ParseFileClass(want.String())
		if err != nil || got != want || !got.IsValid() {
			t.Errorf("ParseFileClass(%q) = %q, %v, want %q, nil", want, got, err, want)
		}
	}

	for _, text := range []string{"", "Video", "subtitle", "images"} {
		got, err := library.ParseFileClass(text)
		if !errors.Is(err, domain.ErrInvalid) || got != "" {
			t.Errorf("ParseFileClass(%q) = %q, %v, want empty, ErrInvalid", text, got, err)
		}
	}
}

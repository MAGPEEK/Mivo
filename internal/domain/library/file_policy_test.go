package library_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/domain/library"
)

func TestIsVideoFileName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "movie.mkv", want: true},
		{name: "MOVIE.MKV", want: true},
		{name: "Movie.Mp4", want: true},
		{name: "film.mk3d", want: true},
		{name: "clip.m4v", want: true},
		{name: "clip.mov", want: true},
		{name: "old.avi", want: true},
		{name: "old.divx", want: true},
		{name: "broadcast.ts", want: true},
		{name: "bluray.m2ts", want: true},
		{name: "camera.mts", want: true},
		{name: "web.webm", want: true},
		{name: "flash.flv", want: true},
		{name: "windows.wmv", want: true},
		{name: "windows.asf", want: true},
		{name: "theora.ogv", want: true},
		{name: "phone.3gp", want: true},
		{name: "phone.3g2", want: true},
		{name: "dvd.mpg", want: true},
		{name: "dvd.mpeg", want: true},
		{name: "VTS_01_1.VOB", want: true},
		{name: "real.rm", want: true},
		{name: "real.rmvb", want: true},
		{name: "broadcast.mxf", want: true},
		{name: "Über Film.2019.mkv", want: true},
		{name: "sample.m4a", want: false},
		{name: "song.mp3", want: false},
		{name: "noextension", want: false},
		{name: "x.srt", want: false},
		{name: "poster.jpg", want: false},
		{name: "movie.nfo", want: false},
		{name: "movie.mkv.part", want: false},
		{name: "mkv", want: false},
		{name: "movie.", want: false},
		{name: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.IsVideoFileName(test.name); got != test.want {
				t.Errorf("IsVideoFileName(%q) = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestIsIgnoredDirectory(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "@eaDir", want: true},
		{name: "#recycle", want: true},
		{name: "#snapshot", want: true},
		{name: ".@__thumb", want: true},
		{name: "$RECYCLE.BIN", want: true},
		{name: "$Recycle.Bin", want: true},
		{name: "System Volume Information", want: true},
		{name: "lost+found", want: true},
		{name: "@EADIR", want: true},
		{name: "Movies", want: false},
		{name: "recycle", want: false},
		{name: "@eaDir2", want: false},
		{name: "Season 1", want: false},
		{name: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.IsIgnoredDirectory(test.name); got != test.want {
				t.Errorf("IsIgnoredDirectory(%q) = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

func TestIsHiddenEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: ".hidden", want: true},
		{name: ".DS_Store", want: true},
		{name: ".@__thumb", want: true},
		{name: "._movie.mkv", want: true},
		{name: "movie.mkv", want: false},
		{name: "Movies", want: false},
		{name: "", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := library.IsHiddenEntry(test.name); got != test.want {
				t.Errorf("IsHiddenEntry(%q) = %v, want %v", test.name, got, test.want)
			}
		})
	}
}

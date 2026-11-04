package library_test

import (
	"errors"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/library"
)

func TestClassifyImage(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		want library.ImageClass
		ok   bool
	}{
		{"a.jpg", library.ImageClassWeb, true},
		{"a.JPEG", library.ImageClassWeb, true},
		{"a.png", library.ImageClassWeb, true},
		{"a.webp", library.ImageClassWeb, true},
		{"a.gif", library.ImageClassWeb, true},
		{"a.avif", library.ImageClassWeb, true},
		{"a.heic", library.ImageClassHEIC, true},
		{"a.HEIF", library.ImageClassHEIC, true},
		{"a.tiff", library.ImageClassTIFF, true},
		{"a.tif", library.ImageClassTIFF, true},
		{"a.cr2", library.ImageClassRAW, true},
		{"a.cr3", library.ImageClassRAW, true},
		{"a.nef", library.ImageClassRAW, true},
		{"a.arw", library.ImageClassRAW, true},
		{"a.dng", library.ImageClassRAW, true},
		{"a.orf", library.ImageClassRAW, true},
		{"a.rw2", library.ImageClassRAW, true},
		{"a.RAF", library.ImageClassRAW, true},
		{"a.mkv", "", false},
		{"a.tbn", "", false},
		{"jpg", "", false},
	}

	for _, testCase := range cases {
		got, ok := library.ClassifyImage(testCase.name)
		if got != testCase.want || ok != testCase.ok {
			t.Errorf("ClassifyImage(%q) = %q, %v; want %q, %v", testCase.name, got, ok, testCase.want, testCase.ok)
		}
	}
}

func TestClassifyFileInPhotos(t *testing.T) {
	t.Parallel()

	cases := []struct {
		kind library.Kind
		name string
		want library.FileClass
		ok   bool
	}{
		{library.KindPhotos, "a.jpg", library.FileClassPhoto, true},
		{library.KindPhotos, "a.cr2", library.FileClassPhoto, true},
		{library.KindPhotos, "a.mp4", library.FileClassVideo, true},
		{library.KindPhotos, "a.nfo", library.FileClassNFO, true},
		{library.KindPhotos, "a.tbn", library.FileClassImage, true},
		{library.KindPhotos, "a.txt", "", false},
		{library.KindMovies, "a.jpg", library.FileClassImage, true},
		{library.KindMovies, "a.cr2", "", false},
		{library.KindShows, "a.mkv", library.FileClassVideo, true},
	}

	for _, testCase := range cases {
		got, ok := library.ClassifyFileIn(testCase.kind, testCase.name)
		if got != testCase.want || ok != testCase.ok {
			t.Errorf("ClassifyFileIn(%q, %q) = %q, %v; want %q, %v",
				testCase.kind, testCase.name, got, ok, testCase.want, testCase.ok)
		}
	}
}

func TestPhotoKinds(t *testing.T) {
	t.Parallel()

	if kind, err := library.ParseKind("photos"); err != nil || kind != library.KindPhotos {
		t.Errorf("ParseKind(photos) = %q, %v", kind, err)
	}

	if class, err := library.ParseFileClass("photo"); err != nil || class != library.FileClassPhoto {
		t.Errorf("ParseFileClass(photo) = %q, %v", class, err)
	}

	for _, text := range []string{"photo_album", "photo"} {
		if _, err := library.ParseItemKind(text); err != nil {
			t.Errorf("ParseItemKind(%q) error = %v", text, err)
		}
	}

	itemKind, err := library.ItemKindForLibrary(library.KindPhotos)
	if err != nil || itemKind != library.ItemKindPhoto {
		t.Errorf("ItemKindForLibrary(photos) = %q, %v", itemKind, err)
	}

	definition := library.Definition{Name: "Fotos", Root: "/photos", Kind: library.KindPhotos}
	if err := definition.Validate(); err != nil {
		t.Errorf("Validate photos definition: %v", err)
	}

	if !library.FileClassPhoto.AllowedIn(library.KindPhotos) || library.FileClassPhoto.AllowedIn(library.KindMovies) {
		t.Error("photo class must be allowed in photos libraries only")
	}

	if library.ItemKindPhotoAlbum.OwnsFiles() || !library.ItemKindPhoto.OwnsFiles() {
		t.Error("album must not own files, photo must")
	}
}

func TestPhotoHierarchy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		child, parent library.ItemKind
		want          bool
	}{
		{library.ItemKindPhotoAlbum, "", true},
		{library.ItemKindPhotoAlbum, library.ItemKindPhotoAlbum, true},
		{library.ItemKindPhotoAlbum, library.ItemKindPhoto, false},
		{library.ItemKindPhotoAlbum, library.ItemKindShow, false},
		{library.ItemKindPhoto, library.ItemKindPhotoAlbum, true},
		{library.ItemKindPhoto, "", true},
		{library.ItemKindPhoto, library.ItemKindShow, false},
		{library.ItemKindPhoto, library.ItemKindPhoto, false},
	}

	for _, testCase := range cases {
		if got := library.ValidParent(testCase.child, testCase.parent); got != testCase.want {
			t.Errorf("ValidParent(%q, %q) = %v, want %v", testCase.child, testCase.parent, got, testCase.want)
		}
	}
}

func TestPhotoLibraryRejectsUnknownKind(t *testing.T) {
	t.Parallel()

	_, err := library.ItemKindForLibrary("nope")
	if !errors.Is(err, domain.ErrInvalid) {
		t.Errorf("error = %v, want ErrInvalid", err)
	}
}

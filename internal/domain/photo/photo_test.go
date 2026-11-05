package photo_test

import (
	"errors"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/photo"
)

func TestCaptureTime(t *testing.T) {
	t.Parallel()

	taken := time.Date(2021, 5, 4, 12, 0, 0, 0, time.UTC)
	modified := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)

	if got := photo.CaptureTime(photo.Info{TakenAt: taken}, modified); !got.Equal(taken) {
		t.Errorf("with EXIF time = %v, want %v", got, taken)
	}

	if got := photo.CaptureTime(photo.Info{}, modified); !got.Equal(modified) {
		t.Errorf("without EXIF time = %v, want %v", got, modified)
	}
}

func TestInfoValidate(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		info photo.Info
		ok   bool
	}{
		{"empty", photo.Info{}, true},
		{"full", photo.Info{
			Orientation: 6, ISO: 100, FocalLengthMM: 35, ExposureSeconds: 0.01, Width: 4000, Height: 3000,
			GPS: &photo.Coordinates{Latitude: -33.9, Longitude: 151.2},
		}, true},
		{"orientation 8", photo.Info{Orientation: 8}, true},
		{"orientation 9", photo.Info{Orientation: 9}, false},
		{"orientation negative", photo.Info{Orientation: -1}, false},
		{"NaN exposure", photo.Info{ExposureSeconds: math.NaN()}, false},
		{"infinite focal length", photo.Info{FocalLengthMM: math.Inf(1)}, false},
		{"NaN latitude", photo.Info{GPS: &photo.Coordinates{Latitude: math.NaN()}}, false},
		{"infinite longitude", photo.Info{GPS: &photo.Coordinates{Longitude: math.Inf(-1)}}, false},
		{"negative ISO", photo.Info{ISO: -1}, false},
		{"negative exposure", photo.Info{ExposureSeconds: -1}, false},
		{"negative focal length", photo.Info{FocalLengthMM: -1}, false},
		{"f-number", photo.Info{FNumber: 1.8}, true},
		{"negative f-number", photo.Info{FNumber: -2.8}, false},
		{"NaN f-number", photo.Info{FNumber: math.NaN()}, false},
		{"negative width", photo.Info{Width: -1}, false},
		{"negative height", photo.Info{Height: -1}, false},
		{"latitude too high", photo.Info{GPS: &photo.Coordinates{Latitude: 90.1}}, false},
		{"latitude too low", photo.Info{GPS: &photo.Coordinates{Latitude: -90.1}}, false},
		{"longitude too high", photo.Info{GPS: &photo.Coordinates{Longitude: 180.1}}, false},
		{"longitude too low", photo.Info{GPS: &photo.Coordinates{Longitude: -180.1}}, false},
		{"poles and date line", photo.Info{GPS: &photo.Coordinates{Latitude: 90, Longitude: -180}}, true},
	}

	for _, testCase := range cases {
		err := testCase.info.Validate()
		if (err == nil) != testCase.ok {
			t.Errorf("%s: error = %v, want ok=%v", testCase.name, err, testCase.ok)
		}

		if err != nil && !errors.Is(err, domain.ErrInvalid) {
			t.Errorf("%s: error %v does not wrap ErrInvalid", testCase.name, err)
		}
	}
}

func TestDisplaySize(t *testing.T) {
	t.Parallel()

	for orientation := 0; orientation <= 8; orientation++ {
		width, height := photo.Info{Orientation: orientation, Width: 400, Height: 300}.DisplaySize()
		swapped := orientation >= 5

		if swapped && (width != 300 || height != 400) || !swapped && (width != 400 || height != 300) {
			t.Errorf("orientation %d: %dx%d", orientation, width, height)
		}
	}
}

func TestSort(t *testing.T) {
	t.Parallel()

	early := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)
	entries := []photo.Entry{
		{Name: "b.jpg", CaptureTime: late},
		{Name: "B.jpg", CaptureTime: early},
		{Name: "a.jpg", CaptureTime: early},
		{Name: "A.jpg", CaptureTime: early},
		{Name: "c.jpg", CaptureTime: early},
	}

	photo.Sort(entries)

	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name)
	}

	want := []string{"A.jpg", "a.jpg", "B.jpg", "c.jpg", "b.jpg"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestGroupFiles(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		files []string
		want  []photo.Group
	}{
		{"raw joins jpeg", []string{"IMG_1.CR2", "IMG_1.JPG"}, []photo.Group{{Primary: "IMG_1.JPG", Alternates: []string{"IMG_1.CR2"}}}},
		{"stem is case-insensitive", []string{"img_1.jpg", "IMG_1.dng"}, []photo.Group{{Primary: "img_1.jpg", Alternates: []string{"IMG_1.dng"}}}},
		{"lone raw", []string{"a.nef"}, []photo.Group{{Primary: "a.nef"}}},
		{"two raws one jpeg", []string{"a.arw", "a.dng", "a.jpg"}, []photo.Group{{Primary: "a.jpg", Alternates: []string{"a.arw", "a.dng"}}}},
		{"web beats heic", []string{"a.heic", "a.jpg", "a.cr3"}, []photo.Group{
			{Primary: "a.heic"}, {Primary: "a.jpg", Alternates: []string{"a.cr3"}},
		}},
		{"heic beats tiff", []string{"a.tif", "a.heic", "a.raf"}, []photo.Group{
			{Primary: "a.heic", Alternates: []string{"a.raf"}}, {Primary: "a.tif"},
		}},
		{"tiff partner", []string{"a.tiff", "a.orf"}, []photo.Group{{Primary: "a.tiff", Alternates: []string{"a.orf"}}}},
		{"equal rank by name", []string{"a.png", "a.jpg", "a.rw2"}, []photo.Group{
			{Primary: "a.jpg", Alternates: []string{"a.rw2"}}, {Primary: "a.png"},
		}},
		{"video not paired", []string{"a.mp4", "a.jpg", "a.cr2"}, []photo.Group{
			{Primary: "a.jpg", Alternates: []string{"a.cr2"}}, {Primary: "a.mp4"},
		}},
		{"other files skipped", []string{"notes.txt", "a.nfo", "b.gif"}, []photo.Group{{Primary: "b.gif"}}},
		{"sorted by primary", []string{"c.jpg", "B.jpg", "a.jpg"}, []photo.Group{
			{Primary: "a.jpg"}, {Primary: "B.jpg"}, {Primary: "c.jpg"},
		}},
		{"empty", nil, []photo.Group{}},
	}

	for _, testCase := range cases {
		got := photo.GroupFiles(testCase.files)
		if !reflect.DeepEqual(got, testCase.want) {
			t.Errorf("%s: got %+v, want %+v", testCase.name, got, testCase.want)
		}
	}
}

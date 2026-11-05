package photo_test

import (
	"testing"
	"time"

	"github.com/magpeek/mivo/internal/domain/photo"
)

func TestThumbnailStateFor(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		file        string
		unsupported bool
		want        photo.ThumbnailState
	}{
		{"JPEG with metadata", "a.jpg", false, photo.ThumbnailReady},
		{"PNG without readable metadata", "a.png", true, photo.ThumbnailReady},
		{"HEIC without readable metadata", "a.HEIC", true, photo.ThumbnailReady},
		{"TIFF", "a.tif", true, photo.ThumbnailReady},
		{"RAW with metadata", "a.cr2", false, photo.ThumbnailReady},
		{"RAW in an unreadable layout", "a.NEF", true, photo.ThumbnailUnsupported},
		{"video", "a.mp4", true, photo.ThumbnailReady},
	}

	for _, testCase := range cases {
		if got := photo.ThumbnailStateFor(testCase.file, testCase.unsupported); got != testCase.want {
			t.Errorf("%s: ThumbnailStateFor(%q, %v) = %q, want %q",
				testCase.name, testCase.file, testCase.unsupported, got, testCase.want)
		}
	}
}

func TestWallClock(t *testing.T) {
	t.Parallel()

	berlin := time.FixedZone("CEST", 2*60*60)
	instant := time.Date(2024, 7, 1, 10, 30, 15, 500, time.UTC)

	cases := []struct {
		name string
		zone *time.Location
		want time.Time
	}{
		{"server zone ahead of UTC", berlin, time.Date(2024, 7, 1, 12, 30, 15, 500, time.UTC)},
		{"UTC server", time.UTC, instant},
		{"no zone means UTC", nil, instant},
	}

	for _, testCase := range cases {
		got := photo.WallClock(instant, testCase.zone)
		if !got.Equal(testCase.want) || got.Location() != time.UTC {
			t.Errorf("%s: WallClock() = %v, want %v in UTC", testCase.name, got, testCase.want)
		}
	}
}

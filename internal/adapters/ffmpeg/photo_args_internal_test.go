package ffmpeg

import (
	"errors"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/domain"
	"github.com/magpeek/mivo/internal/domain/artwork"
)

func TestOrientationFilters(t *testing.T) {
	t.Parallel()

	want := map[int]string{
		0: "", 1: "", 2: "hflip", 3: "hflip,vflip", 4: "vflip",
		5: "transpose=0", 6: "transpose=1", 7: "transpose=3", 8: "transpose=2", 9: "",
	}

	for orientation, filters := range want {
		if got := strings.Join(orientationFilters(orientation), ","); got != filters {
			t.Errorf("orientation %d = %q, want %q", orientation, got, filters)
		}
	}
}

// argumentValue returns the argument after flag, or "" without flag.
func argumentValue(arguments []string, flag string) string {
	index := slices.Index(arguments, flag)
	if index < 0 || index+1 >= len(arguments) {
		return ""
	}

	return arguments[index+1]
}

// assertGridArguments expects the complex filter graph of a tile grid mapped as output, or
// none for wantComplex "", and the allocation bound of a HEIF or AVIF canvas exactly when
// wantAllocationBound is set.
func assertGridArguments(t *testing.T, arguments []string, wantComplex string, wantAllocationBound bool) {
	t.Helper()

	if got := argumentValue(arguments, "-filter_complex"); got != wantComplex {
		t.Errorf("-filter_complex = %q, want %q", got, wantComplex)
	}

	if wantComplex != "" {
		if got := argumentValue(arguments, "-map"); got != gridOutputLabel {
			t.Errorf("-map = %q, want %q", got, gridOutputLabel)
		}
	}

	wantAllocation := ""
	if wantAllocationBound {
		wantAllocation = strconv.Itoa(maxGridAllocationBytes)
	}

	if got := argumentValue(arguments, "-max_alloc"); got != wantAllocation {
		t.Errorf("-max_alloc = %q, want %q", got, wantAllocation)
	}
}

func TestPhotoArgumentsByInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "photo.jpg")
	scale := "scale='min(iw,320)':-2:flags=lanczos,format=yuvj420p"

	tests := []struct {
		name          string
		job           photoJob
		wantFilter    string
		wantComplex   string
		wantAllocated bool
		wantMap       bool
		wantNoRotate  bool
		wantPipeInput bool
	}{
		{
			name:       "file with EXIF orientation",
			job:        photoJob{input: inputFileWithEXIF, path: path, width: artwork.Width320, format: artwork.FormatJPEG, orientation: 6},
			wantFilter: "transpose=1," + scale, wantMap: true, wantNoRotate: true,
		},
		{
			name:          "HEIF with container rotation ignores EXIF and bounds allocations",
			job:           photoJob{input: inputFileWithContainerRotation, path: path, width: artwork.Width320, format: artwork.FormatJPEG, orientation: 6},
			wantFilter:    scale,
			wantAllocated: true,
		},
		{
			name:          "HEIF tile grid scales the assembled canvas in the complex graph",
			job:           photoJob{input: inputTileGrid, path: path, width: artwork.Width320, format: artwork.FormatJPEG, orientation: 6},
			wantMap:       true,
			wantComplex:   "[0:g:0]" + scale + "[photo]",
			wantAllocated: true,
		},
		{
			name:       "preview from the pipe",
			job:        photoJob{input: inputPreview, width: artwork.Width320, format: artwork.FormatJPEG, orientation: 3},
			wantFilter: "hflip,vflip," + scale, wantMap: true, wantNoRotate: true, wantPipeInput: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			arguments, err := photoArguments(test.job)
			if err != nil {
				t.Fatalf("photoArguments: %v", err)
			}

			checks := []struct {
				what      string
				got, want bool
			}{
				{"-map", slices.Contains(arguments, "-map"), test.wantMap},
				{"-noautorotate", slices.Contains(arguments, noAutorotateOption), test.wantNoRotate},
				{"pipe input", argumentValue(arguments, "-i") == standardInput, test.wantPipeInput},
				{"-max_pixels", argumentValue(arguments, "-max_pixels") != "", true},
				{"protocol whitelist", argumentValue(arguments, protocolWhitelistOption) != "", true},
			}
			for _, check := range checks {
				if check.got != check.want {
					t.Errorf("%s present = %v, want %v in %q", check.what, check.got, check.want, arguments)
				}
			}

			if got := argumentValue(arguments, "-vf"); got != test.wantFilter {
				t.Errorf("-vf = %q, want %q", got, test.wantFilter)
			}

			assertGridArguments(t, arguments, test.wantComplex, test.wantAllocated)
		})
	}
}

func TestPhotoArgumentsRefusals(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()

	tests := []struct {
		name string
		job  photoJob
		want error
	}{
		{"zero width", photoJob{path: filepath.Join(directory, "a.jpg"), format: artwork.FormatJPEG}, domain.ErrInvalid},
		{"relative path", photoJob{path: "a.jpg", width: artwork.Width160, format: artwork.FormatJPEG}, domain.ErrInvalid},
		{"sequence marker", photoJob{path: filepath.Join(directory, "a%d.jpg"), width: artwork.Width160, format: artwork.FormatJPEG}, domain.ErrUnsupported},
	}

	for _, test := range tests {
		if _, err := photoArguments(test.job); !errors.Is(err, test.want) {
			t.Errorf("%s: err = %v, want %v", test.name, err, test.want)
		}
	}
}

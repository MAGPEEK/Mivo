package ffmpeg

import (
	"errors"
	"path/filepath"
	"slices"
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

func TestPhotoArgumentsByInput(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "photo.jpg")
	scale := "scale='min(iw,320)':-2:flags=lanczos,format=yuvj420p"

	tests := []struct {
		name          string
		job           photoJob
		wantFilter    string
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
			name:       "HEIF with container rotation ignores EXIF",
			job:        photoJob{input: inputFileWithContainerRotation, path: path, width: artwork.Width320, format: artwork.FormatJPEG, orientation: 6},
			wantFilter: scale,
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

package photometa

import (
	"bytes"
	"context"
	"testing"
)

// fuzzParser runs both entry points on data: they must not panic, and a successful read
// must yield an Info the domain accepts and a preview within the size bound.
func fuzzParser(t *testing.T, data []byte) {
	t.Helper()

	ctx := context.Background()

	info, err := ReadInfoFrom(ctx, bytes.NewReader(data), int64(len(data)))
	if err == nil {
		if validateErr := info.Validate(); validateErr != nil {
			t.Fatalf("ReadInfoFrom returned an invalid Info %+v: %v", info, validateErr)
		}
	}

	preview, err := ExtractPreviewFrom(ctx, bytes.NewReader(data), int64(len(data)))
	if err == nil && (len(preview) == 0 || len(preview) > maxPreviewBytes || len(preview) > len(data)) {
		t.Fatalf("ExtractPreviewFrom returned %d bytes from a %d-byte file", len(preview), len(data))
	}
}

func withThumbnailCamera() cameraOptions {
	options := defaultCamera()
	options.thumbnail = paddedJPEG(4, markerSOF0)

	return options
}

func FuzzJPEG(f *testing.F) {
	f.Add(jpegWithExif(cameraTIFF(withThumbnailCamera()), 640, 480))
	f.Add(testJPEG(1, 1, markerSOF2))
	f.Fuzz(fuzzParser)
}

func FuzzTIFF(f *testing.F) {
	f.Add(cameraTIFF(withThumbnailCamera()))
	f.Add(loopTIFF())
	f.Add(chainLoopTIFF(paddedJPEG(4, markerSOF0)))
	f.Fuzz(fuzzParser)
}

func FuzzHEIF(f *testing.F) {
	f.Add(heifFile("heic", cameraTIFF(withThumbnailCamera()), 0, false))
	f.Add(heifFile("avif", cameraTIFF(defaultCamera()), 2, false))
	f.Add(heifFile("heic", cameraTIFF(defaultCamera()), 1, true))
	f.Fuzz(fuzzParser)
}

func FuzzCR3(f *testing.F) {
	tiff := cameraTIFF(defaultCamera())
	f.Add(cr3File(tiff, tiff, tiff, paddedJPEG(4, markerSOF0)))
	f.Fuzz(fuzzParser)
}

func FuzzRAF(f *testing.F) {
	f.Add(rafFile(jpegWithExif(cameraTIFF(withThumbnailCamera()), 1, 1)))
	f.Fuzz(fuzzParser)
}

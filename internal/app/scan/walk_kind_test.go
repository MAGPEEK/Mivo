package scan_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// Until M3-15f a photos library keeps its images as image sidecars, as before the walker
// classified by library kind; a photo class would be refused by the sidecar catalog.
func TestPhotosLibraryKeepsImagesAsImageSidecars(t *testing.T) {
	t.Parallel()

	const photosID, photosRoot = "lib-photos", "photos"

	h := newHarness(t)
	h.libraries.set(oneFolder(photosID, "Photos", photosRoot, library.KindPhotos))
	h.start(t)

	h.walker.set(photosRoot, walkResult{files: []library.FileState{fileAt("Trip/beach.jpg", 1)}})
	status := h.scan(t, photosID)

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

	sidecars, err := h.catalogV2.SidecarStates(t.Context(), photosID)
	if err != nil {
		t.Fatalf("SidecarStates() error = %v", err)
	}

	if len(sidecars) != 1 || sidecars[0].Class != library.FileClassImage {
		t.Errorf("sidecars = %+v, want the image as an image sidecar", sidecars)
	}
}

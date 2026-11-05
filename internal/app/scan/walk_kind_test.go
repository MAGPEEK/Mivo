package scan_test

import (
	"testing"

	"github.com/magpeek/mivo/internal/app/scan"
	"github.com/magpeek/mivo/internal/domain/library"
)

// A photos library places its images as photos (M3-15f); they are no image sidecars.
func TestPhotosLibraryPlacesImagesInsteadOfKeepingSidecars(t *testing.T) {
	t.Parallel()

	h, _, _ := newPhotoHarness(t)
	h.start(t)

	status := h.scanPhotos(t, fileAt("Trip/beach.jpg", 1))

	assertState(t, status, scan.StateIdle, scan.ErrorCodeNone)

	sidecars, err := h.catalogV2.SidecarStates(t.Context(), photosID)
	if err != nil {
		t.Fatalf("SidecarStates() error = %v", err)
	}

	if len(sidecars) != 0 || h.catalogV2.count(library.ItemKindPhoto) != 1 {
		t.Errorf("sidecars = %+v, photos = %d, want the image as one photo", sidecars, h.catalogV2.count(library.ItemKindPhoto))
	}
}

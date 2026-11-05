package restapi

import (
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// TestAudiobookValuesMapToTheContract pins the audiobook values API 1.8.0 added (task
// M3-14b): a stored audiobook, its library and its author and narrator credits are sent
// instead of failing as values outside the contract.
func TestAudiobookValuesMapToTheContract(t *testing.T) {
	t.Parallel()

	if got, err := itemKindResponse(library.ItemKindAudiobook); err != nil || got != apigen.ItemKindAudiobook {
		t.Errorf("itemKindResponse(audiobook) = %q, %v; want %q", got, err, apigen.ItemKindAudiobook)
	}

	if got, err := libraryKindResponse(library.KindAudiobooks); err != nil || got != apigen.LibraryKindAudiobooks {
		t.Errorf("libraryKindResponse(audiobooks) = %q, %v; want %q", got, err, apigen.LibraryKindAudiobooks)
	}

	roles := map[metadata.Role]apigen.PersonRole{
		metadata.RoleAuthor:   apigen.PersonRoleAuthor,
		metadata.RoleNarrator: apigen.PersonRoleNarrator,
	}

	for role, want := range roles {
		if got, err := personRoleResponse(role); err != nil || got != want {
			t.Errorf("personRoleResponse(%q) = %q, %v; want %q", role, got, err, want)
		}
	}
}

// TestPhotoValuesMapToTheContract pins the photo values API 1.9.0 added (task M3-15b): a
// stored photo album, photo and photos library are sent instead of failing as values
// outside the contract.
func TestPhotoValuesMapToTheContract(t *testing.T) {
	t.Parallel()

	kinds := map[library.ItemKind]apigen.ItemKind{
		library.ItemKindPhotoAlbum: apigen.ItemKindPhotoAlbum,
		library.ItemKindPhoto:      apigen.ItemKindPhoto,
	}

	for kind, want := range kinds {
		if got, err := itemKindResponse(kind); err != nil || got != want {
			t.Errorf("itemKindResponse(%q) = %q, %v; want %q", kind, got, err, want)
		}
	}

	if got, err := libraryKindResponse(library.KindPhotos); err != nil || got != apigen.LibraryKindPhotos {
		t.Errorf("libraryKindResponse(photos) = %q, %v; want %q", got, err, apigen.LibraryKindPhotos)
	}
}

// TestKindsOutsideTheMappedContractFail keeps the kinds whose mapping a later task adds
// (music: M3-13g) failing instead of leaking unlisted values.
func TestKindsOutsideTheMappedContractFail(t *testing.T) {
	t.Parallel()

	for _, kind := range []library.ItemKind{library.ItemKindTrack, "mixed"} {
		if _, err := itemKindResponse(kind); err == nil {
			t.Errorf("itemKindResponse(%q) succeeded, want an error", kind)
		}
	}

	for _, kind := range []library.Kind{library.KindMusic, "mixed"} {
		if _, err := libraryKindResponse(kind); err == nil {
			t.Errorf("libraryKindResponse(%q) succeeded, want an error", kind)
		}
	}
}

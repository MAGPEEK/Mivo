package restapi

import (
	"github.com/magpeek/mivo/internal/domain/library"
)

// firstDiscNumber is the lowest disc number the contract allows (ItemSummary.discNumber).
const firstDiscNumber = 1

// showsSquareArtwork reports whether the kind carries its image in the 1:1 square slot of
// the contract: the cover of an album and the image of an artist. The scan stores both as
// poster artwork until artwork has a square kind (BACKLOG m3-13e notes), so the adapter
// moves the poster into the square slot instead of sending a 2:3 poster the client would crop.
func showsSquareArtwork(kind library.ItemKind) bool {
	return kind == library.ItemKindAlbum || kind == library.ItemKindArtist
}

// discNumberResponse omits an unknown disc and a stored disc below 1, which the contract
// does not allow (a disc tag of 0 means "none").
func discNumberResponse(disc library.OptionalNumber) *int32 {
	value, present := disc.Get()
	if !present || value < firstDiscNumber {
		return nil
	}

	return optionalNumber(disc)
}

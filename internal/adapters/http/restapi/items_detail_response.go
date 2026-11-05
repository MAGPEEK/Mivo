package restapi

import (
	"fmt"
	"math"
	"time"
	"uuid"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/magpeek/mivo/internal/adapters/http/apigen"
	applibrary "github.com/magpeek/mivo/internal/app/library"
	"github.com/magpeek/mivo/internal/domain/artwork"
	"github.com/magpeek/mivo/internal/domain/library"
	"github.com/magpeek/mivo/internal/domain/metadata"
)

// itemIdentifiers are the parsed IDs and the kind every item response starts with.
type itemIdentifiers struct {
	id        uuid.UUID
	libraryID uuid.UUID
	parentID  *uuid.UUID
	kind      apigen.ItemKind
}

func itemIdentity(item library.Item) (itemIdentifiers, error) {
	id, err := parseStoredID("item", item.ID)
	if err != nil {
		return itemIdentifiers{}, err
	}

	libraryID, err := parseStoredID("library", item.LibraryID)
	if err != nil {
		return itemIdentifiers{}, fmt.Errorf("item %s: %w", item.ID, err)
	}

	parentID, err := optionalStoredID("parent", item.ParentID)
	if err != nil {
		return itemIdentifiers{}, fmt.Errorf("item %s: %w", item.ID, err)
	}

	kind, err := itemKindResponse(item.Kind)
	if err != nil {
		return itemIdentifiers{}, fmt.Errorf("item %s: %w", item.ID, err)
	}

	return itemIdentifiers{id: id, libraryID: libraryID, parentID: parentID, kind: kind}, nil
}

// itemKindResponse maps every kind explicitly. A kind this adapter does not know cannot be
// sent as another one, so it is a server fault (the stored row is not readable) instead of
// a cast that would leak an unlisted value.
func itemKindResponse(kind library.ItemKind) (apigen.ItemKind, error) {
	switch kind {
	case library.ItemKindMovie:
		return apigen.ItemKindMovie, nil
	case library.ItemKindShow:
		return apigen.ItemKindShow, nil
	case library.ItemKindSeason:
		return apigen.ItemKindSeason, nil
	case library.ItemKindEpisode:
		return apigen.ItemKindEpisode, nil
	case library.ItemKindExtra:
		return apigen.ItemKindExtra, nil
	case library.ItemKindArtist:
		return apigen.ItemKindArtist, nil
	case library.ItemKindAlbum:
		return apigen.ItemKindAlbum, nil
	case library.ItemKindTrack:
		return apigen.ItemKindTrack, nil
	case library.ItemKindPhotoAlbum:
		return apigen.ItemKindPhotoAlbum, nil
	case library.ItemKindPhoto:
		return apigen.ItemKindPhoto, nil
	case library.ItemKindAudiobook:
		return apigen.ItemKindAudiobook, nil
	default:
		return "", fmt.Errorf("item kind %q not in the contract", kind)
	}
}

// extraTypeResponse omits the empty type of non-extras and sends a type this adapter does
// not know as "other", which the contract defines for every unclassified extra.
func extraTypeResponse(extraType library.ExtraType) *apigen.ExtraType {
	var response apigen.ExtraType

	switch extraType {
	case "":
		return nil
	case library.ExtraTypeTrailer:
		response = apigen.ExtraTypeTrailer
	case library.ExtraTypeFeaturette:
		response = apigen.ExtraTypeFeaturette
	case library.ExtraTypeBehindTheScenes:
		response = apigen.ExtraTypeBehindTheScenes
	case library.ExtraTypeDeletedScene:
		response = apigen.ExtraTypeDeletedScene
	case library.ExtraTypeInterview:
		response = apigen.ExtraTypeInterview
	case library.ExtraTypeScene:
		response = apigen.ExtraTypeScene
	case library.ExtraTypeShort:
		response = apigen.ExtraTypeShort
	case library.ExtraTypeOther:
		response = apigen.ExtraTypeOther
	default:
		response = apigen.ExtraTypeOther
	}

	return &response
}

// optionalNumber omits an absent number. Unlike optionalInt32 it sends 0, because season 0
// (specials) is a real number.
func optionalNumber(number library.OptionalNumber) *int32 {
	value, present := number.Get()
	if !present || value < 0 || value > math.MaxInt32 {
		return nil
	}

	converted := int32(value)

	return &converted
}

// optionalStoredID omits an empty ID and fails on a malformed one.
func optionalStoredID(entity, text string) (*uuid.UUID, error) {
	if text == "" {
		return nil, nil //nolint:nilnil // nil pointer is the "absent" value of an optional field
	}

	id, err := parseStoredID(entity, text)
	if err != nil {
		return nil, err
	}

	return &id, nil
}

// imageRefResponse omits a missing artwork; the tag is the stored one.
func imageRefResponse(ref *artwork.Ref) (*apigen.ImageRef, error) {
	if ref == nil {
		return nil, nil //nolint:nilnil // nil pointer is the "absent" value of an optional field
	}

	id, err := parseStoredID("image", ref.ID)
	if err != nil {
		return nil, err
	}

	return &apigen.ImageRef{ID: id, Tag: ref.Tag}, nil
}

func itemDetailResponse(detail applibrary.ItemDetail) (apigen.ItemDetail, error) {
	identity, err := itemIdentity(detail.Item)
	if err != nil {
		return apigen.ItemDetail{}, err
	}

	response := apigen.ItemDetail{
		ID:         identity.id,
		LibraryID:  identity.libraryID,
		Kind:       identity.kind,
		Title:      detail.Item.Title,
		Year:       detailYear(detail),
		Number:     optionalNumber(detail.Item.Number),
		NumberEnd:  optionalNumber(detail.Item.NumberEnd),
		DiscNumber: discNumberResponse(detail.Item.DiscNumber),
		ChildCount: optionalInt32(detail.ChildCount),
		LeafCount:  optionalInt32(detail.LeafCount),
	}

	applyMetadata(&response, detail.Metadata)

	steps := []func(*apigen.ItemDetail, applibrary.ItemDetail) error{
		applyFiles, applyNavigation, applyAncestors, applyArtwork, applyExtras,
	}

	for _, step := range steps {
		err := step(&response, detail)
		if err != nil {
			return apigen.ItemDetail{}, fmt.Errorf("item %s: %w", detail.Item.ID, err)
		}
	}

	return response, nil
}

// detailYear prefers the year of the catalog item and falls back to the merged metadata.
func detailYear(detail applibrary.ItemDetail) *int32 {
	if year := optionalNumber(detail.Item.Year); year != nil {
		return year
	}

	return optionalInt32(detail.Metadata.Year)
}

// applyMetadata copies the descriptive text blocks of the merged record.
func applyMetadata(response *apigen.ItemDetail, record metadata.Record) {
	response.OriginalTitle = optionalString(record.OriginalTitle)
	response.Summary = optionalString(record.Summary)
	response.Tagline = optionalString(record.Tagline)
	response.ContentRating = optionalString(record.ContentRating)
	response.Genres = optionalStrings(record.Genres)
	response.Studios = optionalStrings(record.Studios)
	response.Countries = optionalStrings(record.Countries)
	response.ReleaseDate = releaseDateResponse(record.ReleaseDate)
	response.ProviderIds = providerIDsResponse(record.ProviderIDs)
}

// optionalStrings omits an empty list.
func optionalStrings(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}

	copied := append([]string(nil), values...)

	return &copied
}

// releaseDateResponse sends a calendar date without time or zone (format: date).
func releaseDateResponse(date metadata.Date) *openapi_types.Date {
	if date.IsZero() {
		return nil
	}

	moment := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)

	return &openapi_types.Date{Time: moment}
}

func providerIDsResponse(ids []metadata.ProviderID) *apigen.ProviderIds {
	var response apigen.ProviderIds

	found := false

	for _, id := range ids {
		switch id.Provider {
		case metadata.ProviderIMDb:
			response.Imdb = optionalString(id.Value)
		case metadata.ProviderTMDB:
			response.Tmdb = optionalString(id.Value)
		case metadata.ProviderTVDB:
			response.Tvdb = optionalString(id.Value)
		default:
			continue
		}

		found = true
	}

	if !found {
		return nil
	}

	return &response
}

// personRoleResponse fails for a role the contract does not list: a stored credit the
// domain accepted but this adapter cannot describe is a server fault.
func personRoleResponse(role metadata.Role) (apigen.PersonRole, error) {
	switch role {
	case metadata.RoleActor:
		return apigen.PersonRoleActor, nil
	case metadata.RoleDirector:
		return apigen.PersonRoleDirector, nil
	case metadata.RoleWriter:
		return apigen.PersonRoleWriter, nil
	case metadata.RoleProducer:
		return apigen.PersonRoleProducer, nil
	case metadata.RoleGuestStar:
		return apigen.PersonRoleGuestStar, nil
	case metadata.RoleAuthor:
		return apigen.PersonRoleAuthor, nil
	case metadata.RoleNarrator:
		return apigen.PersonRoleNarrator, nil
	default:
		return "", fmt.Errorf("person role %q not in the contract", role)
	}
}

func peopleResponse(credits []metadata.Credit) (*[]apigen.ItemPerson, error) {
	if len(credits) == 0 {
		return nil, nil //nolint:nilnil // nil pointer is the "absent" value of an optional field
	}

	people := make([]apigen.ItemPerson, 0, len(credits))

	for _, credit := range credits {
		role, err := personRoleResponse(credit.Role)
		if err != nil {
			return nil, err
		}

		people = append(people, apigen.ItemPerson{Name: credit.Name, Role: role, Character: optionalString(credit.Character)})
	}

	return &people, nil
}

// applyFiles prefers the milestone-3 files with version label and part number and falls back
// to the plain file list of the milestone-2 read path.
func applyFiles(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	itemFiles := detail.ItemFiles
	if len(itemFiles) == 0 {
		itemFiles = make([]applibrary.ItemFile, 0, len(detail.Files))
		for _, file := range detail.Files {
			itemFiles = append(itemFiles, applibrary.ItemFile{File: file})
		}
	}

	response.Files = make([]apigen.MediaFile, 0, len(itemFiles))

	for _, itemFile := range itemFiles {
		file, err := itemFileResponse(itemFile)
		if err != nil {
			return err
		}

		response.Files = append(response.Files, file)
	}

	return nil
}

// applyNavigation sets the siblings and the item Play starts, and the people block.
func applyNavigation(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	var err error

	if response.PreviousID, err = optionalStoredID("previous sibling", detail.PreviousID); err != nil {
		return err
	}

	if response.NextID, err = optionalStoredID("next sibling", detail.NextID); err != nil {
		return err
	}

	if response.PlayItemID, err = optionalStoredID("play item", detail.PlayItemID); err != nil {
		return err
	}

	response.People, err = peopleResponse(detail.Metadata.People)

	return err
}

func applyAncestors(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	if len(detail.Ancestors) == 0 {
		return nil
	}

	ancestors := make([]apigen.ItemAncestor, 0, len(detail.Ancestors))

	for _, ref := range detail.Ancestors {
		id, err := parseStoredID("ancestor", ref.ID)
		if err != nil {
			return err
		}

		kind, err := itemKindResponse(ref.Kind)
		if err != nil {
			return fmt.Errorf("ancestor %s: %w", ref.ID, err)
		}

		ancestors = append(ancestors, apigen.ItemAncestor{ID: id, Kind: kind, Title: ref.Title, Number: optionalNumber(ref.Number)})
	}

	response.Ancestors = &ancestors

	return nil
}

// applyArtwork maps the artwork kinds one to one. A kind the contract does not list is a
// server fault, like an unknown item kind.
func applyArtwork(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	if len(detail.Artwork) == 0 {
		return nil
	}

	var images apigen.Artwork

	for kind, ref := range detail.Artwork {
		image, err := imageRefResponse(&ref)
		if err != nil {
			return fmt.Errorf("artwork %s: %w", kind, err)
		}

		target, err := artworkTarget(&images, kind, detail.Item.Kind)
		if err != nil {
			return err
		}

		*target = image
	}

	response.Artwork = &images

	return nil
}

// artworkTarget picks the slot of an artwork kind; the poster of an album or artist goes to
// the square slot (see showsSquareArtwork).
func artworkTarget(images *apigen.Artwork, kind artwork.Kind, itemKind library.ItemKind) (**apigen.ImageRef, error) {
	switch kind {
	case artwork.KindPoster:
		if showsSquareArtwork(itemKind) {
			return &images.Square, nil
		}

		return &images.Poster, nil
	case artwork.KindBackdrop:
		return &images.Backdrop, nil
	case artwork.KindThumb:
		return &images.Thumb, nil
	case artwork.KindLogo:
		return &images.Logo, nil
	case artwork.KindBanner:
		return &images.Banner, nil
	default:
		return nil, fmt.Errorf("artwork kind %q not in the contract", kind)
	}
}

func applyExtras(response *apigen.ItemDetail, detail applibrary.ItemDetail) error {
	if len(detail.Extras) == 0 {
		return nil
	}

	extras := make([]apigen.ItemSummary, 0, len(detail.Extras))

	for _, summary := range detail.Extras {
		extra, err := itemSummaryResponse(summary)
		if err != nil {
			return err
		}

		extras = append(extras, extra)
	}

	response.Extras = &extras

	return nil
}

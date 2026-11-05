package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// numberBound is the expected type, format and bounds of one numeric property.
type numberBound struct {
	kind         string
	format       string
	minimum      *float64
	maximum      *float64
	exclusiveMin bool
}

func bound(value float64) *float64 { return &value }

// photoNumberBounds lists the numeric fields of the EXIF block: what is absent is omitted,
// so present values are positive (or within the globe and the EXIF orientation range).
func photoNumberBounds() map[string]numberBound {
	positiveDouble := numberBound{kind: openapi3.TypeNumber, format: "double", minimum: bound(0), exclusiveMin: true}
	positiveInteger := numberBound{kind: openapi3.TypeInteger, format: "int32", minimum: bound(1)}

	return map[string]numberBound{
		"ItemPhoto.exposureTime":  positiveDouble,
		"ItemPhoto.fNumber":       positiveDouble,
		"ItemPhoto.focalLengthMm": positiveDouble,
		"ItemPhoto.iso":           positiveInteger,
		"ItemPhoto.width":         positiveInteger,
		"ItemPhoto.height":        positiveInteger,
		"ItemPhoto.orientation":   {kind: openapi3.TypeInteger, format: "int32", minimum: bound(1), maximum: bound(8)},
		"ItemPhoto.latitude":      {kind: openapi3.TypeNumber, format: "double", minimum: bound(-90), maximum: bound(90)},
		"ItemPhoto.longitude":     {kind: openapi3.TypeNumber, format: "double", minimum: bound(-180), maximum: bound(180)},
	}
}

// TestItemPhotoNumberFormats pins the EXIF block of a photo (task M3-15b).
func TestItemPhotoNumberFormats(t *testing.T) {
	spec := loadSpec(t)

	for location, want := range photoNumberBounds() {
		property := propertySchema(spec, location)
		if property == nil {
			t.Errorf("%s is not declared", location)

			continue
		}

		schema := property.Value
		got := numberBound{
			kind: firstType(schema), format: schema.Format, minimum: schema.Min, maximum: schema.Max,
			exclusiveMin: schema.ExclusiveMin.Bool != nil && *schema.ExclusiveMin.Bool,
		}

		if describeBound(got) != describeBound(want) {
			t.Errorf("%s = %s, want %s", location, describeBound(got), describeBound(want))
		}
	}
}

func firstType(schema *openapi3.Schema) string {
	if schema.Type == nil || len(*schema.Type) == 0 {
		return ""
	}

	return (*schema.Type)[0]
}

func describeBound(value numberBound) string {
	return fmt.Sprintf("%s/%s min %s max %s exclusive %t",
		value.kind, value.format, describeLimit(value.minimum), describeLimit(value.maximum), value.exclusiveMin)
}

func describeLimit(limit *float64) string {
	if limit == nil {
		return "none"
	}

	return fmt.Sprint(*limit)
}

// TestPhotoTextsAndTimes pins the bounded EXIF texts and the capture time the timeline
// groups by, in the detail and in the listing.
func TestPhotoTextsAndTimes(t *testing.T) {
	spec := loadSpec(t)

	const wantMaxTextLength = 200

	for _, location := range []string{"ItemPhoto.make", "ItemPhoto.model", "ItemPhoto.lens"} {
		property := propertySchema(spec, location)
		if property == nil || !property.Value.Type.Is(openapi3.TypeString) ||
			property.Value.MaxLength == nil || *property.Value.MaxLength != wantMaxTextLength {
			t.Errorf("%s is not a string with maxLength %d", location, wantMaxTextLength)
		}
	}

	for _, location := range []string{"ItemPhoto.takenAt", "ItemSummary.takenAt"} {
		if property := propertySchema(spec, location); property == nil || property.Value.Format != "date-time" {
			t.Errorf("%s is not format date-time", location)
		}
	}

	takenAt := propertySchema(spec, "ItemPhoto.takenAt")
	if takenAt == nil {
		return
	}

	description := strings.Join(strings.Fields(takenAt.Value.Description), " ")
	for _, want := range []string{"no time zone", "OffsetTimeOriginal is not applied", "local wall-clock", "never convert it to the viewer's time zone"} {
		if !strings.Contains(description, want) {
			t.Errorf("ItemPhoto.takenAt does not state %q", want)
		}
	}
}

// TestPhotoPrivacy pins the GPS rules of the m3-15a review: the position is visible to every
// admitted client until accounts exist, may be missing and never travels in a URL, and a
// rendered image strips the source's metadata.
func TestPhotoPrivacy(t *testing.T) {
	spec := loadSpec(t)

	photo := spec.Components.Schemas["ItemPhoto"]
	if photo == nil {
		t.Fatal("components.schemas.ItemPhoto is not declared")
	}

	gpsTexts := map[string]string{"ItemPhoto": photo.Value.Description}
	for _, location := range []string{"ItemPhoto.latitude", "ItemPhoto.longitude"} {
		if property := propertySchema(spec, location); property != nil {
			gpsTexts[location] = property.Value.Description
		}
	}

	for location, text := range gpsTexts {
		description := strings.Join(strings.Fields(text), " ")
		for _, want := range []string{"every admitted client", "omit", "never", "signed or share URL"} {
			if !strings.Contains(description, want) {
				t.Errorf("%s does not state %q about the GPS position", location, want)
			}
		}
	}

	operation := getOperation(t, spec, http.MethodGet, "/api/v1/images/{imageId}")
	if description := strings.Join(strings.Fields(operation.Description), " "); !strings.Contains(description, "carries no EXIF, GPS or XMP metadata") {
		t.Error("getImage does not state that rendered images carry no EXIF, GPS or XMP metadata")
	}
}

// TestPhotoIsItsOwnImage pins ADR 0029: a photo is artwork of itself, so its tile is the
// square ImageRef that points at the photo item and getImage accepts a photo item ID.
func TestPhotoIsItsOwnImage(t *testing.T) {
	spec := loadSpec(t)

	for _, name := range []string{"ItemSummary", "ItemDetail", "ArtworkKind"} {
		schema := spec.Components.Schemas[name]
		if schema == nil {
			t.Errorf("components.schemas.%s is not declared", name)

			continue
		}

		description := strings.Join(strings.Fields(schema.Value.Description), " ")
		if !strings.Contains(description, "photo") || !strings.Contains(description, "square") {
			t.Errorf("%s does not document the square photo tile", name)
		}
	}

	operation := getOperation(t, spec, http.MethodGet, "/api/v1/images/{imageId}")
	if description := strings.Join(strings.Fields(operation.Description), " "); !strings.Contains(description, "photo item") {
		t.Error("getImage does not accept a photo item ID")
	}
}

// TestPhotoListingScopes pins how a photos library is listed: the timeline sorts by
// capture time and the type filter names the photo kinds.
func TestPhotoListingScopes(t *testing.T) {
	spec := loadSpec(t)

	parameter := spec.Components.Parameters["ItemTypeFilter"]
	if parameter == nil {
		t.Fatal("components.parameters.ItemTypeFilter is not declared")
	}

	description := strings.Join(strings.Fields(parameter.Value.Description), " ")
	for _, want := range []string{`"photo_album"`, `"photo"`, "timeline", `"takenAt"`} {
		if !strings.Contains(description, want) {
			t.Errorf("ItemTypeFilter does not mention %s", want)
		}
	}
}

// TestSpecVersionCoversPhotos: the additive photos contract raised the minor version.
func TestSpecVersionCoversPhotos(t *testing.T) {
	spec := loadSpec(t)

	const wantMajor, wantMinor = 1, 9

	var major, minor, patch int
	if _, err := fmt.Sscanf(spec.Info.Version, "%d.%d.%d", &major, &minor, &patch); err != nil {
		t.Fatalf("info.version = %q is no semantic version: %v", spec.Info.Version, err)
	}

	if major != wantMajor || minor < wantMinor {
		t.Errorf("info.version = %q, want %d.%d.x or a later minor", spec.Info.Version, wantMajor, wantMinor)
	}
}

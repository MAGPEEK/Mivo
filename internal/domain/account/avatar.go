package account

import "slices"

// AvatarColor is the name of a profile avatar colour token. The web app resolves the name to
// a colour in ui-tokens; the server stores and returns only the name, never a hex value.
type AvatarColor string

// Known avatar colour tokens. The text values are stored in the database and used in the API.
const (
	AvatarColorCoral  AvatarColor = "coral"
	AvatarColorAmber  AvatarColor = "amber"
	AvatarColorGreen  AvatarColor = "green"
	AvatarColorTeal   AvatarColor = "teal"
	AvatarColorBlue   AvatarColor = "blue"
	AvatarColorViolet AvatarColor = "violet"
	AvatarColorPink   AvatarColor = "pink"
	AvatarColorSlate  AvatarColor = "slate"
)

// DefaultAvatarColor is used when a profile is created without a choice; coral is the brand
// accent (ADR 0011).
const DefaultAvatarColor = AvatarColorCoral

// AvatarColors returns every avatar colour token in picker order.
func AvatarColors() []AvatarColor {
	return []AvatarColor{
		AvatarColorCoral, AvatarColorAmber, AvatarColorGreen, AvatarColorTeal,
		AvatarColorBlue, AvatarColorViolet, AvatarColorPink, AvatarColorSlate,
	}
}

// ParseAvatarColor accepts the exact name of a known token. The error is an
// *InvalidFieldError for FieldAvatarColor.
func ParseAvatarColor(text string) (AvatarColor, error) {
	color := AvatarColor(text)
	if !color.IsValid() {
		return "", invalidField(FieldAvatarColor, "must be a known colour token")
	}

	return color, nil
}

// IsValid reports whether the colour is one of the known tokens.
func (color AvatarColor) IsValid() bool {
	return slices.Contains(AvatarColors(), color)
}

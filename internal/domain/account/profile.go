package account

// Profile is a viewer of an account (ADR 0032 §1). Every account has exactly one primary
// profile and up to MaxManagedProfiles managed ones. Watch state, the PIN and the content
// limit belong to a profile; the PIN hash itself stays in the adapters, only HasPIN is here.
type Profile struct {
	// ID is the canonical UUID text assigned by the use cases.
	ID        string
	AccountID string
	// Name is the normalised display name (NormalizeName).
	Name        string
	Primary     bool
	AvatarColor AvatarColor
	HasPIN      bool
}

// NewProfile validates fields and returns them as the profile. Name must already be
// normalised (NormalizeName). The error is an *InvalidFieldError naming the first invalid
// field.
func NewProfile(fields Profile) (Profile, error) {
	if fields.ID == "" {
		return Profile{}, invalidField(FieldID, "must not be empty")
	}

	if fields.AccountID == "" {
		return Profile{}, invalidField(FieldAccountID, "must not be empty")
	}

	if err := validateName(fields.Name); err != nil {
		return Profile{}, err
	}

	if !fields.AvatarColor.IsValid() {
		return Profile{}, invalidField(FieldAvatarColor, "must be a known colour token")
	}

	return fields, nil
}

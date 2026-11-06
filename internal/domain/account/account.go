package account

// Account is a local sign-in identity (ADR 0032 §1). The password hash is not part of it: the
// domain never handles hashes.
type Account struct {
	// ID is the canonical UUID text assigned by the use cases.
	ID string
	// Name is the normalised display name (NormalizeName); NameKey(Name) is unique.
	Name     string
	Role     Role
	Disabled bool
}

// NewAccount validates fields and returns them as the account. Name must already be
// normalised (NormalizeName); the constructor checks its shape but cannot compose it. The
// error is an *InvalidFieldError naming the first invalid field.
func NewAccount(fields Account) (Account, error) {
	if fields.ID == "" {
		return Account{}, invalidField(FieldID, "must not be empty")
	}

	if err := validateName(fields.Name); err != nil {
		return Account{}, err
	}

	if !fields.Role.IsValid() {
		return Account{}, invalidField(FieldRole, "must be admin or user")
	}

	return fields, nil
}

// IsEnabledAdmin reports whether the account counts towards the last-admin invariant.
func (account Account) IsEnabledAdmin() bool {
	return account.Role == RoleAdmin && !account.Disabled
}

package account

import "github.com/magpeek/mivo/internal/domain"

// Field names reported by InvalidFieldError. They match the API property names, so the HTTP
// adapter can point a client at the offending input.
const (
	FieldID          = "id"
	FieldAccountID   = "accountId"
	FieldName        = "name"
	FieldRole        = "role"
	FieldPassword    = "password"
	FieldPIN         = "pin"
	FieldAvatarColor = "avatarColor"
)

// InvalidFieldError reports which input field broke a rule. It unwraps to domain.ErrInvalid,
// so callers that only need the category test with errors.Is.
type InvalidFieldError struct {
	// Field is one of the Field* constants.
	Field string
	// Reason describes the broken rule in English; it never contains the rejected value,
	// because the value may be a password or a PIN.
	Reason string
}

// Error returns "<field>: <reason>: invalid".
func (fieldError *InvalidFieldError) Error() string {
	return fieldError.Field + ": " + fieldError.Reason + ": " + domain.ErrInvalid.Error()
}

// Unwrap returns domain.ErrInvalid.
func (fieldError *InvalidFieldError) Unwrap() error {
	return domain.ErrInvalid
}

func invalidField(field, reason string) error {
	return &InvalidFieldError{Field: field, Reason: reason}
}

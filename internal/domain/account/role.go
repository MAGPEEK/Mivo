package account

// Role is the permission level of an account. Admin powers additionally need the account's
// primary profile to be selected (IsAdminViewer).
type Role string

// Known roles. The text values are stored in the database and used in the API.
const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
)

// Roles returns every role in a stable order.
func Roles() []Role {
	return []Role{RoleAdmin, RoleUser}
}

// ParseRole accepts the exact text of a known role. The error is an *InvalidFieldError for
// FieldRole.
func ParseRole(text string) (Role, error) {
	role := Role(text)
	if !role.IsValid() {
		return "", invalidField(FieldRole, "must be admin or user")
	}

	return role, nil
}

// IsValid reports whether the role is one of the known roles.
func (role Role) IsValid() bool {
	switch role {
	case RoleAdmin, RoleUser:
		return true
	default:
		return false
	}
}

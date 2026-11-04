package folderbrowse

import "os/user"

// ServiceUser returns the name of the user the process runs as, or "" when unknown.
func (*Browser) ServiceUser() string {
	current, err := user.Current()
	if err != nil {
		return ""
	}

	return current.Username
}

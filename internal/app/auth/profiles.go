package auth

import "github.com/magpeek/mivo/internal/domain/account"

// This file is the seam for profile selection: M7-16 adds Me, SelectProfile, ClearProfile and
// the PIN lockout here. M7-15 only decides the automatic selection after sign-in.

// autoSelectProfile returns the primary profile when ADR 0032 §4 allows selecting it without
// the picker: the account has no managed profile and the primary has no PIN.
func autoSelectProfile(profiles []account.Profile) (account.Profile, bool) {
	var primary account.Profile

	found := false

	for _, profile := range profiles {
		if !profile.Primary {
			return account.Profile{}, false
		}

		primary, found = profile, true
	}

	if !found || primary.HasPIN {
		return account.Profile{}, false
	}

	return primary, true
}

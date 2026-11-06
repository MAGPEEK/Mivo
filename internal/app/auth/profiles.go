package auth

import (
	"cmp"
	"slices"

	"github.com/magpeek/mivo/internal/domain/account"
)

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

// findProfile returns the profile with profileID when it belongs to accountID; the account
// check guards against a faulty store as well.
func findProfile(profiles []account.Profile, accountID, profileID string) (account.Profile, bool) {
	for _, profile := range profiles {
		if profile.ID == profileID && profile.AccountID == accountID && profileID != "" {
			return profile, true
		}
	}

	return account.Profile{}, false
}

// pickerProfiles orders the profiles for the picker: the primary profile first, then the
// managed ones by name (api/openapi.yaml Me.profiles), and marks the one with admin powers.
func pickerProfiles(owner account.Account, profiles []account.Profile) []PickerProfile {
	ordered := slices.Clone(profiles)
	slices.SortStableFunc(ordered, func(left, right account.Profile) int {
		if left.Primary != right.Primary {
			if left.Primary {
				return -1
			}

			return 1
		}

		return cmp.Or(
			cmp.Compare(account.NameKey(left.Name), account.NameKey(right.Name)),
			cmp.Compare(left.ID, right.ID),
		)
	})

	picker := make([]PickerProfile, 0, len(ordered))
	for _, profile := range ordered {
		picker = append(picker, PickerProfile{Profile: profile, Admin: account.IsAdminViewer(owner, profile)})
	}

	return picker
}

package auth

import (
	"github.com/magpeek/mivo/internal/domain/access"
	"github.com/magpeek/mivo/internal/domain/account"
)

// buildViewer derives the effective access of ADR 0032 §7 from the stored inputs. Without a
// selected profile the viewer sees no library and has the strictest limit, so a use case that
// forgets to demand a profile still fails closed.
func buildViewer(sessionID string, inputs ViewerInputs, profileSelected bool) access.Viewer {
	viewer := access.Viewer{AuthSessionID: sessionID, AccountID: inputs.Account.ID}

	if !profileSelected {
		viewer.Libraries = access.OnlyLibraries()
		viewer.Limit = strictestLimit()

		return viewer
	}

	viewer.ProfileID = inputs.Profile.ID
	viewer.Admin = account.IsAdminViewer(inputs.Account, inputs.Profile)
	viewer.Libraries = effectiveLibraries(inputs)
	viewer.Limit = effectiveLimit(inputs)

	return viewer
}

// effectiveLibraries is the account's set, narrowed by a managed profile's own set. An admin
// account always has every library (ADR 0032 §7); its managed profiles may still be narrowed.
func effectiveLibraries(inputs ViewerInputs) access.LibraryAccess {
	accountLibraries := inputs.AccountLibraries
	if inputs.Account.Role == account.RoleAdmin {
		accountLibraries = access.AllLibraries()
	}

	if inputs.Profile.Primary {
		return accountLibraries
	}

	return accountLibraries.Intersect(inputs.ProfileLibraries)
}

// effectiveLimit is the profile's own limit; a managed profile never gets a weaker one than
// its account's primary profile.
func effectiveLimit(inputs ViewerInputs) access.ContentLimit {
	if inputs.Profile.Primary {
		return inputs.ProfileLimit
	}

	return access.Stricter(inputs.ProfileLimit, inputs.PrimaryLimit)
}

// strictestLimit allows only the lowest age bucket and hides unrated titles. MaxAgeLimit
// cannot fail for AgeAll; should it ever, unrated titles stay hidden.
func strictestLimit() access.ContentLimit {
	limit, err := access.MaxAgeLimit(access.AgeAll, true)
	if err != nil {
		return access.NoLimit().HidingUnrated(true)
	}

	return limit
}

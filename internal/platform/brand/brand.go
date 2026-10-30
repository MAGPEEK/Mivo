// Package brand is the single Go source of the product name and name-derived values.
//
// The product name is a working title (ADR 0002). Renaming means editing this file plus
// the structural identifiers listed in ADR 0002; no other Go file may contain the name.
package brand

const (
	// DisplayName is the user-visible product name.
	DisplayName = "Mivo"

	// ID is the lowercase identifier used for binaries, OS directories and log fields.
	ID = "mivo"

	// EnvPrefix is the prefix of all environment variables read by the server.
	EnvPrefix = "MIVO"

	// RepositoryURL is the canonical link to the source repository.
	RepositoryURL = "https://github.com/MAGPEEK/Mivo"
)

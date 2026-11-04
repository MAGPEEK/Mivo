package metadata

import (
	"fmt"
	"strings"

	"github.com/magpeek/mivo/internal/domain"
)

// Provider is an online metadata database an item can be identified with (UX ED-07).
type Provider string

// Known providers. The text values are stored in the database and used in the API.
const (
	ProviderTMDB Provider = "tmdb"
	ProviderIMDb Provider = "imdb"
	ProviderTVDB Provider = "tvdb"
)

// Limits of provider ID texts. IMDb IDs are "tt" followed by 7 to 10 digits (8 since 2020,
// room left for growth). Numeric IDs are capped at 18 digits so they always fit a signed
// 64-bit integer and a crafted NFO cannot push arbitrarily long text into the catalog.
const (
	imdbIDPrefix           = "tt"
	imdbMinimumDigits      = 7
	imdbMaximumDigits      = 10
	numericIDMaximumDigits = 18
)

// ProviderID identifies an item in one provider's database.
type ProviderID struct {
	Provider Provider
	Value    string
}

// ParseProvider accepts the exact lowercase text of a known provider. The error wraps
// domain.ErrInvalid.
func ParseProvider(text string) (Provider, error) {
	provider := Provider(text)
	if !provider.IsValid() {
		return "", fmt.Errorf("parse provider %q: unknown provider: %w", text, domain.ErrInvalid)
	}

	return provider, nil
}

// IsValid reports whether the provider is one of the known providers.
func (provider Provider) IsValid() bool {
	switch provider {
	case ProviderTMDB, ProviderIMDb, ProviderTVDB:
		return true
	default:
		return false
	}
}

// String returns the text value of the provider.
func (provider Provider) String() string {
	return string(provider)
}

// ValidateProviderID checks the form of an ID: TMDB and TVDB IDs are decimal digits without
// a leading zero, IMDb IDs are "tt" followed by 7 to 10 digits. It does not check that the
// ID exists. The error wraps domain.ErrInvalid.
func ValidateProviderID(provider Provider, id string) error {
	var valid bool

	switch provider {
	case ProviderTMDB, ProviderTVDB:
		valid = isNumericID(id)
	case ProviderIMDb:
		valid = isIMDbID(id)
	default:
		return fmt.Errorf("validate provider ID %q: unknown provider %q: %w", id, provider, domain.ErrInvalid)
	}

	if !valid {
		return fmt.Errorf("validate %s ID %q: malformed ID: %w", provider, id, domain.ErrInvalid)
	}

	return nil
}

func isNumericID(id string) bool {
	return len(id) <= numericIDMaximumDigits && isDigits(id) && id[0] != '0'
}

func isIMDbID(id string) bool {
	digits, found := strings.CutPrefix(id, imdbIDPrefix)

	return found && len(digits) >= imdbMinimumDigits && len(digits) <= imdbMaximumDigits && isDigits(digits)
}

// isDigits reports whether text is non-empty and consists of ASCII digits only; Unicode
// digits such as "١" are no valid ID characters.
func isDigits(text string) bool {
	if text == "" {
		return false
	}

	for index := range len(text) {
		if text[index] < '0' || text[index] > '9' {
			return false
		}
	}

	return true
}

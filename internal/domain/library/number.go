package library

import "strconv"

// OptionalNumber is a whole number that may be absent. Placements and items need the
// difference because 0 is meaningful (season 0 holds the specials, ADR 0027 §2) while many
// files carry no number at all. The zero value is absent; NumberOf makes a present one.
type OptionalNumber struct {
	value   int
	present bool
}

// NumberOf returns a present number. Range checks belong to the type that holds it (for
// example Placement.Validate rejects negative numbers).
func NumberOf(value int) OptionalNumber {
	return OptionalNumber{value: value, present: true}
}

// Get returns the number and whether it is present; an absent number returns 0, false.
func (number OptionalNumber) Get() (value int, ok bool) {
	return number.value, number.present
}

// IsSet reports whether the number is present.
func (number OptionalNumber) IsSet() bool {
	return number.present
}

// String returns the decimal text of a present number and "" for an absent one.
func (number OptionalNumber) String() string {
	if !number.present {
		return ""
	}

	return strconv.Itoa(number.value)
}

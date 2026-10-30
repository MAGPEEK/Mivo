// Package uuidgen implements the app/system IDGenerator port with random (version 4) UUIDs
// from the standard library uuid package, which draws from a cryptographically secure source.
package uuidgen

import "uuid"

// Generator creates random UUIDs in canonical lowercase text form. The zero value is ready
// to use.
type Generator struct{}

// New returns a Generator.
func New() Generator {
	return Generator{}
}

// NewID returns a new version 4 UUID such as "0b8c6d4e-3f2a-4c1b-9d8e-7f6a5b4c3d2e".
// The error is always nil: the standard library generator cannot fail, but the port allows
// generators that can.
func (Generator) NewID() (string, error) {
	return uuid.NewV4().String(), nil
}

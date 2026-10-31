// Package spkmeta derives the metadata of the Synology package (SPK) of the media server from
// the brand, the server version and the architecture family (ADR 0026, task SPK-03): the DSM
// package version and beta flag, the family table, the generated build.env and package.env,
// and the @TOKEN@ templates under packaging/synology/package/.
//
// Every function is pure and uses the standard library only; the staging step (SPK-14) does
// the file I/O. The product name comes only from internal/platform/brand.
package spkmeta

import "errors"

var (
	// ErrInvalidVersion reports a server version or package revision outside the mapping of
	// ADR 0026 section 5 (OD-SPK-5).
	ErrInvalidVersion = errors.New("invalid version")

	// ErrUnknownFamily reports an architecture family that has no SPK.
	ErrUnknownFamily = errors.New("unknown architecture family")

	// ErrInvalidValue reports an empty or malformed package value, such as an identifier or
	// a port.
	ErrInvalidValue = errors.New("invalid package value")

	// ErrUnsafeValue reports a value with a character that INFO or a single-quoted sh
	// assignment cannot carry.
	ErrUnsafeValue = errors.New("unsafe package value")

	// ErrUnknownToken reports an @UPPER_CASE@ token in a template that no build value fills.
	ErrUnknownToken = errors.New("unknown template token")

	// ErrNonASCII reports a non-ASCII byte in a rendered file that must stay ASCII.
	ErrNonASCII = errors.New("non-ASCII byte in an ASCII-only file")
)

package system

import "time"

// Identity is the persistent identity of a server installation. It is created on first
// start and never changes afterwards.
type Identity struct {
	// ID is the stable server ID reported to clients.
	ID ServerID
	// CreatedAt is the moment the identity was created, in UTC.
	CreatedAt time.Time
}

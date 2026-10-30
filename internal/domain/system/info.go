package system

// APIVersion is the major version of the HTTP API contract (api/openapi.yaml) this server
// speaks. Clients compare it before they use the API; bump it only on breaking changes.
const APIVersion = "1"

// Info describes the running server to clients.
type Info struct {
	// ProductName is the configurable product name (working title "Mivo").
	ProductName string
	// Version is the release version of the binary.
	Version string
	// Commit is the VCS revision the binary was built from; it may be empty.
	Commit string
	// BuildTime is the commit time in RFC 3339; it may be empty.
	BuildTime string
	// ServerID is the persistent identity of this installation.
	ServerID ServerID
	// APIVersion is the major version of the HTTP API, see the APIVersion constant.
	APIVersion string
}

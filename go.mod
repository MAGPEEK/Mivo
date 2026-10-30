module github.com/magpeek/mivo

go 1.27.0

// GOTOOLCHAIN=auto downloads this patched toolchain when the installed Go is older; it
// fixes the standard library advisories of go1.27.0.
toolchain go1.27.3

// The web workspace contains node_modules with stray .go files; keep it out of ./...
ignore ./web

// Development tools live in tools/go.mod so their dependencies stay out of this module
// graph; run them with `go tool -modfile=tools/go.mod <tool>` (ADR 0019).

require (
	github.com/getkin/kin-openapi v0.152.0
	go.yaml.in/yaml/v3 v3.0.5
)

require (
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	golang.org/x/text v0.14.0 // indirect
)

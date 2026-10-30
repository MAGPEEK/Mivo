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
	github.com/oapi-codegen/runtime v1.8.0
	github.com/pressly/goose/v3 v3.29.0
	go.yaml.in/yaml/v3 v3.0.5
	modernc.org/sqlite v1.61.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.5 // indirect
	github.com/go-openapi/swag/jsonname v0.25.5 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mfridman/interpolate v0.0.2 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/oasdiff/yaml v0.1.1 // indirect
	github.com/oasdiff/yaml3 v0.0.14 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.3 // indirect
	github.com/sethvargo/go-retry v0.5.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

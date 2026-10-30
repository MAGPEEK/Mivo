module github.com/magpeek/mivo

go 1.27.0

// GOTOOLCHAIN=auto downloads this patched toolchain when the installed Go is older; it
// fixes the standard library advisories of go1.27.0.
toolchain go1.27.3

// The web workspace contains node_modules with stray .go files; keep it out of ./...
ignore ./web

// Development tools live in tools/go.mod so their dependencies stay out of this module
// graph; run them with `go tool -modfile=tools/go.mod <tool>` (ADR 0019).

require go.yaml.in/yaml/v3 v3.0.5

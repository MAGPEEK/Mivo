module github.com/magpeek/mivo

go 1.27.0

// The web workspace contains node_modules with stray .go files; keep it out of ./...
ignore ./web

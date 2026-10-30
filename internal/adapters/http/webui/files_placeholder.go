//go:build !webui

package webui

import (
	"html"
	"io/fs"
	"testing/fstest"

	"github.com/magpeek/mivo/internal/platform/brand"
)

// Embedded reports whether this build contains the real web app.
const Embedded = false

// Files returns a single placeholder index.html that explains how to build the web app into
// the binary, so plain `go build` and `go test` work without Node or a web build.
//
// fstest.MapFS is used as a plain in-memory fs.FS: it has no test-only side effects and
// supports seeking, which ranges and HEAD rely on.
func Files() (fs.FS, error) {
	return fstest.MapFS{indexFile: {Data: []byte(placeholderPage())}}, nil
}

// placeholderPage renders the page with the product name from brand; the module directory
// cmd/<id> is a structural identifier (ADR 0002), so it is derived from brand as well.
func placeholderPage() string {
	name := html.EscapeString(brand.DisplayName)
	buildCommand := html.EscapeString("go build -tags webui ./cmd/" + brand.ID)

	return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + name + `</title>
</head>
<body>
<main>
<h1>` + name + `</h1>
<p>This server was built without the web app. The API is available under <code>/api/v1/</code>.</p>
<p>To include the web app, build it first and then build the server with the <code>webui</code> tag:</p>
<pre><code>pnpm -C web install
pnpm -C web build
` + buildCommand + `</code></pre>
</main>
</body>
</html>
`
}

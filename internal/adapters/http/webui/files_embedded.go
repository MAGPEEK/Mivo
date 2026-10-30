//go:build webui

package webui

import (
	"embed"
	"fmt"
	"io/fs"
)

// Embedded reports whether this build contains the real web app.
const Embedded = true

// distDirectory is where `pnpm -C web build` writes the app. The webui build fails at compile
// time when it is missing, which is intended (ADR 0020).
const distDirectory = "dist"

// distFiles holds the Vite build. "all:" keeps files that start with "." or "_".
//
//go:embed all:dist
var distFiles embed.FS

// Files returns the embedded web app, rooted at its index.html.
func Files() (fs.FS, error) {
	files, err := fs.Sub(distFiles, distDirectory)
	if err != nil {
		return nil, fmt.Errorf("open embedded web ui: %w", err)
	}

	return files, nil
}

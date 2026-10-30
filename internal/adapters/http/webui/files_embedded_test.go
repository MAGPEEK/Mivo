//go:build webui

package webui_test

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/webui"
)

func TestEmbeddedFilesContainTheBuiltApp(t *testing.T) {
	t.Parallel()

	if !webui.Embedded {
		t.Error("Embedded = false, want true with the webui tag")
	}

	files, err := webui.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	handler := newHandler(t, files)

	recorder := serve(t, handler, http.MethodGet, "/")
	assertStatus(t, recorder, http.StatusOK)
	assertHeader(t, recorder, "Cache-Control", wantRevalidate)

	if body := recorder.Body.String(); !strings.Contains(body, `id="root"`) {
		t.Errorf("index.html lacks the React root:\n%s", body)
	}

	script := firstScriptAsset(t, files)

	recorder = serve(t, handler, http.MethodGet, "/"+script)
	assertStatus(t, recorder, http.StatusOK)
	assertHeader(t, recorder, "Content-Type", "text/javascript; charset=utf-8")
	assertHeader(t, recorder, "Cache-Control", wantImmutable)
}

// firstScriptAsset returns the path of a hashed script, whose name changes with every build.
func firstScriptAsset(t *testing.T, files fs.FS) string {
	t.Helper()

	scripts, err := fs.Glob(files, "assets/*.js")
	if err != nil {
		t.Fatalf("glob assets: %v", err)
	}

	if len(scripts) == 0 {
		t.Fatal("embedded build has no assets/*.js")
	}

	return path.Clean(scripts[0])
}

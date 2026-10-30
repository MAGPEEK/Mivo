//go:build !webui

package webui_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/magpeek/mivo/internal/adapters/http/webui"
	"github.com/magpeek/mivo/internal/platform/brand"
)

func TestPlaceholderExplainsHowToBuildTheWebApp(t *testing.T) {
	t.Parallel()

	if webui.Embedded {
		t.Error("Embedded = true, want false without the webui tag")
	}

	files, err := webui.Files()
	if err != nil {
		t.Fatalf("Files: %v", err)
	}

	handler := newHandler(t, files)

	for _, target := range []string{"/", "/library/x"} {
		recorder := serve(t, handler, http.MethodGet, target)

		assertStatus(t, recorder, http.StatusOK)
		assertHeader(t, recorder, "Content-Type", "text/html; charset=utf-8")
		assertHeader(t, recorder, "Cache-Control", wantRevalidate)

		body := recorder.Body.String()
		for _, want := range []string{"<title>" + brand.DisplayName + "</title>", "pnpm -C web build", "go build -tags webui ./cmd/" + brand.ID} {
			if !strings.Contains(body, want) {
				t.Errorf("GET %s body lacks %q:\n%s", target, want, body)
			}
		}
	}
}
